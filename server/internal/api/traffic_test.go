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
	newKey := func(userID int64, name string) (uuid, id string) {
		t.Helper()
		body := e.post("/admin/users/"+itoa(userID)+"/keys", map[string]string{"name": name}).JSON()
		return body["uuid"].(string), itoa(int64(body["id"].(float64)))
	}
	keyA, keyAID := newKey(anna.ID, "a")
	keyA2, keyA2ID := newKey(anna.ID, "a2")
	keyB, keyBID := newKey(boris.ID, "b")

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
	add(keyA2, 100, 1000, 1)

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
		{"все за 30 дней", "/admin/traffic", 30, 116, 1152, 102},
		{"все за 7 дней", "/admin/traffic?days=7", 7, 116, 1152, 102},
		{"все за сегодня", "/admin/traffic?days=1", 1, 11, 102, 102},
		{"один пользователь", "/admin/traffic?user_id=" + itoa(anna.ID), 30, 115, 1150, 100},
		{"год, включая старое", "/admin/traffic?days=366&user_id=" + itoa(boris.ID), 366, 10, 11, 2},
		{"один ключ", "/admin/traffic?key_id=" + keyAID, 30, 15, 150, 100},
		{"второй ключ того же пользователя", "/admin/traffic?days=7&key_id=" + keyA2ID, 7, 100, 1000, 0},
		{"ключ вместе с его владельцем", "/admin/traffic?user_id=" + itoa(anna.ID) + "&key_id=" + keyA2ID, 30, 100, 1000, 0},
		{"чужой ключ за год", "/admin/traffic?days=366&key_id=" + keyBID, 366, 10, 11, 2},
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

	t.Run("ключ не того пользователя", func(t *testing.T) {
		path := "/admin/traffic?user_id=" + itoa(anna.ID) + "&key_id=" + keyBID
		if resp := e.get(path); resp.Status != 404 {
			t.Errorf("status %d: %s", resp.Status, resp.Raw)
		}
	})

	e.login("anna@example.com")
	own := []struct {
		name   string
		path   string
		wantUp float64
	}{
		{"все свои ключи", "/keys/traffic?days=7", 115},
		{"один свой ключ", "/keys/traffic?days=7&key_id=" + keyA2ID, 100},
	}
	for _, tt := range own {
		t.Run("свой: "+tt.name, func(t *testing.T) {
			if days, up, _, _ := sum(tt.path); days != 7 || up != tt.wantUp {
				t.Errorf("дней %d, up %v", days, up)
			}
		})
	}
	t.Run("свой: чужой ключ не виден", func(t *testing.T) {
		if resp := e.get("/keys/traffic?key_id=" + keyBID); resp.Status != 404 {
			t.Errorf("status %d: %s", resp.Status, resp.Raw)
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
		{"key_id не число", "/admin/traffic?key_id=x", false, 422},
		{"key_id ноль", "/admin/traffic?key_id=0", false, 422},
		{"нет такого ключа", "/admin/traffic?key_id=999", false, 404},
		{"свой: нет такого ключа", "/keys/traffic?key_id=999", true, 404},
		{"свой: key_id не число", "/keys/traffic?key_id=x", true, 422},
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
