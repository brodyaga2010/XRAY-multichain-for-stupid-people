package store

import (
	"context"
	"database/sql"
	"errors"
)

const planCols = "p.id, p.name, p.key_limit, p.duration_days, p.is_default, p.created_at"

func scanPlan(row scanner, extra ...any) (*Plan, error) {
	var p Plan
	var created nullTime
	if err := row.Scan(append([]any{&p.ID, &p.Name, &p.KeyLimit, &p.DurationDays, &p.IsDefault, &created}, extra...)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.CreatedAt = created.T
	return &p, nil
}

// ListPlans — тарифы по имени, с числом пользователей на каждом.
func (q *Queries) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := q.db.QueryContext(ctx, "SELECT "+planCols+`, (SELECT COUNT(*) FROM users u WHERE u.plan_id = p.id)
		FROM plans p ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := []Plan{}
	for rows.Next() {
		var users int
		p, err := scanPlan(rows, &users)
		if err != nil {
			return nil, err
		}
		p.Users = users
		plans = append(plans, *p)
	}
	return plans, rows.Err()
}

func (q *Queries) PlanByID(ctx context.Context, id int64) (*Plan, error) {
	return scanPlan(q.db.QueryRowContext(ctx, "SELECT "+planCols+" FROM plans p WHERE p.id = ?", id))
}

// DefaultPlan — тариф для новых пользователей; ErrNotFound — не выбран.
func (q *Queries) DefaultPlan(ctx context.Context) (*Plan, error) {
	return scanPlan(q.db.QueryRowContext(ctx, "SELECT "+planCols+" FROM plans p WHERE p.is_default = 1 LIMIT 1"))
}

// SavePlan создаёт тариф (ID == 0) или обновляет существующий. Тариф по умолчанию может быть только один.
func (q *Queries) SavePlan(ctx context.Context, p *Plan) error {
	now := formatTime(Now())
	if p.IsDefault {
		if _, err := q.db.ExecContext(ctx, "UPDATE plans SET is_default = 0, updated_at = ? WHERE is_default = 1 AND id <> ?", now, p.ID); err != nil {
			return err
		}
	}
	if p.ID == 0 {
		res, err := q.db.ExecContext(ctx, `INSERT INTO plans (name, key_limit, duration_days, is_default, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, p.Name, p.KeyLimit, p.DurationDays, p.IsDefault, now, now)
		if isUnique(err) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		p.ID, err = res.LastInsertId()
		return err
	}
	res, err := q.db.ExecContext(ctx, `UPDATE plans SET name = ?, key_limit = ?, duration_days = ?, is_default = ?, updated_at = ?
		WHERE id = ?`, p.Name, p.KeyLimit, p.DurationDays, p.IsDefault, now, p.ID)
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlan удаляет тариф без пользователей; есть пользователи — ErrConflict.
func (q *Queries) DeletePlan(ctx context.Context, id int64) error {
	var users int
	if err := q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE plan_id = ?", id).Scan(&users); err != nil {
		return err
	}
	if users > 0 {
		return ErrConflict
	}
	res, err := q.db.ExecContext(ctx, "DELETE FROM plans WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
