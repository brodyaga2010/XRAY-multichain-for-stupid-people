// Package backup — ежедневная резервная копия базы кабинета в чат администратора Telegram.
package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Source — откуда берётся копия базы (store.Store).
type Source interface {
	BackupTo(ctx context.Context, path string) error
}

// Sender — куда отправляется копия и сообщение о сбое (telegram.Bot).
type Sender interface {
	SendBackup(ctx context.Context, filename string, data []byte, caption string) error
	Alert(ctx context.Context, text string)
}

// Backup делает копию базы и отправляет её.
type Backup struct {
	src    Source
	send   Sender
	tmpDir string // каталог для временного файла копии (рядом с базой, там есть права на запись)
	label  string // имя сайта в названии файла
	now    func() time.Time
	log    *slog.Logger
}

// New создаёт резервное копирование; tmpDir — каталог базы, label — имя сайта (может быть пустым).
func New(src Source, send Sender, tmpDir, label string) *Backup {
	return &Backup{src: src, send: send, tmpDir: tmpDir, label: label, now: time.Now, log: slog.Default()}
}

// RunOnce снимает копию, сжимает gzip и отправляет. Возвращает размер отправленного файла.
func (b *Backup) RunOnce(ctx context.Context) (int, error) {
	at := b.now()
	tmp := filepath.Join(b.tmpDir, fmt.Sprintf(".panel-backup-%d.db", at.UnixNano()))
	defer os.Remove(tmp)
	if err := b.src.BackupTo(ctx, tmp); err != nil {
		return 0, err
	}
	raw, err := os.ReadFile(tmp)
	if err != nil {
		return 0, fmt.Errorf("чтение копии: %w", err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(raw); err != nil {
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	name := "panel"
	if b.label != "" {
		name = b.label + "-panel"
	}
	filename := fmt.Sprintf("%s-%s.db.gz", name, at.Format("2006-01-02-1504"))
	caption := fmt.Sprintf("💾 <b>Резервная копия базы кабинета</b>\n%s · %s\n\nВосстановление: <code>gunzip</code>, "+
		"остановить xray-panel, заменить panel.db, запустить.", at.Format("02.01.2006 15:04 MST"), humanSize(gz.Len()))
	if err := b.send.SendBackup(ctx, filename, gz.Bytes(), caption); err != nil {
		return 0, err
	}
	return gz.Len(), nil
}

// Run делает копию каждый день в hour:00 по местному времени (TZ) до отмены ctx.
func (b *Backup) Run(ctx context.Context, hour int) {
	for {
		next := NextRun(b.now(), hour)
		b.log.Info("резервная копия базы запланирована", "at", next.Format(time.RFC3339))
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		}
		if size, err := b.RunOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Error("резервная копия базы не отправлена", "error", err)
			b.send.Alert(ctx, "⚠️ Не удалось сделать или отправить резервную копию базы кабинета: "+err.Error())
		} else {
			b.log.Info("резервная копия базы отправлена", "bytes", size)
		}
	}
}

// NextRun — ближайший момент hour:00 строго позже now в часовом поясе now.
func NextRun(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f КБ", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d Б", n)
}
