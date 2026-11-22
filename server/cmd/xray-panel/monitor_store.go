package main

import (
	"context"
	"time"

	"xraypanel/internal/monitor"
	"xraypanel/internal/store"
)

// monitorStore хранит историю мониторинга в базе кабинета.
type monitorStore struct{ st *store.Store }

func (m monitorStore) SavePoint(ctx context.Context, p monitor.Point) error {
	return m.st.SaveMonitorPoint(ctx, store.MonitorPoint{At: p.At, Ms: p.Ms, Status: string(p.Status)})
}

func (m monitorStore) SaveIncident(ctx context.Context, inc monitor.Incident) error {
	return m.st.SaveMonitorIncident(ctx, store.MonitorIncident{
		CheckID: inc.CheckID, Name: inc.Name, Status: string(inc.Status), Detail: inc.Detail, Start: inc.Start, End: inc.End,
	})
}

func (m monitorStore) LoadPoints(ctx context.Context, since time.Time) ([]monitor.Point, error) {
	rows, err := m.st.MonitorPointsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]monitor.Point, 0, len(rows))
	for _, r := range rows {
		out = append(out, monitor.Point{At: r.At, Ms: r.Ms, Status: monitor.Status(r.Status)})
	}
	return out, nil
}

func (m monitorStore) LoadIncidents(ctx context.Context, limit int) ([]monitor.Incident, error) {
	rows, err := m.st.LastMonitorIncidents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]monitor.Incident, 0, len(rows))
	for _, r := range rows {
		out = append(out, monitor.Incident{
			CheckID: r.CheckID, Name: r.Name, Status: monitor.Status(r.Status), Detail: r.Detail, Start: r.Start, End: r.End,
		})
	}
	return out, nil
}

func (m monitorStore) PrunePoints(ctx context.Context, before time.Time) error {
	return m.st.DeleteMonitorPointsBefore(ctx, before)
}
