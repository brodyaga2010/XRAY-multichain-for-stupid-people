// Package monitor периодически проверяет VPN-цепочку и сервер, хранит историю и шлёт тревоги.
package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Status — итог проверки.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"    // работает, но плохо (медленно, мало места, скоро истекает сертификат)
	StatusFail    Status = "fail"    // не работает
	StatusUnknown Status = "unknown" // проверка неприменима или ещё не выполнялась
)

func (s Status) bad() bool { return s == StatusWarn || s == StatusFail }

// Result — результат одного запуска проверки.
type Result struct {
	Status Status
	Value  string   // короткое значение для списка: «0,42 с», «онлайн: 7»
	Detail string   // пояснение при проблеме
	Ms     *float64 // задержка, если проверка её измеряет (попадает в историю)
}

// Check — одна проверка.
type Check struct {
	ID      string
	Name    string
	Every   time.Duration // как часто запускать; 0 — на каждом цикле
	Timeout time.Duration
	History bool // хранить задержку в истории (для графика)
	Run     func(ctx context.Context) Result
}

// Notifier получает тревоги и сообщения о восстановлении (Telegram-бот).
type Notifier interface {
	Alert(ctx context.Context, text string)
}

// Point — точка истории задержки; Ms == nil — проверка не прошла.
type Point struct {
	At     time.Time
	Ms     *float64
	Status Status
}

// Incident — сбой, о котором отправлена тревога.
type Incident struct {
	CheckID string
	Name    string
	Status  Status
	Detail  string
	Start   time.Time
	End     *time.Time
}

// CheckState — текущее состояние проверки.
type CheckState struct {
	ID        string
	Name      string
	Result    Result
	CheckedAt time.Time
	Since     time.Time // с какого момента держится текущий статус
	// Alerted — сбой подтверждён: плохих результатов подряд набралось до порога и отправлена тревога.
	Alerted  bool
	bad      int // плохих результатов подряд
	badSince time.Time
	lastRun  time.Time
}

// Snapshot — состояние мониторинга для API и бота.
type Snapshot struct {
	Enabled   bool
	Interval  time.Duration
	CheckedAt time.Time
	Checks    []CheckState
	History   []Point
	Incidents []Incident
}

const (
	historyWindow = 24 * time.Hour
	maxIncidents  = 30
)

// Monitor выполняет проверки по расписанию.
type Monitor struct {
	checks    []Check
	interval  time.Duration
	threshold int
	notifier  Notifier
	log       *slog.Logger
	now       func() time.Time

	mu        sync.RWMutex
	states    map[string]*CheckState
	history   []Point
	incidents []Incident
	checkedAt time.Time
}

// New создаёт монитор; threshold — сколько плохих проверок подряд нужно для тревоги.
func New(checks []Check, interval time.Duration, threshold int) *Monitor {
	if threshold < 1 {
		threshold = 1
	}
	m := &Monitor{
		checks: checks, interval: interval, threshold: threshold,
		log: slog.Default(), now: time.Now, states: map[string]*CheckState{},
	}
	for _, c := range checks {
		m.states[c.ID] = &CheckState{ID: c.ID, Name: c.Name, Result: Result{Status: StatusUnknown}}
	}
	return m
}

// SetNotifier подключает отправку тревог.
func (m *Monitor) SetNotifier(n Notifier) {
	m.mu.Lock()
	m.notifier = n
	m.mu.Unlock()
}

// Run выполняет проверки до отмены ctx: сразу и затем каждые interval.
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		m.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce выполняет назревшие проверки параллельно и обновляет состояние.
func (m *Monitor) RunOnce(ctx context.Context) {
	now := m.now()
	type outcome struct {
		check  Check
		result Result
	}
	var (
		wg      sync.WaitGroup
		results = make(chan outcome, len(m.checks))
	)
	m.mu.RLock()
	for _, c := range m.checks {
		if st := m.states[c.ID]; c.Every > 0 && !st.lastRun.IsZero() && now.Sub(st.lastRun) < c.Every {
			continue
		}
		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			results <- outcome{c, runCheck(ctx, c)}
		}(c)
	}
	m.mu.RUnlock()
	wg.Wait()
	close(results)
	if ctx.Err() != nil {
		return // остановка сервиса: незавершённые проверки не считаем сбоем
	}

	var messages []string
	m.mu.Lock()
	for o := range results {
		if msg := m.apply(o.check, o.result, now); msg != "" {
			messages = append(messages, msg)
		}
	}
	m.checkedAt = now
	notifier := m.notifier
	m.mu.Unlock()

	for _, msg := range messages {
		m.log.Warn("мониторинг", "событие", msg)
		if notifier != nil {
			notifier.Alert(ctx, msg)
		}
	}
}

func runCheck(ctx context.Context, c Check) (result Result) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			result = Result{Status: StatusFail, Detail: fmt.Sprintf("ошибка проверки: %v", r)}
		}
	}()
	return c.Run(ctx)
}

// apply обновляет состояние проверки и возвращает текст тревоги/восстановления (или "").
// Вызывается под m.mu.
func (m *Monitor) apply(c Check, r Result, now time.Time) string {
	st := m.states[c.ID]
	if st.Result.Status != r.Status || st.Since.IsZero() {
		st.Since = now
	}
	st.Result, st.CheckedAt, st.lastRun = r, now, now

	if c.History {
		m.history = append(m.history, Point{At: now, Ms: r.Ms, Status: r.Status})
		cut := 0
		for cut < len(m.history) && now.Sub(m.history[cut].At) > historyWindow {
			cut++
		}
		m.history = m.history[cut:]
	}

	if r.Status.bad() {
		if st.bad == 0 {
			st.badSince = now
		}
		st.bad++
		if st.bad >= m.threshold && !st.Alerted {
			st.Alerted = true
			m.incidents = append(m.incidents, Incident{CheckID: c.ID, Name: c.Name, Status: r.Status, Detail: r.Detail, Start: st.badSince})
			if len(m.incidents) > maxIncidents {
				m.incidents = m.incidents[len(m.incidents)-maxIncidents:]
			}
			return alertText(c.Name, r)
		}
		return ""
	}
	if r.Status == StatusUnknown {
		return "" // проверку не удалось выполнить по существу — счётчики и открытый сбой не трогаем
	}

	wasAlerted, since := st.Alerted, st.badSince
	st.bad, st.Alerted = 0, false
	if !wasAlerted || r.Status != StatusOK {
		return ""
	}
	for i := len(m.incidents) - 1; i >= 0; i-- {
		if m.incidents[i].CheckID == c.ID && m.incidents[i].End == nil {
			end := now
			m.incidents[i].End = &end
			break
		}
	}
	text := fmt.Sprintf("🟢 %s — снова в норме (сбой длился %s)", c.Name, FormatDuration(now.Sub(since)))
	if r.Value != "" {
		text += "\n" + r.Value
	}
	return text
}

func alertText(name string, r Result) string {
	icon := "🔴"
	if r.Status == StatusWarn {
		icon = "🟡"
	}
	text := fmt.Sprintf("%s %s", icon, name)
	if r.Detail != "" {
		text += ": " + r.Detail
	}
	if r.Value != "" {
		text += "\n" + r.Value
	}
	return text
}

// FormatDuration — «45 с», «12 мин», «2 ч 5 мин».
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d с", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d мин", int(d.Minutes()))
	default:
		h := int(d.Hours())
		if mins := int(d.Minutes()) % 60; mins > 0 {
			return fmt.Sprintf("%d ч %d мин", h, mins)
		}
		return fmt.Sprintf("%d ч", h)
	}
}

// Snapshot возвращает копию текущего состояния.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snap := Snapshot{Enabled: true, Interval: m.interval, CheckedAt: m.checkedAt}
	for _, c := range m.checks {
		snap.Checks = append(snap.Checks, *m.states[c.ID])
	}
	snap.History = append([]Point(nil), m.history...)
	snap.Incidents = append([]Incident(nil), m.incidents...)
	return snap
}
