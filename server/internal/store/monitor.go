package store

import (
	"context"
	"database/sql"
	"time"
)

// MonitorPoint — точка истории задержки цепочки.
type MonitorPoint struct {
	At     time.Time
	Ms     *float64
	Status string
}

// MonitorIncident — подтверждённый сбой проверки.
type MonitorIncident struct {
	CheckID string
	Name    string
	Status  string
	Detail  string
	Start   time.Time
	End     *time.Time
}

// SaveMonitorPoint записывает точку (повтор того же момента перезаписывает её).
func (q *Queries) SaveMonitorPoint(ctx context.Context, p MonitorPoint) error {
	var ms sql.NullFloat64
	if p.Ms != nil {
		ms = sql.NullFloat64{Float64: *p.Ms, Valid: true}
	}
	_, err := q.db.ExecContext(ctx, `INSERT INTO monitor_points (at, ms, status) VALUES (?, ?, ?)
ON CONFLICT (at) DO UPDATE SET ms = excluded.ms, status = excluded.status`, formatTime(p.At), ms, p.Status)
	return err
}

// MonitorPointsSince — точки начиная с since, по времени.
func (q *Queries) MonitorPointsSince(ctx context.Context, since time.Time) ([]MonitorPoint, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT at, ms, status FROM monitor_points WHERE at >= ? ORDER BY at`, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MonitorPoint
	for rows.Next() {
		var (
			at nullTime
			ms sql.NullFloat64
			p  MonitorPoint
		)
		if err := rows.Scan(&at, &ms, &p.Status); err != nil {
			return nil, err
		}
		p.At = at.T
		if ms.Valid {
			v := ms.Float64
			p.Ms = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteMonitorPointsBefore удаляет точки старше before.
func (q *Queries) DeleteMonitorPointsBefore(ctx context.Context, before time.Time) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM monitor_points WHERE at < ?`, formatTime(before))
	return err
}

// SaveMonitorIncident добавляет сбой или обновляет существующий с тем же CheckID и Start.
// Хранятся последние 100 сбоев.
func (q *Queries) SaveMonitorIncident(ctx context.Context, inc MonitorIncident) error {
	var end any
	if inc.End != nil {
		end = formatTime(*inc.End)
	}
	res, err := q.db.ExecContext(ctx, `UPDATE monitor_incidents SET status = ?, detail = ?, ended_at = ?
WHERE check_id = ? AND started_at = ?`, inc.Status, inc.Detail, end, inc.CheckID, formatTime(inc.Start))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	if _, err := q.db.ExecContext(ctx, `INSERT INTO monitor_incidents (check_id, name, status, detail, started_at, ended_at)
VALUES (?, ?, ?, ?, ?, ?)`, inc.CheckID, inc.Name, inc.Status, inc.Detail, formatTime(inc.Start), end); err != nil {
		return err
	}
	_, err = q.db.ExecContext(ctx, `DELETE FROM monitor_incidents WHERE id NOT IN
(SELECT id FROM monitor_incidents ORDER BY started_at DESC, id DESC LIMIT 100)`)
	return err
}

// LastMonitorIncidents — последние limit сбоев, старые первыми.
func (q *Queries) LastMonitorIncidents(ctx context.Context, limit int) ([]MonitorIncident, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT check_id, name, status, detail, started_at, ended_at FROM
(SELECT * FROM monitor_incidents ORDER BY started_at DESC, id DESC LIMIT ?) ORDER BY started_at, id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MonitorIncident
	for rows.Next() {
		var (
			inc        MonitorIncident
			start, end nullTime
		)
		if err := rows.Scan(&inc.CheckID, &inc.Name, &inc.Status, &inc.Detail, &start, &end); err != nil {
			return nil, err
		}
		inc.Start, inc.End = start.T, end.ptr()
		out = append(out, inc)
	}
	return out, rows.Err()
}
