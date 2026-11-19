package api_test

import (
	"testing"
	"time"

	"xraypanel/internal/api"
	"xraypanel/internal/monitor"
)

type fakeStatus monitor.Snapshot

func (f fakeStatus) Snapshot() monitor.Snapshot { return monitor.Snapshot(f) }

func TestAdminStatus(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	ended := at.Add(-time.Hour)
	ms := 412.5
	snap := monitor.Snapshot{
		Enabled: true, Interval: time.Minute, CheckedAt: at,
		Checks: []monitor.CheckState{{ID: "chain", Name: "Цепочка", CheckedAt: at, Since: at.Add(-time.Hour),
			Result: monitor.Result{Status: monitor.StatusOK, Value: "0,41 с"}}},
		History: []monitor.Point{{At: at, Ms: &ms, Status: monitor.StatusOK}},
		Incidents: []monitor.Incident{
			{CheckID: "exit", Name: "Старый", Status: monitor.StatusWarn, Start: ended.Add(-time.Minute), End: &ended},
			{CheckID: "chain", Name: "Новый", Status: monitor.StatusFail, Detail: "нет ответа", Start: at},
		},
	}
	tests := []struct {
		name   string
		source api.StatusSource
		login  string
		status int
		check  func(t *testing.T, body map[string]any)
	}{
		{"пользователю нельзя", fakeStatus(snap), "user@example.com", 403, nil},
		{"мониторинг выключен — пустые списки", nil, "admin@example.com", 200, func(t *testing.T, b map[string]any) {
			if b["enabled"] != false || len(b["checks"].([]any)) != 0 || len(b["incidents"].([]any)) != 0 || b["checked_at"] != nil {
				t.Errorf("body = %v", b)
			}
		}},
		{"снимок: сбои новыми первыми", fakeStatus(snap), "admin@example.com", 200, func(t *testing.T, b map[string]any) {
			check := b["checks"].([]any)[0].(map[string]any)
			point := b["history"].([]any)[0].(map[string]any)
			incidents := b["incidents"].([]any)
			first, second := incidents[0].(map[string]any), incidents[1].(map[string]any)
			if b["enabled"] != true || b["interval_seconds"] != float64(60) || check["status"] != "ok" ||
				check["value"] != "0,41 с" || point["ms"] != 412.5 || first["name"] != "Новый" ||
				first["ended_at"] != nil || second["ended_at"] == nil {
				t.Errorf("body = %v", b)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.makeUser("admin@example.com", asAdmin)
			e.makeUser("user@example.com")
			if tt.source != nil {
				e.app.SetStatusSource(tt.source)
			}
			e.login(tt.login)

			resp := e.get("/admin/status")

			if resp.Status != tt.status {
				t.Fatalf("%d %s", resp.Status, resp.Raw)
			}
			if tt.check != nil {
				tt.check(t, resp.JSON())
			}
		})
	}
}
