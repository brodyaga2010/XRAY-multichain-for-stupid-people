package api_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"xraypanel/internal/store"
)

type notifierFunc func(ctx context.Context, user store.User, ip string)

func (f notifierFunc) UserRegistered(ctx context.Context, user store.User, ip string) {
	f(ctx, user, ip)
}

func TestRegisterNotifiesAdmin(t *testing.T) {
	e := newEnv(t)
	type event struct {
		email, ip string
	}
	got := make(chan event, 1)
	e.app.SetNotifier(notifierFunc(func(_ context.Context, u store.User, ip string) { got <- event{u.Email, ip} }))

	resp := e.post("/auth/register", map[string]string{"email": "New@Example.com", "password": "password123"})

	if resp.Status != 201 {
		t.Fatalf("status %d: %s", resp.Status, resp.Raw)
	}
	select {
	case ev := <-got:
		if ev.email != "new@example.com" || ev.ip == "" {
			t.Errorf("событие = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("уведомление о регистрации не пришло")
	}
}

func TestRegisterConflictDoesNotNotify(t *testing.T) {
	e := newEnv(t)
	e.makeUser("dup@example.com")
	called := make(chan struct{}, 1)
	e.app.SetNotifier(notifierFunc(func(context.Context, store.User, string) { called <- struct{}{} }))

	resp := e.post("/auth/register", map[string]string{"email": "dup@example.com", "password": "password123"})

	if resp.Status != 409 {
		t.Fatalf("status %d", resp.Status)
	}
	select {
	case <-called:
		t.Error("при конфликте уведомления быть не должно")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestModerateUser(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		approve     bool
		wantStatus  string
		wantChanged bool
		wantAudit   string
	}{
		{"подтверждение заявки", store.UserPending, true, store.UserApproved, true, "user.approve"},
		{"отклонение заявки", store.UserPending, false, store.UserSuspended, true, "user.suspend"},
		{"уже подтверждён", store.UserApproved, false, store.UserApproved, false, ""},
		{"уже заблокирован", store.UserSuspended, true, store.UserSuspended, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.makeUser("admin@example.com", asAdmin)
			user := e.makeUser("user@example.com", withStatus(tt.status))

			got, changed, err := e.app.ModerateUser(context.Background(), user.ID, tt.approve, "telegram")

			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.wantStatus || changed != tt.wantChanged {
				t.Errorf("status=%s changed=%v, want %s %v", got.Status, changed, tt.wantStatus, tt.wantChanged)
			}
			entries, err := e.st.ListAudit(context.Background(), 10)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantAudit == "" {
				if len(entries) != 0 {
					t.Errorf("журнал не должен меняться: %v", entries)
				}
				return
			}
			if len(entries) != 1 || entries[0].Action != tt.wantAudit || !strings.Contains(entries[0].Metadata, `"via":"telegram"`) ||
				entries[0].ActorEmail == nil || *entries[0].ActorEmail != "admin@example.com" {
				t.Errorf("журнал = %+v", entries)
			}
		})
	}
}

func TestModerateUserErrors(t *testing.T) {
	t.Run("нет пользователя", func(t *testing.T) {
		e := newEnv(t)
		e.makeUser("admin@example.com", asAdmin)
		if _, _, err := e.app.ModerateUser(context.Background(), 999, true, "telegram"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("нет администратора", func(t *testing.T) {
		e := newEnv(t)
		user := e.makeUser("user@example.com", withStatus(store.UserPending))
		if _, _, err := e.app.ModerateUser(context.Background(), user.ID, true, "telegram"); err == nil {
			t.Error("ожидалась ошибка")
		}
	})
}

func TestPendingUsersOldestFirst(t *testing.T) {
	e := newEnv(t)
	first := e.makeUser("first@example.com", withStatus(store.UserPending))
	e.makeUser("approved@example.com")
	second := e.makeUser("second@example.com", withStatus(store.UserPending))

	users, err := e.app.PendingUsers(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].ID != first.ID || users[1].ID != second.ID {
		t.Errorf("users = %+v", users)
	}
}
