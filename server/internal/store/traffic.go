package store

import (
	"context"
	"strings"
)

// TrafficDay — трафик за календарный день.
type TrafficDay struct {
	Day  string // "2006-01-02"
	Up   int64
	Down int64
}

// TrafficFilter — выборка истории трафика: дни From..To включительно; OwnerID > 0 — только ключи пользователя.
type TrafficFilter struct {
	From, To string
	OwnerID  int64
}

// TrafficByDay — суммарный трафик по дням (только дни, где он был), старые первыми.
func (q *Queries) TrafficByDay(ctx context.Context, f TrafficFilter) ([]TrafficDay, error) {
	where := []string{"t.day >= ?", "t.day <= ?"}
	args := []any{f.From, f.To}
	join := ""
	if f.OwnerID > 0 {
		join = " JOIN vpn_keys k ON k.id = t.key_id"
		where = append(where, "k.owner_id = ?")
		args = append(args, f.OwnerID)
	}
	rows, err := q.db.QueryContext(ctx, `SELECT t.day, SUM(t.up), SUM(t.down) FROM traffic_daily t`+join+
		` WHERE `+strings.Join(where, " AND ")+` GROUP BY t.day ORDER BY t.day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrafficDay{}
	for rows.Next() {
		var d TrafficDay
		if err := rows.Scan(&d.Day, &d.Up, &d.Down); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
