package api_test

import (
	"net/url"
	"strings"
	"testing"
)

func TestAdminAuditFilters(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")
	anna := e.makeUser("anna@example.com", withStatus("pending"))
	boris := e.makeUser("boris@example.com")
	e.post("/admin/users/"+itoa(anna.ID)+"/approve", nil)                                // user.approve → anna
	e.post("/admin/users/"+itoa(boris.ID)+"/keys", map[string]string{"name": "Ноутбук"}) // admin.key.create → ключ бориса
	e.post("/admin/users/"+itoa(boris.ID)+"/suspend", nil)                               // user.suspend → boris
	e.post("/auth/password", map[string]string{"current_password": "password123", "new_password": "password456"})

	actions := func(path string) string {
		t.Helper()
		resp := e.get(path)
		if resp.Status != 200 {
			t.Fatalf("%s: %d %s", path, resp.Status, resp.Raw)
		}
		var out []string
		for _, entry := range resp.List() {
			out = append(out, entry["action"].(string))
		}
		return strings.Join(out, ",")
	}

	tests := []struct {
		name  string
		query url.Values
		want  string
	}{
		{"всё, новые первыми", nil, "user.password_change,user.suspend,admin.key.create,user.approve"},
		{"пользователи", url.Values{"category": {"users"}}, "user.suspend,user.approve"},
		{"ключи", url.Values{"category": {"keys"}}, "admin.key.create"},
		{"аккаунт", url.Values{"category": {"account"}}, "user.password_change"},
		{"поиск по email объекта", url.Values{"q": {"anna@"}}, "user.approve"},
		{"поиск по имени ключа", url.Values{"q": {"Ноутбук"}}, "admin.key.create"},
		{"поиск по владельцу ключа и пользователю", url.Values{"q": {"boris"}}, "user.suspend,admin.key.create"},
		{"поиск по актору находит всё", url.Values{"q": {"ADMIN@example"}}, "user.password_change,user.suspend,admin.key.create,user.approve"},
		{"спецсимволы LIKE не работают как шаблон", url.Values{"q": {"%"}}, ""},
		{"категория и поиск вместе", url.Values{"category": {"users"}, "q": {"boris"}}, "user.suspend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actions("/admin/audit?" + tt.query.Encode()); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("постраничная загрузка", func(t *testing.T) {
		first := e.get("/admin/audit?limit=2").List()
		if len(first) != 2 {
			t.Fatalf("первая страница: %d", len(first))
		}
		cursor := itoa(int64(first[1]["id"].(float64)))
		if got := actions("/admin/audit?limit=2&before=" + cursor); got != "admin.key.create,user.approve" {
			t.Errorf("вторая страница: %q", got)
		}
		if got := actions("/admin/audit?category=users&before=" + cursor); got != "user.approve" {
			t.Errorf("курсор вместе с категорией: %q", got)
		}
	})
}

func TestAdminAuditValidation(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")
	tests := []struct {
		name  string
		query string
	}{
		{"неизвестная категория", "category=all"},
		{"курсор не число", "before=abc"},
		{"курсор ноль", "before=0"},
		{"длинный поиск", "q=" + strings.Repeat("я", 101)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if resp := e.get("/admin/audit?" + tt.query); resp.Status != 422 {
				t.Errorf("status %d: %s", resp.Status, resp.Raw)
			}
		})
	}
}
