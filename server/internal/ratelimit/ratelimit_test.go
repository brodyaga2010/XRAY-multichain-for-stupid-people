package ratelimit

import (
	"net/http"
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	l := New(2, time.Minute)
	base := time.Now()
	tests := []struct {
		name string
		key  string
		at   time.Duration
		want bool
	}{
		{"первая", "a", 0, true},
		{"вторая", "a", 10 * time.Second, true},
		{"третья в окне", "a", 20 * time.Second, false},
		{"другой ключ", "b", 20 * time.Second, true},
		{"после окна первой попытки", "a", 61 * time.Second, true},
		{"снова лимит", "a", 62 * time.Second, false},
	}
	for _, tt := range tests {
		if got := l.HitAt(tt.key, base.Add(tt.at)); got != tt.want {
			t.Errorf("%s: %v, ожидалось %v", tt.name, got, tt.want)
		}
	}
	l.Reset()
	if !l.HitAt("a", base.Add(63*time.Second)) {
		t.Error("после Reset счётчики должны очищаться")
	}
}

func TestLimiterSweepsStaleKeys(t *testing.T) {
	l := New(1, time.Second)
	base := time.Now()
	for i := 0; i < sweepThreshold+10; i++ {
		l.HitAt(string(rune('a'))+time.Duration(i).String(), base)
	}

	l.HitAt("fresh", base.Add(time.Hour))

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 2 {
		t.Errorf("устаревшие ключи не вычищены: %d", len(l.hits))
	}
}

func TestClientIP(t *testing.T) {
	trusted := map[string]struct{}{"127.0.0.1": {}, "::1": {}}
	tests := []struct {
		name, remote, forwarded, want string
	}{
		{"прямое подключение", "203.0.113.5:4000", "", "203.0.113.5"},
		{"заголовок от доверенного", "127.0.0.1:4000", "198.51.100.7, 10.0.0.1", "198.51.100.7"},
		{"IPv6 доверенный", "[::1]:4000", " 198.51.100.8 ", "198.51.100.8"},
		{"заголовок от недоверенного игнорируется", "203.0.113.5:4000", "198.51.100.7", "203.0.113.5"},
		{"доверенный без заголовка", "127.0.0.1:4000", "", "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.remote, Header: http.Header{}}
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := ClientIP(r, trusted); got != tt.want {
				t.Errorf("ClientIP = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}
