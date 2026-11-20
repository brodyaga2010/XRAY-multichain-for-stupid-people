package api_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"xraypanel/internal/api"
	"xraypanel/internal/config"
	"xraypanel/internal/store"
	"xraypanel/internal/xray"
)

// Схема, как её оставляет alembic (0001 + 0002; users пересоздана batch-режимом SQLite).
const alembicSchema0002 = `
CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL, CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num));
CREATE TABLE vpn_keys (
	id INTEGER NOT NULL, owner_id INTEGER NOT NULL, uuid VARCHAR(64) NOT NULL, name VARCHAR(120) NOT NULL,
	type VARCHAR(32) NOT NULL, sni VARCHAR(255) NOT NULL, status VARCHAR(32) DEFAULT 'active' NOT NULL,
	public_key VARCHAR(255) NOT NULL, short_id VARCHAR(64) NOT NULL, connection_uri TEXT NOT NULL,
	client_config_json JSON, created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL, traffic_up BIGINT DEFAULT '0' NOT NULL,
	traffic_down BIGINT DEFAULT '0' NOT NULL, last_seen_at DATETIME,
	PRIMARY KEY (id), UNIQUE (uuid), FOREIGN KEY(owner_id) REFERENCES users (id) ON DELETE CASCADE);
CREATE INDEX ix_vpn_keys_owner_id ON vpn_keys (owner_id);
CREATE TABLE audit_logs (
	id INTEGER NOT NULL, actor_id INTEGER, action VARCHAR(120) NOT NULL, target_type VARCHAR(80) NOT NULL,
	target_id VARCHAR(80) NOT NULL, metadata JSON DEFAULT '{}' NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL, PRIMARY KEY (id),
	FOREIGN KEY(actor_id) REFERENCES users (id) ON DELETE SET NULL);
CREATE TABLE "users" (
	id INTEGER NOT NULL, email VARCHAR(320) NOT NULL, password_hash VARCHAR(255) NOT NULL,
	role VARCHAR(32) DEFAULT 'user' NOT NULL, status VARCHAR(32) DEFAULT 'pending' NOT NULL,
	key_limit INTEGER DEFAULT '3' NOT NULL, created_at DATETIME DEFAULT (CURRENT_TIMESTAMP) NOT NULL,
	updated_at DATETIME DEFAULT (CURRENT_TIMESTAMP) NOT NULL, sub_token VARCHAR(64),
	PRIMARY KEY (id), CONSTRAINT uq_users_sub_token UNIQUE (sub_token), UNIQUE (email));
CREATE INDEX ix_users_email ON users (email);
INSERT INTO alembic_version VALUES ('0002_panel_v2');
`

const alembicSchema0001 = `
CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL, CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num));
CREATE TABLE users (
	id INTEGER NOT NULL, email VARCHAR(320) NOT NULL, password_hash VARCHAR(255) NOT NULL,
	role VARCHAR(32) DEFAULT 'user' NOT NULL, status VARCHAR(32) DEFAULT 'pending' NOT NULL,
	key_limit INTEGER DEFAULT '3' NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL, PRIMARY KEY (id), UNIQUE (email));
CREATE INDEX ix_users_email ON users (email);
CREATE TABLE vpn_keys (
	id INTEGER NOT NULL, owner_id INTEGER NOT NULL, uuid VARCHAR(64) NOT NULL, name VARCHAR(120) NOT NULL,
	type VARCHAR(32) NOT NULL, sni VARCHAR(255) NOT NULL, status VARCHAR(32) DEFAULT 'active' NOT NULL,
	public_key VARCHAR(255) NOT NULL, short_id VARCHAR(64) NOT NULL, connection_uri TEXT NOT NULL,
	client_config_json JSON, created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL, PRIMARY KEY (id), UNIQUE (uuid),
	FOREIGN KEY(owner_id) REFERENCES users (id) ON DELETE CASCADE);
CREATE INDEX ix_vpn_keys_owner_id ON vpn_keys (owner_id);
CREATE TABLE audit_logs (
	id INTEGER NOT NULL, actor_id INTEGER, action VARCHAR(120) NOT NULL, target_type VARCHAR(80) NOT NULL,
	target_id VARCHAR(80) NOT NULL, metadata JSON DEFAULT '{}' NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL, PRIMARY KEY (id),
	FOREIGN KEY(actor_id) REFERENCES users (id) ON DELETE SET NULL);
INSERT INTO alembic_version VALUES ('0001_initial');
`

// Пароль password123, хеш создан bcrypt из Python ($2b$, cost 12).
const pythonBcryptHash = "$2b$12$6umehH/2T5PyaPUhQKlxu.56KfmvwYDzdNeSQq9XuJxrDWaElmjxi"

// Токен, выпущенный python-jose (HS256, sub="1", exp=2099-01-01) с секретом testSecret.
const pythonJoseToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIiwiZXhwIjo0MDcwOTA4ODAwfQ.RQlgb9vtU6dGDg83ny0hhCuGK7UR8ax5u0gcc-Yf4MY"

const legacyData = `
INSERT INTO users (id, email, password_hash, role, status, key_limit, created_at, updated_at) VALUES
 (1, 'admin@example.com', '` + pythonBcryptHash + `', 'admin', 'approved', 100, '2026-05-31 10:00:00', '2026-05-31 10:00:00'),
 (2, 'user@example.com', '` + pythonBcryptHash + `', 'user', 'approved', 3, '2026-06-01 12:30:45.654321', '2026-06-01 12:30:45.654321');
INSERT INTO vpn_keys (id, owner_id, uuid, name, type, sni, status, public_key, short_id, connection_uri, created_at, updated_at) VALUES
 (1, 1, 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'старый ключ', 'tcp_reality', 'www.yahoo.com', 'active', 'pub', 'sid', 'vless://a', '2026-06-02 08:00:00', '2026-06-02 08:00:00'),
 (2, 1, 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'новый ключ', 'multi', 'www.yahoo.com', 'active', 'pub', 'sid', 'vless://b', '2026-06-03 09:15:00.000001', '2026-06-03 09:15:00.000001');
`

// legacyEnv поднимает панель поверх базы, созданной «как alembic», и выполняет migrate.
func legacyEnv(t *testing.T, schema string) (*http.Client, string, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, script := range []string{schema, legacyData} {
		if _, err := st.DB().Exec(script); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{
		"DATABASE_URL": "sqlite:///" + dbPath, "JWT_SECRET": testSecret, "XRAY_MODE": "dry_run", "XRAY_PUBLIC_HOST": "vpn.example.com",
	}
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := vars[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	manager, err := xray.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.New(api.Deps{Config: cfg, Store: st, Xray: manager}).Handler())
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}, server.URL, st
}

func TestExistingAlembicDatabaseWorks(t *testing.T) {
	for name, schema := range map[string]string{"0002_panel_v2": alembicSchema0002, "0001_initial": alembicSchema0001} {
		t.Run(name, func(t *testing.T) {
			client, base, st := legacyEnv(t, schema)
			e := &env{t: t, server: &httptest.Server{URL: base}, client: client}

			login := e.post("/auth/login", map[string]string{"email": "Admin@Example.com", "password": "password123"})
			keys := e.get("/keys")
			all := e.get("/admin/keys")
			users := e.get("/admin/users")

			if login.Status != 200 {
				t.Fatalf("вход старым паролем: %d %s", login.Status, login.Raw)
			}
			list := keys.List()
			if len(list) != 2 || list[0]["name"] != "новый ключ" || list[1]["name"] != "старый ключ" {
				t.Errorf("ключи (новые первыми): %s", keys.Raw)
			}
			if list[0]["created_at"] != "2026-06-03T09:15:00.000001Z" || list[1]["created_at"] != "2026-06-02T08:00:00Z" {
				t.Errorf("даты: %v %v", list[0]["created_at"], list[1]["created_at"])
			}
			if list[0]["type"] != "multi" || list[1]["type"] != "tcp_reality" || list[0]["client_config_json"] != nil {
				t.Errorf("поля: %v", list[0])
			}
			if all.Status != 200 || len(all.List()) != 2 || all.List()[0]["owner_email"] != "admin@example.com" {
				t.Errorf("admin/keys: %s", all.Raw)
			}
			if u := users.List(); len(u) != 2 || u[0]["email"] != "user@example.com" {
				t.Errorf("admin/users (новые первыми): %s", users.Raw)
			}
			// запись в базу, созданную alembic, и повторное чтение
			created := e.post("/keys", map[string]string{"name": "после миграции"})
			if created.Status != 201 {
				t.Fatalf("создание ключа: %d %s", created.Status, created.Raw)
			}
			if got := e.get("/keys").List(); len(got) != 3 || got[0]["name"] != "после миграции" {
				t.Errorf("порядок после вставки: %s", e.get("/keys").Raw)
			}
			var version string
			_ = st.DB().QueryRow("SELECT version_num FROM alembic_version").Scan(&version)
			if version != store.LatestRevision {
				t.Errorf("версия схемы %s", version)
			}
		})
	}
}

func TestPythonIssuedJWTIsAccepted(t *testing.T) {
	_, base, _ := legacyEnv(t, alembicSchema0002)
	req, _ := http.NewRequest("GET", base+"/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "xray_access_token", Value: pythonJoseToken})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("токен python-jose отклонён: %d", resp.StatusCode)
	}
	if !strings.Contains(strings.Join(resp.Header.Values("Content-Type"), ""), "json") {
		t.Errorf("Content-Type = %v", resp.Header["Content-Type"])
	}
}
