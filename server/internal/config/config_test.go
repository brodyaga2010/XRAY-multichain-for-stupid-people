package config

import (
	"fmt"
	"strings"
	"testing"
)

func lookupOf(env map[string]string) Lookup {
	return func(key string) (string, bool) { v, ok := env[key]; return v, ok }
}

func TestDefaultsMatchPythonSettings(t *testing.T) {
	cfg, err := Load(lookupOf(nil))
	if err != nil {
		t.Fatal(err)
	}

	want := Config{
		DatabaseURL: "sqlite:///panel.db", JWTSecret: "change-me-in-production",
		AccessTokenMinutes: 1440, CookieName: "xray_access_token", CORSOrigins: "http://localhost:5173,http://localhost:8080",
		DefaultKeyLimit: 3, PublicBaseURL: "http://localhost:5173", SubscriptionTitle: "Personal", SubscriptionUpdateHours: 12,
		AuthRateLimit: 10, AuthRateWindowSeconds: 300, TrustedProxies: "127.0.0.1,::1", XrayMode: "dry_run",
		XrayConfigPath: "/usr/local/etc/xray/config.json", XrayBackupDir: "/var/lib/xray-panel/backups", XrayBackupKeep: 30,
		XrayLockPath: "/run/xray-panel/config.lock", XrayBinary: "xray", XrayAPIServer: "127.0.0.1:10085",
		XrayRestartCommand: "systemctl restart xray", XrayXHTTPPort: 2053, XrayPublicHost: "127.0.0.1", XrayProbePort: 10811, XrayExitTCPPort: 443,
		StatsPollSeconds: 15, OnlineWindowSeconds: 60, ListenAddr: "127.0.0.1:8000", TelegramAPIURL: "https://api.telegram.org",
		MonitorIntervalSeconds: 60, MonitorFailThreshold: 3, MonitorProbeURL: "https://www.google.com/generate_204",
		MonitorChainSlowMs: 3000, MonitorExitSlowMs: 300, MonitorAutoRestartMinutes: 30, BackupHour: 4,
		SMTPPort: 465, SMTPFromName: "Personal",
	}
	if *cfg != want {
		t.Errorf("настройки по умолчанию:\n got %+v\nwant %+v", *cfg, want)
	}
	if cfg.EffectiveXrayMode() != "dry_run" || cfg.CookieSecure || cfg.AdminEmail != "" {
		t.Errorf("прочее: %+v", cfg)
	}
}

func TestLoadOverridesAndValidation(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		check   func(*Config) bool
		wantErr string
	}{
		{"все переопределения", map[string]string{
			"JWT_SECRET": "0123456789abcdef", "COOKIE_SECURE": "true", "COOKIE_NAME": "sid", "COOKIE_DOMAIN": "example.com",
			"ACCESS_TOKEN_MINUTES": "5", "XRAY_MODE": "local", "XRAY_XHTTP_PORT": "0", "LISTEN_ADDR": ":9000",
			"STATS_POLL_SECONDS": "15", "ADMIN_EMAIL": "a@example.com",
		}, func(c *Config) bool {
			return c.CookieSecure && c.CookieName == "sid" && c.CookieDomain == "example.com" && c.AccessTokenMinutes == 5 &&
				c.XrayMode == "local" && c.XrayXHTTPPort == 0 && c.ListenAddr == ":9000" && c.StatsPollSeconds == 15 &&
				c.AdminEmail == "a@example.com"
		}, ""},
		{"устаревший XRAY_DRY_RUN", map[string]string{"XRAY_MODE": "local", "XRAY_DRY_RUN": "1"},
			func(c *Config) bool { return c.EffectiveXrayMode() == "dry_run" }, ""},
		{"пустое число — значение по умолчанию", map[string]string{"AUTH_RATE_LIMIT": ""},
			func(c *Config) bool { return c.AuthRateLimit == 10 }, ""},
		{"короткий секрет", map[string]string{"JWT_SECRET": "short"}, nil, "JWT_SECRET"},
		{"плохой режим", map[string]string{"XRAY_MODE": "docker"}, nil, "XRAY_MODE"},
		{"не число", map[string]string{"AUTH_RATE_LIMIT": "ten"}, nil, "AUTH_RATE_LIMIT"},
		{"не bool", map[string]string{"COOKIE_SECURE": "maybe"}, nil, "COOKIE_SECURE"},
		{"копия выключена", map[string]string{"BACKUP_HOUR": "-1"}, func(c *Config) bool { return c.BackupHour == -1 }, ""},
		{"копия в полночь", map[string]string{"BACKUP_HOUR": "0"}, func(c *Config) bool { return c.BackupHour == 0 }, ""},
		{"час больше 23", map[string]string{"BACKUP_HOUR": "24"}, nil, "BACKUP_HOUR"},
		{"час меньше -1", map[string]string{"BACKUP_HOUR": "-2"}, nil, "BACKUP_HOUR"},
		{"почта: отправитель из логина", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_USERNAME": "noreply@example.com"},
			func(c *Config) bool { return c.SMTPFrom == "noreply@example.com" && c.SMTPPort == 465 }, ""},
		{"почта: своё имя отправителя", map[string]string{"SMTP_FROM_NAME": "VPN", "SUBSCRIPTION_TITLE": "X"},
			func(c *Config) bool { return c.SMTPFromName == "VPN" }, ""},
		{"почта: имя по умолчанию — из SUBSCRIPTION_TITLE", map[string]string{"SUBSCRIPTION_TITLE": "kemsekov"},
			func(c *Config) bool { return c.SMTPFromName == "kemsekov" }, ""},
		{"почта: нет адреса отправителя", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_USERNAME": "login"}, nil, "SMTP_FROM"},
		{"почта: порт вне диапазона", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "a@b.c", "SMTP_PORT": "70000"}, nil, "SMTP_PORT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(lookupOf(tt.env))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ожидалась ошибка с %q, получено %v", tt.wantErr, err)
				}
				return
			}
			if err != nil || !tt.check(cfg) {
				t.Errorf("cfg=%+v err=%v", cfg, err)
			}
		})
	}
}

func TestSQLitePath(t *testing.T) {
	tests := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"sqlite:////var/lib/xray-panel/panel.db", "/var/lib/xray-panel/panel.db", false},
		{"sqlite:///relative.db", "relative.db", false},
		{"sqlite+pysqlite:///:memory:", ":memory:", false},
		{"sqlite:////tmp/a b.db?mode=rw", "/tmp/a b.db", false},
		{"postgresql+psycopg://xray:xray@postgres:5432/db", "", true},
		{"sqlite://", "", true},
		{"/just/a/path", "", true},
	}
	for _, tt := range tests {
		got, err := (&Config{DatabaseURL: tt.url}).SQLitePath()
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("SQLitePath(%q) = %q, %v", tt.url, got, err)
		}
	}
}

func TestParseEnvFile(t *testing.T) {
	input := `
# комментарий
DATABASE_URL=sqlite:////var/lib/xray-panel/panel.db
JWT_SECRET="quoted secret value"
SUBSCRIPTION_TITLE='Мой сервис'
export COOKIE_SECURE=true
  PUBLIC_BASE_URL = https://example.com
EMPTY=
ESCAPED="a \"b\" c"
URL_WITH_EQUALS=https://x.example/?a=b&c=d
`

	got, err := ParseEnvFile(strings.NewReader(input))

	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"DATABASE_URL": "sqlite:////var/lib/xray-panel/panel.db", "JWT_SECRET": "quoted secret value",
		"SUBSCRIPTION_TITLE": "Мой сервис", "COOKIE_SECURE": "true", "PUBLIC_BASE_URL": "https://example.com",
		"EMPTY": "", "ESCAPED": `a "b" c`, "URL_WITH_EQUALS": "https://x.example/?a=b&c=d",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, ожидалось %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("лишние ключи: %v", got)
	}
	if _, err := ParseEnvFile(strings.NewReader("не пара")); err == nil {
		t.Error("строка без = должна давать ошибку")
	}
}

func TestListsAreTrimmed(t *testing.T) {
	cfg := Config{CORSOrigins: " http://a.test , ,http://b.test", TrustedProxies: "127.0.0.1, ::1"}

	if got := cfg.CORSOriginList(); len(got) != 2 || got[0] != "http://a.test" || got[1] != "http://b.test" {
		t.Errorf("CORS: %v", got)
	}
	proxies := cfg.TrustedProxyList()
	if _, ok := proxies["::1"]; !ok || len(proxies) != 2 {
		t.Errorf("прокси: %v", proxies)
	}
}

func TestTelegramChats(t *testing.T) {
	tests := []struct {
		env     string
		raw     string
		want    []int64
		wantErr bool
	}{
		{"TELEGRAM_ADMIN_CHAT_IDS", "", nil, false},
		{"TELEGRAM_ADMIN_CHAT_IDS", "111222333", []int64{111222333}, false},
		{"TELEGRAM_ADMIN_CHAT_IDS", " 1, -100200300 ,", []int64{1, -100200300}, false},
		{"TELEGRAM_ADMIN_CHAT_IDS", "me", nil, true},
		{"TELEGRAM_ALERT_CHAT_IDS", "-4000000001", []int64{-4000000001}, false},
		{"TELEGRAM_ALERT_CHAT_IDS", "-4000000001,group", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.env+"="+tt.raw, func(t *testing.T) {
			_, err := Load(func(key string) (string, bool) {
				switch key {
				case "JWT_SECRET":
					return "long-enough-secret-value", true
				case tt.env:
					return tt.raw, true
				}
				return "", false
			})
			if (err != nil) != tt.wantErr || (err != nil && !strings.Contains(err.Error(), tt.env)) {
				t.Fatalf("Load err = %v, wantErr %v", err, tt.wantErr)
			}
			cfg := Config{TelegramAdminChatIDs: tt.raw, TelegramAlertChatIDs: tt.raw}
			get := map[string]func() ([]int64, error){
				"TELEGRAM_ADMIN_CHAT_IDS": cfg.TelegramAdminChats, "TELEGRAM_ALERT_CHAT_IDS": cfg.TelegramAlertChats,
			}[tt.env]
			got, _ := get()
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("chats = %v, want %v", got, tt.want)
			}
		})
	}
}
