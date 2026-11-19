package telegram

import (
	"context"
	"fmt"
	"html"
	"strings"

	"xraypanel/internal/monitor"
)

// StatusSource — откуда бот берёт состояние мониторинга.
type StatusSource interface {
	Snapshot() monitor.Snapshot
}

// SetStatusSource подключает кнопку «Состояние»; nil — мониторинг выключен.
func (b *Bot) SetStatusSource(s StatusSource) { b.status = s }

// Alert отправляет тревогу мониторинга всем администраторам (реализует monitor.Notifier).
func (b *Bot) Alert(ctx context.Context, text string) {
	for _, chat := range b.chats {
		if err := b.api.SendMessage(ctx, chat, html.EscapeString(text), nil); err != nil {
			b.log.Warn("telegram: тревога не отправлена", "chat_id", chat, "error", err)
		}
	}
}

var statusIcons = map[monitor.Status]string{
	monitor.StatusOK: "🟢", monitor.StatusWarn: "🟡", monitor.StatusFail: "🔴", monitor.StatusUnknown: "⚪",
}

const maxIncidentsInStatus = 5

func (b *Bot) statusText() string {
	if b.status == nil {
		return "Мониторинг выключен (MONITOR_INTERVAL_SECONDS=0)."
	}
	snap := b.status.Snapshot()
	var sb strings.Builder
	sb.WriteString("📊 <b>Состояние VPN</b>")
	if snap.CheckedAt.IsZero() {
		sb.WriteString("\n\nПервая проверка ещё идёт — нажмите через минуту.")
		return sb.String()
	}
	fmt.Fprintf(&sb, " · проверено в %s\n", snap.CheckedAt.In(b.Location).Format("15:04"))
	for _, c := range snap.Checks {
		fmt.Fprintf(&sb, "\n%s %s", statusIcons[c.Result.Status], html.EscapeString(c.Name))
		if c.Result.Value != "" {
			fmt.Fprintf(&sb, " — %s", html.EscapeString(c.Result.Value))
		}
		if c.Result.Detail != "" && c.Result.Status != monitor.StatusOK {
			fmt.Fprintf(&sb, "\n    <i>%s</i>", html.EscapeString(c.Result.Detail))
		}
	}
	if len(snap.Incidents) > 0 {
		sb.WriteString("\n\n<b>Последние сбои</b>")
		for i := len(snap.Incidents) - 1; i >= 0 && i >= len(snap.Incidents)-maxIncidentsInStatus; i-- {
			inc := snap.Incidents[i]
			start := inc.Start.In(b.Location)
			period := start.Format("02.01 15:04") + " — идёт сейчас"
			if inc.End != nil {
				period = fmt.Sprintf("%s–%s (%s)", start.Format("02.01 15:04"), inc.End.In(b.Location).Format("15:04"),
					monitor.FormatDuration(inc.End.Sub(inc.Start)))
			}
			fmt.Fprintf(&sb, "\n• %s %s: %s", statusIcons[inc.Status], period, html.EscapeString(inc.Name))
		}
	}
	return sb.String()
}
