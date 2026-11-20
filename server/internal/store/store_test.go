package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// DDL и данные, как их оставляют alembic 0001 и 0001+0002 (SQLAlchemy, SQLite).
const alembic0001 = `
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

const alembicData = `
INSERT INTO users (id, email, password_hash, role, status, key_limit, created_at, updated_at) VALUES
 (1, 'admin@example.com', '$2b$12$6umehH/2T5PyaPUhQKlxu.56KfmvwYDzdNeSQq9XuJxrDWaElmjxi', 'admin', 'approved', 100,
  '2026-05-31 10:00:00', '2026-05-31 10:00:00.123456'),
 (2, 'user@example.com', 'x', 'user', 'approved', 3, '2026-06-01 12:30:45.654321', '2026-06-01 12:30:45');
INSERT INTO vpn_keys (id, owner_id, uuid, name, type, sni, status, public_key, short_id, connection_uri, client_config_json, created_at, updated_at) VALUES
 (1, 1, 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'старый ключ', 'tcp_reality', 'www.yahoo.com', 'active', 'pub', 'sid', 'vless://a', NULL,
  '2026-06-02 08:00:00', '2026-06-02 08:00:00'),
 (2, 2, 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'второй', 'multi', 'www.yahoo.com', 'revoked', 'pub', 'sid', 'vless://b', 'null',
  '2026-06-03 09:15:00.000001', '2026-06-03 09:15:00.000001');
INSERT INTO audit_logs (id, actor_id, action, target_type, target_id, metadata, created_at) VALUES
 (1, 1, 'key.create', 'vpn_key', '1', '{"uuid": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}', '2026-06-02 08:00:00');
`

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// openMigrated — временная база с актуальной схемой.
func openMigrated(t *testing.T) *Store {
	t.Helper()
	st := openTemp(t)
	if _, err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

func columns(t *testing.T, st *Store, table string) []string {
	t.Helper()
	rows, err := st.DB().Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		cols = append(cols, name)
	}
	sort.Strings(cols)
	return cols
}

func version(t *testing.T, st *Store) string {
	t.Helper()
	var v string
	if err := st.DB().QueryRow("SELECT version_num FROM alembic_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMigrateCreatesSchemaOnEmptyDatabase(t *testing.T) {
	st := openTemp(t)

	if _, err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	if v := version(t, st); v != LatestRevision {
		t.Errorf("версия = %s", v)
	}
	want := map[string][]string{
		"users":             {"created_at", "email", "id", "key_limit", "password_hash", "role", "status", "sub_token", "updated_at"},
		"vpn_keys":          {"client_config_json", "connection_uri", "created_at", "id", "last_seen_at", "name", "owner_id", "public_key", "short_id", "sni", "status", "traffic_down", "traffic_up", "type", "updated_at", "uuid"},
		"audit_logs":        {"action", "actor_id", "created_at", "id", "metadata", "target_id", "target_type"},
		"monitor_points":    {"at", "ms", "status"},
		"monitor_incidents": {"check_id", "detail", "ended_at", "id", "name", "started_at", "status"},
		"traffic_daily":     {"day", "down", "key_id", "up"},
	}
	for table, cols := range want {
		if got := columns(t, st, table); !reflect.DeepEqual(got, cols) {
			t.Errorf("%s: колонки %v", table, got)
		}
	}
	var indexes []string
	rows, _ := st.DB().Query("SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'ix_%' ORDER BY name")
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		indexes = append(indexes, n)
	}
	rows.Close()
	if !reflect.DeepEqual(indexes, []string{"ix_monitor_incidents_started_at", "ix_traffic_daily_day", "ix_users_email", "ix_vpn_keys_owner_id"}) {
		t.Errorf("индексы: %v", indexes)
	}
	var journal string
	_ = st.DB().QueryRow("PRAGMA journal_mode").Scan(&journal)
	if journal != "wal" {
		t.Errorf("journal_mode = %s", journal)
	}
	var busy int
	_ = st.DB().QueryRow("PRAGMA busy_timeout").Scan(&busy)
	if busy != 5000 {
		t.Errorf("busy_timeout = %d", busy)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	first, err1 := st.Migrate(ctx)
	second, err2 := st.Migrate(ctx)

	if err1 != nil || err2 != nil {
		t.Fatalf("%v %v", err1, err2)
	}
	if !strings.Contains(first, "создана") || !strings.Contains(second, "актуальна") {
		t.Errorf("результаты: %q, %q", first, second)
	}
}

func TestMigrateUpgrades0001KeepingData(t *testing.T) {
	st := openTemp(t)
	for _, script := range []string{alembic0001, alembicData} {
		if _, err := st.DB().Exec(script); err != nil {
			t.Fatal(err)
		}
	}

	result, err := st.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "0001_initial -> "+LatestRevision) || version(t, st) != LatestRevision {
		t.Errorf("результат %q, версия %s", result, version(t, st))
	}
	ctx := context.Background()
	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("пользователи: %v %v", users, err)
	}
	// новые колонки с значениями по умолчанию
	keys, err := st.KeysByOwner(ctx, 1)
	if err != nil || len(keys) != 1 || keys[0].TrafficUp != 0 || keys[0].LastSeenAt != nil {
		t.Fatalf("ключи: %+v %v", keys, err)
	}
	// уникальность sub_token работает
	u1, _ := st.UserByID(ctx, 1)
	u2, _ := st.UserByID(ctx, 2)
	if err := st.SetSubToken(ctx, u1.ID, "same-token-same-token"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSubToken(ctx, u2.ID, "same-token-same-token"); err == nil {
		t.Error("sub_token должен быть уникальным")
	}
}

func TestReadsAlembicDatesAndValues(t *testing.T) {
	st := openTemp(t)
	for _, script := range []string{alembic0001, alembicData} {
		if _, err := st.DB().Exec(script); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	admin, err := st.UserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := st.UserByID(ctx, 2)
	keys, _ := st.KeysByOwner(ctx, 2)
	audit, _ := st.ListAudit(ctx, 10)

	if !admin.CreatedAt.Equal(time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)) ||
		!admin.UpdatedAt.Equal(time.Date(2026, 5, 31, 10, 0, 0, 123456000, time.UTC)) {
		t.Errorf("даты администратора: %v %v", admin.CreatedAt, admin.UpdatedAt)
	}
	if !user.CreatedAt.Equal(time.Date(2026, 6, 1, 12, 30, 45, 654321000, time.UTC)) {
		t.Errorf("created_at пользователя: %v", user.CreatedAt)
	}
	if keys[0].ClientConfigJSON != nil {
		t.Errorf("JSON 'null' должен читаться как отсутствие значения: %v", *keys[0].ClientConfigJSON)
	}
	if len(audit) != 1 || audit[0].ActorEmail == nil || *audit[0].ActorEmail != "admin@example.com" ||
		!strings.Contains(audit[0].Metadata, "uuid") {
		t.Errorf("аудит: %+v", audit)
	}
}

func TestMigrateRejectsUnknownState(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{"таблицы есть, версии нет", `CREATE TABLE users (id INTEGER)`},
		{"неизвестная версия", `CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL); INSERT INTO alembic_version VALUES ('0099_future')`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := openTemp(t)
			if _, err := st.DB().Exec(tt.script); err != nil {
				t.Fatal(err)
			}
			if _, err := st.Migrate(context.Background()); err == nil {
				t.Error("ожидалась ошибка")
			}
		})
	}
}

func TestDatesAreWrittenInSQLAlchemyFormat(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	_, _ = st.Migrate(ctx)
	u := &User{Email: "a@example.com", PasswordHash: "h", Role: RoleUser, Status: UserPending, KeyLimit: 3}

	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	var raw string
	if err := st.DB().QueryRow("SELECT CAST(created_at AS TEXT) FROM users WHERE id = ?", u.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse("2006-01-02 15:04:05.000000", raw); err != nil {
		t.Errorf("формат даты %q: %v", raw, err)
	}
	if err := st.CreateUser(ctx, &User{Email: "a@example.com", PasswordHash: "h", Role: RoleUser, Status: UserPending}); err != ErrConflict {
		t.Errorf("повторный email: %v", err)
	}
}

func TestTxRollsBackOnError(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	_, _ = st.Migrate(ctx)

	err := st.Tx(ctx, func(q *Queries) error {
		if err := q.CreateUser(ctx, &User{Email: "a@example.com", PasswordHash: "h", Role: RoleUser, Status: UserPending}); err != nil {
			return err
		}
		return sql.ErrConnDone
	})

	if err != sql.ErrConnDone {
		t.Fatalf("err = %v", err)
	}
	if _, err := st.UserByEmail(ctx, "a@example.com"); err != ErrNotFound {
		t.Errorf("пользователь должен быть откатён: %v", err)
	}
}

func TestAddTrafficAccumulates(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	_, _ = st.Migrate(ctx)
	owner := &User{Email: "a@example.com", PasswordHash: "h", Role: RoleUser, Status: UserApproved, KeyLimit: 3}
	_ = st.CreateUser(ctx, owner)
	key := &VpnKey{OwnerID: owner.ID, UUID: "u-1", Name: "k", Type: "multi", SNI: "s", Status: KeyActive, PublicKey: "p", ShortID: "s", ConnectionURI: "vless://u-1"}
	if err := st.InsertKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	seen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	found1, _ := st.AddTraffic(ctx, "u-1", 10, 20, &seen, "2026-11-20")
	found2, _ := st.AddTraffic(ctx, "u-1", 1, 2, nil, "2026-11-20")
	missing, _ := st.AddTraffic(ctx, "nope", 1, 1, nil, "2026-11-20")

	got, _ := st.KeyByID(ctx, key.ID)
	if !found1 || !found2 || missing || got.TrafficUp != 11 || got.TrafficDown != 22 || got.LastSeenAt == nil || !got.LastSeenAt.Equal(seen) {
		t.Errorf("ключ: %+v", got)
	}
}
