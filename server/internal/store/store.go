// Package store — доступ к SQLite. Схема совместима с базой, созданной alembic прежней версии панели.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // чистый Go драйвер SQLite
)

// ErrNotFound — запись не найдена.
var ErrNotFound = errors.New("запись не найдена")

// ErrConflict — нарушение уникальности.
var ErrConflict = errors.New("запись уже существует")

// DBTX — то, что умеют и *sql.DB, и *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Queries — запросы к БД поверх соединения или транзакции.
type Queries struct{ db DBTX }

// Store — база данных панели.
type Store struct {
	*Queries
	db *sql.DB
}

// Open открывает SQLite-файл с WAL и busy_timeout. Схему создаёт Migrate.
func Open(path string) (*Store, error) {
	dsn := "file:" + escapePath(path) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("открытие базы %s: %w", path, err)
	}
	return &Store{Queries: &Queries{db: db}, db: db}, nil
}

func escapePath(path string) string {
	return strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(path)
}

// Close закрывает базу.
func (s *Store) Close() error { return s.db.Close() }

// DB — низкоуровневый доступ (тесты, миграции).
func (s *Store) DB() *sql.DB { return s.db }

// Tx выполняет fn в транзакции: ошибка — откат, успех — коммит.
func (s *Store) Tx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Queries{db: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ---------- время ----------

const timeLayout = "2006-01-02 15:04:05.000000"

// Now — текущее время в UTC с точностью до микросекунд.
func Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05.999999999Z07:00",
}

// nullTime читает DATETIME: строка SQLAlchemy ('YYYY-MM-DD HH:MM:SS[.ffffff]', UTC) или time.Time драйвера.
type nullTime struct {
	T     time.Time
	Valid bool
}

func (n *nullTime) Scan(value any) error {
	n.T, n.Valid = time.Time{}, false
	var text string
	switch v := value.(type) {
	case nil:
		return nil
	case time.Time:
		n.T, n.Valid = v.UTC(), true
		return nil
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return fmt.Errorf("неподдерживаемый тип даты %T", value)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, text, time.UTC); err == nil {
			n.T, n.Valid = t.UTC(), true
			return nil
		}
	}
	return fmt.Errorf("не удалось разобрать дату %q", text)
}

func (n nullTime) ptr() *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.T
	return &t
}
