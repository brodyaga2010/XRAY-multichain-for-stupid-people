package telegram

import (
	"context"
	"errors"
	"fmt"
)

// SetBackup подключает резервное копирование: команда /backup делает копию сразу.
func (b *Bot) SetBackup(run func(ctx context.Context) (int, error)) { b.backup = run }

// SendBackup отправляет файл копии базы во все чаты администраторов (реализует backup.Sender).
// Ошибка — только если не удалось отправить ни в один чат.
func (b *Bot) SendBackup(ctx context.Context, filename string, data []byte, caption string) error {
	if len(b.chats) == 0 {
		return errors.New("не задан TELEGRAM_ADMIN_CHAT_IDS — копию отправить некуда")
	}
	var lastErr error
	sent := 0
	for _, chat := range b.chats {
		if err := b.api.SendDocument(ctx, chat, filename, data, caption); err != nil {
			b.log.Warn("telegram: копия базы не отправлена", "chat_id", chat, "error", err)
			lastErr = err
			continue
		}
		sent++
	}
	if sent == 0 {
		return fmt.Errorf("копия не отправлена ни в один чат: %w", lastErr)
	}
	return nil
}

func (b *Bot) runBackup(ctx context.Context, chat int64) {
	if b.backup == nil {
		b.send(ctx, chat, "Резервное копирование выключено (BACKUP_HOUR=-1).", adminKeyboard)
		return
	}
	if _, err := b.backup(ctx); err != nil {
		b.log.Error("telegram: /backup", "error", err)
		b.send(ctx, chat, "Не получилось сделать копию, подробности в логе кабинета.", adminKeyboard)
	}
}
