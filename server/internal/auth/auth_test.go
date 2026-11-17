package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const secret = "test-secret-that-is-long-enough"

func TestHashUsesCost12AndVerifies(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}

	cost, _ := bcrypt.Cost([]byte(hash))
	if cost != 12 || !strings.HasPrefix(hash, "$2") {
		t.Errorf("hash %q cost %d", hash, cost)
	}
	if !VerifyPassword("password123", hash) || VerifyPassword("password124", hash) {
		t.Error("проверка пароля работает неверно")
	}
}

func TestVerifyPythonBcryptHash(t *testing.T) {
	// $2b$ хеш, созданный bcrypt из Python
	const pythonHash = "$2b$12$6umehH/2T5PyaPUhQKlxu.56KfmvwYDzdNeSQq9XuJxrDWaElmjxi"

	if !VerifyPassword("password123", pythonHash) || VerifyPassword("wrong", pythonHash) {
		t.Error("хеш, созданный Python-версией, не проверяется")
	}
}

func TestLongPasswordsAreTruncatedLikeBcrypt(t *testing.T) {
	old := Cost
	Cost = 4
	defer func() { Cost = old }()
	long := strings.Repeat("я", 60) // 120 байт

	hash, err := HashPassword(long)

	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(long, hash) || !VerifyPassword(string([]byte(long)[:72]), hash) {
		t.Error("пароль длиннее 72 байт должен проверяться по первым 72 байтам")
	}
}

func TestTokenRoundTripAndFormat(t *testing.T) {
	now := time.Now()

	token, err := NewToken(secret, 42, time.Hour, now)

	if err != nil {
		t.Fatal(err)
	}
	id, err := ParseToken(secret, token)
	if err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	claims := jwt.MapClaims{}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, claims)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != "HS256" || claims["sub"] != "42" {
		t.Errorf("alg=%s claims=%v", parsed.Method.Alg(), claims)
	}
	if exp, _ := claims["exp"].(float64); int64(exp) != now.Add(time.Hour).Unix() {
		t.Errorf("exp = %v", claims["exp"])
	}
	if _, has := claims["iat"]; has {
		t.Error("payload должен содержать только sub и exp")
	}
}

func TestParseTokenRejects(t *testing.T) {
	sign := func(method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	future := time.Now().Add(time.Hour).Unix()
	tests := []struct {
		name  string
		token string
	}{
		{"просрочен", sign(jwt.SigningMethodHS256, []byte(secret), jwt.MapClaims{"sub": "1", "exp": time.Now().Add(-time.Minute).Unix()})},
		{"чужой секрет", sign(jwt.SigningMethodHS256, []byte("other-secret-long-enough"), jwt.MapClaims{"sub": "1", "exp": future})},
		{"алгоритм HS512", sign(jwt.SigningMethodHS512, []byte(secret), jwt.MapClaims{"sub": "1", "exp": future})},
		{"alg none", sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, jwt.MapClaims{"sub": "1", "exp": future})},
		{"sub не число", sign(jwt.SigningMethodHS256, []byte(secret), jwt.MapClaims{"sub": "abc", "exp": future})},
		{"нет sub", sign(jwt.SigningMethodHS256, []byte(secret), jwt.MapClaims{"exp": future})},
		{"мусор", "not.a.token"},
		{"пусто", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseToken(secret, tt.token); err == nil {
				t.Error("токен должен быть отклонён")
			}
		})
	}
}

func TestCookies(t *testing.T) {
	opts := CookieOptions{Name: "sid", Domain: "example.com", Secure: true, MaxAge: 90 * time.Minute}
	rec := httptest.NewRecorder()

	opts.SetCookie(rec, "tok")
	opts.ClearCookie(rec)

	set, clear := rec.Result().Cookies()[0], rec.Result().Cookies()[1]
	if set.Value != "tok" || set.MaxAge != 5400 || !set.HttpOnly || !set.Secure || set.Path != "/" || set.Domain != "example.com" {
		t.Errorf("set: %+v", set)
	}
	if !strings.Contains(rec.Header().Values("Set-Cookie")[0], "SameSite=Lax") {
		t.Errorf("SameSite: %v", rec.Header().Values("Set-Cookie"))
	}
	if clear.MaxAge != -1 || clear.Value != "" || clear.Path != "/" {
		t.Errorf("clear: %+v", clear)
	}
}
