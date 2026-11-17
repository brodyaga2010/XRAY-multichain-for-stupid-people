// Package auth — пароли (bcrypt), JWT HS256 и cookie сессии.
package auth

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Cost — стоимость bcrypt (как в Python-версии: 12). Переменная только ради быстрых тестов.
var Cost = 12

// bcrypt учитывает только первые 72 байта пароля; усечение совместимо с хешами,
// созданными прежней версией (bcrypt 4.x усекал молча).
const maxBcryptBytes = 72

func passwordBytes(password string) []byte {
	b := []byte(password)
	if len(b) > maxBcryptBytes {
		b = b[:maxBcryptBytes]
	}
	return b
}

// HashPassword возвращает bcrypt-хеш ($2a$/$2b$ читаются одинаково).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(passwordBytes(password), Cost)
	return string(hash), err
}

// VerifyPassword сравнивает пароль с хешем.
func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), passwordBytes(password)) == nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// BurnPasswordCheck тратит столько же времени, сколько проверка пароля: выравнивает время ответа
// для несуществующего email.
func BurnPasswordCheck(password string) {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("dummy-password") })
	VerifyPassword(password, dummyHash)
}

// ErrInvalidToken — токен недействителен.
var ErrInvalidToken = errors.New("invalid session")

// NewToken выпускает JWT HS256 с {"sub": "<id>", "exp": unix}.
func NewToken(secret string, userID int64, ttl time.Duration, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseToken проверяет подпись и срок и возвращает id пользователя.
func ParseToken(secret, token string) (int64, error) {
	var claims jwt.RegisteredClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !parsed.Valid {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}

// CookieOptions — параметры cookie сессии.
type CookieOptions struct {
	Name   string
	Domain string
	Secure bool
	MaxAge time.Duration
}

// SetCookie ставит httpOnly-cookie сессии (SameSite=Lax, path=/).
func (o CookieOptions) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     o.Name,
		Value:    token,
		Path:     "/",
		Domain:   o.Domain,
		MaxAge:   int(o.MaxAge / time.Second),
		HttpOnly: true,
		Secure:   o.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie удаляет cookie сессии.
func (o CookieOptions) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     o.Name,
		Value:    "",
		Path:     "/",
		Domain:   o.Domain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   o.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}
