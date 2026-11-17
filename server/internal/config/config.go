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

	XrayMode            string
	XrayDryRun          bool
	XrayConfigPath      string
	XrayBackupDir       string
	XrayBackupKeep      int
	XrayLockPath        string
	XrayBinary          string
	XrayAPIServer       string
	XrayRestartCommand  string
	XrayXHTTPPort       int
	XrayPublicHost      string
	StatsPollSeconds    int
	OnlineWindowSeconds int

	ListenAddr string
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
		StatsPollSeconds:        60,
		OnlineWindowSeconds:     180,
		ListenAddr:              "127.0.0.1:8000",
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
	num(&cfg.StatsPollSeconds, "STATS_POLL_SECONDS")
	num(&cfg.OnlineWindowSeconds, "ONLINE_WINDOW_SECONDS")
	str(&cfg.ListenAddr, "LISTEN_ADDR")

	if len([]rune(cfg.JWTSecret)) < 16 {
		errs = append(errs, "JWT_SECRET: минимум 16 символов")
	}
	switch cfg.XrayMode {
	case "local", "ssh", "dry_run":
	default:
		errs = append(errs, fmt.Sprintf("XRAY_MODE: допустимо local, ssh или dry_run, получено %q", cfg.XrayMode))
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
