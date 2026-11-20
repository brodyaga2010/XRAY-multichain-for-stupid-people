package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"xraypanel/internal/store"
)

type planOut struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	KeyLimit     int      `json:"key_limit"`
	DurationDays int      `json:"duration_days"`
	IsDefault    bool     `json:"is_default"`
	Users        int      `json:"users"`
	CreatedAt    jsonTime `json:"created_at"`
}

func toPlanOut(p *store.Plan) planOut {
	return planOut{ID: p.ID, Name: p.Name, KeyLimit: p.KeyLimit, DurationDays: p.DurationDays, IsDefault: p.IsDefault,
		Users: p.Users, CreatedAt: jsonTime{p.CreatedAt}}
}

type planIn struct {
	Name         *string         `json:"name"`
	KeyLimit     json.RawMessage `json:"key_limit"`
	DurationDays *int            `json:"duration_days"`
	IsDefault    bool            `json:"is_default"`
}

// toPlan проверяет поля тарифа: имя 1..64 символа, лимит 0..100, срок 0..3660 дней (0 — бессрочно).
func (in planIn) toPlan() (*store.Plan, error) {
	name, err := requireString(in.Name, "name")
	if err != nil {
		return nil, err
	}
	name = strings.Join(strings.Fields(name), " ")
	if n := utf8.RuneCountInString(name); n < 1 || n > 64 {
		return nil, invalid("Название тарифа должно быть длиной от 1 до 64 символов")
	}
	limit, err := parseLimit(in.KeyLimit)
	if err != nil {
		return nil, err
	}
	if in.DurationDays == nil || *in.DurationDays < 0 || *in.DurationDays > 3660 {
		return nil, invalid("Срок тарифа — число дней от 0 (бессрочно) до 3660")
	}
	return &store.Plan{Name: name, KeyLimit: limit, DurationDays: *in.DurationDays, IsDefault: in.IsDefault}, nil
}

func (a *App) adminListPlans(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	plans, err := a.st.ListPlans(r.Context())
	if err != nil {
		return err
	}
	out := make([]planOut, 0, len(plans))
	for i := range plans {
		out = append(out, toPlanOut(&plans[i]))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// adminSavePlan создаёт (POST) или обновляет (PUT /{plan_id}) тариф.
func (a *App) adminSavePlan(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	var body planIn
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	plan, err := body.toPlan()
	if err != nil {
		return err
	}
	status, action := http.StatusCreated, "plan.create"
	if r.PathValue("plan_id") != "" {
		if plan.ID, err = pathID(r, "plan_id"); err != nil {
			return err
		}
		status, action = http.StatusOK, "plan.update"
	}
	ctx := detached(r)
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.SavePlan(ctx, plan); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{ActorID: &admin.ID, Action: action, TargetType: "plan", TargetID: itoa(plan.ID),
			Metadata: map[string]any{"name": plan.Name, "key_limit": plan.KeyLimit, "duration_days": plan.DurationDays, "is_default": plan.IsDefault}})
	})
	switch {
	case errors.Is(err, store.ErrConflict):
		return httpErr(http.StatusConflict, "Тариф с таким названием уже есть")
	case errors.Is(err, store.ErrNotFound):
		return httpErr(http.StatusNotFound, "Тариф не найден")
	case err != nil:
		return err
	}
	saved, err := a.st.PlanByID(ctx, plan.ID)
	if err != nil {
		return err
	}
	writeJSON(w, status, toPlanOut(saved))
	return nil
}

func (a *App) adminDeletePlan(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "plan_id")
	if err != nil {
		return err
	}
	ctx := detached(r)
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.DeletePlan(ctx, id); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{ActorID: &admin.ID, Action: "plan.delete", TargetType: "plan", TargetID: itoa(id)})
	})
	switch {
	case errors.Is(err, store.ErrConflict):
		return httpErr(http.StatusConflict, "На тарифе есть пользователи — сначала переведите их на другой")
	case errors.Is(err, store.ErrNotFound):
		return httpErr(http.StatusNotFound, "Тариф не найден")
	case err != nil:
		return err
	}
	writeJSON(w, http.StatusOK, message{Message: "Тариф удалён"})
	return nil
}

// subscriptionIn — изменение подписки. Отсутствующее поле не меняется; null — без тарифа / бессрочно.
type subscriptionIn struct {
	PlanID       json.RawMessage `json:"plan_id"`
	PaidUntil    json.RawMessage `json:"paid_until"`
	ExtendMonths *int            `json:"extend_months"`
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// toChange разбирает тело: plan_id (id или null), paid_until ("YYYY-MM-DD" — до конца этого дня, или null)
// либо extend_months (1..36) — не вместе с paid_until.
func (in subscriptionIn) toChange(user *store.User) (subscriptionChange, string, error) {
	var ch subscriptionChange
	action := "subscription.update"
	if in.PlanID != nil {
		ch.SetPlan = true
		if !isNull(in.PlanID) {
			var id int64
			if err := json.Unmarshal(in.PlanID, &id); err != nil || id < 1 {
				return ch, "", invalid("plan_id должен быть id тарифа или null")
			}
			ch.PlanID = &id
		}
	}
	if in.PaidUntil != nil && in.ExtendMonths != nil {
		return ch, "", invalid("Укажите либо paid_until, либо extend_months")
	}
	if in.PaidUntil != nil {
		ch.SetPaid = true
		if !isNull(in.PaidUntil) {
			bad := invalid("paid_until — дата в формате ГГГГ-ММ-ДД или null")
			var text string
			if err := json.Unmarshal(in.PaidUntil, &text); err != nil {
				return ch, "", bad
			}
			day, err := time.ParseInLocation(dayLayout, text, time.Local)
			if err != nil {
				return ch, "", bad
			}
			until := endOfDay(day)
			ch.PaidUntil = &until
		}
	}
	if in.ExtendMonths != nil {
		if *in.ExtendMonths < 1 || *in.ExtendMonths > 36 {
			return ch, "", invalid("Продлить можно на 1–36 месяцев")
		}
		until := extendPaidUntil(user.PaidUntil, time.Now(), *in.ExtendMonths)
		ch.SetPaid, ch.PaidUntil, action = true, &until, "subscription.extend"
	}
	if !ch.SetPlan && !ch.SetPaid {
		return ch, "", invalid("Нечего менять: укажите plan_id, paid_until или extend_months")
	}
	return ch, action, nil
}

func (a *App) adminUpdateSubscription(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	var body subscriptionIn
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	user, err := a.getUser(r, id)
	if err != nil {
		return err
	}
	ch, action, err := body.toChange(user)
	if err != nil {
		return err
	}
	extra := map[string]any{}
	if body.ExtendMonths != nil {
		extra["months"] = *body.ExtendMonths
	}
	updated, err := a.changeSubscription(detached(r), admin, user, ch, action, extra)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toUserOut(updated))
	return nil
}
