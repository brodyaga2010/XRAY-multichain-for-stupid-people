package monitor

import "time"

// Public — состояние VPN для пользователей: итог и задержка, без подробностей о серверах.
type Public struct {
	Status    Status
	Ms        *float64 // задержка последнего запроса через цепочку
	CheckedAt time.Time
}

// userFacing — проверки, от которых зависит, работает ли VPN у пользователей
// (сертификаты, диск и память — забота администратора).
var userFacing = map[string]bool{CheckChain: true, CheckExit: true, CheckXray: true}

var severity = map[Status]int{StatusOK: 0, StatusWarn: 1, StatusFail: 2}

// Public сводит снимок к статусу для пользователей. Сбой показывается только подтверждённый
// (тот же порог, что и для тревог), чтобы разовая неудачная проверка не пугала пользователей.
func (s Snapshot) Public() Public {
	p := Public{Status: StatusUnknown, CheckedAt: s.CheckedAt}
	if !s.Enabled || s.CheckedAt.IsZero() {
		return p
	}
	for _, c := range s.Checks {
		if !userFacing[c.ID] {
			continue
		}
		status := StatusUnknown
		switch {
		case c.Alerted && c.Result.Status.bad():
			status = c.Result.Status
		case c.Alerted: // сбой не закрыт, но проверить сейчас не удалось
			status = StatusWarn
		case c.Result.Status != StatusUnknown:
			status = StatusOK
		}
		if status == StatusUnknown {
			continue
		}
		if p.Status == StatusUnknown || severity[status] > severity[p.Status] {
			p.Status = status
		}
		if c.ID == CheckChain && c.Result.Ms != nil && c.Result.Status != StatusFail {
			p.Ms = c.Result.Ms
		}
	}
	return p
}
