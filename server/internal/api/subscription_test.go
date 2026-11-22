package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"xraypanel/internal/store"
)

func dayFromNow(days int) string {
	return time.Now().In(time.Local).AddDate(0, 0, days).Format("2006-01-02")
}

// keyStates — статусы ключей пользователя в БД и наличие их в Xray.
func keyStates(e *env, userID int64) (statuses []string, inXray []bool) {
	e.t.Helper()
	keys, err := e.st.KeysByOwner(context.Background(), userID)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, k := range keys {
		statuses = append(statuses, k.Status)
		inXray = append(inXray, e.inXray(k.UUID))
	}
	return statuses, inXray
}

func TestSubscriptionLifecycle(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com")
	e.login("admin@example.com")
	uid := itoa(user.ID)
	key := e.post("/admin/users/"+uid+"/keys", map[string]string{"name": "телефон"}).JSON()
	path := "/admin/users/" + uid + "/subscription"

	steps := []struct {
		name       string
		body       map[string]any
		wantKey    string
		wantXray   bool
		wantActive bool
	}{
		{"срок в прошлом — ключи отключены", map[string]any{"paid_until": dayFromNow(-1)}, store.KeyExpired, false, false},
		{"продление на месяц — ключи вернулись", map[string]any{"extend_months": 1}, store.KeyActive, true, true},
		{"снова в прошлое", map[string]any{"paid_until": dayFromNow(-3)}, store.KeyExpired, false, false},
		{"бессрочно", map[string]any{"paid_until": nil}, store.KeyActive, true, true},
	}
	for _, s := range steps {
		resp := e.patch(path, s.body)
		if resp.Status != 200 {
			t.Fatalf("%s: %d %s", s.name, resp.Status, resp.Raw)
		}
		statuses, inXray := keyStates(e, user.ID)
		if statuses[0] != s.wantKey || inXray[0] != s.wantXray || resp.JSON()["subscription_active"] != s.wantActive {
			t.Errorf("%s: ключ %s, в Xray %v, ответ %s", s.name, statuses[0], inXray[0], resp.Raw)
		}
	}
	keys, _ := e.st.KeysByOwner(context.Background(), user.ID)
	if keys[0].UUID != key["uuid"] {
		t.Error("после продления ключ должен остаться прежним")
	}
	actions := strings.Join(e.auditActions(), ",")
	if !strings.Contains(actions, "subscription.update,subscription.extend,subscription.update,subscription.update") {
		t.Errorf("журнал: %s", actions)
	}
}

func TestExpiredSubscriptionBlocksNewKeys(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com")
	past := time.Now().Add(-time.Hour)
	_ = e.st.SetSubscription(context.Background(), user.ID, nil, &past)

	e.login("user@example.com")
	own := e.post("/keys", map[string]string{"name": "x"})
	e.login("admin@example.com")
	byAdmin := e.post("/admin/users/"+itoa(user.ID)+"/keys", map[string]string{"name": "x"})

	if own.Status != 409 || !strings.Contains(own.Detail(), "Подписка закончилась") {
		t.Errorf("свой ключ: %d %s", own.Status, own.Raw)
	}
	if byAdmin.Status != 409 || !strings.Contains(byAdmin.Detail(), "продлите") {
		t.Errorf("ключ от администратора: %d %s", byAdmin.Status, byAdmin.Raw)
	}
}

func TestExpireSubscriptions(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	expired := e.makeUser("expired@example.com")
	paid := e.makeUser("paid@example.com")
	e.login("admin@example.com")
	for _, u := range []*store.User{expired, paid} {
		e.post("/admin/users/"+itoa(u.ID)+"/keys", map[string]string{"name": "a"})
		e.post("/admin/users/"+itoa(u.ID)+"/keys", map[string]string{"name": "b"})
	}
	// срок вышел сам собой: в базе дата в прошлом, ключи ещё активны
	past, future := time.Now().Add(-time.Minute), time.Now().Add(time.Hour)
	ctx := context.Background()
	_ = e.st.SetSubscription(ctx, expired.ID, nil, &past)
	_ = e.st.SetSubscription(ctx, paid.ID, nil, &future)

	got, err := e.app.ExpireSubscriptions(ctx)

	if err != nil || len(got) != 1 || got[0].Email != "expired@example.com" || got[0].KeysDisabled != 2 {
		t.Fatalf("истекли: %+v, err %v", got, err)
	}
	if statuses, inXray := keyStates(e, expired.ID); statuses[0] != store.KeyExpired || inXray[0] || inXray[1] {
		t.Errorf("ключи истёкшего: %v %v", statuses, inXray)
	}
	if statuses, inXray := keyStates(e, paid.ID); statuses[0] != store.KeyActive || !inXray[0] {
		t.Errorf("ключи оплатившего: %v %v", statuses, inXray)
	}
	entries, _ := e.st.ListAudit(ctx, 1)
	if entries[0].Action != "subscription.expire" || entries[0].ActorID != nil {
		t.Errorf("запись журнала: %+v", entries[0])
	}
	if again, _ := e.app.ExpireSubscriptions(ctx); len(again) != 0 {
		t.Errorf("повторный проход: %+v", again)
	}
}

func TestSubscriptionWithSuspendedUser(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com")
	e.login("admin@example.com")
	uid := itoa(user.ID)
	e.post("/admin/users/"+uid+"/keys", map[string]string{"name": "a"})

	steps := []struct {
		name     string
		do       func() response
		wantKey  string
		wantXray bool
	}{
		{"блокировка", func() response { return e.post("/admin/users/"+uid+"/suspend", nil) }, store.KeySuspended, false},
		{"срок вышел у заблокированного", func() response {
			return e.patch("/admin/users/"+uid+"/subscription", map[string]any{"paid_until": dayFromNow(-1)})
		}, store.KeyExpired, false},
		{"разблокировка без оплаты — ключи не включаются", func() response { return e.post("/admin/users/"+uid+"/restore", nil) }, store.KeyExpired, false},
		{"продление — ключи работают", func() response {
			return e.patch("/admin/users/"+uid+"/subscription", map[string]any{"extend_months": 3})
		}, store.KeyActive, true},
		{"блокировка с оплаченной подпиской", func() response { return e.post("/admin/users/"+uid+"/suspend", nil) }, store.KeySuspended, false},
		{"продление заблокированному не включает ключи", func() response {
			return e.patch("/admin/users/"+uid+"/subscription", map[string]any{"extend_months": 1})
		}, store.KeySuspended, false},
	}
	for _, s := range steps {
		if resp := s.do(); resp.Status != 200 {
			t.Fatalf("%s: %d %s", s.name, resp.Status, resp.Raw)
		}
		statuses, inXray := keyStates(e, user.ID)
		if statuses[0] != s.wantKey || inXray[0] != s.wantXray {
			t.Errorf("%s: ключ %s, в Xray %v", s.name, statuses[0], inXray[0])
		}
	}
}

func TestDefaultPlanOnApprove(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")
	plan := e.post("/admin/plans", map[string]any{"name": "Базовый", "key_limit": 2, "duration_days": 30, "is_default": true}).JSON()
	pending := e.makeUser("new@example.com", withStatus(store.UserPending))

	resp := e.post("/admin/users/"+itoa(pending.ID)+"/approve", nil)

	got := resp.JSON()
	paidUntil, _ := got["paid_until"].(string)
	wantDay := dayFromNow(30)
	parsed, err := time.Parse(time.RFC3339, paidUntil)
	if resp.Status != 200 || got["plan_id"] != plan["id"] || got["key_limit"] != float64(2) || err != nil ||
		parsed.In(time.Local).Format("2006-01-02") != wantDay || got["subscription_active"] != true {
		t.Errorf("после подтверждения: %s (ждём срок %s)", resp.Raw, wantDay)
	}
}

func TestPlansAPI(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")
	basic := e.post("/admin/plans", map[string]any{"name": "  Базовый  ", "key_limit": 2, "duration_days": 30})
	if basic.Status != 201 || basic.JSON()["name"] != "Базовый" {
		t.Fatalf("создание: %d %s", basic.Status, basic.Raw)
	}
	id := itoa(int64(basic.JSON()["id"].(float64)))

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
	}{
		{"дубль имени", "POST", "/admin/plans", map[string]any{"name": "Базовый", "key_limit": 1, "duration_days": 0}, 409},
		{"без имени", "POST", "/admin/plans", map[string]any{"key_limit": 1, "duration_days": 0}, 422},
		{"лимит больше 100", "POST", "/admin/plans", map[string]any{"name": "X", "key_limit": 101, "duration_days": 0}, 422},
		{"отрицательный срок", "POST", "/admin/plans", map[string]any{"name": "X", "key_limit": 1, "duration_days": -1}, 422},
		{"без срока", "POST", "/admin/plans", map[string]any{"name": "X", "key_limit": 1}, 422},
		{"обновление", "PUT", "/admin/plans/" + id, map[string]any{"name": "Базовый+", "key_limit": 3, "duration_days": 90}, 200},
		{"обновление несуществующего", "PUT", "/admin/plans/999", map[string]any{"name": "Y", "key_limit": 1, "duration_days": 0}, 404},
		{"подписка: неизвестный тариф", "PATCH", "/admin/users/1/subscription", map[string]any{"plan_id": 999}, 404},
		{"подписка: дата и продление вместе", "PATCH", "/admin/users/1/subscription", map[string]any{"paid_until": nil, "extend_months": 1}, 422},
		{"подписка: кривая дата", "PATCH", "/admin/users/1/subscription", map[string]any{"paid_until": "20.12.2026"}, 422},
		{"подписка: продление на 0", "PATCH", "/admin/users/1/subscription", map[string]any{"extend_months": 0}, 422},
		{"подписка: пустое тело", "PATCH", "/admin/users/1/subscription", map[string]any{}, 422},
		{"назначить тариф", "PATCH", "/admin/users/1/subscription", map[string]any{"plan_id": basic.JSON()["id"]}, 200},
		{"удалить тариф с пользователями", "DELETE", "/admin/plans/" + id, nil, 409},
		{"снять тариф", "PATCH", "/admin/users/1/subscription", map[string]any{"plan_id": nil}, 200},
		{"удалить пустой тариф", "DELETE", "/admin/plans/" + id, nil, 200},
		{"удалить ещё раз", "DELETE", "/admin/plans/" + id, nil, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if resp := e.do(tt.method, tt.path, tt.body); resp.Status != tt.wantStatus {
				t.Errorf("status %d: %s", resp.Status, resp.Raw)
			}
		})
	}
	if list := e.get("/admin/plans").List(); len(list) != 0 {
		t.Errorf("тарифы после удаления: %v", list)
	}
	e.makeUser("user@example.com")
	e.login("user@example.com")
	if resp := e.get("/admin/plans"); resp.Status != 403 {
		t.Errorf("пользователю тарифы недоступны: %d", resp.Status)
	}
}

func TestSubscriptionHeaderHasExpiry(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser("user@example.com")
	until := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	_ = e.st.SetSubscription(context.Background(), user.ID, nil, &until)
	e.login("user@example.com")
	url := e.get("/keys/subscription").JSON()["url"].(string)

	resp := e.raw("GET", mustURL(url).Path, nil)

	if info := resp.Header.Get("subscription-userinfo"); !strings.HasSuffix(info, "expire="+itoa(until.Unix())) {
		t.Errorf("subscription-userinfo = %q", info)
	}
}
