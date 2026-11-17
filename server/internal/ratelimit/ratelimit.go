// Package ratelimit — ограничение частоты запросов в памяти процесса.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter — скользящее окно: не более limit попыток за window на ключ.
type Limiter struct {
	limit  int
	window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
}

// New создаёт ограничитель.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, hits: map[string][]time.Time{}}
}

const sweepThreshold = 4096

// Hit регистрирует попытку. false — лимит превышен.
func (l *Limiter) Hit(key string) bool { return l.HitAt(key, time.Now()) }

// HitAt — Hit с явным временем (для тестов).
func (l *Limiter) HitAt(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > sweepThreshold {
		l.sweep(now)
	}
	hits := l.prune(l.hits[key], now)
	if len(hits) >= l.limit {
		l.hits[key] = hits
		return false
	}
	l.hits[key] = append(hits, now)
	return true
}

func (l *Limiter) prune(hits []time.Time, now time.Time) []time.Time {
	i := 0
	for i < len(hits) && now.Sub(hits[i]) > l.window {
		i++
	}
	return hits[i:]
}

func (l *Limiter) sweep(now time.Time) {
	for key, hits := range l.hits {
		if hits = l.prune(hits, now); len(hits) == 0 {
			delete(l.hits, key)
		} else {
			l.hits[key] = hits
		}
	}
}

// Reset очищает счётчики.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = map[string][]time.Time{}
}

// ClientIP — адрес клиента; X-Forwarded-For учитывается только от доверенных прокси.
func ClientIP(r *http.Request, trusted map[string]struct{}) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if peer == "" {
		peer = "unknown"
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if _, ok := trusted[peer]; ok {
			first, _, _ := strings.Cut(forwarded, ",")
			return strings.TrimSpace(first)
		}
	}
	return peer
}
