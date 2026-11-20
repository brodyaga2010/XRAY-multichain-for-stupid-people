package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const userCols = "id, email, password_hash, role, status, key_limit, sub_token, plan_id, paid_until, created_at, updated_at"

type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (*User, error) {
	var u User
	var sub sql.NullString
	var plan sql.NullInt64
	var paid, created, updated nullTime
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.KeyLimit, &sub, &plan, &paid,
		&created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if sub.Valid {
		u.SubToken = &sub.String
	}
	if plan.Valid {
		u.PlanID = &plan.Int64
	}
	u.PaidUntil, u.CreatedAt, u.UpdatedAt = paid.ptr(), created.T, updated.T
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

// SetSubscription задаёт тариф и срок подписки (nil — без тарифа / бессрочно).
func (q *Queries) SetSubscription(ctx context.Context, id int64, planID *int64, paidUntil *time.Time) error {
	var paid any
	if paidUntil != nil {
		paid = formatTime(*paidUntil)
	}
	return q.updateUser(ctx, id, "plan_id = ?, paid_until = ?", planID, paid)
}

// ExpiredUsers — подтверждённые пользователи с закончившейся к now подпиской и ещё активными ключами.
func (q *Queries) ExpiredUsers(ctx context.Context, now time.Time) ([]User, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT "+userCols+` FROM users WHERE status = ?
		AND paid_until IS NOT NULL AND paid_until <= ?
		AND EXISTS (SELECT 1 FROM vpn_keys k WHERE k.owner_id = users.id AND k.status = ?) ORDER BY id`,
		UserApproved, formatTime(now), KeyActive)
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
// day — календарный день ("2006-01-02"), к которому относится трафик в истории по дням.
func (q *Queries) AddTraffic(ctx context.Context, uuid string, up, down int64, seen *time.Time, day string) (bool, error) {
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
	if n == 0 || (up == 0 && down == 0) {
		return n > 0, nil
	}
	_, err = q.db.ExecContext(ctx,
		`INSERT INTO traffic_daily (key_id, day, up, down) SELECT id, ?, ?, ? FROM vpn_keys WHERE uuid = ?
		 ON CONFLICT (key_id, day) DO UPDATE SET up = up + excluded.up, down = down + excluded.down`,
		day, up, down, uuid)
	return true, err
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

// AuditFilter — выборка из журнала. Пустые поля не фильтруют.
type AuditFilter struct {
	Limit int
	// BeforeID — курсор: записи старше записи с этим id (в порядке created_at DESC, id DESC).
	BeforeID int64
	// Actions — коды действий; элемент с «%» на конце — префикс (например, "key.%").
	Actions []string
	// ExcludeActions — коды, которые не показывать (точное совпадение).
	ExcludeActions []string
	// Query — подстрока email актора, email/имени объекта или метаданных (без учёта регистра).
	Query string
}

// ListAudit — последние записи с email актора (created_at DESC, id DESC).
func (q *Queries) ListAudit(ctx context.Context, limit int) ([]AuditLog, error) {
	return q.SearchAudit(ctx, AuditFilter{Limit: limit})
}

// SearchAudit — записи журнала по фильтру, новые первыми.
func (q *Queries) SearchAudit(ctx context.Context, f AuditFilter) ([]AuditLog, error) {
	var (
		where []string
		args  []any
	)
	if f.BeforeID > 0 {
		where = append(where, `(a.created_at, a.id) < (SELECT created_at, id FROM audit_logs WHERE id = ?)`)
		args = append(args, f.BeforeID)
	}
	if len(f.Actions) > 0 {
		var or []string
		for _, action := range f.Actions {
			if strings.HasSuffix(action, "%") {
				or = append(or, "a.action LIKE ?")
			} else {
				or = append(or, "a.action = ?")
			}
			args = append(args, action)
		}
		where = append(where, "("+strings.Join(or, " OR ")+")")
	}
	for _, action := range f.ExcludeActions {
		where = append(where, "a.action <> ?")
		args = append(args, action)
	}
	if query := strings.TrimSpace(f.Query); query != "" {
		// LIKE в SQLite не различает регистр только латиницы; кириллицу ищем как ввели.
		like := "%" + escapeLike(query) + "%"
		where = append(where, `(coalesce(u.email, '') LIKE ? ESCAPE '\'
			OR coalesce(tu.email, '') LIKE ? ESCAPE '\'
			OR coalesce(k.name, '') LIKE ? ESCAPE '\'
			OR coalesce(ko.email, '') LIKE ? ESCAPE '\'
			OR coalesce(a.metadata, '') LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like, like)
	}
	sqlText := `SELECT a.id, a.actor_id, u.email, a.action, a.target_type, a.target_id, a.metadata, a.created_at
		 FROM audit_logs a
		 LEFT JOIN users u ON u.id = a.actor_id
		 LEFT JOIN users tu ON a.target_type = 'user' AND tu.id = CAST(a.target_id AS INTEGER)
		 LEFT JOIN vpn_keys k ON a.target_type = 'vpn_key' AND k.id = CAST(a.target_id AS INTEGER)
		 LEFT JOIN users ko ON ko.id = k.owner_id`
	if len(where) > 0 {
		sqlText += " WHERE " + strings.Join(where, " AND ")
	}
	sqlText += " ORDER BY a.created_at DESC, a.id DESC LIMIT ?"
	args = append(args, f.Limit)

	rows, err := q.db.QueryContext(ctx, sqlText, args...)
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

// escapeLike экранирует спецсимволы LIKE (ESCAPE '\').
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
