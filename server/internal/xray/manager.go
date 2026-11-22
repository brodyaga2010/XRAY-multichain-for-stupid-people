package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"xraypanel/internal/config"
)

const profileTTL = 60 * time.Second

// GatewayError — Xray недоступен или конфиг непригоден (HTTP 502).
type GatewayError struct{ Detail string }

func (e *GatewayError) Error() string { return e.Detail }

func badGateway(format string, args ...any) error {
	return &GatewayError{Detail: fmt.Sprintf(format, args...)}
}

// TrafficStat — трафик ключа с прошлого сбора.
type TrafficStat struct {
	Uplink   int64
	Downlink int64
}

type cachedProfile struct {
	at        time.Time
	profile   ServerProfile
	publicKey string
}

// Manager управляет пользователями Xray: изменения применяются через API Xray
// (без перезапуска) и сохраняются в config.json; если API недоступно — Xray перезапускается.
type Manager struct {
	cfg    *config.Config
	runner Runner

	mu      sync.Mutex
	profile *cachedProfile

	stampMu   sync.Mutex
	lastStamp time.Time
}

// NewManager создаёт менеджер поверх исполнителя команд.
func NewManager(cfg *config.Config, runner Runner) *Manager {
	return &Manager{cfg: cfg, runner: runner}
}

// Runner возвращает исполнитель (нужен тестам и dry-run).
func (m *Manager) Runner() Runner { return m.runner }

// Build создаёт менеджер по режиму из настроек.
func Build(cfg *config.Config) (*Manager, error) {
	switch cfg.EffectiveXrayMode() {
	case "local":
		return NewManager(cfg, NewLocalRunner(cfg.XrayLockPath)), nil
	case "ssh":
		return nil, errors.New("XRAY_MODE=ssh не поддерживается в Go-версии панели: используйте local (панель на сервере Xray) или dry_run")
	default:
		raw, err := MarshalConfig(DryRunConfig())
		if err != nil {
			return nil, err
		}
		return NewManager(cfg, NewMemoryRunner(map[string]string{cfg.XrayConfigPath: string(raw)})), nil
	}
}

// ---------- ключи ----------

// AddKey добавляет UUID во все пользовательские inbound.
func (m *Manager) AddKey(keyUUID string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := m.readConfig()
	if err != nil {
		return err
	}
	added, err := AddClient(cfg, keyUUID)
	if err != nil {
		return badGateway("%s", err)
	}
	if len(added) == 0 {
		return nil
	}
	if err := m.saveConfig(cfg); err != nil {
		return err
	}
	byTag := map[string]map[string]any{}
	for _, inbound := range UserInbounds(cfg) {
		byTag[asStr(inbound["tag"])] = inbound
	}
	inbounds := make([]any, 0, len(added))
	for _, tag := range added {
		inbounds = append(inbounds, map[string]any{
			"tag":      tag,
			"port":     byTag[tag]["port"],
			"protocol": "vless",
			"settings": map[string]any{
				"clients":    []any{ClientEntry(byTag[tag], keyUUID)},
				"decryption": "none",
			},
		})
	}
	if !m.apiAdd(map[string]any{"inbounds": inbounds}, len(added)) {
		return m.restart()
	}
	return nil
}

// RemoveKey удаляет UUID из всех пользовательских inbound.
func (m *Manager) RemoveKey(keyUUID string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := m.readConfig()
	if err != nil {
		return err
	}
	removed := RemoveClient(cfg, keyUUID)
	if len(removed) == 0 {
		return nil
	}
	if err := m.saveConfig(cfg); err != nil {
		return err
	}
	ok := true
	for _, item := range removed {
		email := item.Email
		if email == "" {
			email = keyUUID
		}
		ok = m.apiRemove(item.Tag, email) && ok
	}
	if !ok {
		return m.restart()
	}
	return nil
}

// ListServerKeys — UUID всех ключей из конфига Xray.
func (m *Manager) ListServerKeys() ([]string, error) {
	cfg, err := m.readConfig()
	if err != nil {
		return nil, err
	}
	return ListClientIDs(cfg), nil
}

// Normalize: email = UUID и каждый ключ во всех inbound. Перезапускает Xray, если что-то поменялось.
func (m *Manager) Normalize() (bool, error) {
	unlock, err := m.lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	cfg, err := m.readConfig()
	if err != nil {
		return false, err
	}
	if !NormalizeClients(cfg) {
		return false, nil
	}
	if err := m.saveConfig(cfg); err != nil {
		return false, err
	}
	return true, m.restart()
}

// Bootstrap включает API/статистику, XHTTP inbound, служебный вход и проверку соединения до выхода. Перезапускает Xray, если конфиг изменился.
func (m *Manager) Bootstrap() (bool, error) {
	unlock, err := m.lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	cfg, err := m.readConfig()
	if err != nil {
		return false, err
	}
	changed, err := EnsurePanelFeatures(cfg, m.cfg.XrayAPIServer, m.cfg.XrayXHTTPPort)
	if err != nil {
		return false, badGateway("%s", err)
	}
	if EnsureProbeInbound(cfg, m.cfg.XrayProbePort) {
		changed = true
	}
	if EnsureExitHealthCheck(cfg) {
		changed = true
	}
	if changed {
		if err := m.saveConfig(cfg); err != nil {
			return false, err
		}
		if err := m.restart(); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// ExitServer — адрес выходного сервера из текущего конфига; ok=false, если цепочки нет.
func (m *Manager) ExitServer() (host string, port int, ok bool, err error) {
	cfg, err := m.readConfig()
	if err != nil {
		return "", 0, false, err
	}
	host, port, ok = ExitServer(cfg)
	return host, port, ok, nil
}

// ---------- ссылки ----------

// Links — ссылки подключения ключа для всех видов.
func (m *Manager) Links(keyUUID, name string) ([]KeyLink, error) {
	profile, publicKey, err := m.serverProfile()
	if err != nil {
		return nil, err
	}
	return BuildLinks(profile, m.cfg.XrayPublicHost, publicKey, keyUUID, name), nil
}

// RealityParams — (sni, public_key, short_id); сохраняются в БД вместе с ключом.
func (m *Manager) RealityParams() (sni, publicKey, shortID string, err error) {
	profile, publicKey, err := m.serverProfile()
	if err != nil {
		return "", "", "", err
	}
	return profile.SNI, publicKey, profile.ShortID, nil
}

// ---------- статистика ----------

// CollectTraffic — трафик по email с момента прошлого вызова (счётчики Xray сбрасываются).
func (m *Manager) CollectTraffic() (map[string]TrafficStat, error) {
	raw, err := m.api("statsquery", "-pattern", "user>>>", "-reset")
	if err != nil {
		return nil, err
	}
	var data struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, badGateway("API Xray вернуло некорректный ответ: %v", err)
	}
	result := map[string]TrafficStat{}
	for _, item := range data.Stat {
		parts := strings.Split(item.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}
		value := parseStatValue(item.Value)
		stat := result[parts[1]]
		switch parts[3] {
		case "uplink":
			stat.Uplink = value
		case "downlink":
			stat.Downlink = value
		default:
			continue
		}
		result[parts[1]] = stat
	}
	return result, nil
}

// parseStatValue: xray отдаёт int64 строкой, MemoryRunner — числом; ноль может отсутствовать.
func parseStatValue(raw json.RawMessage) int64 {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return 0
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// OnlineEmails — email пользователей, которые сейчас онлайн.
func (m *Manager) OnlineEmails() (map[string]bool, error) {
	raw, err := m.api("statsgetallonlineusers")
	if err != nil {
		return nil, err
	}
	var data struct {
		Users []string `json:"users"`
	}
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, badGateway("API Xray вернуло некорректный ответ: %v", err)
	}
	emails := map[string]bool{}
	for _, name := range data.Users {
		parts := strings.Split(name, ">>>")
		if len(parts) == 3 && parts[0] == "user" {
			emails[parts[1]] = true
		}
	}
	return emails, nil
}

// ---------- внутреннее ----------

func (m *Manager) lock() (func(), error) {
	unlock, err := m.runner.Lock()
	if err != nil {
		return nil, badGateway("Не удалось получить блокировку конфига Xray: %v", err)
	}
	return unlock, nil
}

func (m *Manager) readConfig() (Config, error) {
	raw, err := m.runner.ReadText(m.cfg.XrayConfigPath)
	if err != nil {
		return nil, badGateway("Не удалось прочитать конфиг Xray: %v", err)
	}
	cfg, err := ParseConfig([]byte(raw))
	if err != nil {
		return nil, badGateway("Конфиг Xray — некорректный JSON")
	}
	return cfg, nil
}

func (m *Manager) saveConfig(cfg Config) error {
	path := m.cfg.XrayConfigPath
	// Xray определяет формат по расширению — временный файл обязан быть .json
	tmp := strings.TrimSuffix(path, ".json") + ".panel-tmp.json"
	payload, err := MarshalConfig(cfg)
	if err != nil {
		return badGateway("Не удалось сериализовать конфиг Xray: %v", err)
	}
	if err := m.runner.WriteText(tmp, string(payload)); err == nil {
		_, err = m.runner.Run([]string{m.cfg.XrayBinary, "run", "-test", "-config", tmp}, 20*time.Second)
		if err != nil {
			m.safeRemove(tmp)
			return badGateway("Новый конфиг Xray не прошёл проверку: %v", err)
		}
	} else {
		m.safeRemove(tmp)
		return badGateway("Новый конфиг Xray не прошёл проверку: %v", err)
	}
	if err := m.backup(path); err != nil {
		return badGateway("Не удалось сохранить конфиг Xray: %v", err)
	}
	if err := m.runner.Replace(tmp, path); err != nil {
		return badGateway("Не удалось сохранить конфиг Xray: %v", err)
	}
	m.mu.Lock()
	m.profile = nil
	m.mu.Unlock()
	return nil
}

// backupStamp — %Y%m%d%H%M%S%f в UTC; строго возрастает, чтобы бэкапы не затирали друг друга.
func (m *Manager) backupStamp() string {
	m.stampMu.Lock()
	defer m.stampMu.Unlock()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if !now.After(m.lastStamp) {
		now = m.lastStamp.Add(time.Microsecond)
	}
	m.lastStamp = now
	return now.Format("20060102150405") + fmt.Sprintf("%06d", now.Nanosecond()/1000)
}

func (m *Manager) backup(path string) error {
	dir := strings.TrimRight(m.cfg.XrayBackupDir, "/")
	if err := m.runner.Copy(path, fmt.Sprintf("%s/config.%s.json", dir, m.backupStamp())); err != nil {
		return err
	}
	names, err := m.runner.ListDir(dir)
	if err != nil {
		return err
	}
	var backups []string
	for _, name := range names {
		if strings.HasPrefix(name, "config.") {
			backups = append(backups, name)
		}
	}
	if excess := len(backups) - m.cfg.XrayBackupKeep; excess > 0 {
		for _, name := range backups[:excess] {
			m.safeRemove(dir + "/" + name)
		}
	}
	return nil
}

func (m *Manager) safeRemove(path string) {
	if err := m.runner.Remove(path); err != nil {
		slog.Warn("Не удалось удалить файл", "path", path, "error", err)
	}
}

func (m *Manager) restart() error {
	args, err := ShellSplit(m.cfg.XrayRestartCommand)
	if err == nil && len(args) == 0 {
		err = errors.New("пустая команда")
	}
	if err == nil {
		_, err = m.runner.Run(args, 30*time.Second)
	}
	if err != nil {
		return badGateway("Не удалось перезапустить Xray: %v", err)
	}
	return nil
}

func (m *Manager) api(args ...string) (string, error) {
	full := append([]string{m.cfg.XrayBinary, "api", args[0], "--server=" + m.cfg.XrayAPIServer}, args[1:]...)
	out, err := m.runner.Run(full, 20*time.Second)
	if err != nil {
		return "", badGateway("API Xray недоступно: %v", err)
	}
	return out, nil
}

func (m *Manager) apiAdd(payload map[string]any, expected int) bool {
	tmp := fmt.Sprintf("%s/adu.%d.json", strings.TrimRight(m.cfg.XrayBackupDir, "/"), time.Now().UnixNano())
	defer m.safeRemove(tmp)
	raw, err := json.Marshal(payload)
	if err == nil {
		err = m.runner.WriteText(tmp, string(raw))
	}
	var out string
	if err == nil {
		out, err = m.api("adu", tmp)
	}
	if err != nil {
		slog.Warn("adu не сработал, будет перезапуск Xray", "error", err)
		return false
	}
	return strings.Contains(out, fmt.Sprintf("Added %d user(s)", expected))
}

func (m *Manager) apiRemove(tag, email string) bool {
	out, err := m.api("rmu", "-tag="+tag, email)
	if err != nil {
		slog.Warn("rmu не сработал, будет перезапуск Xray", "error", err)
		return false
	}
	return strings.Contains(out, "Removed 1 user(s)")
}

func (m *Manager) serverProfile() (ServerProfile, string, error) {
	m.mu.Lock()
	if p := m.profile; p != nil && time.Since(p.at) < profileTTL {
		m.mu.Unlock()
		return p.profile, p.publicKey, nil
	}
	m.mu.Unlock()

	cfg, err := m.readConfig()
	if err != nil {
		return ServerProfile{}, "", err
	}
	profile, err := BuildServerProfile(cfg)
	if err != nil {
		return ServerProfile{}, "", badGateway("%s", err)
	}
	out, err := m.runner.Run([]string{m.cfg.XrayBinary, "x25519", "-i", profile.PrivateKey}, 20*time.Second)
	if err != nil {
		return ServerProfile{}, "", badGateway("%s", err)
	}
	publicKey, err := ParsePublicKey(out)
	if err != nil {
		return ServerProfile{}, "", badGateway("%s", err)
	}
	m.mu.Lock()
	m.profile = &cachedProfile{at: time.Now(), profile: profile, publicKey: publicKey}
	m.mu.Unlock()
	return profile, publicKey, nil
}

// DryRunConfig — конфиг входного сервера для локальной разработки (как после настройки на реальном).
func DryRunConfig() Config {
	reality := func() map[string]any {
		return map[string]any{
			"dest":        "127.0.0.1:8444",
			"serverNames": []any{"vpn.example.com", ""},
			"privateKey":  "dry-run-private",
			"shortIds":    []any{"dryrun01"},
		}
	}
	cfg := Config{
		"inbounds": []any{
			map[string]any{
				"tag":      "user-inbound",
				"port":     443,
				"protocol": "vless",
				"settings": map[string]any{"clients": []any{}, "decryption": "none"},
				"streamSettings": map[string]any{
					"network": "tcp", "security": "reality", "realitySettings": reality(),
				},
			},
			map[string]any{
				"tag":      "user-inbound-grpc",
				"port":     8443,
				"protocol": "vless",
				"settings": map[string]any{"clients": []any{}, "decryption": "none"},
				"streamSettings": map[string]any{
					"network":         "grpc",
					"security":        "reality",
					"realitySettings": reality(),
					"grpcSettings":    map[string]any{"serviceName": "xray_grpc_service"},
				},
			},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "to-exit"}},
		"routing": map[string]any{"rules": []any{
			map[string]any{
				"type":        "field",
				"inboundTag":  []any{"user-inbound", "user-inbound-grpc"},
				"outboundTag": "to-exit",
			},
		}},
	}
	_, _ = EnsurePanelFeatures(cfg, "127.0.0.1:10085", 2053)
	EnsureProbeInbound(cfg, 10811) // порт по умолчанию XRAY_PROBE_PORT
	return cfg
}
