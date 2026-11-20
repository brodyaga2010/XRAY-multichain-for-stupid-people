package mail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuild(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	raw := Build("noreply@example.com", "Кабинет", Message{
		To: "user@example.com", Subject: "Подтвердите почту", Text: "Ссылка:\nhttps://example.com/x?token=abc", HTML: "<p>Ссылка</p>",
	}, now)

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	from, _ := mail.ParseAddress(msg.Header.Get("From"))
	if subject != "Подтвердите почту" || from.Name != "Кабинет" || from.Address != "noreply@example.com" ||
		msg.Header.Get("To") != "user@example.com" || !strings.HasSuffix(msg.Header.Get("Message-ID"), "@example.com>") {
		t.Errorf("заголовки: %v", msg.Header)
	}
	_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	reader := multipart.NewReader(msg.Body, params["boundary"])
	var types, bodies []string
	for {
		part, err := reader.NextPart() // quoted-printable раскодируется автоматически
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(part)
		types = append(types, part.Header.Get("Content-Type"))
		bodies = append(bodies, string(body))
	}
	if len(types) != 2 || !strings.HasPrefix(types[0], "text/plain") || !strings.HasPrefix(types[1], "text/html") {
		t.Fatalf("части: %v", types)
	}
	if bodies[0] != "Ссылка:\r\nhttps://example.com/x?token=abc" || bodies[1] != "<p>Ссылка</p>" {
		t.Errorf("тексты: %q", bodies)
	}
	// Письмо уходит без SMTPUTF8, поэтому в сыром виде допустима только латиница.
	for i, c := range raw {
		if c >= 0x80 {
			t.Fatalf("байт %#x на позиции %d: письмо должно быть в ASCII", c, i)
		}
	}
}

// fakeSMTP — минимальный SMTP-сервер без TLS; возвращает адрес и канал с принятыми командами и письмом.
// mechs — что сервер объявляет в AUTH («PLAIN», «LOGIN», «LOGIN PLAIN»).
func fakeSMTP(t *testing.T, mechs string, rejectAuth bool) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		var log strings.Builder
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				out <- log.String()
				return
			}
			cmd := strings.TrimSpace(line)
			log.WriteString(cmd + "\n")
			switch upper := strings.ToUpper(cmd); {
			case strings.HasPrefix(upper, "EHLO"):
				write("250-fake")
				write("250-SMTPUTF8")
				write("250-8BITMIME")
				write("250 AUTH " + mechs)
			case strings.HasPrefix(upper, "AUTH"):
				if upper == "AUTH LOGIN" {
					for _, prompt := range []string{"Username:", "Password:"} {
						write("334 " + base64.StdEncoding.EncodeToString([]byte(prompt)))
						answer, _ := r.ReadString('\n')
						decoded, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(answer))
						log.WriteString("LOGIN " + prompt + " " + string(decoded) + "\n")
					}
				}
				if rejectAuth {
					write("535 bad credentials")
				} else {
					write("235 ok")
				}
			case upper == "DATA":
				write("354 go")
				for {
					l, _ := r.ReadString('\n')
					if l == ".\r\n" {
						break
					}
					log.WriteString(l)
				}
				write("250 queued")
			case upper == "QUIT":
				write("221 bye")
				out <- log.String()
				return
			default:
				write("250 ok")
			}
		}
	}()
	return ln.Addr().String(), out
}

func TestSMTPSend(t *testing.T) {
	tests := []struct {
		name       string
		mechs      string
		rejectAuth bool
		wantAuth   []string
		wantErr    string
	}{
		{"PLAIN", "PLAIN", false, []string{"AUTH PLAIN " + base64.StdEncoding.EncodeToString([]byte("\x00noreply@example.com\x00secret"))}, ""},
		{"только LOGIN, как у Beget", "LOGIN", false, []string{"AUTH LOGIN", "LOGIN Username: noreply@example.com", "LOGIN Password: secret"}, ""},
		{"оба — берём PLAIN", "LOGIN PLAIN", false, []string{"AUTH PLAIN "}, ""},
		{"неверный пароль", "PLAIN", true, nil, "вход на сервер"},
		{"неверный пароль через LOGIN", "LOGIN", true, nil, "вход на сервер"},
		{"нет общего способа входа", "CRAM-MD5", false, nil, "не предлагает PLAIN или LOGIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, got := fakeSMTP(t, tt.mechs, tt.rejectAuth)
			host, port, _ := net.SplitHostPort(addr)
			p, _ := strconv.Atoi(port)
			s, err := NewSMTP(SMTPConfig{Host: host, Port: p, Username: "noreply@example.com", Password: "secret", From: "noreply@example.com", FromName: "Кабинет"})
			if err != nil {
				t.Fatal(err)
			}

			err = s.Send(context.Background(), Message{To: "user@example.com", Subject: "Тест", Text: "привет"})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			log := <-got
			// MAIL FROM без SMTPUTF8 и BODY=8BITMIME, хотя сервер их объявил: иначе Beget не довезёт письмо до Яндекса.
			for _, want := range append(tt.wantAuth, "MAIL FROM:<noreply@example.com>\n", "RCPT TO:<user@example.com>", "Subject: =?utf-8?q?") {
				if !strings.Contains(log, want) {
					t.Errorf("нет %q в диалоге:\n%s", want, log)
				}
			}
		})
	}
}

func TestNewSMTPValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  SMTPConfig
		ok   bool
	}{
		{"всё задано", SMTPConfig{Host: "smtp.example.com", Port: 465, From: "noreply@example.com"}, true},
		{"нет хоста", SMTPConfig{Port: 465, From: "noreply@example.com"}, false},
		{"кривой From", SMTPConfig{Host: "smtp.example.com", Port: 465, From: "не адрес"}, false},
		{"From кириллицей", SMTPConfig{Host: "smtp.example.com", Port: 465, From: "noreply@почта.рф"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSMTP(tt.cfg); (err == nil) != tt.ok {
				t.Errorf("err = %v", err)
			}
		})
	}
	if err := (&SMTP{cfg: SMTPConfig{Host: "x", Port: 1, From: "a@b.c"}}).Send(context.Background(), Message{To: "плохой"}); err == nil {
		t.Error("кривой адрес получателя должен отклоняться")
	}
}

func TestSendRejectsBadRecipient(t *testing.T) {
	s, err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: 1, From: "noreply@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []string{"не адрес", "user@почта.рф", "user@example.com\r\nBcc: x@example.com"}
	for _, to := range tests {
		t.Run(to, func(t *testing.T) {
			// До подключения: порт 1 закрыт, и ошибка подключения означала бы, что адрес не проверен.
			if err := s.Send(context.Background(), Message{To: to}); err == nil || !strings.Contains(err.Error(), "адрес получателя") {
				t.Errorf("err = %v", err)
			}
		})
	}
}
