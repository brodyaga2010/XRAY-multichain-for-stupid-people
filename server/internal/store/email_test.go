package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEmailTokens(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)
	u := &User{Email: "a@example.com", PasswordHash: "h", Role: RoleUser, Status: UserApproved, KeyLimit: 3}
	other := &User{Email: "b@example.com", PasswordHash: "h", Role: RoleUser, Status: UserApproved, KeyLimit: 3}
	_ = st.CreateUser(ctx, u)
	_ = st.CreateUser(ctx, other)
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	first := &EmailToken{UserID: u.ID, Purpose: TokenResetPass, TokenHash: "h1", ExpiresAt: expires}
	second := &EmailToken{UserID: u.ID, Purpose: TokenResetPass, TokenHash: "h2", ExpiresAt: expires}
	verify := &EmailToken{UserID: u.ID, Purpose: TokenVerifyEmail, TokenHash: "h3", ExpiresAt: expires}
	newEmail := "new@example.com"
	change := &EmailToken{UserID: u.ID, Purpose: TokenChangeEmail, TokenHash: "h4", NewEmail: &newEmail, ExpiresAt: expires}
	for _, tok := range []*EmailToken{first, second, verify, change} {
		if err := st.CreateEmailToken(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		hash     string
		wantErr  error
		wantUsed bool
	}{
		{"новое письмо гасит прежнее того же назначения", "h1", nil, true},
		{"последнее письмо действует", "h2", nil, false},
		{"другое назначение не затронуто", "h3", nil, false},
		{"нет такой ссылки", "nope", ErrNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.EmailTokenByHash(ctx, tt.hash)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v", err)
			}
			if err == nil && (got.UsedAt != nil) != tt.wantUsed {
				t.Errorf("used_at = %v", got.UsedAt)
			}
		})
	}

	got, _ := st.EmailTokenByHash(ctx, "h4")
	if got.NewEmail == nil || *got.NewEmail != newEmail || !got.ExpiresAt.Equal(expires) || got.Purpose != TokenChangeEmail {
		t.Errorf("ссылка смены почты: %+v", got)
	}
	if err := st.UseEmailToken(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.UseEmailToken(ctx, second.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("повторное использование: %v", err)
	}
	if n, _ := st.CountEmailTokensSince(ctx, u.ID, TokenResetPass, time.Now().Add(-time.Minute)); n != 2 {
		t.Errorf("писем о сбросе: %d", n)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := st.ChangeEmail(ctx, u.ID, "b@example.com", now); !errors.Is(err, ErrConflict) {
		t.Errorf("занятый адрес: %v", err)
	}
	if err := st.ChangeEmail(ctx, u.ID, newEmail, now); err != nil {
		t.Fatal(err)
	}
	_ = st.SetSessionsValidAfter(ctx, u.ID, now)
	user, _ := st.UserByID(ctx, u.ID)
	if user.Email != newEmail || user.EmailVerifiedAt == nil || !user.SessionsValidAfter.Equal(now) {
		t.Errorf("пользователь: %+v", user)
	}
}
