package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"xraypanel/internal/store"
)

// Moderator — операции кабинета, доступные из бота.
type Moderator interface {
	PendingUsers(ctx context.Context) ([]store.User, error)
	ModerateUser(ctx context.Context, userID int64, approve bool, via string) (*store.User, bool, error)
}

const (
	actionApprove = "approve"
	actionReject  = "reject"

	// PendingButton — постоянная кнопка администратора: список заявок, ждущих решения.
	PendingButton = "📋 Активные заявки"
	// StatusButton — постоянная кнопка администратора: сводка мониторинга.
	StatusButton = "📊 Состояние"
)

// adminKeyboard — кнопки под полем ввода в чате администратора.
var adminKeyboard = &ReplyKeyboard{
	Rows:   [][]KeyboardButton{{{Text: PendingButton}, {Text: StatusButton}}},
	Resize: true, Persistent: true,
}

// Bot присылает администраторам заявки на регистрацию и принимает нажатия «Подтвердить»/«Отклонить».
// Работает через long polling: входящие порты и вебхуки не нужны.
type Bot struct {
	api    *Client
	admins map[int64]bool
	chats  []int64
	mod    Moderator
	status StatusSource
	backup func(ctx context.Context) (int, error)
	log    *slog.Logger

	// Location — часовой пояс времени в сообщениях (по умолчанию time.Local, задаётся TZ).
	Location *time.Location
	// PollTimeout — сколько секунд Telegram держит getUpdates; RetryDelay — пауза после ошибки.
	PollTimeout int
	RetryDelay  time.Duration
}

// New создаёт бота; adminChats — чаты, куда идут заявки и откуда принимаются нажатия.
func New(api *Client, adminChats []int64, mod Moderator) *Bot {
	admins := make(map[int64]bool, len(adminChats))
	for _, id := range adminChats {
		admins[id] = true
	}
	return &Bot{
		api: api, admins: admins, chats: adminChats, mod: mod, log: slog.Default(),
		Location: time.Local, PollTimeout: 50, RetryDelay: 5 * time.Second,
	}
}

// UserRegistered отправляет заявку всем администраторам (реализует api.RegistrationNotifier).
func (b *Bot) UserRegistered(ctx context.Context, user store.User, ip string) {
	text := b.requestText(user, ip)
	keyboard := moderationKeyboard(user.ID)
	for _, chat := range b.chats {
		if err := b.api.SendMessage(ctx, chat, text, keyboard); err != nil {
			b.log.Warn("telegram: заявка не отправлена", "chat_id", chat, "user_id", user.ID, "error", err)
		}
	}
}

// Run обрабатывает события до отмены ctx.
func (b *Bot) Run(ctx context.Context) {
	me, err := b.api.GetMe(ctx)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == 401 {
		b.log.Error("telegram: неверный TELEGRAM_BOT_TOKEN, бот выключен")
		return
	}
	if err != nil {
		b.log.Warn("telegram: getMe не ответил, продолжаю", "error", err)
	} else {
		b.log.Info("telegram-бот запущен", "bot", "@"+me.Username, "admin_chats", len(b.chats))
	}
	if len(b.chats) == 0 {
		b.log.Warn("telegram: не задан TELEGRAM_ADMIN_CHAT_IDS — заявки отправлять некуда; напишите боту /start, он покажет chat id")
	}
	if err := b.api.DeleteMyCommands(ctx); err != nil && ctx.Err() == nil {
		b.log.Warn("telegram: меню команд не убрано", "error", err)
	}

	var offset int64
	for ctx.Err() == nil {
		updates, err := b.api.GetUpdates(ctx, offset, b.PollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Warn("telegram: getUpdates", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(b.RetryDelay):
			}
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			b.Handle(ctx, update)
		}
	}
}

// Handle обрабатывает одно событие.
func (b *Bot) Handle(ctx context.Context, update Update) {
	switch {
	case update.CallbackQuery != nil:
		b.handleCallback(ctx, update.CallbackQuery)
	case update.Message != nil:
		b.handleMessage(ctx, update.Message)
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *Message) {
	chat := msg.Chat.ID
	if !b.admins[chat] {
		b.log.Info("telegram: сообщение из чата без доступа", "chat_id", chat)
		b.send(ctx, chat, fmt.Sprintf("Это служебный бот. Ваш chat id: <code>%d</code>", chat), nil)
		return
	}
	text := strings.TrimSpace(msg.Text)
	command, _, _ := strings.Cut(text, " ")
	command, _, _ = strings.Cut(command, "@") // /pending@botname в группах
	if text == StatusButton || command == "/status" {
		b.send(ctx, chat, b.statusText(), adminKeyboard)
		return
	}
	if command == "/backup" {
		b.runBackup(ctx, chat)
		return
	}
	if text != PendingButton && command != "/pending" {
		b.send(ctx, chat, "Я пришлю сюда каждую новую регистрацию с кнопками «Подтвердить» и «Отклонить», "+
			"а также сообщу, если VPN начнёт работать плохо, и когда всё восстановится.\n\n"+
			"«"+PendingButton+"» — заявки, которые ждут решения.\n«"+StatusButton+"» — как работает VPN сейчас.", adminKeyboard)
		return
	}
	users, err := b.mod.PendingUsers(ctx)
	if err != nil {
		b.log.Error("telegram: список заявок", "error", err)
		b.send(ctx, chat, "Не получилось загрузить заявки, подробности в логе кабинета.", nil)
		return
	}
	if len(users) == 0 {
		b.send(ctx, chat, "Новых заявок нет.", adminKeyboard)
		return
	}
	b.send(ctx, chat, fmt.Sprintf("Ждут решения: %d", len(users)), adminKeyboard)
	for _, user := range users {
		b.send(ctx, chat, b.requestText(user, ""), moderationKeyboard(user.ID))
	}
}

func (b *Bot) handleCallback(ctx context.Context, cb *CallbackQuery) {
	if cb.Message == nil || !b.admins[cb.Message.Chat.ID] {
		b.log.Warn("telegram: нажатие из чата без доступа", "from_id", cb.From.ID)
		b.answer(ctx, cb.ID, "Нет доступа")
		return
	}
	action, rawID, _ := strings.Cut(cb.Data, ":")
	userID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || (action != actionApprove && action != actionReject) {
		b.answer(ctx, cb.ID, "Неизвестная кнопка")
		return
	}

	user, changed, err := b.mod.ModerateUser(ctx, userID, action == actionApprove, "telegram")
	var result, toast string
	switch {
	case errors.Is(err, store.ErrNotFound):
		result, toast = "❔ Пользователь не найден", "Пользователь не найден"
	case err != nil:
		b.log.Error("telegram: модерация", "user_id", userID, "error", err)
		b.answer(ctx, cb.ID, "Ошибка, попробуйте ещё раз")
		return // кнопки остаются, можно нажать снова
	case !changed:
		result = "ℹ️ Уже решено в кабинете: " + statusName(user.Status)
		toast = "Уже решено: " + statusName(user.Status)
	case action == actionApprove:
		result, toast = "✅ Подтверждён", "Подтверждён"
		b.log.Info("telegram: заявка подтверждена", "user_id", userID, "email", user.Email)
	default:
		result, toast = "⛔ Отклонён — восстановить можно в кабинете", "Отклонён"
		b.log.Info("telegram: заявка отклонена", "user_id", userID, "email", user.Email)
	}
	// Текст сообщения приходит без разметки: экранируем и дописываем итог, кнопки убираются.
	text := html.EscapeString(cb.Message.Text) + "\n\n<b>" + html.EscapeString(result) + "</b>"
	if err := b.api.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, text, nil); err != nil {
		b.log.Warn("telegram: сообщение не обновлено", "error", err)
	}
	b.answer(ctx, cb.ID, toast)
}

func (b *Bot) requestText(user store.User, ip string) string {
	var sb strings.Builder
	sb.WriteString("🆕 <b>Заявка на регистрацию</b>\n\n")
	fmt.Fprintf(&sb, "<code>%s</code>\n", html.EscapeString(user.Email))
	fmt.Fprintf(&sb, "Время: %s", user.CreatedAt.In(b.Location).Format("02.01.2006 15:04 MST"))
	if ip != "" {
		fmt.Fprintf(&sb, "\nIP: <code>%s</code>", html.EscapeString(ip))
	}
	return sb.String()
}

func moderationKeyboard(userID int64) *InlineKeyboard {
	id := strconv.FormatInt(userID, 10)
	return &InlineKeyboard{Rows: [][]Button{{
		{Text: "✅ Подтвердить", Data: actionApprove + ":" + id},
		{Text: "⛔ Отклонить", Data: actionReject + ":" + id},
	}}}
}

func statusName(status string) string {
	switch status {
	case store.UserApproved:
		return "подтверждён"
	case store.UserSuspended:
		return "заблокирован"
	case store.UserPending:
		return "ожидает"
	}
	return status
}

func (b *Bot) send(ctx context.Context, chat int64, text string, markup ReplyMarkup) {
	if err := b.api.SendMessage(ctx, chat, text, markup); err != nil {
		b.log.Warn("telegram: сообщение не отправлено", "chat_id", chat, "error", err)
	}
}

func (b *Bot) answer(ctx context.Context, id, text string) {
	if err := b.api.AnswerCallbackQuery(ctx, id, text); err != nil {
		b.log.Warn("telegram: answerCallbackQuery", "error", err)
	}
}
