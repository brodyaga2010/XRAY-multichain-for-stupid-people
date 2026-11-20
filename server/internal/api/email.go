package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"xraypanel/internal/auth"
	"xraypanel/internal/mail"
	"xraypanel/internal/store"
)

// Сроки действия ссылок из писем и лимит писем одному пользователю.
const (
	verifyLinkTTL   = 72 * time.Hour
	resetLinkTTL    = time.Hour
	changeLinkTTL   = 24 * time.Hour
	mailsPerHour    = 3
	mailSendTimeout = 30 * time.Second
)

// mailLogo — щит с галочкой, как значок сайта и аватар бота; кладётся в письмо картинкой cid:logo.
//
//go:embed assets/mail-logo.png
var mailLogo []byte

var errTooManyMails = httpErr(http.StatusTooManyRequests, "Писем уже отправлено много — попробуйте через час")

// SetMailer подключает отправку писем; nil — почта не настроена (сброс пароля недоступен).
func (a *App) SetMailer(m mail.Sender) { a.mailer = m }

// WaitMail ждёт писем, отправляемых в фоне (тесты, остановка сервиса).
func (a *App) WaitMail() { a.mailWG.Wait() }

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// issueEmailToken создаёт одноразовую ссылку и возвращает сам токен (в базе — только хеш).
// Больше mailsPerHour писем одного назначения в час одному пользователю не отправляется.
func (a *App) issueEmailToken(ctx context.Context, user *store.User, purpose string, newEmail *string, ttl time.Duration) (string, error) {
	sent, err := a.st.CountEmailTokensSince(ctx, user.ID, purpose, time.Now().Add(-time.Hour))
	if err != nil {
		return "", err
	}
	if sent >= mailsPerHour {
		return "", errTooManyMails
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	err = a.st.CreateEmailToken(ctx, &store.EmailToken{
		UserID: user.ID, Purpose: purpose, TokenHash: hashToken(raw), NewEmail: newEmail, ExpiresAt: time.Now().Add(ttl),
	})
	return raw, err
}

func (a *App) emailLink(path, token string) string {
	return strings.TrimRight(a.cfg.PublicBaseURL, "/") + path + "?token=" + token
}

// sendMail отправляет письмо в фоне: ответ пользователю не ждёт почтовый сервер.
func (a *App) sendMail(msg mail.Message) {
	m := a.mailer
	if m == nil {
		return
	}
	a.mailWG.Add(1)
	go func() {
		defer a.mailWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), mailSendTimeout)
		defer cancel()
		if err := m.Send(ctx, msg); err != nil {
			a.log.Warn("письмо не отправлено", "to", msg.To, "subject", msg.Subject, "error", err)
		}
	}()
}

// letter — письмо с одной кнопкой-ссылкой.
func (a *App) letter(to, subject, intro, button, link, outro string) mail.Message {
	site := a.cfg.SMTPFromName
	text := fmt.Sprintf("Здравствуйте!\n\n%s\n\n%s: %s\n\n%s\n\n— %s", intro, button, link, outro, site)
	body := fmt.Sprintf(`<!doctype html><html><body style="margin:0;padding:24px;background:#f6f7f6;font-family:-apple-system,Segoe UI,Roboto,Arial,sans-serif;color:#1c1f1d">
<div style="max-width:480px;margin:0 auto;background:#fff;border-radius:12px;padding:28px;border:1px solid #e3e6e4">
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 20px;border-collapse:collapse"><tr>
<td style="vertical-align:middle"><img src="cid:logo" width="40" height="40" alt="" style="display:block;border:0"></td>
<td style="vertical-align:middle;padding-left:12px;font-size:16px;font-weight:600">%s</td>
</tr></table>
<p style="margin:0 0 16px;font-size:15px;line-height:1.5">Здравствуйте!</p>
<p style="margin:0 0 24px;font-size:15px;line-height:1.5">%s</p>
<p style="margin:0 0 24px"><a href="%s" style="display:inline-block;background:#2f7d57;color:#fff;text-decoration:none;padding:11px 20px;border-radius:8px;font-size:15px;font-weight:600">%s</a></p>
<p style="margin:0 0 8px;font-size:13px;line-height:1.5;color:#6b716d">%s</p>
<p style="margin:0;font-size:12px;line-height:1.5;color:#8a908c;word-break:break-all">Если кнопка не открывается, скопируйте ссылку: %s</p>
</div>
</body></html>`, html.EscapeString(site), html.EscapeString(intro), html.EscapeString(link), html.EscapeString(button), html.EscapeString(outro),
		html.EscapeString(link))
	return mail.Message{To: to, Subject: subject, Text: text, HTML: body,
		Inline: []mail.Inline{{ContentID: "logo", ContentType: "image/png", Data: mailLogo}}}
}

// sendVerification отправляет ссылку подтверждения адреса.
func (a *App) sendVerification(ctx context.Context, user *store.User) error {
	if a.mailer == nil {
		return nil
	}
	token, err := a.issueEmailToken(ctx, user, store.TokenVerifyEmail, nil, verifyLinkTTL)
	if err != nil {
		return err
	}
	a.sendMail(a.letter(user.Email, "Подтвердите почту", "Подтвердите, что этот адрес ваш: на него придёт ссылка, если вы забудете пароль.",
		"Подтвердить почту", a.emailLink("/verify-email", token),
		"Ссылка действует 3 дня. Если вы не регистрировались, просто удалите письмо."))
	return nil
}

// emailToken находит действующую ссылку нужного назначения.
func (a *App) emailToken(ctx context.Context, raw *string, purposes ...string) (*store.EmailToken, error) {
	bad := httpErr(http.StatusBadRequest, "Ссылка устарела или уже использована — запросите новое письмо")
	if raw == nil || *raw == "" || len(*raw) > 100 {
		return nil, bad
	}
	tok, err := a.st.EmailTokenByHash(ctx, hashToken(*raw))
	if errors.Is(err, store.ErrNotFound) {
		return nil, bad
	}
	if err != nil {
		return nil, err
	}
	valid := tok.UsedAt == nil && time.Now().Before(tok.ExpiresAt)
	for _, p := range purposes {
		if valid && tok.Purpose == p {
			return tok, nil
		}
	}
	return nil, bad
}

type tokenIn struct {
	Token *string `json:"token"`
}

// verifyEmail подтверждает адрес или переход на новый (ссылка из письма; вход не нужен).
func (a *App) verifyEmail(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	var body tokenIn
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	ctx := detached(r)
	tok, err := a.emailToken(ctx, body.Token, store.TokenVerifyEmail, store.TokenChangeEmail)
	if err != nil {
		return err
	}
	user, err := a.st.UserByID(ctx, tok.UserID)
	if err != nil {
		return err
	}
	now := time.Now()
	action, meta := "auth.email_verify", map[string]any{"email": user.Email}
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.UseEmailToken(ctx, tok.ID); err != nil {
			return err
		}
		if tok.Purpose == store.TokenChangeEmail && tok.NewEmail != nil {
			action, meta = "auth.email_change", map[string]any{"from": user.Email, "to": *tok.NewEmail}
			if err := q.ChangeEmail(ctx, user.ID, *tok.NewEmail, now); err != nil {
				return err
			}
		} else if err := q.SetEmailVerified(ctx, user.ID, now); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{ActorID: &user.ID, Action: action, TargetType: "user", TargetID: itoa(user.ID), Metadata: meta})
	})
	switch {
	case errors.Is(err, store.ErrConflict):
		return httpErr(http.StatusConflict, "Этот адрес уже занят другим пользователем")
	case errors.Is(err, store.ErrNotFound):
		return httpErr(http.StatusBadRequest, "Ссылка уже использована")
	case err != nil:
		return err
	}
	updated, err := a.st.UserByID(ctx, user.ID)
	if err != nil {
		return err
	}
	text := "Почта подтверждена"
	if action == "auth.email_change" {
		text = "Адрес изменён на " + updated.Email
	}
	writeJSON(w, http.StatusOK, struct {
		Message string  `json:"message"`
		User    userOut `json:"user"`
	}{text, toUserOut(updated)})
	return nil
}

// resendVerification — ещё одно письмо для подтверждения адреса.
func (a *App) resendVerification(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	user, err := a.currentUser(r)
	if err != nil {
		return err
	}
	if user.EmailVerifiedAt != nil {
		return httpErr(http.StatusBadRequest, "Почта уже подтверждена")
	}
	if a.mailer == nil {
		return httpErr(http.StatusServiceUnavailable, "Отправка писем не настроена")
	}
	if err := a.sendVerification(detached(r), user); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, message{Message: "Письмо отправлено на " + user.Email})
	return nil
}

// forgotPassword отправляет ссылку для нового пароля. Ответ одинаковый, есть такой адрес или нет.
func (a *App) forgotPassword(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	if a.mailer == nil {
		return httpErr(http.StatusServiceUnavailable, "Восстановление по почте не настроено — обратитесь к администратору")
	}
	var body struct {
		Email *string `json:"email"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	email, err := requireEmail(body.Email)
	if err != nil {
		return err
	}
	ctx := detached(r)
	done := message{Message: "Если такой адрес зарегистрирован, на него отправлено письмо со ссылкой. Она действует 1 час."}
	user, err := a.st.UserByEmail(ctx, strings.ToLower(email))
	if errors.Is(err, store.ErrNotFound) || (err == nil && user.Status == store.UserSuspended) {
		writeJSON(w, http.StatusOK, done)
		return nil
	}
	if err != nil {
		return err
	}
	token, err := a.issueEmailToken(ctx, user, store.TokenResetPass, nil, resetLinkTTL)
	if errors.Is(err, errTooManyMails) {
		writeJSON(w, http.StatusOK, done) // не выдаём, что адрес существует
		return nil
	}
	if err != nil {
		return err
	}
	a.sendMail(a.letter(user.Email, "Новый пароль", "Кто-то (надеемся, вы) попросил сменить пароль от кабинета.",
		"Задать новый пароль", a.emailLink("/reset-password", token),
		"Ссылка действует 1 час. Если вы ничего не запрашивали, просто удалите письмо — пароль останется прежним."))
	writeJSON(w, http.StatusOK, done)
	return nil
}

// resetPassword задаёт новый пароль по ссылке из письма и завершает все прежние сессии.
func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	var body struct {
		Token    *string `json:"token"`
		Password *string `json:"new_password"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	password, err := newPassword(body.Password, "new_password")
	if err != nil {
		return err
	}
	ctx := detached(r)
	tok, err := a.emailToken(ctx, body.Token, store.TokenResetPass)
	if err != nil {
		return err
	}
	user, err := a.st.UserByID(ctx, tok.UserID)
	if err != nil {
		return err
	}
	if err := a.setPassword(ctx, user, password, "auth.password_reset", func(q *store.Queries) error {
		if err := q.UseEmailToken(ctx, tok.ID); err != nil {
			return err
		}
		if user.EmailVerifiedAt == nil { // ссылка пришла на этот адрес — значит, он рабочий
			return q.SetEmailVerified(ctx, user.ID, time.Now())
		}
		return nil
	}); errors.Is(err, store.ErrNotFound) {
		return httpErr(http.StatusBadRequest, "Ссылка уже использована")
	} else if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, message{Message: "Пароль изменён — войдите с новым паролем"})
	return nil
}

// setPassword меняет пароль и делает недействительными сессии, выпущенные до этого момента.
// extra выполняется в той же транзакции.
func (a *App) setPassword(ctx context.Context, user *store.User, password, action string, extra func(q *store.Queries) error) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return a.st.Tx(ctx, func(q *store.Queries) error {
		if extra != nil {
			if err := extra(q); err != nil {
				return err
			}
		}
		if err := q.SetPasswordHash(ctx, user.ID, hash); err != nil {
			return err
		}
		if err := q.SetSessionsValidAfter(ctx, user.ID, time.Now()); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{ActorID: &user.ID, Action: action, TargetType: "user", TargetID: itoa(user.ID)})
	})
}

// changeEmail отправляет на новый адрес ссылку; адрес сменится, когда по ней перейдут.
func (a *App) changeEmail(w http.ResponseWriter, r *http.Request) error {
	if err := a.limitAuth(r); err != nil {
		return err
	}
	user, err := a.currentUser(r)
	if err != nil {
		return err
	}
	var body struct {
		Email    *string `json:"new_email"`
		Password *string `json:"password"`
	}
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
	email = strings.ToLower(email)
	if !auth.VerifyPassword(password, user.PasswordHash) {
		return httpErr(http.StatusBadRequest, "Пароль указан неверно")
	}
	if email == user.Email {
		return httpErr(http.StatusBadRequest, "Это и так ваш адрес")
	}
	if a.mailer == nil {
		return httpErr(http.StatusServiceUnavailable, "Отправка писем не настроена")
	}
	ctx := detached(r)
	if _, err := a.st.UserByEmail(ctx, email); err == nil {
		return httpErr(http.StatusConflict, "Этот email уже зарегистрирован")
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	token, err := a.issueEmailToken(ctx, user, store.TokenChangeEmail, &email, changeLinkTTL)
	if err != nil {
		return err
	}
	a.sendMail(a.letter(email, "Подтвердите новый адрес", "Этот адрес указали как новую почту для входа в кабинет.",
		"Подтвердить адрес", a.emailLink("/verify-email", token),
		"Ссылка действует сутки. Пока вы по ней не перешли, вход остаётся по прежнему адресу. Если это были не вы, удалите письмо."))
	writeJSON(w, http.StatusOK, message{Message: "Письмо отправлено на " + email + " — перейдите по ссылке, чтобы сменить адрес"})
	return nil
}
