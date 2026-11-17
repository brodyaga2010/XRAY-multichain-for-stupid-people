package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"xraypanel/internal/store"
)

func TestRegisterCreatesPendingUser(t *testing.T) {
	e := newEnv(t)

	resp := e.post("/auth/register", map[string]string{"email": "new@example.com", "password": "password123"})

	if resp.Status != 201 {
		t.Fatalf("status %d: %s", resp.Status, resp.Raw)
	}
	user := resp.JSON()["user"].(map[string]any)
	if user["status"] != "pending" || user["role"] != "user" || user["key_limit"] != float64(3) {
		t.Errorf("user = %v", user)
	}
	if _, ok := user["password_hash"]; ok {
		t.Error("хеш пароля не должен попадать в ответ")
	}
}

func TestRegisterValidationAndConflict(t *testing.T) {
	e := newEnv(t)
	e.makeUser("taken@example.com")
	tests := []struct {
		name   string
		body   any
		status int
		detail string
	}{
		{"занятый email", map[string]string{"email": "Taken@Example.com", "password": "password123"}, 409, "Этот email уже зарегистрирован"},
		{"плохой email", map[string]string{"email": "not-an-email", "password": "password123"}, 422, ""},
		{"email без точки в домене", map[string]string{"email": "a@localhost", "password": "password123"}, 422, ""},
		{"короткий пароль", map[string]string{"email": "a@example.com", "password": "short"}, 422, ""},
		{"длинный пароль", map[string]string{"email": "a@example.com", "password": string(make([]byte, 129))}, 422, ""},
		{"нет полей", map[string]string{}, 422, ""},
		{"не JSON", "just a string", 422, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.post("/auth/register", tt.body)
			if resp.Status != tt.status {
				t.Fatalf("status %d: %s", resp.Status, resp.Raw)
			}
			if tt.detail != "" && resp.Detail() != tt.detail {
				t.Errorf("detail = %q", resp.Detail())
			}
			if tt.status == 422 && resp.Detail() == "" {
				t.Errorf("detail 422 должен быть непустой строкой: %s", resp.Raw)
			}
		})
	}
}

func TestLoginStatuses(t *testing.T) {
	e := newEnv(t)
	e.makeUser("pending@example.com", withStatus(store.UserPending))
	e.makeUser("suspended@example.com", withStatus(store.UserSuspended))
	e.makeUser("ok@example.com")
	tests := []struct {
		name     string
		email    string
		password string
		status   int
		detail   string
	}{
		{"pending не входит", "pending@example.com", "password123", 403, "Аккаунт ещё не подтверждён администратором"},
		{"suspended не входит", "suspended@example.com", "password123", 403, "Аккаунт ещё не подтверждён администратором"},
		{"approved входит", "ok@example.com", "password123", 200, ""},
		{"email без учёта регистра", "OK@Example.com", "password123", 200, ""},
		{"неверный пароль", "ok@example.com", "wrong", 401, "Неверный email или пароль"},
		{"неизвестный email", "nobody@example.com", "password123", 401, "Неверный email или пароль"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e.app.Limiter().Reset()
			resp := e.post("/auth/login", map[string]string{"email": tt.email, "password": tt.password})
			if resp.Status != tt.status {
				t.Fatalf("status %d: %s", resp.Status, resp.Raw)
			}
			if tt.detail != "" && resp.Detail() != tt.detail {
				t.Errorf("detail = %q", resp.Detail())
			}
			if tt.status == 200 && resp.JSON()["user"].(map[string]any)["email"] != "ok@example.com" {
				t.Errorf("ответ: %s", resp.Raw)
			}
		})
	}
}

func TestApprovedUserLoginSetsCookie(t *testing.T) {
	e := newEnv(t)
	e.makeUser("ok@example.com")

	resp := e.post("/auth/login", map[string]string{"email": "ok@example.com", "password": "password123"})

	var cookie *http.Cookie
	for _, c := range resp.Cookies {
		if c.Name == "xray_access_token" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("cookie xray_access_token не выставлена")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Secure {
		t.Errorf("атрибуты cookie: %+v", cookie)
	}
	if cookie.MaxAge != 24*60*60 {
		t.Errorf("Max-Age = %d", cookie.MaxAge)
	}
	if e.get("/auth/me").Status != 200 {
		t.Error("сессия не работает после входа")
	}
}

func TestCookieSecureAndDomainFromConfig(t *testing.T) {
	e := newEnv(t, map[string]string{"COOKIE_SECURE": "true", "COOKIE_DOMAIN": "panel.example.com", "COOKIE_NAME": "sid", "ACCESS_TOKEN_MINUTES": "5"})
	e.makeUser("ok@example.com")

	resp := e.post("/auth/login", map[string]string{"email": "ok@example.com", "password": "password123"})

	if len(resp.Cookies) != 1 {
		t.Fatalf("cookies: %v", resp.Cookies)
	}
	c := resp.Cookies[0]
	if c.Name != "sid" || !c.Secure || c.Domain != "panel.example.com" || c.MaxAge != 300 {
		t.Errorf("cookie: %+v", c)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	e := newEnv(t)

	resp := e.post("/auth/logout", nil)

	if resp.Status != 200 || resp.JSON()["message"] != "Вы вышли" {
		t.Fatalf("%d %s", resp.Status, resp.Raw)
	}
	if len(resp.Cookies) != 1 || resp.Cookies[0].MaxAge != -1 {
		t.Errorf("cookie не сброшена: %v", resp.Cookies)
	}
}

// pythonToken — токен в формате прежней версии: HS256, {"sub": "<id>", "exp": unix}.
func pythonToken(t *testing.T, secret, sub string, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub, "exp": exp.Unix()})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestSessionTokens(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser("ok@example.com")
	id := itoa(user.ID)
	tests := []struct {
		name   string
		token  string
		status int
		detail string
	}{
		{"выпущен Python-версией", pythonToken(t, testSecret, id, time.Now().Add(time.Hour)), 200, ""},
		{"просрочен", pythonToken(t, testSecret, id, time.Now().Add(-time.Hour)), 401, "Invalid session"},
		{"чужой секрет", pythonToken(t, "another-secret-long-enough", id, time.Now().Add(time.Hour)), 401, "Invalid session"},
		{"sub не число", pythonToken(t, testSecret, "abc", time.Now().Add(time.Hour)), 401, "Invalid session"},
		{"мусор", "garbage", 401, "Invalid session"},
		{"пользователь удалён", pythonToken(t, testSecret, "9999", time.Now().Add(time.Hour)), 401, "Пользователь не найден"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", e.server.URL+"/api/auth/me", nil)
			req.AddCookie(&http.Cookie{Name: "xray_access_token", Value: tt.token})
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Fatalf("status %d", resp.StatusCode)
			}
		})
	}
	if got := e.get("/auth/me"); got.Status != 401 || got.Detail() != "Не выполнен вход" {
		t.Errorf("без cookie: %d %s", got.Status, got.Raw)
	}
}

func TestChangePassword(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")

	wrong := e.post("/auth/password", map[string]string{"current_password": "nope", "new_password": "newpassword1"})
	short := e.post("/auth/password", map[string]string{"current_password": "password123", "new_password": "short"})
	ok := e.post("/auth/password", map[string]string{"current_password": "password123", "new_password": "newpassword1"})

	if wrong.Status != 400 || wrong.Detail() != "Текущий пароль указан неверно" {
		t.Errorf("wrong: %d %s", wrong.Status, wrong.Raw)
	}
	if short.Status != 422 {
		t.Errorf("short: %d", short.Status)
	}
	if ok.Status != 200 || ok.JSON()["message"] != "Пароль изменён" {
		t.Errorf("ok: %d %s", ok.Status, ok.Raw)
	}
	e.login("user@example.com", "newpassword1")
	if actions := e.auditActions(); len(actions) != 1 || actions[0] != "user.password_change" {
		t.Errorf("аудит: %v", actions)
	}
}

func TestLoginRateLimited(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	limit := e.cfg.AuthRateLimit

	var codes []int
	for i := 0; i < limit+1; i++ {
		codes = append(codes, e.post("/auth/login", map[string]string{"email": "user@example.com", "password": "bad"}).Status)
	}

	for i, code := range codes[:limit] {
		if code != 401 {
			t.Fatalf("попытка %d: %d", i, code)
		}
	}
	last := e.post("/auth/login", map[string]string{"email": "user@example.com", "password": "bad"})
	if codes[limit] != 429 || last.Detail() != "Слишком много попыток, попробуйте через несколько минут" {
		t.Errorf("последняя попытка: %d %s", codes[limit], last.Raw)
	}
}

func TestRateLimitUsesForwardedForOnlyFromTrustedProxy(t *testing.T) {
	// httptest слушает 127.0.0.1 — он в TRUSTED_PROXIES по умолчанию, поэтому X-Forwarded-For учитывается.
	e := newEnv(t, map[string]string{"AUTH_RATE_LIMIT": "1"})
	e.makeUser("user@example.com")
	login := func(ip string) int {
		req, _ := http.NewRequest("POST", e.server.URL+"/api/auth/login", stringsReader(`{"email":"user@example.com","password":"bad"}`))
		req.Header.Set("X-Forwarded-For", ip+", 10.0.0.1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if a, b, c := login("1.1.1.1"), login("2.2.2.2"), login("1.1.1.1"); a != 401 || b != 401 || c != 429 {
		t.Errorf("коды: %d %d %d", a, b, c)
	}

	untrusted := newEnv(t, map[string]string{"AUTH_RATE_LIMIT": "1", "TRUSTED_PROXIES": "10.9.9.9"})
	untrusted.makeUser("user@example.com")
	loginUntrusted := func(ip string) int {
		req, _ := http.NewRequest("POST", untrusted.server.URL+"/api/auth/login", stringsReader(`{"email":"user@example.com","password":"bad"}`))
		req.Header.Set("X-Forwarded-For", ip)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if a, b := loginUntrusted("1.1.1.1"), loginUntrusted("2.2.2.2"); a != 401 || b != 429 {
		t.Errorf("недоверенный прокси: %d %d (заголовок должен игнорироваться)", a, b)
	}
}
