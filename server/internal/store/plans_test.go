package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPlansCRUD(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)

	basic := &Plan{Name: "Базовый", KeyLimit: 2, DurationDays: 30, IsDefault: true}
	family := &Plan{Name: "Семья", KeyLimit: 5, DurationDays: 30}
	for _, p := range []*Plan{basic, family} {
		if err := st.SavePlan(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SavePlan(ctx, &Plan{Name: "Семья", KeyLimit: 1}); !errors.Is(err, ErrConflict) {
		t.Errorf("повтор имени: %v", err)
	}

	// новый тариф по умолчанию снимает флаг со старого
	family.IsDefault = true
	if err := st.SavePlan(ctx, family); err != nil {
		t.Fatal(err)
	}
	def, err := st.DefaultPlan(ctx)
	if err != nil || def.ID != family.ID {
		t.Fatalf("по умолчанию: %+v %v", def, err)
	}

	user := &User{Email: "a@example.com", PasswordHash: "h", Role: RoleUser, Status: UserApproved, KeyLimit: 3}
	_ = st.CreateUser(ctx, user)
	until := time.Date(2026, 12, 31, 18, 59, 59, 0, time.UTC)
	if err := st.SetSubscription(ctx, user.ID, &basic.ID, &until); err != nil {
		t.Fatal(err)
	}
	got, _ := st.UserByID(ctx, user.ID)
	if got.PlanID == nil || *got.PlanID != basic.ID || got.PaidUntil == nil || !got.PaidUntil.Equal(until) {
		t.Errorf("подписка: %+v", got)
	}

	plans, _ := st.ListPlans(ctx)
	if len(plans) != 2 || plans[0].Name != "Базовый" || plans[0].Users != 1 || plans[0].IsDefault || !plans[1].IsDefault {
		t.Errorf("тарифы: %+v", plans)
	}

	tests := []struct {
		name string
		id   int64
		want error
	}{
		{"на тарифе есть пользователи", basic.ID, ErrConflict},
		{"нет такого", 999, ErrNotFound},
		{"пустой тариф", family.ID, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := st.DeletePlan(ctx, tt.id); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if _, err := st.DefaultPlan(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("после удаления тарифа по умолчанию: %v", err)
	}
	if err := st.SetSubscription(ctx, user.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.UserByID(ctx, user.ID); got.PlanID != nil || got.PaidUntil != nil {
		t.Errorf("сброс подписки: %+v", got)
	}
}

func TestExpiredUsers(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Hour)

	cases := []struct {
		email     string
		status    string
		paidUntil *time.Time
		keyStatus string
		want      bool
	}{
		{"expired@example.com", UserApproved, &past, KeyActive, true},
		{"exact@example.com", UserApproved, &now, KeyActive, true},
		{"paid@example.com", UserApproved, &future, KeyActive, false},
		{"forever@example.com", UserApproved, nil, KeyActive, false},
		{"already@example.com", UserApproved, &past, KeyExpired, false},
		{"blocked@example.com", UserSuspended, &past, KeySuspended, false},
	}
	for i, c := range cases {
		u := &User{Email: c.email, PasswordHash: "h", Role: RoleUser, Status: c.status, KeyLimit: 3}
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := st.SetSubscription(ctx, u.ID, nil, c.paidUntil); err != nil {
			t.Fatal(err)
		}
		uuid := string(rune('a' + i))
		key := &VpnKey{OwnerID: u.ID, UUID: uuid, Name: "k", Type: "multi", SNI: "s", Status: c.keyStatus,
			PublicKey: "p", ShortID: "s", ConnectionURI: "vless://" + uuid}
		if err := st.InsertKey(ctx, key); err != nil {
			t.Fatal(err)
		}
	}

	got, err := st.ExpiredUsers(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	var emails []string
	for _, u := range got {
		emails = append(emails, u.Email)
	}
	if len(emails) != 2 || emails[0] != "expired@example.com" || emails[1] != "exact@example.com" {
		t.Errorf("истёкшие: %v", emails)
	}
}

func TestSubscriptionActive(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Second), now.Add(-time.Second)
	tests := []struct {
		name string
		paid *time.Time
		want bool
	}{
		{"бессрочно", nil, true},
		{"ещё действует", &later, true},
		{"ровно сейчас — закончилась", &now, false},
		{"закончилась", &earlier, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&User{PaidUntil: tt.paid}).SubscriptionActive(now); got != tt.want {
				t.Errorf("got %v", got)
			}
		})
	}
}
