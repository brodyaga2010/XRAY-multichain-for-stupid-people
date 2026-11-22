package api

import (
	"net/http"
	"slices"

	"xraypanel/internal/store"
)

// maxBulkUsers — сколько пользователей можно обработать одним запросом.
const maxBulkUsers = 500

type bulkIn struct {
	Action  string  `json:"action"`
	UserIDs []int64 `json:"user_ids"`
	// Months — на сколько продлить подписку (только для extend).
	Months int `json:"months"`
}

type bulkFailure struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Detail string `json:"detail"`
}

type bulkOut struct {
	Updated []userOut     `json:"updated"`
	Skipped int           `json:"skipped"`
	Failed  []bulkFailure `json:"failed"`
}

// bulkApplies — подходит ли действие пользователю: подтвердить — ждущих, восстановить — заблокированных,
// заблокировать — всех, кроме уже заблокированных и самого администратора;
// продлить — подтверждённых и заблокированных с ограниченным сроком (бессрочных не трогаем).
func bulkApplies(action string, user *store.User, adminID int64) bool {
	switch action {
	case "approve":
		return user.Status == store.UserPending
	case "restore":
		return user.Status == store.UserSuspended
	case "suspend":
		return user.Status != store.UserSuspended && user.ID != adminID
	case "extend":
		return user.Status != store.UserPending && user.PaidUntil != nil
	}
	return false
}

// adminBulkUsers применяет одно действие к списку пользователей. Каждый обрабатывается отдельно:
// ошибка у одного не откатывает остальных, неподходящие пропускаются.
func (a *App) adminBulkUsers(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	var body bulkIn
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	if !slices.Contains([]string{"approve", "restore", "suspend", "extend"}, body.Action) {
		return invalid("Действие должно быть approve, restore, suspend или extend")
	}
	if body.Action == "extend" && (body.Months < 1 || body.Months > 36) {
		return invalid("Продлить можно на 1–36 месяцев")
	}
	slices.Sort(body.UserIDs)
	ids := slices.Compact(body.UserIDs)
	if len(ids) == 0 || len(ids) > maxBulkUsers {
		return invalid("Выберите от 1 до 500 пользователей")
	}

	ctx := detached(r)
	out := bulkOut{Updated: []userOut{}, Failed: []bulkFailure{}}
	extra := map[string]any{"bulk": true}
	for _, id := range ids {
		user, err := a.st.UserByID(ctx, id)
		if err != nil || !bulkApplies(body.Action, user, admin.ID) {
			out.Skipped++
			continue
		}
		var updated *store.User
		switch body.Action {
		case "approve":
			updated, err = a.activateUser(ctx, admin, user, "user.approve", extra)
		case "restore":
			updated, err = a.activateUser(ctx, admin, user, "user.restore", extra)
		case "suspend":
			updated, err = a.suspendUser(ctx, admin, user, extra)
		case "extend":
			updated, err = a.ExtendSubscription(ctx, admin, user.ID, body.Months, "bulk")
		}
		if err != nil {
			a.log.Warn("массовое действие не выполнено", "action", body.Action, "user_id", id, "error", err)
			out.Failed = append(out.Failed, bulkFailure{ID: id, Email: user.Email, Detail: "не удалось применить"})
			continue
		}
		out.Updated = append(out.Updated, toUserOut(updated))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
