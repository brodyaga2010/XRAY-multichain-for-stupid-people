package xray

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"xraypanel/internal/config"
)

func testConfig(t *testing.T, extra map[string]string) *config.Config {
	t.Helper()
	env := map[string]string{
		"JWT_SECRET":       "test-secret-that-is-long-enough",
		"XRAY_MODE":        "dry_run",
		"XRAY_PUBLIC_HOST": "vpn.example.com",
	}
	for k, v := range extra {
		env[k] = v
	}
	cfg, err := config.Load(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newTestManager(t *testing.T) (*Manager, *MemoryRunner, *config.Config) {
	t.Helper()
	cfg := testConfig(t, nil)
	m, err := Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runner, ok := m.Runner().(*MemoryRunner)
	if !ok {
		t.Fatalf("ожидался MemoryRunner, получено %T", m.Runner())
	}
	return m, runner, cfg
}

func savedClients(t *testing.T, runner *MemoryRunner, cfg *config.Config) map[string][]string {
	t.Helper()
	parsed, err := ParseConfig([]byte(runner.Files[cfg.XrayConfigPath]))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for tag, clients := range clientsByTag(parsed) {
		ids := []string{}
		for _, c := range clients {
			ids = append(ids, asStr(c["id"]))
		}
		out[tag] = ids
	}
	return out
}

func hasCommand(runner *MemoryRunner, want []string) bool {
	for _, c := range runner.Commands {
		if reflect.DeepEqual(c, want) {
			return true
		}
	}
	return false
}

func countCommand(runner *MemoryRunner, want []string) int {
	n := 0
	for _, c := range runner.Commands {
		if reflect.DeepEqual(c, want) {
			n++
		}
	}
	return n
}

func restartCommand(cfg *config.Config) []string { return strings.Fields(cfg.XrayRestartCommand) }

func allEqual(clients map[string][]string, want []string) bool {
	if len(clients) == 0 {
		return false
	}
	for _, ids := range clients {
		if !reflect.DeepEqual(ids, want) {
			return false
		}
	}
	return true
}

func TestAddKeyUsesAPIWithoutRestart(t *testing.T) {
	m, runner, cfg := newTestManager(t)

	if err := m.AddKey(uuidA); err != nil {
		t.Fatal(err)
	}

	clients := savedClients(t, runner, cfg)
	if !allEqual(clients, []string{uuidA}) {
		t.Errorf("clients = %v", clients)
	}
	for tag := range clients {
		if !runner.APIUsers[tag][uuidA] {
			t.Errorf("пользователь не добавлен через API в %s", tag)
		}
	}
	if hasCommand(runner, restartCommand(cfg)) {
		t.Error("перезапуск не должен вызываться")
	}
	for name := range runner.Files {
		if strings.Contains(name, "adu.") {
			t.Errorf("остался временный файл adu: %s", name)
		}
	}
}

func TestAddKeyFallsBackToRestartWhenAPIFails(t *testing.T) {
	m, runner, cfg := newTestManager(t)
	runner.FailAPI = true

	if err := m.AddKey(uuidA); err != nil {
		t.Fatal(err)
	}

	if !allEqual(savedClients(t, runner, cfg), []string{uuidA}) {
		t.Error("ключ не сохранён в конфиге")
	}
	if !hasCommand(runner, restartCommand(cfg)) {
		t.Error("ожидался перезапуск Xray")
	}
}

func TestAddExistingKeyIsNoop(t *testing.T) {
	m, runner, _ := newTestManager(t)
	_ = m.AddKey(uuidA)
	before := len(runner.Commands)

	if err := m.AddKey(uuidA); err != nil {
		t.Fatal(err)
	}

	if len(runner.Commands) != before {
		t.Errorf("повторное добавление выполнило команды: %v", runner.Commands[before:])
	}
}

func TestInvalidConfigIsNotSaved(t *testing.T) {
	m, runner, cfg := newTestManager(t)
	original := runner.Files[cfg.XrayConfigPath]
	runner.FailTest = true

	err := m.AddKey(uuidA)

	var gw *GatewayError
	if !errors.As(err, &gw) {
		t.Fatalf("ожидалась GatewayError (502), получено %v", err)
	}
	if runner.Files[cfg.XrayConfigPath] != original {
		t.Error("конфиг изменился")
	}
	for name := range runner.Files {
		if strings.Contains(name, "panel-tmp") {
			t.Errorf("остался временный файл %s", name)
		}
	}
	if len(runner.APIUsers) != 0 {
		t.Errorf("API не должно вызываться: %v", runner.APIUsers)
	}
}

func TestTempConfigKeepsJSONExtension(t *testing.T) {
	m, runner, cfg := newTestManager(t)

	if err := m.AddKey(uuidA); err != nil {
		t.Fatal(err)
	}

	want := []string{"xray", "run", "-test", "-config", "/usr/local/etc/xray/config.panel-tmp.json"}
	if !hasCommand(runner, want) {
		t.Errorf("нет проверки временного конфига %v среди %v", want, runner.Commands)
	}
	_ = cfg
}

func TestRemoveKeyFromAllInbounds(t *testing.T) {
	m, runner, cfg := newTestManager(t)
	_ = m.AddKey(uuidA)

	if err := m.RemoveKey(uuidA); err != nil {
		t.Fatal(err)
	}

	for tag, ids := range savedClients(t, runner, cfg) {
		if len(ids) != 0 {
			t.Errorf("%s: остались %v", tag, ids)
		}
	}
	for tag, users := range runner.APIUsers {
		if len(users) != 0 {
			t.Errorf("%s: в API остались %v", tag, users)
		}
	}
	if hasCommand(runner, restartCommand(cfg)) {
		t.Error("перезапуск не должен вызываться")
	}
}

func TestRemoveKeyRestartsWhenUserMissingInAPI(t *testing.T) {
	m, runner, cfg := newTestManager(t)
	_ = m.AddKey(uuidA)
	runner.APIUsers = map[string]map[string]bool{} // например, Xray перезапустили со старым конфигом

	if err := m.RemoveKey(uuidA); err != nil {
		t.Fatal(err)
	}

	if !hasCommand(runner, restartCommand(cfg)) {
		t.Error("ожидался перезапуск Xray")
	}
}

func TestBackupsAreRotated(t *testing.T) {
	m, runner, cfg := newTestManager(t)
	keep := cfg.XrayBackupKeep

	for i := 0; i < keep+5; i++ {
		if err := m.AddKey("00000000-0000-0000-0000-" + strings.Repeat("0", 12-len(itoa(i))) + itoa(i)); err != nil {
			t.Fatal(err)
		}
	}

	backups, _ := runner.ListDir(cfg.XrayBackupDir)
	if len(backups) != keep {
		t.Errorf("бэкапов %d, ожидалось %d", len(backups), keep)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestLinksAndRealityParams(t *testing.T) {
	m, _, _ := newTestManager(t)

	links, err := m.Links(uuidA, "phone")
	if err != nil {
		t.Fatal(err)
	}
	var transports []string
	for _, l := range links {
		transports = append(transports, l.Transport)
	}
	if !reflect.DeepEqual(transports, []string{"tcp", "grpc", "xhttp"}) {
		t.Errorf("transports = %v", transports)
	}
	sni, pub, sid, err := m.RealityParams()
	if err != nil || sni != "vpn.example.com" || pub != "dry-run-public-key" || sid != "dryrun01" {
		t.Errorf("RealityParams = %q %q %q %v", sni, pub, sid, err)
	}
}

func TestCollectTrafficAndOnline(t *testing.T) {
	m, runner, _ := newTestManager(t)
	runner.Stats = map[string]int64{
		"user>>>" + uuidA + ">>>traffic>>>uplink":   10,
		"user>>>" + uuidA + ">>>traffic>>>downlink": 20,
		"inbound>>>user-inbound>>>traffic>>>uplink": 99,
	}
	runner.Online = map[string]bool{uuidA: true}

	traffic, err := m.CollectTraffic()
	if err != nil {
		t.Fatal(err)
	}
	online, err := m.OnlineEmails()
	if err != nil {
		t.Fatal(err)
	}

	if len(traffic) != 1 || traffic[uuidA] != (TrafficStat{Uplink: 10, Downlink: 20}) {
		t.Errorf("traffic = %v", traffic)
	}
	if !reflect.DeepEqual(online, map[string]bool{uuidA: true}) {
		t.Errorf("online = %v", online)
	}
	again, _ := m.CollectTraffic()
	if len(again) != 0 {
		t.Errorf("счётчики должны быть сброшены: %v", again)
	}
}

func TestCollectTrafficParsesStringValues(t *testing.T) {
	// настоящий xray отдаёт int64 строкой и опускает нулевые значения
	m, runner, _ := newTestManager(t)
	runner.Files["raw"] = ""
	custom := &stubRunner{MemoryRunner: runner, statsOutput: `{"stat":[{"name":"user>>>` + uuidA + `>>>traffic>>>uplink","value":"1500"},{"name":"user>>>` + uuidA + `>>>traffic>>>downlink"}]}`}
	m.runner = custom

	traffic, err := m.CollectTraffic()

	if err != nil || traffic[uuidA] != (TrafficStat{Uplink: 1500}) {
		t.Errorf("traffic = %v, err = %v", traffic, err)
	}
}

// stubRunner подменяет вывод statsquery.
type stubRunner struct {
	*MemoryRunner
	statsOutput string
}

func (s *stubRunner) Run(args []string, timeout_ time.Duration) (string, error) {
	if len(args) > 2 && args[2] == "statsquery" {
		return s.statsOutput, nil
	}
	return s.MemoryRunner.Run(args, timeout_)
}

func TestImportNormalizeRestartsOnce(t *testing.T) {
	m, runner, cfg := newTestManager(t)
	parsed, _ := ParseConfig([]byte(runner.Files[cfg.XrayConfigPath]))
	inbound := asObj(asList(parsed["inbounds"])[0])
	asObj(inbound["settings"])["clients"] = []any{map[string]any{"id": uuidA, "email": "legacy", "flow": "xtls-rprx-vision"}}
	raw, _ := json.Marshal(parsed)
	runner.Files[cfg.XrayConfigPath] = string(raw)

	first, err1 := m.Normalize()
	second, err2 := m.Normalize()

	if err1 != nil || err2 != nil || !first || second {
		t.Fatalf("first=%v second=%v err=%v %v", first, second, err1, err2)
	}
	if n := countCommand(runner, restartCommand(cfg)); n != 1 {
		t.Errorf("перезапусков %d, ожидался 1", n)
	}
	if !allEqual(savedClients(t, runner, cfg), []string{uuidA}) {
		t.Errorf("clients = %v", savedClients(t, runner, cfg))
	}
}

func TestBootstrapIsIdempotentOnDryRunConfig(t *testing.T) {
	m, runner, cfg := newTestManager(t)

	changed, err := m.Bootstrap()

	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if hasCommand(runner, restartCommand(cfg)) {
		t.Error("перезапуск не нужен")
	}
}

func TestBuildRejectsSSHMode(t *testing.T) {
	cfg := testConfig(t, map[string]string{"XRAY_MODE": "ssh"})

	if _, err := Build(cfg); err == nil || !strings.Contains(err.Error(), "ssh") {
		t.Errorf("ожидалась понятная ошибка про ssh, получено %v", err)
	}
}

func TestLocalRunnerFilesAndLock(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalRunner(dir + "/run/config.lock")
	path := dir + "/conf/config.json"

	if err := r.WriteText(path, "{}"); err != nil {
		t.Fatal(err)
	}
	if err := r.Copy(path, dir+"/backups/config.1.json"); err != nil {
		t.Fatal(err)
	}
	if err := r.Replace(path, dir+"/conf/new.json"); err != nil {
		t.Fatal(err)
	}
	names, _ := r.ListDir(dir + "/conf")
	if !reflect.DeepEqual(names, []string{"new.json"}) {
		t.Errorf("ListDir = %v", names)
	}
	if out, err := r.Run([]string{"echo", "привет"}, 5*time.Second); err != nil || strings.TrimSpace(out) != "привет" {
		t.Errorf("Run = %q, %v", out, err)
	}
	if _, err := r.Run([]string{"sh", "-c", "echo bad >&2; exit 3"}, 5*time.Second); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Errorf("ожидалась ошибка со stderr, получено %v", err)
	}
	unlock, err := r.Lock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if err := r.Remove(dir + "/conf/missing.json"); err != nil {
		t.Errorf("удаление несуществующего файла не должно быть ошибкой: %v", err)
	}
}
