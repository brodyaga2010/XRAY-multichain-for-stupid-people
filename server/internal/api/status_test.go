package api_test

import (
	"strings"
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

func TestUserStatus(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	ms := 190.0
	snap := monitor.Snapshot{Enabled: true, CheckedAt: at, Checks: []monitor.CheckState{
		{ID: monitor.CheckChain, Result: monitor.Result{Status: monitor.StatusOK, Ms: &ms}},
		{ID: monitor.CheckExit, Alerted: true, Result: monitor.Result{Status: monitor.StatusWarn, Detail: "потери до 212.43.155.144"}},
		{ID: monitor.CheckSystem, Alerted: true, Result: monitor.Result{Status: monitor.StatusFail, Value: "диск 98%"}},
	}}
	tests := []struct {
		name   string
		source api.StatusSource
		login  string
		status int
		want   string // подстрока ответа
	}{
		{"аноним", fakeStatus(snap), "", 401, "Не выполнен вход"},
		{"пользователь видит итог и задержку", fakeStatus(snap), "user@example.com", 200, `"status":"warn","latency_ms":190`},
		{"мониторинг выключен", nil, "user@example.com", 200, `"status":"unknown","latency_ms":null,"checked_at":null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.makeUser("user@example.com")
			if tt.source != nil {
				e.app.SetStatusSource(tt.source)
			}
			if tt.login != "" {
				e.login(tt.login)
			}

			resp := e.get("/status")

			if resp.Status != tt.status || !strings.Contains(string(resp.Raw), tt.want) {
				t.Fatalf("%d %s", resp.Status, resp.Raw)
			}
			for _, secret := range []string{"212.43", "диск", "потери", "value"} {
				if strings.Contains(string(resp.Raw), secret) {
					t.Errorf("пользователю попали подробности %q: %s", secret, resp.Raw)
				}
			}
		})
	}
}
