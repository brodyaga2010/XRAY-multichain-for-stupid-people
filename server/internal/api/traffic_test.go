package api_test

import (
	"context"
	"testing"
	"time"
)

func TestTrafficHistory(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	anna := e.makeUser("anna@example.com")
	boris := e.makeUser("boris@example.com")
	e.login("admin@example.com")
	keyA := e.post("/admin/users/"+itoa(anna.ID)+"/keys", map[string]string{"name": "a"}).JSON()["uuid"].(string)
	keyB := e.post("/admin/users/"+itoa(boris.ID)+"/keys", map[string]string{"name": "b"}).JSON()["uuid"].(string)

	today := time.Now().In(time.Local)
	ctx := context.Background()
	add := func(uuid string, up, down int64, daysAgo int) {
		t.Helper()
		if _, err := e.st.AddTraffic(ctx, uuid, up, down, nil, today.AddDate(0, 0, -daysAgo).Format("2006-01-02")); err != nil {
			t.Fatal(err)
		}
	}
	add(keyA, 10, 100, 0)
	add(keyB, 1, 2, 0)
	add(keyA, 5, 50, 6)
	add(keyB, 9, 9, 40) // за пределами 30 дней

	sum := func(path string) (days int, up, down float64, last map[string]any) {
		t.Helper()
		resp := e.get(path)
		if resp.Status != 200 {
			t.Fatalf("%s: %d %s", path, resp.Status, resp.Raw)
		}
		list := resp.List()
		for _, d := range list {
			up += d["up"].(float64)
			down += d["down"].(float64)
		}
		return len(list), up, down, list[len(list)-1]
	}

	tests := []struct {
		name     string
		path     string
		wantDays int
		wantUp   float64
		wantDown float64
		wantLast float64 // down за сегодня
	}{
		{"все за 30 дней", "/admin/traffic", 30, 16, 152, 102},
		{"все за 7 дней", "/admin/traffic?days=7", 7, 16, 152, 102},
		{"все за сегодня", "/admin/traffic?days=1", 1, 11, 102, 102},
		{"один пользователь", "/admin/traffic?user_id=" + itoa(anna.ID), 30, 15, 150, 100},
		{"год, включая старое", "/admin/traffic?days=366&user_id=" + itoa(boris.ID), 366, 10, 11, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, up, down, last := sum(tt.path)
			if days != tt.wantDays || up != tt.wantUp || down != tt.wantDown || last["down"] != tt.wantLast ||
				last["day"] != today.Format("2006-01-02") {
				t.Errorf("дней %d, up %v, down %v, последний %v", days, up, down, last)
			}
		})
	}

	t.Run("свой трафик", func(t *testing.T) {
		e.login("anna@example.com")
		if days, up, _, _ := sum("/keys/traffic?days=7"); days != 7 || up != 15 {
			t.Errorf("дней %d, up %v", days, up)
		}
	})
}

func TestTrafficHistoryValidation(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		asUser     bool
		wantStatus int
	}{
		{"days=0", "/admin/traffic?days=0", false, 422},
		{"days=367", "/admin/traffic?days=367", false, 422},
		{"days не число", "/admin/traffic?days=x", false, 422},
		{"user_id не число", "/admin/traffic?user_id=x", false, 422},
		{"нет такого пользователя", "/admin/traffic?user_id=999", false, 404},
		{"не администратор", "/admin/traffic", true, 403},
		{"свой: неверный days", "/keys/traffic?days=-1", true, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.makeUser("admin@example.com", asAdmin)
			e.makeUser("user@example.com")
			if tt.asUser {
				e.login("user@example.com")
			} else {
				e.login("admin@example.com")
			}
			if resp := e.get(tt.path); resp.Status != tt.wantStatus {
				t.Errorf("status %d: %s", resp.Status, resp.Raw)
			}
		})
	}
}
