package api

import (
	"context"
	"errors"
	"slices"
	"time"

	"xraypanel/internal/store"
)

// RegistrationNotifier узнаёт о новых регистрациях (например, Telegram-бот).
type RegistrationNotifier interface {
	UserRegistered(ctx context.Context, user store.User, ip string)
}

// SetNotifier подключает уведомления о регистрациях; nil — выключить.
func (a *App) SetNotifier(n RegistrationNotifier) { a.notifier = n }

// notifyRegistered сообщает о регистрации в фоне: ответ пользователю не ждёт Telegram.
func (a *App) notifyRegistered(user store.User, ip string) {
	n := a.notifier
	if n == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n.UserRegistered(ctx, user, ip)
	}()
}

// PendingUsers — заявки, ожидающие подтверждения, старые первыми.
func (a *App) PendingUsers(ctx context.Context) ([]store.User, error) {
	users, err := a.st.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	pending := slices.DeleteFunc(users, func(u store.User) bool { return u.Status != store.UserPending })
	slices.Reverse(pending)
	return pending, nil
}

// ModerateUser подтверждает (approve=true) или отклоняет (блокирует) заявку от имени первого администратора.
// Меняется только пользователь в статусе pending; иначе возвращается текущее состояние и changed=false.
// via попадает в журнал действий (например, "telegram").
func (a *App) ModerateUser(ctx context.Context, userID int64, approve bool, via string) (*store.User, bool, error) {
	user, err := a.st.UserByID(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if user.Status != store.UserPending {
		return user, false, nil
	}
	admin, err := a.st.FirstAdmin(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, errors.New("в базе нет администратора")
	}
	if err != nil {
		return nil, false, err
	}
	extra := map[string]any{"via": via}
	if approve {
		user, err = a.activateUser(ctx, admin, user, "user.approve", extra)
	} else {
		user, err = a.suspendUser(ctx, admin, user, extra)
	}
	if err != nil {
		return nil, false, err
	}
	return user, true, nil
}
