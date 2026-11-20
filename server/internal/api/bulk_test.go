package api_test

import (
	"strings"
	"testing"

	"xraypanel/internal/store"
)

func TestAdminBulkUsers(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		statuses    []string // статусы трёх пользователей
		includeSelf bool
		wantUpdated int
		wantSkipped int
		wantStatus  []string
		wantAudit   int
	}{
		{"подтвердить ждущих", "approve", []string{store.UserPending, store.UserPending, store.UserApproved}, false, 2, 1,
			[]string{store.UserApproved, store.UserApproved, store.UserApproved}, 2},
		{"заблокировать всех, кроме себя и уже заблокированных", "suspend",
			[]string{store.UserApproved, store.UserPending, store.UserSuspended}, true, 2, 2,
			[]string{store.UserSuspended, store.UserSuspended, store.UserSuspended}, 2},
		{"восстановить заблокированных", "restore", []string{store.UserSuspended, store.UserApproved, store.UserSuspended}, false, 2, 1,
			[]string{store.UserApproved, store.UserApproved, store.UserApproved}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			admin := e.makeUser("admin@example.com", asAdmin)
			e.login("admin@example.com")
			var ids []int64
			for i, s := range tt.statuses {
				ids = append(ids, e.makeUser(string(rune('a'+i))+"@example.com", withStatus(s)).ID)
			}
			if tt.includeSelf {
				ids = append(ids, admin.ID)
			}
			before := len(e.auditActions())

			resp := e.post("/admin/users/bulk", map[string]any{"action": tt.action, "user_ids": append(ids, ids[0])})

			if resp.Status != 200 {
				t.Fatalf("status %d: %s", resp.Status, resp.Raw)
			}
			body := resp.JSON()
			if got := len(body["updated"].([]any)); got != tt.wantUpdated || int(body["skipped"].(float64)) != tt.wantSkipped {
				t.Errorf("ответ = %s", resp.Raw)
			}
			for i, id := range ids[:len(tt.statuses)] {
				u, _ := e.st.UserByID(t.Context(), id)
				if u.Status != tt.wantStatus[i] {
					t.Errorf("пользователь %d: %s, want %s", i, u.Status, tt.wantStatus[i])
				}
			}
			if self, _ := e.st.UserByID(t.Context(), admin.ID); self.Status != store.UserApproved {
				t.Error("администратор заблокировал сам себя")
			}
			if got := len(e.auditActions()) - before; got != tt.wantAudit {
				t.Errorf("записей в журнале %d, want %d", got, tt.wantAudit)
			}
		})
	}
}

func TestAdminBulkUsersRestoresKeys(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")
	user := e.makeUser("u@example.com")
	key := e.post("/admin/users/"+itoa(user.ID)+"/keys", map[string]string{"name": "телефон"}).JSON()
	uuid := key["uuid"].(string)

	e.post("/admin/users/bulk", map[string]any{"action": "suspend", "user_ids": []int64{user.ID}})
	if e.inXray(uuid) {
		t.Fatal("после блокировки ключ остался в Xray")
	}
	e.post("/admin/users/bulk", map[string]any{"action": "restore", "user_ids": []int64{user.ID}})
	if !e.inXray(uuid) {
		t.Error("после восстановления ключ не вернулся в Xray")
	}
}

func TestAdminBulkUsersValidation(t *testing.T) {
	many := make([]int64, 501)
	for i := range many {
		many[i] = int64(i + 1)
	}
	tests := []struct {
		name       string
		body       map[string]any
		asUser     bool
		wantStatus int
		wantDetail string
	}{
		{"неизвестное действие", map[string]any{"action": "delete", "user_ids": []int64{1}}, false, 422, "approve"},
		{"пустой список", map[string]any{"action": "approve", "user_ids": []int64{}}, false, 422, "от 1 до 500"},
		{"слишком много", map[string]any{"action": "approve", "user_ids": many}, false, 422, "от 1 до 500"},
		{"не администратор", map[string]any{"action": "approve", "user_ids": []int64{1}}, true, 403, ""},
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
			resp := e.post("/admin/users/bulk", tt.body)
			if resp.Status != tt.wantStatus || !strings.Contains(resp.Detail(), tt.wantDetail) {
				t.Errorf("status %d: %s", resp.Status, resp.Raw)
			}
		})
	}
}
