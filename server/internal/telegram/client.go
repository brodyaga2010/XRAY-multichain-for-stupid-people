// Package telegram — минимальный клиент Telegram Bot API и бот уведомлений администратора.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client вызывает методы Bot API (https://core.telegram.org/bots/api).
type Client struct {
	base string // <api>/bot<token>/ — содержит токен, наружу не выводится
	http *http.Client
}

// NewClient создаёт клиента; apiURL обычно https://api.telegram.org.
func NewClient(apiURL, token string) *Client {
	return &Client{
		base: strings.TrimRight(apiURL, "/") + "/bot" + token + "/",
		// больше таймаута long polling (getUpdates держит соединение до 50 с)
		http: &http.Client{Timeout: 70 * time.Second},
	}
}

// APIError — ответ Telegram с ok=false.
type APIError struct {
	Method      string
	Code        int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// Update — входящее событие.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// Message — сообщение.
type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from,omitempty"`
	Text      string `json:"text,omitempty"`
}

// Chat — чат.
type Chat struct {
	ID int64 `json:"id"`
}

// User — пользователь или бот Telegram.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
}

// CallbackQuery — нажатие inline-кнопки.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// ReplyMarkup — разметка кнопок сообщения: InlineKeyboard или ReplyKeyboard.
type ReplyMarkup interface{ isReplyMarkup() }

// InlineKeyboard — кнопки под сообщением.
type InlineKeyboard struct {
	Rows [][]Button `json:"inline_keyboard"`
}

// ReplyKeyboard — постоянные кнопки под полем ввода; нажатие отправляет текст кнопки.
type ReplyKeyboard struct {
	Rows       [][]KeyboardButton `json:"keyboard"`
	Resize     bool               `json:"resize_keyboard"`
	Persistent bool               `json:"is_persistent"`
}

// KeyboardButton — кнопка ReplyKeyboard.
type KeyboardButton struct {
	Text string `json:"text"`
}

func (*InlineKeyboard) isReplyMarkup() {}
func (*ReplyKeyboard) isReplyMarkup()  {}

// Button — inline-кнопка с данными для callback.
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

type response struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s: некорректный адрес API", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error содержит адрес с токеном — в логи идёт только причина
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d, ответ не JSON", method, resp.StatusCode)
	}
	if !r.OK {
		return &APIError{Method: method, Code: r.ErrorCode, Description: r.Description}
	}
	if result != nil {
		return json.Unmarshal(r.Result, result)
	}
	return nil
}

// GetMe возвращает самого бота (проверка токена).
func (c *Client) GetMe(ctx context.Context) (User, error) {
	var me User
	err := c.call(ctx, "getMe", struct{}{}, &me)
	return me, err
}

// GetUpdates ждёт новые события до timeoutSec секунд (long polling).
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}

// SendMessage отправляет HTML-сообщение; markup может быть nil.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, markup ReplyMarkup) error {
	params := map[string]any{"chat_id": chatID, "text": text, "parse_mode": "HTML", "link_preview_options": map[string]bool{"is_disabled": true}}
	if markup != nil {
		params["reply_markup"] = markup
	}
	return c.call(ctx, "sendMessage", params, nil)
}

// EditMessageText меняет текст сообщения; без keyboard кнопки убираются.
func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string, keyboard *InlineKeyboard) error {
	params := map[string]any{"chat_id": chatID, "message_id": messageID, "text": text, "parse_mode": "HTML"}
	if keyboard != nil {
		params["reply_markup"] = keyboard
	}
	return c.call(ctx, "editMessageText", params, nil)
}

// AnswerCallbackQuery закрывает «часики» на кнопке и показывает короткое уведомление.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

// DeleteMyCommands убирает меню команд бота (вместо команд — кнопка под полем ввода).
func (c *Client) DeleteMyCommands(ctx context.Context) error {
	return c.call(ctx, "deleteMyCommands", struct{}{}, nil)
}
