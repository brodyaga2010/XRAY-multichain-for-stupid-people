package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"xraypanel/internal/store"
)

// Подписка: тариф и срок «оплачено до» у пользователя. Пока срок не вышел, ключи работают;
// вышел — ключи переходят в статус expired (отключены в Xray, ссылки прежние); продление возвращает их.
// Все изменения идут через changeSubscription — её же позже вызовет приём оплаты.

// endOfDay — последняя секунда календарного дня t по часовому поясу сервера (TZ).
func endOfDay(t time.Time) time.Time {
	l := t.In(time.Local)
	return time.Date(l.Year(), l.Month(), l.Day(), 23, 59, 59, 0, time.Local)
}

// planPaidUntil — срок при назначении тарифа в момент now; nil — тариф бессрочный.
func planPaidUntil(p *store.Plan, now time.Time) *time.Time {
	if p.DurationDays <= 0 {
		return nil
	}
	t := endOfDay(now.AddDate(0, 0, p.DurationDays))
	return &t
}

// extendPaidUntil продлевает на months месяцев от конца текущего срока, а если он прошёл или не задан — от now.
func extendPaidUntil(current *time.Time, now time.Time, months int) time.Time {
	base := now
	if current != nil && current.After(now) {
		base = *current
	}
	return endOfDay(base.In(time.Local).AddDate(0, months, 0))
}

// subscriptionChange — что меняется; Set* = false — оставить как есть.
type subscriptionChange struct {
	SetPlan   bool
	PlanID    *int64
	SetPaid   bool
	PaidUntil *time.Time
}

// keyTransition — какие ключи пользователя переводятся в какой статус при данной подписке.
// Подтверждённый: действует — expired → active, закончилась — active → expired.
// Заблокированный: действует — expired → suspended (вернутся при разблокировке), закончилась — suspended → expired.
func keyTransition(userStatus string, active bool) (from, to string) {
	switch {
	case userStatus == store.UserApproved && active:
		return store.KeyExpired, store.KeyActive
	case userStatus == store.UserApproved:
		return store.KeyActive, store.KeyExpired
	case userStatus == store.UserSuspended && active:
		return store.KeyExpired, store.KeySuspended
	case userStatus == store.UserSuspended:
		return store.KeySuspended, store.KeyExpired
	}
	return "", ""
}

// changeSubscription меняет тариф/срок и приводит ключи в соответствие (сначала Xray, затем БД).
// actor == nil — действие системы (истечение срока). При смене тарифа лимит ключей берётся из тарифа.
func (a *App) changeSubscription(ctx context.Context, actor, user *store.User, ch subscriptionChange,
	action string, extra map[string]any) (*store.User, error) {
	planID, paid := user.PlanID, user.PaidUntil
	var plan *store.Plan
	if ch.SetPlan {
		planID = ch.PlanID
		if planID != nil {
			p, err := a.st.PlanByID(ctx, *planID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpErr(http.StatusNotFound, "Тариф не найден")
			}
			if err != nil {
				return nil, err
			}
			plan = p
		}
	}
	if ch.SetPaid {
		paid = ch.PaidUntil
	}

	from, to := keyTransition(user.Status, paid == nil || paid.After(time.Now()))
	var keys []store.VpnKey
	if from != "" {
		var err error
		if keys, err = a.st.KeysByOwnerStatus(ctx, user.ID, from); err != nil {
			return nil, err
		}
	}
	for _, key := range keys {
		var err error
		switch {
		case to == store.KeyActive:
			err = a.xray.AddKey(key.UUID)
		case from == store.KeyActive:
			err = a.xray.RemoveKey(key.UUID)
		}
		if err != nil {
			return nil, err
		}
	}

	meta := map[string]any{"plan_id": planID, "paid_until": nil}
	if paid != nil {
		meta["paid_until"] = paid.UTC().Format(time.RFC3339)
	}
	if len(keys) > 0 {
		meta["keys_"+to] = len(keys)
	}
	for k, v := range extra {
		meta[k] = v
	}
	var actorID *int64
	if actor != nil {
		actorID = &actor.ID
	}
	err := a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.SetSubscription(ctx, user.ID, planID, paid); err != nil {
			return err
		}
		if plan != nil {
			if err := q.SetKeyLimit(ctx, user.ID, plan.KeyLimit); err != nil {
				return err
			}
		}
		for _, key := range keys {
			if err := q.SetKeyStatus(ctx, key.ID, to); err != nil {
				return err
			}
		}
		return q.InsertAudit(ctx, store.Audit{
			ActorID: actorID, Action: action, TargetType: "user", TargetID: itoa(user.ID), Metadata: meta,
		})
	})
	if err != nil {
		return nil, err
	}
	return a.st.UserByID(ctx, user.ID)
}

// ExtendSubscription продлевает подписку на months месяцев (кнопки администратора, в будущем — оплата).
func (a *App) ExtendSubscription(ctx context.Context, actor *store.User, userID int64, months int, via string) (*store.User, error) {
	user, err := a.st.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	until := extendPaidUntil(user.PaidUntil, time.Now(), months)
	return a.changeSubscription(ctx, actor, user, subscriptionChange{SetPaid: true, PaidUntil: &until},
		"subscription.extend", map[string]any{"months": months, "via": via})
}

// ExpiredSubscription — пользователь, у которого только что закончилась подписка.
type ExpiredSubscription struct {
	Email        string
	KeysDisabled int
}

// ExpireSubscriptions отключает ключи пользователей с закончившейся подпиской.
// Ошибка у одного пользователя не мешает остальным; возвращается первая.
func (a *App) ExpireSubscriptions(ctx context.Context) ([]ExpiredSubscription, error) {
	users, err := a.st.ExpiredUsers(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	var (
		out      []ExpiredSubscription
		firstErr error
	)
	for i := range users {
		active, err := a.st.CountActiveKeys(ctx, users[i].ID)
		if err == nil {
			_, err = a.changeSubscription(ctx, nil, &users[i], subscriptionChange{}, "subscription.expire", nil)
		}
		if err != nil {
			a.log.Warn("подписка: ключи не отключены", "user_id", users[i].ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, ExpiredSubscription{Email: users[i].Email, KeysDisabled: active})
	}
	return out, firstErr
}

// requireSubscription — новый ключ можно создать только при действующей подписке.
func requireSubscription(user *store.User, own bool) error {
	if user.SubscriptionActive(time.Now()) {
		return nil
	}
	if own {
		return httpErr(http.StatusConflict, "Подписка закончилась — обратитесь к администратору")
	}
	return httpErr(http.StatusConflict, "Подписка пользователя закончилась — сначала продлите её")
}
