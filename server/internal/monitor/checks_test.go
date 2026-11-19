package monitor

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSOCKS — SOCKS5-прокси для тестов; hang=true — принимает CONNECT, но не пересылает данные.
func fakeSOCKS(t *testing.T, hang bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSOCKS(conn, hang)
		}
	}()
	return ln.Addr().String()
}

func serveSOCKS(conn net.Conn, hang bool) {
	defer conn.Close()
	buf := make([]byte, 262)
	if _, err := io.ReadFull(conn, buf[:3]); err != nil {
		return
	}
	conn.Write([]byte{5, 0})
	if _, err := io.ReadFull(conn, buf[:5]); err != nil {
		return
	}
	host := make([]byte, buf[4])
	io.ReadFull(conn, host)
	portBuf := make([]byte, 2)
	io.ReadFull(conn, portBuf)
	target := net.JoinHostPort(string(host), strconv.Itoa(int(binary.BigEndian.Uint16(portBuf))))
	conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	if hang {
		time.Sleep(5 * time.Second)
		return
	}
	upstream, err := net.Dial("tcp", target)
	if err != nil {
		return
	}
	defer upstream.Close()
	go io.Copy(upstream, conn)
	io.Copy(conn, upstream)
}

func closedAddr(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestChainCheck(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/broken" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(site.Close)

	tests := []struct {
		name       string
		probe      string
		path       string
		slow       time.Duration
		timeout    time.Duration
		wantStatus Status
		wantDetail string
		wantMs     bool
	}{
		{"цепочка работает", fakeSOCKS(t, false), "/generate_204", 3 * time.Second, 5 * time.Second, StatusOK, "", true},
		{"медленно — деградация", fakeSOCKS(t, false), "/generate_204", time.Nanosecond, 5 * time.Second, StatusWarn, "очень медленно", true},
		{"служебный вход закрыт", closedAddr(t), "/generate_204", 0, 5 * time.Second, StatusFail, "служебный вход Xray", false},
		{"трафик не проходит", fakeSOCKS(t, true), "/generate_204", 0, 300 * time.Millisecond, StatusFail, "нет ответа", false},
		{"сайт ответил 502", fakeSOCKS(t, false), "/broken", 0, 5 * time.Second, StatusFail, "502", true},
		{"выключено", "", "/", 0, time.Second, StatusUnknown, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Deps{ProbeAddr: tt.probe, ProbeURL: site.URL + tt.path, ChainSlow: tt.slow}
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			r := d.chain(ctx)

			if r.Status != tt.wantStatus || !strings.Contains(r.Detail, tt.wantDetail) || (r.Ms != nil) != tt.wantMs {
				t.Errorf("result = %+v", r)
			}
		})
	}
}

func TestExitCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	split := func(addr string) (string, int) {
		host, port, _ := net.SplitHostPort(addr)
		p, _ := strconv.Atoi(port)
		return host, p
	}
	openHost, openPort := split(ln.Addr().String())
	closedHost, closedPort := split(closedAddr(t))

	tests := []struct {
		name       string
		server     func() (string, int, bool, error)
		slow       time.Duration
		wantStatus Status
		wantValue  string
	}{
		{"связь есть", func() (string, int, bool, error) { return openHost, openPort, true, nil }, time.Second, StatusOK, "мс"},
		{"высокая задержка", func() (string, int, bool, error) { return openHost, openPort, true, nil }, time.Nanosecond, StatusWarn, "мс"},
		{"нет связи", func() (string, int, bool, error) { return closedHost, closedPort, true, nil }, time.Second, StatusFail, ""},
		{"одиночный сервер", func() (string, int, bool, error) { return "", 0, false, nil }, 0, StatusUnknown, "без выходного"},
		{"конфиг не читается", func() (string, int, bool, error) { return "", 0, false, errors.New("x") }, 0, StatusUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Deps{ExitServer: tt.server, ExitSlow: tt.slow}.exit(context.Background())
			if r.Status != tt.wantStatus || !strings.Contains(r.Value, tt.wantValue) {
				t.Errorf("result = %+v", r)
			}
		})
	}
}

func TestXrayCheck(t *testing.T) {
	ok := Deps{XrayOnline: func() (int, error) { return 7, nil }}.xray(context.Background())
	bad := Deps{XrayOnline: func() (int, error) { return 0, errors.New("api") }}.xray(context.Background())
	if ok.Status != StatusOK || ok.Value != "онлайн: 7" || bad.Status != StatusFail {
		t.Errorf("ok=%+v bad=%+v", ok, bad)
	}
}

func TestCertsCheck(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler()) // сертификат на example.com, действует до 2084 г.
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "https://")
	trusted := x509.NewCertPool()
	trusted.AddCert(srv.Certificate())

	tests := []struct {
		name       string
		deps       Deps
		wantStatus Status
		wantText   string
	}{
		{"сертификат действует", Deps{CertAddr: addr, CertDomains: []string{"example.com"}, certRoots: trusted}, StatusOK, "действуют ещё"},
		{"недоверенный сертификат", Deps{CertAddr: addr, CertDomains: []string{"example.com"}}, StatusFail, "example.com"},
		{"не тот домен", Deps{CertAddr: addr, CertDomains: []string{"other.test"}, certRoots: trusted}, StatusFail, "other.test"},
		{"нет доменов", Deps{}, StatusUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			r := tt.deps.certs(ctx)
			if r.Status != tt.wantStatus || !strings.Contains(r.Value+r.Detail, tt.wantText) {
				t.Errorf("result = %+v", r)
			}
		})
	}
}

func TestMemoryUsedPct(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
		wantOK  bool
	}{
		{"обычный", "MemTotal:        2000000 kB\nMemFree:  100 kB\nMemAvailable:    1400000 kB\n", 30, true},
		{"нет MemTotal", "MemFree: 1 kB\n", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "meminfo")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, ok := memoryUsedPct(path)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("got %d %v", got, ok)
			}
		})
	}
	if _, ok := memoryUsedPct(filepath.Join(t.TempDir(), "нет")); ok {
		t.Error("отсутствующий файл")
	}
}

func TestSystemCheckReportsDisk(t *testing.T) {
	r := Deps{DataDir: t.TempDir()}.system(context.Background())
	if r.Status == StatusUnknown || !strings.Contains(r.Value, "диск") {
		t.Errorf("result = %+v", r)
	}
}
