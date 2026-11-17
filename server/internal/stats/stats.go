// Package stats — фоновый сбор трафика и онлайна из Xray в БД.
package stats

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"xraypanel/internal/store"
	"xraypanel/internal/xray"
)

// CollectOnce добавляет трафик с прошлого сбора и отмечает время активности.
// Возвращает число обновлённых ключей.
func CollectOnce(ctx context.Context, st *store.Store, x *xray.Manager, now time.Time) (int, error) {
	traffic, err := x.CollectTraffic()
	if err != nil {
		return 0, err
	}
	online, err := x.OnlineEmails()
	if err != nil {
		return 0, err
	}
	active := map[string]bool{}
	for email := range online {
		active[email] = true
	}
	for email, stat := range traffic {
		if stat.Uplink != 0 || stat.Downlink != 0 {
			active[email] = true
		}
	}
	emails := map[string]bool{}
	for email := range traffic {
		emails[email] = true
	}
	for email := range active {
		emails[email] = true
	}
	if len(emails) == 0 {
		return 0, nil
	}
	updated := 0
	err = st.Tx(ctx, func(q *store.Queries) error {
		for email := range emails {
			stat := traffic[email]
			var seen *time.Time
			if active[email] {
				seen = &now
			}
			found, err := q.AddTraffic(ctx, email, stat.Uplink, stat.Downlink, seen)
			if err != nil {
				return err
			}
			if found {
				updated++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}

// Run собирает статистику каждые interval, пока не отменён ctx.
func Run(ctx context.Context, st *store.Store, x *xray.Manager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := CollectOnce(ctx, st, x, store.Now()); err != nil {
				var gw *xray.GatewayError
				if errors.As(err, &gw) {
					slog.Warn("Сбор статистики Xray", "error", gw.Detail)
				} else if ctx.Err() == nil {
					slog.Error("Сбор статистики Xray завершился ошибкой", "error", err)
				}
			}
		}
	}
}
