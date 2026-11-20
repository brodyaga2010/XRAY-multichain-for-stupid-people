package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"xraypanel/internal/auth"
	"xraypanel/internal/ratelimit"
	"xraypanel/internal/store"
)

type credentials struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
}

func (a *App) register(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	var body credentials
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	email, err := requireEmail(body.Email)
	if err != nil {
		return err
	}
	password, err := newPassword(body.Password, "password")
	if err != nil {
		return err
	}
	email = strings.ToLower(email)
	ctx := detached(r)
	if _, err := a.st.UserByEmail(ctx, email); err == nil {
		return httpErr(http.StatusConflict, "Этот email уже зарегистрирован")
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	user := &store.User{
		Email: email, PasswordHash: hash, Role: store.RoleUser, Status: store.UserPending,
		KeyLimit: a.cfg.DefaultKeyLimit,
	}
	if err := a.st.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return httpErr(http.StatusConflict, "Этот email уже зарегистрирован")
		}
		return err
	}
	a.notifyRegistered(*user, ratelimit.ClientIP(r, a.trusted))
	if err := a.sendVerification(ctx, user); err != nil {
		a.log.Warn("письмо для подтверждения почты не отправлено", "user_id", user.ID, "error", err)
	}
	writeJSON(w, http.StatusCreated, authSession{User: toUserOut(user)})
	return nil
}

func (a *App) login(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	var body credentials
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	email, err := requireEmail(body.Email)
	if err != nil {
		return err
	}
	password, err := requireString(body.Password, "password")
	if err != nil {
		return err
	}
	user, err := a.st.UserByEmail(r.Context(), strings.ToLower(email))
	switch {
	case errors.Is(err, store.ErrNotFound):
		auth.BurnPasswordCheck(password)
		return httpErr(http.StatusUnauthorized, "Неверный email или пароль")
	case err != nil:
		return err
	case !auth.VerifyPassword(password, user.PasswordHash):
		return httpErr(http.StatusUnauthorized, "Неверный email или пароль")
	case user.Status != store.UserApproved:
		return httpErr(http.StatusForbidden, "Аккаунт ещё не подтверждён администратором")
	}
	if err := a.startSession(w, user); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, authSession{User: toUserOut(user)})
	return nil
}

func (a *App) logout(w http.ResponseWriter, _ *http.Request) error {
	a.cookie.ClearCookie(w)
	writeJSON(w, http.StatusOK, message{Message: "Вы вышли"})
	return nil
}

func (a *App) me(w http.ResponseWriter, r *http.Request) error {
	user, err := a.currentUser(r)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, authSession{User: toUserOut(user)})
	return nil
}

func (a *App) changePassword(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	user, err := a.currentUser(r)
	if err != nil {
		return err
	}
	var body struct {
		Current *string `json:"current_password"`
		New     *string `json:"new_password"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	current, err := requireString(body.Current, "current_password")
	if err != nil {
		return err
	}
	next, err := newPassword(body.New, "new_password")
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(current, user.PasswordHash) {
		return httpErr(http.StatusBadRequest, "Текущий пароль указан неверно")
	}
	// Другие устройства выходят, текущая сессия выпускается заново.
	if err := a.setPassword(detached(r), user, next, "user.password_change", nil); err != nil {
		return err
	}
	if err := a.startSession(w, user); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, message{Message: "Пароль изменён, на других устройствах нужно войти заново"})
	return nil
}

// startSession выпускает токен сессии и ставит cookie.
func (a *App) startSession(w http.ResponseWriter, user *store.User) error {
	token, err := auth.NewToken(a.cfg.JWTSecret, user.ID, time.Duration(a.cfg.AccessTokenMinutes)*time.Minute, time.Now())
	if err != nil {
		return err
	}
	a.cookie.SetCookie(w, token)
	return nil
}
