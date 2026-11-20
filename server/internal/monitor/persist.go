package monitor

import (
	"context"
	"time"
)

// Persister сохраняет историю и сбои, чтобы они переживали перезапуск кабинета (реализация — над store).
type Persister interface {
	SavePoint(ctx context.Context, p Point) error
	// SaveIncident записывает новый сбой или обновляет конец существующего (ключ — CheckID + Start).
	SaveIncident(ctx context.Context, inc Incident) error
	LoadPoints(ctx context.Context, since time.Time) ([]Point, error)
	// LoadIncidents — последние limit сбоев, старые первыми.
	LoadIncidents(ctx context.Context, limit int) ([]Incident, error)
	PrunePoints(ctx context.Context, before time.Time) error
}

// keepPoints — сколько хранить точки в базе (на графике — последние сутки).
const keepPoints = 48 * time.Hour

// SetPersister подключает хранение истории; вызывать до Run.
func (m *Monitor) SetPersister(p Persister) {
	m.mu.Lock()
	m.persist = p
	m.mu.Unlock()
}

// Restore загружает историю и сбои из базы. Незакрытый сбой снова считается подтверждённым:
// когда проверка вернётся в норму, придёт сообщение о восстановлении и сбой закроется.
func (m *Monitor) Restore(ctx context.Context) error {
	m.mu.RLock()
	p := m.persist
	m.mu.RUnlock()
	if p == nil {
		return nil
	}
	now := m.now()
	points, err := p.LoadPoints(ctx, now.Add(-historyWindow))
	if err != nil {
		return err
	}
	incidents, err := p.LoadIncidents(ctx, maxIncidents)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = points
	m.incidents = incidents
	for _, inc := range incidents {
		st, ok := m.states[inc.CheckID]
		if !ok || inc.End != nil {
			continue
		}
		st.Alerted, st.bad, st.badSince = true, m.threshold, inc.Start
	}
	return nil
}

// flush пишет накопленные изменения в базу (вне m.mu): ошибки только в лог — мониторинг важнее истории.
func (m *Monitor) flush(ctx context.Context, p Persister, points []Point, incidents []Incident, now time.Time) {
	for _, pt := range points {
		if err := p.SavePoint(ctx, pt); err != nil {
			m.log.Warn("мониторинг: точка истории не сохранена", "error", err)
		}
	}
	for _, inc := range incidents {
		if err := p.SaveIncident(ctx, inc); err != nil {
			m.log.Warn("мониторинг: сбой не сохранён", "error", err)
		}
	}
	if len(points) > 0 {
		if err := p.PrunePoints(ctx, now.Add(-keepPoints)); err != nil {
			m.log.Warn("мониторинг: старая история не удалена", "error", err)
		}
	}
}
