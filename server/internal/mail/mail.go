// Package mail — отправка писем: SMTP (порт 465 — сразу TLS, иначе STARTTLS) или в лог для разработки.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Message — письмо: обычный текст и HTML-версия.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// Inline — картинки внутри HTML (<img src="cid:ID">): лежат в самом письме, поэтому почтовики
	// показывают их сразу, не спрашивая «загрузить изображения?».
	Inline []Inline
}

// Inline — вложенная картинка, на которую HTML ссылается как cid:ContentID.
type Inline struct {
	ContentID   string
	ContentType string
	Data        []byte
}

// Sender отправляет письма.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// SMTPConfig — подключение к почтовому серверу.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// From — адрес отправителя; FromName — подпись («kemsekov»).
	From     string
	FromName string
	// tlsConfig — для тестов (самоподписанный сертификат).
	tlsConfig *tls.Config
}

// SMTP — отправка через почтовый сервер. Каждое письмо — отдельное соединение.
type SMTP struct {
	cfg SMTPConfig
	now func() time.Time
}

// NewSMTP проверяет настройки и создаёт отправителя.
func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	if cfg.Host == "" || cfg.Port <= 0 || cfg.From == "" {
		return nil, errors.New("почта: нужны SMTP_HOST, SMTP_PORT и SMTP_FROM")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("почта: некорректный SMTP_FROM: %w", err)
	}
	if !isASCII(cfg.From) {
		return nil, errors.New("почта: SMTP_FROM должен состоять из латиницы")
	}
	return &SMTP{cfg: cfg, now: time.Now}, nil
}

func (s *SMTP) tlsConfig() *tls.Config {
	if s.cfg.tlsConfig != nil {
		return s.cfg.tlsConfig
	}
	return &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
}

// Send отправляет письмо; таймаут — по ctx (не больше 30 с).
func (s *SMTP) Send(ctx context.Context, msg Message) error {
	if _, err := mail.ParseAddress(msg.To); err != nil || !isASCII(msg.To) {
		return fmt.Errorf("почта: некорректный адрес получателя %q", msg.To)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{}
	var (
		conn net.Conn
		err  error
	)
	if s.cfg.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: s.tlsConfig()}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("почта: подключение к %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("почта: %w", err)
	}
	defer client.Close()
	if s.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(s.tlsConfig()); err != nil {
				return fmt.Errorf("почта: STARTTLS: %w", err)
			}
		}
	}
	if s.cfg.Username != "" {
		auth, err := s.auth(client)
		if err != nil {
			return err
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("почта: вход на сервер: %w", err)
		}
	}
	if err := mailFrom(client, s.cfg.From); err != nil {
		return fmt.Errorf("почта: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("почта: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("почта: DATA: %w", err)
	}
	if _, err := w.Write(Build(s.cfg.From, s.cfg.FromName, msg, s.now())); err != nil {
		return fmt.Errorf("почта: отправка: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("почта: сервер не принял письмо: %w", err)
	}
	return client.Quit()
}

// mailFrom — MAIL FROM без параметров. client.Mail добавляет SMTPUTF8, если сервер его объявил, и тогда
// Beget помечает письмо как UTF-8 и не может передать его серверам без SMTPUTF8 (mx.yandex.ru) — письмо
// возвращается с ошибкой 5.0.0. Нам SMTPUTF8 не нужен: адреса проверены на латиницу, заголовки закодированы
// по RFC 2047, тело — quoted-printable.
func mailFrom(client *smtp.Client, from string) error {
	id, err := client.Text.Cmd("MAIL FROM:<%s>", from)
	if err != nil {
		return err
	}
	client.Text.StartResponse(id)
	defer client.Text.EndResponse(id)
	_, _, err = client.Text.ReadResponse(250)
	return err
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 || s[i] < 0x20 {
			return false
		}
	}
	return true
}

// auth выбирает способ входа из объявленных сервером: PLAIN, иначе LOGIN (Beget знает только LOGIN).
func (s *SMTP) auth(client *smtp.Client) (smtp.Auth, error) {
	_, params := client.Extension("AUTH")
	mechs := strings.Fields(strings.ToUpper(params))
	has := func(m string) bool {
		for _, x := range mechs {
			if x == m {
				return true
			}
		}
		return false
	}
	switch {
	case has("PLAIN"):
		return smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host), nil
	case has("LOGIN"):
		return &loginAuth{username: s.cfg.Username, password: s.cfg.Password, host: s.cfg.Host}, nil
	default:
		return nil, fmt.Errorf("почта: вход на сервер: сервер не предлагает PLAIN или LOGIN (AUTH %s)", params)
	}
}

// loginAuth — механизм AUTH LOGIN: сервер спрашивает «Username:» и «Password:», клиент отвечает по очереди.
type loginAuth struct {
	username, password, host string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// Как и smtp.PlainAuth: пароль только по шифрованному соединению, кроме localhost.
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, errors.New("незашифрованное соединение")
	}
	if server.Name != a.host {
		return "", nil, errors.New("неверное имя хоста")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch prompt := strings.ToLower(strings.TrimSpace(string(fromServer))); {
	case strings.HasPrefix(prompt, "username"):
		return []byte(a.username), nil
	case strings.HasPrefix(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("неожиданный запрос сервера при входе: %q", fromServer)
	}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

// Build собирает письмо MIME: multipart/alternative с текстом и HTML в UTF-8 (quoted-printable);
// с картинками — внутри multipart/related. Результат целиком в ASCII.
func Build(from, fromName string, msg Message, now time.Time) []byte {
	boundary := randomHex(12)
	related := randomHex(12)
	inline := msg.HTML != "" && len(msg.Inline) > 0
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 {
		domain = from[at+1:]
	}
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", (&mail.Address{Name: fromName, Address: from}).String())
	header("To", msg.To)
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+randomHex(16)+"@"+domain+">")
	header("MIME-Version", "1.0")
	if inline {
		header("Content-Type", `multipart/related; type="multipart/alternative"; boundary="`+related+`"`)
		fmt.Fprintf(&b, "\r\n--%s\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n", related, boundary)
	} else {
		header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	}
	b.WriteString("\r\n")
	part := func(contentType, body string) {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, contentType)
		qp := quotedprintable.NewWriter(&b)
		_, _ = qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
		_ = qp.Close()
		b.WriteString("\r\n")
	}
	part("text/plain", msg.Text)
	if msg.HTML != "" {
		part("text/html", msg.HTML)
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	if inline {
		for _, img := range msg.Inline {
			fmt.Fprintf(&b, "\r\n--%s\r\nContent-Type: %s\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <%s>\r\nContent-Disposition: inline\r\n\r\n",
				related, img.ContentType, img.ContentID)
			encoded := base64.StdEncoding.EncodeToString(img.Data)
			for len(encoded) > 76 {
				b.WriteString(encoded[:76] + "\r\n")
				encoded = encoded[76:]
			}
			b.WriteString(encoded + "\r\n")
		}
		fmt.Fprintf(&b, "--%s--\r\n", related)
	}
	return b.Bytes()
}

func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Log — «отправка» в журнал сервиса: для разработки, когда SMTP не настроен.
type Log struct{ Logger *slog.Logger }

func (l Log) Send(_ context.Context, msg Message) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("письмо (SMTP не настроен, только в журнал)", "to", msg.To, "subject", msg.Subject, "text", msg.Text)
	return nil
}
