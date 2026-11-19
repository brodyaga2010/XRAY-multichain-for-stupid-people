package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"xraypanel/internal/store"
)

const testToken = "123456:SECRET-TOKEN"

type call struct {
	Method string
	Params map[string]any
}

// fakeAPI имитирует Bot API: пишет вызовы и отвечает заданными результатами.
type fakeAPI struct {
	mu      sync.Mutex
	calls   []call
	replies map[string]string // метод → тело ответа целиком
	updates chan string       // результаты getUpdates по очереди
}

func newFakeAPI(t *testing.T) (*fakeAPI, *Client) {
	f := &fakeAPI{replies: map[string]string{}, updates: make(chan string, 10)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ok := strings.CutPrefix(r.URL.Path, "/bot"+testToken+"/")
		if !ok {
			http.Error(w, `{"ok":false,"error_code":404,"description":"Not Found"}`, 404)
			return
		}
		var params map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &params)
		f.mu.Lock()
		f.calls = append(f.calls, call{method, params})
		reply, custom := f.replies[method]
		f.mu.Unlock()
		switch {
		case custom:
			_, _ = io.WriteString(w, reply)
		case method == "getUpdates":
			select {
			case u := <-f.updates:
				_, _ = io.WriteString(w, `{"ok":true,"result":`+u+`}`)
			case <-r.Context().Done():
			case <-time.After(200 * time.Millisecond):
				_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
			}
		case method == "getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"username":"panel_bot"}}`)
		default:
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		}
	}))
	t.Cleanup(srv.Close)
	return f, NewClient(srv.URL, testToken)
}

func (f *fakeAPI) callsOf(method string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

type fakeModerator struct {
	pending []store.User
	result  *store.User
	changed bool
	err     error
	calls   []string
}

func (m *fakeModerator) PendingUsers(context.Context) ([]store.User, error) { return m.pending, m.err }

func (m *fakeModerator) ModerateUser(_ context.Context, id int64, approve bool, via string) (*store.User, bool, error) {
	m.calls = append(m.calls, map[bool]string{true: "approve", false: "reject"}[approve]+":"+via)
	_ = id
	return m.result, m.changed, m.err
}

const adminChat = int64(432)

func newTestBot(t *testing.T, mod Moderator) (*fakeAPI, *Bot) {
	t.Helper()
	f, client := newFakeAPI(t)
	b := New(client, []int64{adminChat}, mod)
	b.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	b.Location = time.UTC
	b.PollTimeout = 0
	b.RetryDelay = 10 * time.Millisecond
	return f, b
}

func TestUserRegisteredSendsRequestWithButtons(t *testing.T) {
	f, b := newTestBot(t, &fakeModerator{})
	user := store.User{ID: 7, Email: "a<b>@example.com", CreatedAt: time.Date(2026, 11, 18, 9, 5, 0, 0, time.UTC)}

	b.UserRegistered(context.Background(), user, "203.0.113.5")

	sent := f.callsOf("sendMessage")
	if len(sent) != 1 {
		t.Fatalf("sendMessage = %d", len(sent))
	}
	p := sent[0].Params
	text := p["text"].(string)
	for _, want := range []string{"a&lt;b&gt;@example.com", "18.11.2026 09:05 UTC", "203.0.113.5"} {
		if !strings.Contains(text, want) {
			t.Errorf("в тексте нет %q: %s", want, text)
		}
	}
	if p["chat_id"] != float64(adminChat) || p["parse_mode"] != "HTML" {
		t.Errorf("params = %v", p)
	}
	kb, _ := json.Marshal(p["reply_markup"])
	if !strings.Contains(string(kb), `"callback_data":"approve:7"`) || !strings.Contains(string(kb), `"callback_data":"reject:7"`) {
		t.Errorf("кнопки = %s", kb)
	}
}

func callbackUpdate(chat int64, data string) Update {
	return Update{CallbackQuery: &CallbackQuery{
		ID: "cb1", From: User{ID: chat}, Data: data,
		Message: &Message{MessageID: 55, Chat: Chat{ID: chat}, Text: "🆕 Заявка\n\nuser@example.com"},
	}}
}

func TestHandleCallback(t *testing.T) {
	approved := &store.User{ID: 7, Email: "user@example.com", Status: store.UserApproved}
	suspended := &store.User{ID: 7, Email: "user@example.com", Status: store.UserSuspended}
	tests := []struct {
		name      string
		update    Update
		mod       *fakeModerator
		wantCalls []string
		wantEdit  string // подстрока нового текста; "" — сообщение не меняется
		wantToast string
	}{
		{"подтвердить", callbackUpdate(adminChat, "approve:7"), &fakeModerator{result: approved, changed: true},
			[]string{"approve:telegram"}, "✅ Подтверждён", "Подтверждён"},
		{"отклонить", callbackUpdate(adminChat, "reject:7"), &fakeModerator{result: suspended, changed: true},
			[]string{"reject:telegram"}, "⛔ Отклонён", "Отклонён"},
		{"уже решено в кабинете", callbackUpdate(adminChat, "reject:7"), &fakeModerator{result: approved},
			[]string{"reject:telegram"}, "Уже решено в кабинете: подтверждён", "Уже решено: подтверждён"},
		{"пользователь удалён", callbackUpdate(adminChat, "approve:7"), &fakeModerator{err: store.ErrNotFound},
			[]string{"approve:telegram"}, "Пользователь не найден", "Пользователь не найден"},
		{"ошибка — кнопки остаются", callbackUpdate(adminChat, "approve:7"), &fakeModerator{err: errors.New("xray")},
			[]string{"approve:telegram"}, "", "Ошибка, попробуйте ещё раз"},
		{"чужой чат", callbackUpdate(999, "approve:7"), &fakeModerator{}, nil, "", "Нет доступа"},
		{"мусор в данных", callbackUpdate(adminChat, "delete:x"), &fakeModerator{}, nil, "", "Неизвестная кнопка"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, b := newTestBot(t, tt.mod)

			b.Handle(context.Background(), tt.update)

			if strings.Join(tt.mod.calls, ",") != strings.Join(tt.wantCalls, ",") {
				t.Errorf("модерация = %v, want %v", tt.mod.calls, tt.wantCalls)
			}
			edits := f.callsOf("editMessageText")
			switch {
			case tt.wantEdit == "" && len(edits) != 0:
				t.Errorf("сообщение не должно меняться: %v", edits)
			case tt.wantEdit != "":
				if len(edits) != 1 {
					t.Fatalf("editMessageText = %d", len(edits))
				}
				text := edits[0].Params["text"].(string)
				if !strings.Contains(text, tt.wantEdit) || !strings.Contains(text, "user@example.com") {
					t.Errorf("текст = %q", text)
				}
				if _, ok := edits[0].Params["reply_markup"]; ok {
					t.Error("после решения кнопки должны исчезнуть")
				}
			}
			answers := f.callsOf("answerCallbackQuery")
			if len(answers) != 1 || answers[0].Params["text"] != tt.wantToast {
				t.Errorf("answerCallbackQuery = %v", answers)
			}
		})
	}
}

func TestHandleMessage(t *testing.T) {
	pending := []store.User{{ID: 3, Email: "one@example.com"}, {ID: 4, Email: "two@example.com"}}
	const (
		noMarkup = ""
		reply    = `"keyboard":[[{"text":"` + PendingButton + `"},{"text":"` + StatusButton + `"}]]`
		inline   = `"callback_data":"approve:`
	)
	type sent struct{ text, markup string }
	tests := []struct {
		name string
		chat int64
		text string
		mod  *fakeModerator
		want []sent // подстроки текста и reply_markup отправленных сообщений по порядку
	}{
		{"чужой чат узнаёт свой id", 999, "/start", &fakeModerator{}, []sent{{"<code>999</code>", noMarkup}}},
		{"админ: /start — справка и кнопка", adminChat, "/start", &fakeModerator{}, []sent{{PendingButton, reply}}},
		{"админ: любой текст — справка и кнопка", adminChat, "привет", &fakeModerator{}, []sent{{"Отклонить", reply}}},
		{"кнопка: заявок нет", adminChat, PendingButton, &fakeModerator{}, []sent{{"Новых заявок нет", reply}}},
		{"кнопка: две заявки", adminChat, PendingButton, &fakeModerator{pending: pending},
			[]sent{{"Ждут решения: 2", reply}, {"one@example.com", inline}, {"two@example.com", inline}}},
		{"команда /pending тоже работает", adminChat, "/pending@panel_bot", &fakeModerator{pending: pending[:1]},
			[]sent{{"Ждут решения: 1", reply}, {"one@example.com", inline}}},
		{"кнопка: ошибка базы", adminChat, PendingButton, &fakeModerator{err: errors.New("db")}, []sent{{"Не получилось", noMarkup}}},
		{"чужой чат нажал текст кнопки", 999, PendingButton, &fakeModerator{pending: pending}, []sent{{"<code>999</code>", noMarkup}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, b := newTestBot(t, tt.mod)

			b.Handle(context.Background(), Update{Message: &Message{Chat: Chat{ID: tt.chat}, Text: tt.text}})

			calls := f.callsOf("sendMessage")
			if len(calls) != len(tt.want) {
				t.Fatalf("sendMessage = %d, want %d", len(calls), len(tt.want))
			}
			for i, want := range tt.want {
				p := calls[i].Params
				markup, _ := json.Marshal(p["reply_markup"])
				if _, has := p["reply_markup"]; !has {
					markup = nil
				}
				if p["chat_id"] != float64(tt.chat) || !strings.Contains(p["text"].(string), want.text) ||
					(want.markup == noMarkup) != (markup == nil) || !strings.Contains(string(markup), want.markup) {
					t.Errorf("сообщение %d: text=%q markup=%s, want %q / %q", i, p["text"], markup, want.text, want.markup)
				}
			}
		})
	}
}

func TestRunProcessesUpdatesAndAdvancesOffset(t *testing.T) {
	mod := &fakeModerator{result: &store.User{Status: store.UserApproved}, changed: true}
	f, b := newTestBot(t, mod)
	f.updates <- `[{"update_id":41,"callback_query":{"id":"c","from":{"id":432},"data":"approve:1",` +
		`"message":{"message_id":1,"chat":{"id":432},"text":"x"}}}]`
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); b.Run(ctx) }()

	deadline := time.After(3 * time.Second)
	for len(f.callsOf("getUpdates")) < 2 {
		select {
		case <-deadline:
			t.Fatal("бот не опросил getUpdates повторно")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	<-done

	if len(mod.calls) != 1 {
		t.Errorf("модерация = %v", mod.calls)
	}
	if got := f.callsOf("getUpdates")[1].Params["offset"]; got != float64(42) {
		t.Errorf("offset = %v, want 42", got)
	}
	if len(f.callsOf("deleteMyCommands")) != 1 {
		t.Error("меню команд не убрано")
	}
}

func TestRunStopsOnInvalidToken(t *testing.T) {
	f, b := newTestBot(t, &fakeModerator{})
	f.replies["getMe"] = `{"ok":false,"error_code":401,"description":"Unauthorized"}`
	done := make(chan struct{})

	go func() { defer close(done); b.Run(context.Background()) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("при неверном токене бот должен остановиться")
	}
	if len(f.callsOf("getUpdates")) != 0 {
		t.Error("getUpdates не должен вызываться")
	}
}

func TestClientErrors(t *testing.T) {
	t.Run("ok=false превращается в APIError", func(t *testing.T) {
		f, client := newFakeAPI(t)
		f.replies["sendMessage"] = `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`

		err := client.SendMessage(context.Background(), 1, "x", nil)

		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 403 || apiErr.Method != "sendMessage" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("сетевая ошибка не раскрывает токен", func(t *testing.T) {
		client := NewClient("http://127.0.0.1:1", testToken)

		err := client.SendMessage(context.Background(), 1, "x", nil)

		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("err = %v", err)
		}
	})
}
