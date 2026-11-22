package telegram

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"xraypanel/internal/monitor"
	"xraypanel/internal/store"
)

type fakeStatus struct{ snap monitor.Snapshot }

func (f fakeStatus) Snapshot() monitor.Snapshot { return f.snap }

func TestAlertSendsEscapedTextToAllAdmins(t *testing.T) {
	f, b := newTestBot(t, &fakeModerator{})
	b.chats = []int64{adminChat, 555}

	b.Alert(context.Background(), "🔴 Цепочка: запрос <не прошёл> & всё")

	calls := f.callsOf("sendMessage")
	if len(calls) != 2 {
		t.Fatalf("sendMessage = %d", len(calls))
	}
	for i, chat := range []int64{adminChat, 555} {
		p := calls[i].Params
		if p["chat_id"] != float64(chat) || p["text"] != "🔴 Цепочка: запрос &lt;не прошёл&gt; &amp; всё" {
			t.Errorf("сообщение %d: %v", i, p)
		}
	}
}

func TestStatusText(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC)
	ended := at.Add(-time.Hour)
	full := monitor.Snapshot{
		Enabled: true, CheckedAt: at,
		Checks: []monitor.CheckState{
			{ID: "chain", Name: "Цепочка", Result: monitor.Result{Status: monitor.StatusOK, Value: "0,41 с", Detail: "скрыто"}},
			{ID: "exit", Name: "Канал", Result: monitor.Result{Status: monitor.StatusWarn, Value: "440 мс", Detail: "потери <25%>"}},
			{ID: "xray", Name: "Xray", Result: monitor.Result{Status: monitor.StatusFail, Detail: "не отвечает"}},
			{ID: "certs", Name: "Сертификаты", Result: monitor.Result{Status: monitor.StatusUnknown}},
		},
		Incidents: []monitor.Incident{
			{CheckID: "exit", Name: "Канал", Status: monitor.StatusWarn, Start: ended.Add(-20 * time.Minute), End: &ended},
			{CheckID: "xray", Name: "Xray", Status: monitor.StatusFail, Start: at.Add(-3 * time.Minute)},
		},
	}
	tests := []struct {
		name    string
		src     StatusSource
		want    []string
		notWant []string
	}{
		{"мониторинг выключен", nil, []string{"Мониторинг выключен"}, nil},
		{"первая проверка не завершена", fakeStatus{monitor.Snapshot{Enabled: true}}, []string{"Первая проверка"}, nil},
		{"полная сводка", fakeStatus{full}, []string{
			"проверено в 12:30",
			"🟢 Цепочка — 0,41 с",
			"🟡 Канал — 440 мс\n    <i>потери &lt;25%&gt;</i>",
			"🔴 Xray\n    <i>не отвечает</i>",
			"⚪ Сертификаты",
			"Последние сбои",
			"• 🔴 19.11 12:27 — идёт сейчас: Xray\n• 🟡 19.11 11:10–11:30 (20 мин): Канал",
		}, []string{"скрыто"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, b := newTestBot(t, &fakeModerator{})
			b.SetStatusSource(tt.src)

			got := b.statusText()

			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("нет %q в:\n%s", w, got)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("лишнее %q в:\n%s", w, got)
				}
			}
		})
	}
}

func TestStatusButtonSendsSummary(t *testing.T) {
	for _, text := range []string{StatusButton, "/status"} {
		f, b := newTestBot(t, &fakeModerator{})
		b.SetStatusSource(fakeStatus{monitor.Snapshot{Enabled: true}})

		b.Handle(context.Background(), Update{Message: &Message{Chat: Chat{ID: adminChat}, Text: text}})

		calls := f.callsOf("sendMessage")
		if len(calls) != 1 || !strings.Contains(calls[0].Params["text"].(string), "Состояние VPN") {
			t.Errorf("%s: %v", text, calls)
		}
	}
}

func TestAlertChats(t *testing.T) {
	const group = int64(-4000000001)
	tests := []struct {
		name  string
		alert func(b *Bot)
		want  []int64
	}{
		{"тревога о работе системы — админу и в группу", func(b *Bot) {
			b.Alert(context.Background(), "🔴 Цепочка")
		}, []int64{adminChat, group}},
		{"с данными пользователя (подписка) — только админу", func(b *Bot) {
			b.AdminAlerts().Alert(context.Background(), "⏰ Закончилась подписка: user@example.com")
		}, []int64{adminChat}},
		{"заявка — только админу", func(b *Bot) {
			b.UserRegistered(context.Background(), store.User{ID: 1, Email: "new@example.com"}, "")
		}, []int64{adminChat}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, b := newTestBot(t, &fakeModerator{})
			b.SetAlertChats([]int64{group, adminChat, group}) // админ и повтор не дублируются

			tt.alert(b)

			var got []int64
			for _, c := range f.callsOf("sendMessage") {
				got = append(got, int64(c.Params["chat_id"].(float64)))
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("отправлено в %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertChatIsReadOnly(t *testing.T) {
	const group = int64(-4000000001)
	tests := []struct {
		name   string
		update Update
	}{
		{"команда", Update{Message: &Message{Chat: Chat{ID: group}, Text: "/status@panel_bot"}}},
		{"текст кнопки", Update{Message: &Message{Chat: Chat{ID: group}, Text: PendingButton}}},
		{"/start", Update{Message: &Message{Chat: Chat{ID: group}, Text: "/start"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, b := newTestBot(t, &fakeModerator{pending: []store.User{{ID: 3, Email: "one@example.com"}}})
			b.SetAlertChats([]int64{group})

			b.Handle(context.Background(), tt.update)

			if calls := f.callsOf("sendMessage"); len(calls) != 0 {
				t.Errorf("бот ответил в группе тревог: %v", calls[0].Params)
			}
		})
	}
}
