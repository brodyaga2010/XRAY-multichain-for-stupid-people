package api

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"xraypanel/internal/store"
)

func (a *App) adminListUsers(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	users, err := a.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	out := make([]userOut, 0, len(users))
	for i := range users {
		out = append(out, toUserOut(&users[i]))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *App) getUser(r *http.Request, id int64) (*store.User, error) {
	user, err := a.st.UserByID(detached(r), id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpErr(http.StatusNotFound, "Пользователь не найден")
	}
	return user, err
}

func (a *App) adminListUserKeys(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	user, err := a.getUser(r, id)
	if err != nil {
		return err
	}
	keys, err := a.st.KeysByOwner(r.Context(), id)
	if err != nil {
		return err
	}
	out := make([]keyOut, 0, len(keys))
	for i := range keys {
		out = append(out, a.keyOut(&keys[i], &user.Email))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *App) adminListKeys(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	keys, err := a.st.AllKeysWithOwner(r.Context())
	if err != nil {
		return err
	}
	out := make([]keyOut, 0, len(keys))
	for i := range keys {
		out = append(out, a.keyOut(&keys[i].VpnKey, &keys[i].OwnerEmail))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *App) adminImportKeys(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	imported, err := a.importServerKeys(detached(r), admin)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": imported})
	return nil
}

func (a *App) adminApprove(w http.ResponseWriter, r *http.Request) error {
	return a.adminActivate(w, r, "user.approve")
}

func (a *App) adminRestore(w http.ResponseWriter, r *http.Request) error {
	return a.adminActivate(w, r, "user.restore")
}

func (a *App) adminActivate(w http.ResponseWriter, r *http.Request, action string) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	user, err := a.getUser(r, id)
	if err != nil {
		return err
	}
	updated, err := a.activateUser(detached(r), admin, user, action, nil)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toUserOut(updated))
	return nil
}

func (a *App) adminSuspend(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	if id == admin.ID {
		return httpErr(http.StatusBadRequest, "Нельзя заблокировать самого себя")
	}
	user, err := a.getUser(r, id)
	if err != nil {
		return err
	}
	updated, err := a.suspendUser(detached(r), admin, user, nil)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toUserOut(updated))
	return nil
}

// parseLimit принимает целое 0..100 (число или числовая строка, как в pydantic lax-режиме).
func parseLimit(raw json.RawMessage) (int, error) {
	bad := invalid("Лимит ключей должен быть целым числом от 0 до 100")
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if len(raw) == 0 || text == "" || text == "null" || text == "true" || text == "false" {
		return 0, bad
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || f != math.Trunc(f) || f < 0 || f > 100 {
		return 0, bad
	}
	return int(f), nil
}

func (a *App) adminUpdateLimits(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	var body struct {
		KeyLimit json.RawMessage `json:"key_limit"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	limit, err := parseLimit(body.KeyLimit)
	if err != nil {
		return err
	}
	ctx := detached(r)
	user, err := a.getUser(r, id)
	if err != nil {
		return err
	}
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.SetKeyLimit(ctx, user.ID, limit); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{
			ActorID: &admin.ID, Action: "user.limit_update", TargetType: "user", TargetID: itoa(user.ID),
			Metadata: map[string]any{"key_limit": limit},
		})
	})
	if err != nil {
		return err
	}
	updated, err := a.st.UserByID(ctx, user.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toUserOut(updated))
	return nil
}

func (a *App) adminCreateKey(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	var body keyCreate
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	name, err := keyName(body.Name)
	if err != nil {
		return err
	}
	user, err := a.getUser(r, id)
	if err != nil {
		return err
	}
	if user.Status == store.UserSuspended {
		return httpErr(http.StatusConflict, "Пользователь заблокирован")
	}
	if err := requireSubscription(user, false); err != nil {
		return err
	}
	key, err := a.provisionKey(detached(r), user, name, admin, "admin.key.create")
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, a.keyOut(key, &user.Email))
	return nil
}

func (a *App) adminRevokeKey(w http.ResponseWriter, r *http.Request) error {
	admin, err := a.adminUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "key_id")
	if err != nil {
		return err
	}
	ctx := detached(r)
	key, err := a.st.KeyByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return httpErr(http.StatusNotFound, "Ключ не найден")
	}
	if err != nil {
		return err
	}
	if err := a.revokeKey(ctx, key, admin, "admin.key.revoke"); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, message{Message: "Ключ отозван"})
	return nil
}

func (a *App) adminAudit(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	filter, err := parseAuditFilter(r.URL.Query())
	if err != nil {
		return err
	}
	entries, err := a.st.SearchAudit(r.Context(), filter)
	if err != nil {
		return err
	}
	out := make([]auditOut, 0, len(entries))
	for _, e := range entries {
		meta := rawJSON("{}")
		if trimmed := strings.TrimSpace(e.Metadata); strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
			meta = rawJSON(trimmed)
		}
		out = append(out, auditOut{
			ID: e.ID, ActorID: e.ActorID, ActorEmail: e.ActorEmail, Action: e.Action, TargetType: e.TargetType,
			TargetID: e.TargetID, AuditMetadata: meta, CreatedAt: jsonTime{e.CreatedAt},
		})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// auditCategories — группы действий для фильтра журнала.
var auditCategories = map[string]struct{ include, exclude []string }{
	"users":   {include: []string{"user.%", "subscription.%"}, exclude: []string{"user.password_change"}},
	"keys":    {include: []string{"key.%", "admin.key.%"}},
	"account": {include: []string{"user.password_change", "auth.%"}},
}

// parseAuditFilter читает limit (1..1000, по умолчанию 200), before (id записи-курсора),
// category (users, keys, account) и q (поиск, до 100 символов).
func parseAuditFilter(query url.Values) (store.AuditFilter, error) {
	f := store.AuditFilter{Limit: 200}
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			return f, invalid("Параметр limit должен быть целым числом от 1 до 1000")
		}
		f.Limit = n
	}
	if raw := query.Get("before"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 {
			return f, invalid("Параметр before должен быть id записи")
		}
		f.BeforeID = n
	}
	if raw := query.Get("category"); raw != "" {
		c, ok := auditCategories[raw]
		if !ok {
			return f, invalid("Параметр category: users, keys или account")
		}
		f.Actions, f.ExcludeActions = c.include, c.exclude
	}
	f.Query = strings.TrimSpace(query.Get("q"))
	if utf8.RuneCountInString(f.Query) > 100 {
		return f, invalid("Слишком длинный поисковый запрос")
	}
	return f, nil
}
