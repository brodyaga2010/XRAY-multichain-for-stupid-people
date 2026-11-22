package monitor

import (
	"context"
	"errors"
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

// memPersister — хранение в памяти для тестов.
type memPersister struct {
	points    []Point
	incidents []Incident
	pruned    time.Time
}

func (p *memPersister) SavePoint(_ context.Context, pt Point) error {
	p.points = append(p.points, pt)
	return nil
}

func (p *memPersister) SaveIncident(_ context.Context, inc Incident) error {
	for i := range p.incidents {
		if p.incidents[i].CheckID == inc.CheckID && p.incidents[i].Start.Equal(inc.Start) {
			p.incidents[i] = inc
			return nil
		}
	}
	p.incidents = append(p.incidents, inc)
	return nil
}

func (p *memPersister) LoadPoints(_ context.Context, since time.Time) ([]Point, error) {
	var out []Point
	for _, pt := range p.points {
		if !pt.At.Before(since) {
			out = append(out, pt)
		}
	}
	return out, nil
}

func (p *memPersister) LoadIncidents(_ context.Context, limit int) ([]Incident, error) {
	if len(p.incidents) > limit {
		return p.incidents[len(p.incidents)-limit:], nil
	}
	return p.incidents, nil
}

func (p *memPersister) PrunePoints(_ context.Context, before time.Time) error {
	p.pruned = before
	return nil
}

func TestPersistAndRestore(t *testing.T) {
	const ok, fail = StatusOK, StatusFail
	tests := []struct {
		name         string
		statuses     []Status
		wantPoints   int
		wantOpen     int
		wantClosed   int
		wantRecovery bool // после перезапуска первое «ok» присылает сообщение о восстановлении
	}{
		{"без сбоев", []Status{ok, ok}, 2, 0, 0, false},
		{"сбой закрыт до перезапуска", []Status{fail, fail, ok}, 3, 0, 1, false},
		{"сбой открыт на момент перезапуска", []Status{fail, fail, fail}, 3, 1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &memPersister{}
			check := scripted("a", tt.statuses...)
			check.History = true
			m, _, clock := newTestMonitor(2, check)
			m.SetPersister(store)
			for range tt.statuses {
				m.RunOnce(context.Background())
				*clock = clock.Add(time.Minute)
			}

			open, closed := 0, 0
			for _, inc := range store.incidents {
				if inc.End == nil {
					open++
				} else {
					closed++
				}
			}
			if len(store.points) != tt.wantPoints || open != tt.wantOpen || closed != tt.wantClosed {
				t.Fatalf("в базе: точек %d, открытых %d, закрытых %d", len(store.points), open, closed)
			}
			if !store.pruned.Equal(clock.Add(-time.Minute - keepPoints)) {
				t.Errorf("очистка до %v", store.pruned)
			}

			// перезапуск: новый монитор поднимает историю из базы
			m2, rec2, clock2 := newTestMonitor(2, scripted("a", ok))
			*clock2 = *clock
			m2.SetPersister(store)
			if err := m2.Restore(context.Background()); err != nil {
				t.Fatal(err)
			}
			snap := m2.Snapshot()
			if len(snap.History) != tt.wantPoints || len(snap.Incidents) != tt.wantOpen+tt.wantClosed {
				t.Fatalf("после перезапуска: точек %d, сбоев %d", len(snap.History), len(snap.Incidents))
			}
			m2.RunOnce(context.Background())
			gotRecovery := len(rec2.msgs) == 1 && strings.HasPrefix(rec2.msgs[0], "🟢")
			if gotRecovery != tt.wantRecovery || (!tt.wantRecovery && len(rec2.msgs) != 0) {
				t.Errorf("сообщения после перезапуска: %q", rec2.msgs)
			}
		})
	}
}

func TestRestoreWithoutPersister(t *testing.T) {
	m, _, _ := newTestMonitor(1)
	if err := m.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecovery(t *testing.T) {
	const ok, warn, fail = StatusOK, StatusWarn, StatusFail
	tests := []struct {
		name      string
		statuses  []Status // по одному на минуту
		cooldown  time.Duration
		actionErr error
		wantRuns  int
		want      []string // префиксы сообщений по порядку
	}{
		{"сбой подтверждён — перезапуск сразу после тревоги", []Status{fail, fail, fail, ok}, 30 * time.Minute, nil, 1,
			[]string{"🔴", "🔄 перезапущен", "🟢"}},
		{"ниже порога — не трогаем", []Status{fail, fail, ok}, 30 * time.Minute, nil, 0, nil},
		{"медленно (warn) — не перезапускаем", []Status{warn, warn, warn, warn}, 30 * time.Minute, nil, 0, []string{"🟡"}},
		{"долгий сбой — не чаще раза за cooldown", []Status{fail, fail, fail, fail, fail, fail, fail}, 3 * time.Minute, nil, 2,
			[]string{"🔴", "🔄", "🔄"}},
		{"ошибка перезапуска — сообщаем", []Status{fail, fail, fail}, 30 * time.Minute, errors.New("нет прав"), 1,
			[]string{"🔴", "⚠️ не вышло: нет прав"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, rec, clock := newTestMonitor(3, scripted("a", tt.statuses...))
			runs := 0
			m.SetRecovery(&Recovery{CheckID: "a", Cooldown: tt.cooldown,
				Action: func(context.Context) error { runs++; return tt.actionErr },
				Done:   "🔄 перезапущен", Failed: "⚠️ не вышло: %v"})

			for range tt.statuses {
				m.RunOnce(context.Background())
				*clock = clock.Add(time.Minute)
			}

			if runs != tt.wantRuns || len(rec.msgs) != len(tt.want) {
				t.Fatalf("перезапусков %d (want %d), сообщения = %q, want %q", runs, tt.wantRuns, rec.msgs, tt.want)
			}
			for i, prefix := range tt.want {
				if !strings.HasPrefix(rec.msgs[i], prefix) {
					t.Errorf("сообщение %d = %q, want prefix %q", i, rec.msgs[i], prefix)
				}
			}
		})
	}
}
