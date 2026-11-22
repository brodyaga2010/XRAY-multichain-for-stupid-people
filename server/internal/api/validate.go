package api

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func invalid(detail string) error { return httpErr(http.StatusUnprocessableEntity, detail) }

// normalizeEmail проверяет адрес (по правилам EmailStr) и приводит домен к нижнему регистру.
func normalizeEmail(raw string) (string, bool) {
	email := strings.TrimSpace(raw)
	if email == "" || len(email) > 254 || strings.Count(email, "@") != 1 {
		return "", false
	}
	local, domain, _ := strings.Cut(email, "@")
	if !validLocal(local) {
		return "", false
	}
	domain = strings.ToLower(domain)
	if !validDomain(domain) {
		return "", false
	}
	return local + "@" + domain, true
}

func validLocal(local string) bool {
	if local == "" || len(local) > 64 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") ||
		strings.Contains(local, "..") {
		return false
	}
	for _, c := range local {
		switch {
		case c < 128 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'):
		case c < 128 && strings.ContainsRune("!#$%&'*+/=?^_`{|}~.-", c):
		case c >= 128 && (unicode.IsLetter(c) || unicode.IsDigit(c)):
		default:
			return false
		}
	}
	return true
}

func validDomain(domain string) bool {
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c >= 128 && (unicode.IsLetter(c) || unicode.IsDigit(c))) {
				return false
			}
		}
	}
	tld := labels[len(labels)-1]
	if _, err := strconv.Atoi(tld); err == nil {
		return false
	}
	return true
}

func requireEmail(v *string) (string, error) {
	if v == nil {
		return "", invalid("Укажите email")
	}
	email, ok := normalizeEmail(*v)
	if !ok {
		return "", invalid("Некорректный email")
	}
	return email, nil
}

func requireString(v *string, field string) (string, error) {
	if v == nil {
		return "", invalid("Поле «" + field + "» обязательно")
	}
	return *v, nil
}

func newPassword(v *string, field string) (string, error) {
	value, err := requireString(v, field)
	if err != nil {
		return "", err
	}
	if n := utf8.RuneCountInString(value); n < 8 || n > 128 {
		return "", invalid("Пароль должен быть длиной от 8 до 128 символов")
	}
	return value, nil
}

// keyName: 1..60 символов (до очистки), затем пробелы схлопываются; пустое имя недопустимо.
func keyName(v *string) (string, error) {
	if v == nil {
		return "", invalid("Укажите название ключа")
	}
	if n := utf8.RuneCountInString(*v); n < 1 || n > 60 {
		return "", invalid("Название ключа должно быть длиной от 1 до 60 символов")
	}
	name := strings.Join(strings.Fields(*v), " ")
	if name == "" {
		return "", invalid("Название не может быть пустым")
	}
	return name, nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, invalid("Некорректный идентификатор")
	}
	return id, nil
}
