package api

import (
	"net/http"

	"xraypanel/internal/monitor"
)

// StatusSource — откуда брать состояние мониторинга.
type StatusSource interface {
	Snapshot() monitor.Snapshot
}

// SetStatusSource подключает мониторинг к API; nil — мониторинг выключен.
func (a *App) SetStatusSource(s StatusSource) { a.status = s }

type statusCheckOut struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Value     string    `json:"value"`
	Detail    string    `json:"detail"`
	Since     *jsonTime `json:"since"`
	CheckedAt *jsonTime `json:"checked_at"`
}

type statusPointOut struct {
	At     jsonTime `json:"at"`
	Ms     *float64 `json:"ms"`
	Status string   `json:"status"`
}

type statusIncidentOut struct {
	Check     string    `json:"check"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	StartedAt jsonTime  `json:"started_at"`
	EndedAt   *jsonTime `json:"ended_at"`
}

type statusOut struct {
	Enabled         bool                `json:"enabled"`
	IntervalSeconds int                 `json:"interval_seconds"`
	CheckedAt       *jsonTime           `json:"checked_at"`
	Checks          []statusCheckOut    `json:"checks"`
	History         []statusPointOut    `json:"history"`
	Incidents       []statusIncidentOut `json:"incidents"`
}

func nonZero(t jsonTime) *jsonTime {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (a *App) adminStatus(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	out := statusOut{Checks: []statusCheckOut{}, History: []statusPointOut{}, Incidents: []statusIncidentOut{}}
	if a.status == nil {
		writeJSON(w, http.StatusOK, out)
		return nil
	}
	snap := a.status.Snapshot()
	out.Enabled = snap.Enabled
	out.IntervalSeconds = int(snap.Interval.Seconds())
	out.CheckedAt = nonZero(jsonTime{snap.CheckedAt})
	for _, c := range snap.Checks {
		out.Checks = append(out.Checks, statusCheckOut{
			ID: c.ID, Name: c.Name, Status: string(c.Result.Status), Value: c.Result.Value, Detail: c.Result.Detail,
			Since: nonZero(jsonTime{c.Since}), CheckedAt: nonZero(jsonTime{c.CheckedAt}),
		})
	}
	for _, p := range snap.History {
		out.History = append(out.History, statusPointOut{At: jsonTime{p.At}, Ms: p.Ms, Status: string(p.Status)})
	}
	for i := len(snap.Incidents) - 1; i >= 0; i-- { // новые первыми
		inc := snap.Incidents[i]
		out.Incidents = append(out.Incidents, statusIncidentOut{
			Check: inc.CheckID, Name: inc.Name, Status: string(inc.Status), Detail: inc.Detail,
			StartedAt: jsonTime{inc.Start}, EndedAt: toJSONTime(inc.End),
		})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

type publicStatusOut struct {
	Status    string    `json:"status"`
	LatencyMs *float64  `json:"latency_ms"`
	CheckedAt *jsonTime `json:"checked_at"`
}

// userStatus — состояние VPN для пользователей: итог и задержка, без подробностей о серверах.
func (a *App) userStatus(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.approvedUser(r); err != nil {
		return err
	}
	out := publicStatusOut{Status: string(monitor.StatusUnknown)}
	if a.status != nil {
		p := a.status.Snapshot().Public()
		out.Status, out.LatencyMs, out.CheckedAt = string(p.Status), p.Ms, nonZero(jsonTime{p.CheckedAt})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
