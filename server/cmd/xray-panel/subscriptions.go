package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"xraypanel/internal/api"
)

// subscriptionCheckInterval — как часто проверять истёкшие подписки.
const subscriptionCheckInterval = time.Minute

type subscriptionExpirer interface {
	ExpireSubscriptions(ctx context.Context) ([]api.ExpiredSubscription, error)
}

type alerter interface {
	Alert(ctx context.Context, text string)
}

// expireSubscriptions отключает ключи истёкших подписок и сообщает администратору (если есть бот).
func expireSubscriptions(ctx context.Context, app subscriptionExpirer, notify alerter) {
	expired, err := app.ExpireSubscriptions(ctx)
	if err != nil && ctx.Err() == nil {
		slog.Warn("проверка подписок", "error", err)
	}
	for _, e := range expired {
		slog.Info("подписка закончилась", "email", e.Email, "keys_disabled", e.KeysDisabled)
		if notify != nil {
			notify.Alert(ctx, fmt.Sprintf("⏰ Закончилась подписка: %s — отключено ключей: %d", e.Email, e.KeysDisabled))
		}
	}
}

// startSubscriptions проверяет подписки сразу и затем раз в минуту.
func startSubscriptions(ctx context.Context, app subscriptionExpirer, rb runningBot) <-chan struct{} {
	done := make(chan struct{})
	var notify alerter
	if rb.bot != nil {
		notify = rb.bot
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(subscriptionCheckInterval)
		defer ticker.Stop()
		for {
			expireSubscriptions(ctx, app, notify)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}
