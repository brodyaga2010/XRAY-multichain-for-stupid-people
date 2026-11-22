package api

import (
	"testing"
	"time"

	"xraypanel/internal/store"
)

func TestExtendPaidUntil(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.Local)
	future := time.Date(2026, 12, 5, 23, 59, 59, 0, time.Local)
	past := time.Date(2026, 10, 1, 23, 59, 59, 0, time.Local)
	tests := []struct {
		name    string
		current *time.Time
		months  int
		want    time.Time
	}{
		{"без срока — от сегодня", nil, 1, time.Date(2026, 12, 20, 23, 59, 59, 0, time.Local)},
		{"срок прошёл — от сегодня", &past, 3, time.Date(2027, 2, 20, 23, 59, 59, 0, time.Local)},
		{"срок идёт — от его конца", &future, 1, time.Date(2027, 1, 5, 23, 59, 59, 0, time.Local)},
		{"на год", &future, 12, time.Date(2027, 12, 5, 23, 59, 59, 0, time.Local)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extendPaidUntil(tt.current, now, tt.months); !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanPaidUntil(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.Local)
	if got := planPaidUntil(&store.Plan{DurationDays: 0}, now); got != nil {
		t.Errorf("бессрочный тариф: %v", got)
	}
	want := time.Date(2026, 12, 20, 23, 59, 59, 0, time.Local)
	if got := planPaidUntil(&store.Plan{DurationDays: 30}, now); got == nil || !got.Equal(want) {
		t.Errorf("30 дней: %v", got)
	}
}

func TestKeyTransition(t *testing.T) {
	tests := []struct {
		user     string
		active   bool
		from, to string
	}{
		{store.UserApproved, true, store.KeyExpired, store.KeyActive},
		{store.UserApproved, false, store.KeyActive, store.KeyExpired},
		{store.UserSuspended, true, store.KeyExpired, store.KeySuspended},
		{store.UserSuspended, false, store.KeySuspended, store.KeyExpired},
		{store.UserPending, true, "", ""},
	}
	for _, tt := range tests {
		if from, to := keyTransition(tt.user, tt.active); from != tt.from || to != tt.to {
			t.Errorf("%s/%v: %s → %s", tt.user, tt.active, from, to)
		}
	}
}
