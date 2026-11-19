package monitor

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *recorder) Alert(_ context.Context, text string) {
	r.mu.Lock()
	r.msgs = append(r.msgs, text)
	r.mu.Unlock()
}

// scripted — проверка, возвращающая статусы по очереди.
func scripted(id string, statuses ...Status) Check {
	i := 0
	return Check{ID: id, Name: "Проверка " + id, Run: func(context.Context) Result {
		s := statuses[i]
		i++
		r := Result{Status: s, Value: "значение"}
		if s.bad() {
			r.Detail = "плохо"
		}
		return r
	}}
}

func newTestMonitor(threshold int, checks ...Check) (*Monitor, *recorder, *time.Time) {
	m := New(checks, time.Minute, threshold)
	m.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }
	rec := &recorder{}
	m.SetNotifier(rec)
	return m, rec, &clock
}

func TestAlertsAfterThresholdAndRecovery(t *testing.T) {
	const ok, warn, fail, unk = StatusOK, StatusWarn, StatusFail, StatusUnknown
	tests := []struct {
		name          string
		statuses      []Status
		want          []string // префиксы сообщений по порядку
		openIncidents int
		allIncidents  int
	}{
		{"всё хорошо — тишина", []Status{ok, ok, ok, ok}, nil, 0, 0},
		{"разовый сбой ниже порога — тишина", []Status{ok, fail, fail, ok, ok}, nil, 0, 0},
		{"три сбоя подряд — тревога, затем восстановление", []Status{fail, fail, fail, fail, ok},
			[]string{"🔴 Проверка a: плохо", "🟢 Проверка a — снова в норме (сбой длился 4 мин)"}, 0, 1},
		{"деградация — жёлтая тревога", []Status{warn, warn, warn}, []string{"🟡 Проверка a: плохо"}, 1, 1},
		{"warn и fail вперемешку считаются вместе, тревога одна", []Status{warn, fail, warn, fail, fail},
			[]string{"🟡 Проверка a: плохо"}, 1, 1},
		{"unknown не сбрасывает счётчик", []Status{fail, unk, fail, fail}, []string{"🔴 Проверка a: плохо"}, 1, 1},
		{"unknown не закрывает сбой", []Status{fail, fail, fail, unk, unk}, []string{"🔴 Проверка a: плохо"}, 1, 1},
		{"два отдельных сбоя", []Status{fail, fail, fail, ok, fail, fail, fail},
			[]string{"🔴", "🟢", "🔴"}, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, rec, clock := newTestMonitor(3, scripted("a", tt.statuses...))

			for range tt.statuses {
				m.RunOnce(context.Background())
				*clock = clock.Add(time.Minute)
			}

			if len(rec.msgs) != len(tt.want) {
				t.Fatalf("сообщения = %q, want %q", rec.msgs, tt.want)
			}
			for i, prefix := range tt.want {
				if !strings.HasPrefix(rec.msgs[i], prefix) {
					t.Errorf("сообщение %d = %q, want prefix %q", i, rec.msgs[i], prefix)
				}
			}
			snap := m.Snapshot()
			open := 0
			for _, inc := range snap.Incidents {
				if inc.End == nil {
					open++
				}
			}
			if open != tt.openIncidents || len(snap.Incidents) != tt.allIncidents {
				t.Errorf("сбои: открытых %d из %d, want %d из %d", open, len(snap.Incidents), tt.openIncidents, tt.allIncidents)
			}
		})
	}
}

func TestRunOnceRespectsEveryAndKeepsHistory(t *testing.T) {
	calls := map[string]int{}
	var mu sync.Mutex
	counted := func(id string, every time.Duration, history bool) Check {
		return Check{ID: id, Name: id, Every: every, History: history, Run: func(context.Context) Result {
			mu.Lock()
			calls[id]++
			mu.Unlock()
			v := 42.0
			return Result{Status: StatusOK, Ms: &v}
		}}
	}
	m, _, clock := newTestMonitor(3, counted("fast", 0, true), counted("hourly", time.Hour, false))

	for i := 0; i < 61; i++ { // 61 цикл раз в минуту = ровно час
		m.RunOnce(context.Background())
		*clock = clock.Add(time.Minute)
	}

	if calls["fast"] != 61 || calls["hourly"] != 2 {
		t.Errorf("запусков: %v, want fast=61 hourly=2", calls)
	}
	snap := m.Snapshot()
	if len(snap.History) != 61 || *snap.History[0].Ms != 42 {
		t.Errorf("история: %d точек", len(snap.History))
	}
	if snap.Checks[1].Result.Status != StatusOK || snap.CheckedAt.IsZero() {
		t.Errorf("снимок: %+v", snap.Checks)
	}
}

func TestHistoryKeepsOnlyLastDay(t *testing.T) {
	m, _, clock := newTestMonitor(3, Check{ID: "c", History: true, Run: func(context.Context) Result {
		return Result{Status: StatusOK}
	}})
	for i := 0; i < 30; i++ {
		m.RunOnce(context.Background())
		*clock = clock.Add(time.Hour)
	}
	if got := len(m.Snapshot().History); got != 25 { // точки за последние 24 ч включительно
		t.Errorf("точек = %d", got)
	}
}

func TestPanicInCheckIsFailure(t *testing.T) {
	m, _, _ := newTestMonitor(3, Check{ID: "p", Run: func(context.Context) Result { panic("бум") }})
	m.RunOnce(context.Background())
	if r := m.Snapshot().Checks[0].Result; r.Status != StatusFail || !strings.Contains(r.Detail, "бум") {
		t.Errorf("result = %+v", r)
	}
}

func TestCancelledRunIsNotCounted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m, rec, _ := newTestMonitor(1, Check{ID: "c", Run: func(ctx context.Context) Result {
		cancel()
		return Result{Status: StatusFail}
	}})
	m.RunOnce(ctx)
	if len(rec.msgs) != 0 || m.Snapshot().Checks[0].Result.Status != StatusUnknown {
		t.Error("при остановке сервиса результаты не учитываются")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[time.Duration]string{
		45 * time.Second:                "45 с",
		12 * time.Minute:                "12 мин",
		2*time.Hour + 5*time.Minute:     "2 ч 5 мин",
		3 * time.Hour:                   "3 ч",
		59*time.Minute + 59*time.Second: "59 мин",
	}
	for d, want := range tests {
		if got := FormatDuration(d); got != want {
			t.Errorf("%v → %q, want %q", d, got, want)
		}
	}
}
