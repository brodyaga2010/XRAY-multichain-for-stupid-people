package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Назначение ссылки из письма.
const (
	TokenVerifyEmail = "verify"
	TokenResetPass   = "reset"
	TokenChangeEmail = "change"
)

// EmailToken — одноразовая ссылка из письма. В базе только хеш токена.
type EmailToken struct {
	ID        int64
	UserID    int64
	Purpose   string
	TokenHash string
	// NewEmail — новый адрес (для смены почты).
	NewEmail  *string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// CreateEmailToken сохраняет ссылку; прежние неиспользованные ссылки того же назначения гасятся —
// работает только последнее письмо.
func (q *Queries) CreateEmailToken(ctx context.Context, t *EmailToken) error {
	now := Now()
	if _, err := q.db.ExecContext(ctx, `UPDATE email_tokens SET used_at = ? WHERE user_id = ? AND purpose = ? AND used_at IS NULL`,
		formatTime(now), t.UserID, t.Purpose); err != nil {
		return err
	}
	res, err := q.db.ExecContext(ctx, `INSERT INTO email_tokens (user_id, purpose, token_hash, new_email, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, t.UserID, t.Purpose, t.TokenHash, t.NewEmail, formatTime(t.ExpiresAt), formatTime(now))
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	t.CreatedAt = now
	return err
}

// EmailTokenByHash — ссылка по хешу; ErrNotFound — нет такой.
func (q *Queries) EmailTokenByHash(ctx context.Context, hash string) (*EmailToken, error) {
	var (
		t                         EmailToken
		newEmail                  sql.NullString
		expires, used, createdRaw nullTime
	)
	err := q.db.QueryRowContext(ctx, `SELECT id, user_id, purpose, token_hash, new_email, expires_at, used_at, created_at
		FROM email_tokens WHERE token_hash = ?`, hash).
		Scan(&t.ID, &t.UserID, &t.Purpose, &t.TokenHash, &newEmail, &expires, &used, &createdRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if newEmail.Valid {
		t.NewEmail = &newEmail.String
	}
	t.ExpiresAt, t.UsedAt, t.CreatedAt = expires.T, used.ptr(), createdRaw.T
	return &t, nil
}

// UseEmailToken помечает ссылку использованной; уже использованная — ErrNotFound (защита от двойного клика).
func (q *Queries) UseEmailToken(ctx context.Context, id int64) error {
	res, err := q.db.ExecContext(ctx, `UPDATE email_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`, formatTime(Now()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountEmailTokensSince — сколько писем такого назначения пользователю отправлено после since (защита от спама).
func (q *Queries) CountEmailTokensSince(ctx context.Context, userID int64, purpose string, since time.Time) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_tokens WHERE user_id = ? AND purpose = ? AND created_at >= ?`,
		userID, purpose, formatTime(since)).Scan(&n)
	return n, err
}

// SetEmailVerified отмечает адрес подтверждённым.
func (q *Queries) SetEmailVerified(ctx context.Context, userID int64, at time.Time) error {
	return q.updateUser(ctx, userID, "email_verified_at = ?", formatTime(at))
}

// ChangeEmail меняет адрес (сразу подтверждённый); занят — ErrConflict.
func (q *Queries) ChangeEmail(ctx context.Context, userID int64, email string, at time.Time) error {
	err := q.updateUser(ctx, userID, "email = ?, email_verified_at = ?", email, formatTime(at))
	if isUnique(err) {
		return ErrConflict
	}
	return err
}

// SetSessionsValidAfter делает недействительными сессии, выпущенные раньше at.
func (q *Queries) SetSessionsValidAfter(ctx context.Context, userID int64, at time.Time) error {
	return q.updateUser(ctx, userID, "sessions_valid_after = ?", formatTime(at))
}
