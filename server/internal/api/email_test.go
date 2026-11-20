package api_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"xraypanel/internal/mail"
	"xraypanel/internal/store"
)

// mailbox — отправитель писем в память.
type mailbox struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (m *mailbox) Send(_ context.Context, msg mail.Message) error {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
	return nil
}

func (m *mailbox) all() []mail.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.Message(nil), m.msgs...)
}

var tokenRe = regexp.MustCompile(`(/[a-z-]+)\?token=([A-Za-z0-9_-]+)`)

// lastLink — путь и токен из последнего письма на адрес to.
func lastLink(t *testing.T, e *env, box *mailbox, to string) (path, token string) {
	t.Helper()
	e.app.WaitMail()
	msgs := box.all()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].To == to {
			m := tokenRe.FindStringSubmatch(msgs[i].Text)
			if m == nil {
				t.Fatalf("в письме нет ссылки: %q", msgs[i].Text)
			}
			return m[1], m[2]
		}
	}
	t.Fatalf("писем на %s нет", to)
	return "", ""
}

func withMail(t *testing.T) (*env, *mailbox) {
	e := newEnv(t, map[string]string{"PUBLIC_BASE_URL": "https://panel.example.com/"})
	box := &mailbox{}
	e.app.SetMailer(box)
	return e, box
}

// sessionCookie — текущая cookie сессии (как будто это другое устройство). Сессия, выпущенная за 1 мс до
// отзыва, ещё считается действующей (auth.Session.ValidAfter), а тест делает вход и отзыв быстрее — поэтому
// ждём, чтобы сессия гарантированно оказалась «старой».
func sessionCookie(e *env) *http.Cookie {
	u, _ := url.Parse(e.server.URL)
	for _, c := range e.client.Jar.Cookies(u) {
		if c.Name == e.cfg.CookieName {
			time.Sleep(5 * time.Millisecond)
			return c
		}
	}
	e.t.Fatal("нет cookie сессии")
	return nil
}

func meWithCookie(e *env, c *http.Cookie) int {
	req, _ := http.NewRequest("GET", e.server.URL+"/api/auth/me", nil)
	req.AddCookie(c)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestRegisterSendsVerification(t *testing.T) {
	e, box := withMail(t)
	e.post("/auth/register", map[string]string{"email": "New@Example.com", "password": "password123"})

	path, token := lastLink(t, e, box, "new@example.com")
	msg := box.all()[0]
	if path != "/verify-email" || !strings.Contains(msg.Text, "https://panel.example.com/verify-email?token=") ||
		msg.Subject != "Подтвердите почту" || !strings.Contains(msg.HTML, "Подтвердить почту") {
		t.Fatalf("письмо: %+v", msg)
	}

	steps := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"подтверждение", token, 200},
		{"повторный переход", token, 400},
		{"чужой токен", "x" + token[1:], 400},
		{"пустой токен", "", 400},
	}
	for _, s := range steps {
		resp := e.post("/auth/verify-email", map[string]string{"token": s.token})
		if resp.Status != s.wantStatus {
			t.Errorf("%s: %d %s", s.name, resp.Status, resp.Raw)
		}
	}
	user, _ := e.st.UserByEmail(context.Background(), "new@example.com")
	if user.EmailVerifiedAt == nil {
		t.Error("почта не отмечена подтверждённой")
	}
}

func TestPasswordReset(t *testing.T) {
	e, box := withMail(t)
	e.makeUser("user@example.com")
	e.makeUser("blocked@example.com", withStatus(store.UserSuspended))
	e.login("user@example.com")
	oldSession := sessionCookie(e)

	for _, email := range []string{"nobody@example.com", "blocked@example.com"} {
		resp := e.post("/auth/forgot-password", map[string]string{"email": email})
		if resp.Status != 200 || !strings.Contains(resp.JSON()["message"].(string), "Если такой адрес") {
			t.Errorf("%s: %d %s", email, resp.Status, resp.Raw)
		}
	}
	if resp := e.post("/auth/forgot-password", map[string]string{"email": "USER@example.com"}); resp.Status != 200 {
		t.Fatalf("сброс: %d %s", resp.Status, resp.Raw)
	}
	e.app.WaitMail()
	if n := len(box.all()); n != 1 {
		t.Fatalf("писем %d, want 1 (только существующему)", n)
	}
	path, token := lastLink(t, e, box, "user@example.com")

	steps := []struct {
		name       string
		body       map[string]string
		wantStatus int
	}{
		{"короткий пароль", map[string]string{"token": token, "new_password": "short"}, 422},
		{"новый пароль", map[string]string{"token": token, "new_password": "brandnew123"}, 200},
		{"ссылка второй раз", map[string]string{"token": token, "new_password": "another123"}, 400},
	}
	for _, s := range steps {
		if resp := e.post("/auth/reset-password", s.body); resp.Status != s.wantStatus {
			t.Errorf("%s: %d %s", s.name, resp.Status, resp.Raw)
		}
	}
	if path != "/reset-password" {
		t.Errorf("путь ссылки %s", path)
	}
	if code := meWithCookie(e, oldSession); code != 401 {
		t.Errorf("старая сессия после сброса: %d", code)
	}
	if resp := e.post("/auth/login", map[string]string{"email": "user@example.com", "password": "password123"}); resp.Status != 401 {
		t.Errorf("старый пароль: %d", resp.Status)
	}
	e.login("user@example.com", "brandnew123")
	if got := e.get("/auth/me").JSON()["user"].(map[string]any); got["email_verified"] != true {
		t.Errorf("после сброса по ссылке почта подтверждена: %v", got)
	}
}

func TestResetLinkExpiresAndMailLimit(t *testing.T) {
	e, box := withMail(t)
	e.makeUser("user@example.com")
	for i := 0; i < 4; i++ {
		e.post("/auth/forgot-password", map[string]string{"email": "user@example.com"})
	}
	e.app.WaitMail()
	if n := len(box.all()); n != 3 {
		t.Errorf("писем за час: %d, want 3", n)
	}
	_, token := lastLink(t, e, box, "user@example.com")
	if _, err := e.st.DB().Exec("UPDATE email_tokens SET expires_at = '2000-01-01 00:00:00.000000'"); err != nil {
		t.Fatal(err)
	}
	if resp := e.post("/auth/reset-password", map[string]string{"token": token, "new_password": "brandnew123"}); resp.Status != 400 ||
		!strings.Contains(resp.Detail(), "устарела") {
		t.Errorf("просроченная ссылка: %d %s", resp.Status, resp.Raw)
	}
}

func TestChangeEmail(t *testing.T) {
	e, box := withMail(t)
	e.makeUser("user@example.com")
	e.makeUser("taken@example.com")
	e.login("user@example.com")

	tests := []struct {
		name       string
		body       map[string]string
		wantStatus int
	}{
		{"неверный пароль", map[string]string{"new_email": "new@example.com", "password": "nope"}, 400},
		{"тот же адрес", map[string]string{"new_email": "USER@example.com", "password": "password123"}, 400},
		{"занятый адрес", map[string]string{"new_email": "taken@example.com", "password": "password123"}, 409},
		{"кривой адрес", map[string]string{"new_email": "не адрес", "password": "password123"}, 422},
		{"новый адрес", map[string]string{"new_email": "New@example.com", "password": "password123"}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if resp := e.post("/auth/change-email", tt.body); resp.Status != tt.wantStatus {
				t.Errorf("status %d: %s", resp.Status, resp.Raw)
			}
		})
	}
	if me := e.get("/auth/me").JSON()["user"].(map[string]any); me["email"] != "user@example.com" {
		t.Errorf("до перехода по ссылке адрес прежний: %v", me["email"])
	}
	_, token := lastLink(t, e, box, "new@example.com")
	resp := e.post("/auth/verify-email", map[string]string{"token": token})
	if resp.Status != 200 || resp.JSON()["user"].(map[string]any)["email"] != "new@example.com" {
		t.Fatalf("переход по ссылке: %d %s", resp.Status, resp.Raw)
	}
	e.login("new@example.com")
	if actions := strings.Join(e.auditActions(), ","); !strings.HasSuffix(actions, "auth.email_change") {
		t.Errorf("журнал: %s", actions)
	}
}

func TestChangePasswordEndsOtherSessions(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	other := sessionCookie(e)

	resp := e.post("/auth/password", map[string]string{"current_password": "password123", "new_password": "newpassword1"})

	if resp.Status != 200 {
		t.Fatalf("смена пароля: %d %s", resp.Status, resp.Raw)
	}
	if got := e.get("/auth/me").Status; got != 200 {
		t.Errorf("текущая сессия: %d", got)
	}
	if code := meWithCookie(e, other); code != 401 {
		t.Errorf("другое устройство: %d", code)
	}
}

func TestEmailWithoutMailer(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	if resp := e.post("/auth/register", map[string]string{"email": "new@example.com", "password": "password123"}); resp.Status != 201 {
		t.Errorf("регистрация без почты: %d", resp.Status)
	}
	if resp := e.post("/auth/forgot-password", map[string]string{"email": "user@example.com"}); resp.Status != 503 {
		t.Errorf("сброс без почты: %d", resp.Status)
	}
	e.login("user@example.com")
	if resp := e.post("/auth/resend-verification", nil); resp.Status != 503 {
		t.Errorf("повтор письма без почты: %d", resp.Status)
	}
}

func TestResendVerification(t *testing.T) {
	e, box := withMail(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")

	if resp := e.post("/auth/resend-verification", nil); resp.Status != 200 {
		t.Fatalf("повтор: %d %s", resp.Status, resp.Raw)
	}
	_, token := lastLink(t, e, box, "user@example.com")
	e.post("/auth/verify-email", map[string]string{"token": token})
	if resp := e.post("/auth/resend-verification", nil); resp.Status != 400 {
		t.Errorf("уже подтверждена: %d", resp.Status)
	}
}
