// Package config читает настройки панели из переменных окружения.
// Имена и значения по умолчанию совпадают с pydantic Settings прежней (Python) версии.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Config — настройки панели.
type Config struct {
	DatabaseURL        string
	JWTSecret          string
	AccessTokenMinutes int
	CookieName         string
	CookieSecure       bool
	CookieDomain       string
	CORSOrigins        string

	DefaultKeyLimit int
	AdminEmail      string
	AdminPassword   string

	PublicBaseURL           string
	SubscriptionTitle       string
	SubscriptionUpdateHours int

	AuthRateLimit         int
	AuthRateWindowSeconds int
	TrustedProxies        string

	XrayMode           string
	XrayDryRun         bool
	XrayConfigPath     string
	XrayBackupDir      string
	XrayBackupKeep     int
	XrayLockPath       string
	XrayBinary         string
	XrayAPIServer      string
	XrayRestartCommand string
	XrayXHTTPPort      int
	XrayPublicHost     string
	XrayProbePort      int
	// XrayExitTCPPort — порт TCP+Vision входа на выходном сервере: bootstrap-xray переводит на него
	// связь с выходом, если она идёт через gRPC; 0 — транспорт не менять.
	XrayExitTCPPort     int
	StatsPollSeconds    int
	OnlineWindowSeconds int

	ListenAddr string

	// Telegram-бот для уведомлений администратору; пустой токен — бот выключен.
	TelegramBotToken     string
	TelegramAdminChatIDs string
	TelegramAPIURL       string

	// Мониторинг цепочки и сервера; MonitorIntervalSeconds = 0 — выключен.
	MonitorIntervalSeconds int
	MonitorFailThreshold   int
	MonitorProbeURL        string
	MonitorChainSlowMs     int
	MonitorExitSlowMs      int

	// Ежедневная копия базы в чат администратора Telegram: час по местному времени (TZ); -1 — выключено.
	BackupHour int

	// Почта для подтверждения адреса и сброса пароля; SMTPHost пустой — письма только в журнал сервиса.
	// Порт 465 — сразу TLS (SSL), иначе STARTTLS.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
}

// DefaultJWTSecret — значение по умолчанию, пригодное только для разработки.
const DefaultJWTSecret = "change-me-in-production"

// Defaults возвращает настройки со значениями по умолчанию.
func Defaults() Config {
	return Config{
		DatabaseURL:             "sqlite:///panel.db",
		JWTSecret:               DefaultJWTSecret,
		AccessTokenMinutes:      60 * 24,
		CookieName:              "xray_access_token",
		CORSOrigins:             "http://localhost:5173,http://localhost:8080",
		DefaultKeyLimit:         3,
		PublicBaseURL:           "http://localhost:5173",
		SubscriptionTitle:       "Personal",
		SubscriptionUpdateHours: 12,
		AuthRateLimit:           10,
		AuthRateWindowSeconds:   300,
		TrustedProxies:          "127.0.0.1,::1",
		XrayMode:                "dry_run",
		XrayConfigPath:          "/usr/local/etc/xray/config.json",
		XrayBackupDir:           "/var/lib/xray-panel/backups",
		XrayBackupKeep:          30,
		XrayLockPath:            "/run/xray-panel/config.lock",
		XrayBinary:              "xray",
		XrayAPIServer:           "127.0.0.1:10085",
		XrayRestartCommand:      "systemctl restart xray",
		XrayXHTTPPort:           2053,
		XrayPublicHost:          "127.0.0.1",
		XrayProbePort:           10811,
		XrayExitTCPPort:         443,
		StatsPollSeconds:        15,
		OnlineWindowSeconds:     60,
		ListenAddr:              "127.0.0.1:8000",
		TelegramAPIURL:          "https://api.telegram.org",
		MonitorIntervalSeconds:  60,
		MonitorFailThreshold:    3,
		MonitorProbeURL:         "https://www.google.com/generate_204",
		MonitorChainSlowMs:      3000,
		MonitorExitSlowMs:       300,
		BackupHour:              4,
		SMTPPort:                465,
	}
}

// Lookup возвращает значение переменной.
type Lookup func(key string) (string, bool)

// Load собирает настройки из источника переменных и проверяет их.
func Load(lookup Lookup) (*Config, error) {
	cfg := Defaults()
	var errs []string

	str := func(dst *string, key string) {
		if v, ok := lookup(key); ok {
			*dst = v
		}
	}
	num := func(dst *int, key string) {
		v, ok := lookup(key)
		if !ok || strings.TrimSpace(v) == "" {
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: ожидается целое число, получено %q", key, v))
			return
		}
		*dst = n
	}
	flag := func(dst *bool, key string) {
		v, ok := lookup(key)
		if !ok || strings.TrimSpace(v) == "" {
			return
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "t", "yes", "y", "on":
			*dst = true
		case "0", "false", "f", "no", "n", "off":
			*dst = false
		default:
			errs = append(errs, fmt.Sprintf("%s: ожидается true/false, получено %q", key, v))
		}
	}

	str(&cfg.DatabaseURL, "DATABASE_URL")
	str(&cfg.JWTSecret, "JWT_SECRET")
	num(&cfg.AccessTokenMinutes, "ACCESS_TOKEN_MINUTES")
	str(&cfg.CookieName, "COOKIE_NAME")
	flag(&cfg.CookieSecure, "COOKIE_SECURE")
	str(&cfg.CookieDomain, "COOKIE_DOMAIN")
	str(&cfg.CORSOrigins, "CORS_ORIGINS")
	num(&cfg.DefaultKeyLimit, "DEFAULT_KEY_LIMIT")
	str(&cfg.AdminEmail, "ADMIN_EMAIL")
	str(&cfg.AdminPassword, "ADMIN_PASSWORD")
	str(&cfg.PublicBaseURL, "PUBLIC_BASE_URL")
	str(&cfg.SubscriptionTitle, "SUBSCRIPTION_TITLE")
	num(&cfg.SubscriptionUpdateHours, "SUBSCRIPTION_UPDATE_HOURS")
	num(&cfg.AuthRateLimit, "AUTH_RATE_LIMIT")
	num(&cfg.AuthRateWindowSeconds, "AUTH_RATE_WINDOW_SECONDS")
	str(&cfg.TrustedProxies, "TRUSTED_PROXIES")
	str(&cfg.XrayMode, "XRAY_MODE")
	flag(&cfg.XrayDryRun, "XRAY_DRY_RUN")
	str(&cfg.XrayConfigPath, "XRAY_CONFIG_PATH")
	str(&cfg.XrayBackupDir, "XRAY_BACKUP_DIR")
	num(&cfg.XrayBackupKeep, "XRAY_BACKUP_KEEP")
	str(&cfg.XrayLockPath, "XRAY_LOCK_PATH")
	str(&cfg.XrayBinary, "XRAY_BINARY")
	str(&cfg.XrayAPIServer, "XRAY_API_SERVER")
	str(&cfg.XrayRestartCommand, "XRAY_RESTART_COMMAND")
	num(&cfg.XrayXHTTPPort, "XRAY_XHTTP_PORT")
	str(&cfg.XrayPublicHost, "XRAY_PUBLIC_HOST")
	num(&cfg.XrayProbePort, "XRAY_PROBE_PORT")
	num(&cfg.XrayExitTCPPort, "XRAY_EXIT_TCP_PORT")
	num(&cfg.StatsPollSeconds, "STATS_POLL_SECONDS")
	num(&cfg.OnlineWindowSeconds, "ONLINE_WINDOW_SECONDS")
	str(&cfg.ListenAddr, "LISTEN_ADDR")
	str(&cfg.TelegramBotToken, "TELEGRAM_BOT_TOKEN")
	str(&cfg.TelegramAdminChatIDs, "TELEGRAM_ADMIN_CHAT_IDS")
	str(&cfg.TelegramAPIURL, "TELEGRAM_API_URL")
	num(&cfg.MonitorIntervalSeconds, "MONITOR_INTERVAL_SECONDS")
	num(&cfg.MonitorFailThreshold, "MONITOR_FAIL_THRESHOLD")
	str(&cfg.MonitorProbeURL, "MONITOR_PROBE_URL")
	num(&cfg.MonitorChainSlowMs, "MONITOR_CHAIN_SLOW_MS")
	num(&cfg.MonitorExitSlowMs, "MONITOR_EXIT_SLOW_MS")
	num(&cfg.BackupHour, "BACKUP_HOUR")
	str(&cfg.SMTPHost, "SMTP_HOST")
	num(&cfg.SMTPPort, "SMTP_PORT")
	str(&cfg.SMTPUsername, "SMTP_USERNAME")
	str(&cfg.SMTPPassword, "SMTP_PASSWORD")
	str(&cfg.SMTPFrom, "SMTP_FROM")
	str(&cfg.SMTPFromName, "SMTP_FROM_NAME")

	if len([]rune(cfg.JWTSecret)) < 16 {
		errs = append(errs, "JWT_SECRET: минимум 16 символов")
	}
	switch cfg.XrayMode {
	case "local", "ssh", "dry_run":
	default:
		errs = append(errs, fmt.Sprintf("XRAY_MODE: допустимо local, ssh или dry_run, получено %q", cfg.XrayMode))
	}
	if _, err := cfg.TelegramAdminChats(); err != nil {
		errs = append(errs, err.Error())
	}
	if cfg.BackupHour < -1 || cfg.BackupHour > 23 {
		errs = append(errs, fmt.Sprintf("BACKUP_HOUR: час от 0 до 23 или -1 (выключено), получено %d", cfg.BackupHour))
	}
	if cfg.SMTPHost != "" {
		if cfg.SMTPFrom == "" {
			cfg.SMTPFrom = cfg.SMTPUsername
		}
		if cfg.SMTPPort < 1 || cfg.SMTPPort > 65535 {
			errs = append(errs, fmt.Sprintf("SMTP_PORT: порт от 1 до 65535, получено %d", cfg.SMTPPort))
		}
		if !strings.Contains(cfg.SMTPFrom, "@") {
			errs = append(errs, "SMTP_FROM: адрес отправителя (или SMTP_USERNAME в виде адреса)")
		}
	}
	if cfg.SMTPFromName == "" {
		cfg.SMTPFromName = cfg.SubscriptionTitle
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("некорректная конфигурация: %s", strings.Join(errs, "; "))
	}
	return &cfg, nil
}

// FromEnv читает настройки из окружения процесса; значения из envFile (если задан)
// используются, когда переменная в окружении не задана.
func FromEnv(envFile string) (*Config, error) {
	file := map[string]string{}
	if envFile != "" {
		f, err := os.Open(envFile)
		if err != nil {
			return nil, fmt.Errorf("файл окружения: %w", err)
		}
		defer f.Close()
		file, err = ParseEnvFile(f)
		if err != nil {
			return nil, fmt.Errorf("файл окружения %s: %w", envFile, err)
		}
	}
	return Load(func(key string) (string, bool) {
		if v, ok := os.LookupEnv(key); ok {
			return v, true
		}
		v, ok := file[key]
		return v, ok
	})
}

// ParseEnvFile разбирает строки KEY=VALUE и KEY="VALUE" (также 'VALUE' и «export KEY=...»).
func ParseEnvFile(r io.Reader) (map[string]string, error) {
	result := map[string]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("строка %d: ожидается KEY=VALUE", lineNo)
		}
		result[key] = unquote(strings.TrimSpace(value))
	}
	return result, scanner.Err()
}

func unquote(v string) string {
	if len(v) >= 2 {
		switch {
		case v[0] == '"' && v[len(v)-1] == '"':
			inner := v[1 : len(v)-1]
			return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(inner)
		case v[0] == '\'' && v[len(v)-1] == '\'':
			return v[1 : len(v)-1]
		}
	}
	return v
}

// CORSOriginList — список разрешённых origin.
func (c *Config) CORSOriginList() []string { return splitList(c.CORSOrigins) }

// TrustedProxyList — адреса, от которых учитывается X-Forwarded-For.
func (c *Config) TrustedProxyList() map[string]struct{} {
	set := map[string]struct{}{}
	for _, item := range splitList(c.TrustedProxies) {
		set[item] = struct{}{}
	}
	return set
}

// TelegramAdminChats — чаты Telegram, которым бот шлёт заявки и чьи нажатия принимает.
func (c *Config) TelegramAdminChats() ([]int64, error) {
	var chats []int64
	for _, item := range splitList(c.TelegramAdminChatIDs) {
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("TELEGRAM_ADMIN_CHAT_IDS: ожидаются числа через запятую, получено %q", item)
		}
		chats = append(chats, id)
	}
	return chats, nil
}

// EffectiveXrayMode учитывает устаревший флаг XRAY_DRY_RUN.
func (c *Config) EffectiveXrayMode() string {
	if c.XrayDryRun {
		return "dry_run"
	}
	return c.XrayMode
}

// SQLitePath достаёт путь к файлу базы из DATABASE_URL (формат SQLAlchemy:
// sqlite:////abs/path.db, sqlite:///relative.db, sqlite+driver:///:memory:).
func (c *Config) SQLitePath() (string, error) {
	scheme, rest, ok := strings.Cut(c.DatabaseURL, "://")
	if !ok || (scheme != "sqlite" && !strings.HasPrefix(scheme, "sqlite+")) {
		return "", fmt.Errorf("DATABASE_URL: поддерживается только SQLite (sqlite:////путь/к/файлу.db)")
	}
	path, ok := strings.CutPrefix(rest, "/")
	if !ok || path == "" {
		return "", fmt.Errorf("DATABASE_URL: ожидается sqlite:///<путь>")
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return path, nil
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
