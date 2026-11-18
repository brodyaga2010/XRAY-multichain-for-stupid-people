package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const userCols = "id, email, password_hash, role, status, key_limit, sub_token, created_at, updated_at"

type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (*User, error) {
	var u User
	var sub sql.NullString
	var created, updated nullTime
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.KeyLimit, &sub, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if sub.Valid {
		u.SubToken = &sub.String
	}
	u.CreatedAt, u.UpdatedAt = created.T, updated.T
	return &u, nil
}

// ---------- пользователи ----------

func (q *Queries) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(q.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE id = ?", id))
}

func (q *Queries) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(q.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE email = ?", email))
}

func (q *Queries) UserBySubToken(ctx context.Context, token string) (*User, error) {
	return scanUser(q.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE sub_token = ?", token))
}

// FirstAdmin — администратор с наименьшим id (от его имени пишутся действия из Telegram).
func (q *Queries) FirstAdmin(ctx context.Context) (*User, error) {
	return scanUser(q.db.QueryRowContext(ctx,
		"SELECT "+userCols+" FROM users WHERE role = ? ORDER BY id LIMIT 1", RoleAdmin))
}

// ListUsers — новые первыми (created_at DESC, id DESC).
func (q *Queries) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT "+userCols+" FROM users ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// CreateUser вставляет пользователя; created_at/updated_at ставятся здесь.
func (q *Queries) CreateUser(ctx context.Context, u *User) error {
	now := Now()
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role, status, key_limit, sub_token, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Email, u.PasswordHash, u.Role, u.Status, u.KeyLimit, u.SubToken, formatTime(now), formatTime(now))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	u.CreatedAt, u.UpdatedAt = now, now
	return nil
}

func (q *Queries) updateUser(ctx context.Context, id int64, set string, args ...any) error {
	args = append(args, formatTime(Now()), id)
	_, err := q.db.ExecContext(ctx, "UPDATE users SET "+set+", updated_at = ? WHERE id = ?", args...)
	return err
}

func (q *Queries) SetUserStatus(ctx context.Context, id int64, status string) error {
	return q.updateUser(ctx, id, "status = ?", status)
}

func (q *Queries) SetKeyLimit(ctx context.Context, id int64, limit int) error {
	return q.updateUser(ctx, id, "key_limit = ?", limit)
}

func (q *Queries) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	return q.updateUser(ctx, id, "password_hash = ?", hash)
}

func (q *Queries) SetSubToken(ctx context.Context, id int64, token string) error {
	return q.updateUser(ctx, id, "sub_token = ?", token)
}

// ---------- ключи ----------

const keyCols = `id, owner_id, uuid, name, type, sni, status, public_key, short_id, connection_uri,
	client_config_json, traffic_up, traffic_down, last_seen_at, created_at, updated_at`

func scanKey(row scanner, extra ...any) (*VpnKey, error) {
	var k VpnKey
	var cfg sql.NullString
	var seen, created, updated nullTime
	dest := []any{&k.ID, &k.OwnerID, &k.UUID, &k.Name, &k.Type, &k.SNI, &k.Status, &k.PublicKey, &k.ShortID,
		&k.ConnectionURI, &cfg, &k.TrafficUp, &k.TrafficDown, &seen, &created, &updated}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if cfg.Valid && strings.TrimSpace(cfg.String) != "" && strings.TrimSpace(cfg.String) != "null" {
		text := cfg.String
		k.ClientConfigJSON = &text
	}
	k.LastSeenAt, k.CreatedAt, k.UpdatedAt = seen.ptr(), created.T, updated.T
	return &k, nil
}

func (q *Queries) KeyByID(ctx context.Context, id int64) (*VpnKey, error) {
	return scanKey(q.db.QueryRowContext(ctx, "SELECT "+keyCols+" FROM vpn_keys WHERE id = ?", id))
}

func (q *Queries) queryKeys(ctx context.Context, query string, args ...any) ([]VpnKey, error) {
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []VpnKey{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

// KeysByOwner — ключи владельца, новые первыми.
func (q *Queries) KeysByOwner(ctx context.Context, ownerID int64) ([]VpnKey, error) {
	return q.queryKeys(ctx, "SELECT "+keyCols+" FROM vpn_keys WHERE owner_id = ? ORDER BY created_at DESC, id DESC", ownerID)
}

// KeysByOwnerStatus — ключи владельца с заданным статусом.
func (q *Queries) KeysByOwnerStatus(ctx context.Context, ownerID int64, status string) ([]VpnKey, error) {
	return q.queryKeys(ctx, "SELECT "+keyCols+" FROM vpn_keys WHERE owner_id = ? AND status = ? ORDER BY id", ownerID, status)
}

// ActiveKeysOldestFirst — активные ключи владельца для подписки (created_at, id).
func (q *Queries) ActiveKeysOldestFirst(ctx context.Context, ownerID int64) ([]VpnKey, error) {
	return q.queryKeys(ctx, "SELECT "+keyCols+" FROM vpn_keys WHERE owner_id = ? AND status = ? ORDER BY created_at, id",
		ownerID, KeyActive)
}

// AllKeysWithOwner — все ключи с email владельца, новые первыми.
func (q *Queries) AllKeysWithOwner(ctx context.Context) ([]KeyWithOwner, error) {
	rows, err := q.db.QueryContext(ctx,
		"SELECT "+prefixCols("k.", keyCols)+", u.email FROM vpn_keys k JOIN users u ON u.id = k.owner_id ORDER BY k.created_at DESC, k.id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []KeyWithOwner{}
	for rows.Next() {
		var email string
		k, err := scanKey(rows, &email)
		if err != nil {
			return nil, err
		}
		keys = append(keys, KeyWithOwner{VpnKey: *k, OwnerEmail: email})
	}
	return keys, rows.Err()
}

func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func (q *Queries) CountActiveKeys(ctx context.Context, ownerID int64) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, "SELECT COUNT(id) FROM vpn_keys WHERE owner_id = ? AND status = ?", ownerID, KeyActive).Scan(&n)
	return n, err
}

// KnownUUIDs — UUID всех ключей в БД.
func (q *Queries) KnownUUIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT uuid FROM vpn_keys")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		known[id] = true
	}
	return known, rows.Err()
}

// InsertKey вставляет ключ; created_at/updated_at ставятся здесь.
func (q *Queries) InsertKey(ctx context.Context, k *VpnKey) error {
	now := Now()
	var cfg any
	if k.ClientConfigJSON != nil {
		cfg = *k.ClientConfigJSON
	}
	var seen any
	if k.LastSeenAt != nil {
		seen = formatTime(*k.LastSeenAt)
	}
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO vpn_keys (owner_id, uuid, name, type, sni, status, public_key, short_id, connection_uri,
		   client_config_json, traffic_up, traffic_down, last_seen_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.OwnerID, k.UUID, k.Name, k.Type, k.SNI, k.Status, k.PublicKey, k.ShortID, k.ConnectionURI,
		cfg, k.TrafficUp, k.TrafficDown, seen, formatTime(now), formatTime(now))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	k.ID, _ = res.LastInsertId()
	k.CreatedAt, k.UpdatedAt = now, now
	return nil
}

func (q *Queries) SetKeyStatus(ctx context.Context, id int64, status string) error {
	_, err := q.db.ExecContext(ctx, "UPDATE vpn_keys SET status = ?, updated_at = ? WHERE id = ?",
		status, formatTime(Now()), id)
	return err
}

// SetKeyLastSeen задаёт время последней активности ключа (используется тестами).
func (q *Queries) SetKeyLastSeen(ctx context.Context, id int64, seen time.Time) error {
	_, err := q.db.ExecContext(ctx, "UPDATE vpn_keys SET last_seen_at = ? WHERE id = ?", formatTime(seen), id)
	return err
}

// AddTraffic прибавляет трафик к ключу по UUID и (если seen != nil) отмечает время активности.
// Возвращает false, если ключа с таким UUID нет.
func (q *Queries) AddTraffic(ctx context.Context, uuid string, up, down int64, seen *time.Time) (bool, error) {
	var seenArg any
	if seen != nil {
		seenArg = formatTime(*seen)
	}
	res, err := q.db.ExecContext(ctx,
		`UPDATE vpn_keys SET traffic_up = traffic_up + ?, traffic_down = traffic_down + ?,
		   last_seen_at = COALESCE(?, last_seen_at), updated_at = ? WHERE uuid = ?`,
		up, down, seenArg, formatTime(Now()), uuid)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---------- аудит ----------

// Audit — новая запись аудита.
type Audit struct {
	ActorID    *int64
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

func (q *Queries) InsertAudit(ctx context.Context, a Audit) error {
	meta := "{}"
	if len(a.Metadata) > 0 {
		raw, err := json.Marshal(a.Metadata)
		if err != nil {
			return err
		}
		meta = string(raw)
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ActorID, a.Action, a.TargetType, a.TargetID, meta, formatTime(Now()))
	return err
}

// ListAudit — последние записи с email актора (created_at DESC, id DESC).
func (q *Queries) ListAudit(ctx context.Context, limit int) ([]AuditLog, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT a.id, a.actor_id, u.email, a.action, a.target_type, a.target_id, a.metadata, a.created_at
		 FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		 ORDER BY a.created_at DESC, a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []AuditLog{}
	for rows.Next() {
		var e AuditLog
		var actor sql.NullInt64
		var email, meta sql.NullString
		var created nullTime
		if err := rows.Scan(&e.ID, &actor, &email, &e.Action, &e.TargetType, &e.TargetID, &meta, &created); err != nil {
			return nil, err
		}
		if actor.Valid {
			e.ActorID = &actor.Int64
		}
		if email.Valid {
			e.ActorEmail = &email.String
		}
		e.Metadata, e.CreatedAt = meta.String, created.T
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
