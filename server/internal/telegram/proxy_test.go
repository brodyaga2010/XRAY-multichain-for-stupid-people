package telegram

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// forwardProxy — HTTP-прокси для тестов: пересылает запросы дальше, считает их; hang — не отвечает.
func forwardProxy(t *testing.T, hang bool) (string, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if hang {
			// тело дочитываем, иначе сервер не заметит, что клиент отключился
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		r.RequestURI = ""
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits
}

func closedProxy(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return "http://" + addr
}

func TestClientProxyWithDirectFallback(t *testing.T) {
	tests := []struct {
		name        string
		proxy       func(t *testing.T) (string, *atomic.Int32)
		wantProxied int32 // запросов дошло до прокси (из двух)
		wantDirect  bool  // после первого запроса клиент ходит напрямую
	}{
		{"прокси работает — оба запроса через него", func(t *testing.T) (string, *atomic.Int32) { return forwardProxy(t, false) }, 2, false},
		{"прокси недоступен — напрямую, второй сразу напрямую", func(t *testing.T) (string, *atomic.Int32) { return closedProxy(t), &atomic.Int32{} }, 0, true},
		{"прокси завис — по таймауту напрямую", func(t *testing.T) (string, *atomic.Int32) { return forwardProxy(t, true) }, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, client := newFakeAPI(t)
			client.log = slog.New(slog.NewTextHandler(io.Discard, nil))
			client.proxyTimeout = 300 * time.Millisecond
			proxyURL, hits := tt.proxy(t)
			if err := client.UseProxy(proxyURL); err != nil {
				t.Fatal(err)
			}

			for i := 0; i < 2; i++ {
				if err := client.SendMessage(context.Background(), 1, "тревога", nil); err != nil {
					t.Fatalf("запрос %d: %v", i, err)
				}
			}

			if got := len(f.callsOf("sendMessage")); got != 2 {
				t.Errorf("до Telegram дошло %d сообщений, want 2", got)
			}
			if hits.Load() != tt.wantProxied || client.useDirect() != tt.wantDirect {
				t.Errorf("через прокси %d (want %d), напрямую сейчас %v (want %v)", hits.Load(), tt.wantProxied, client.useDirect(), tt.wantDirect)
			}
		})
	}
}

func TestClientReturnsToProxyAfterDirectPeriod(t *testing.T) {
	f, client := newFakeAPI(t)
	client.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return clock }
	if err := client.UseProxy(closedProxy(t)); err != nil {
		t.Fatal(err)
	}
	_ = client.SendMessage(context.Background(), 1, "первое", nil) // прокси не ответил → напрямую на 5 мин

	proxyURL, hits := forwardProxy(t, false)
	if err := client.UseProxy(proxyURL); err != nil {
		t.Fatal(err)
	}
	_ = client.SendMessage(context.Background(), 1, "через минуту", nil)
	clock = clock.Add(client.directFor)
	_ = client.SendMessage(context.Background(), 1, "через 5 минут", nil)

	if hits.Load() != 1 || len(f.callsOf("sendMessage")) != 3 {
		t.Errorf("через прокси %d (want 1 — только после паузы), дошло %d", hits.Load(), len(f.callsOf("sendMessage")))
	}
}

func TestUseProxyValidates(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"socks5://127.0.0.1:10811", false},
		{"http://proxy.example:3128", false},
		{"127.0.0.1:10811", true},
		{"ftp://x:1", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if err := NewClient("https://api.telegram.org", testToken).UseProxy(tt.raw); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
