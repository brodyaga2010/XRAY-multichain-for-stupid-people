package api_test

import (
	"context"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"

	"xraypanel/internal/stats"
	"xraypanel/internal/store"
)

const legacyUUID = "33333333-3333-3333-3333-333333333333"

func (e *env) insertKey(owner *store.User, uuid, status string, mutate ...func(*store.VpnKey)) *store.VpnKey {
	e.t.Helper()
	k := &store.VpnKey{
		OwnerID: owner.ID, UUID: uuid, Name: "owner key", Type: "tcp_reality", SNI: "www.yahoo.com", Status: status,
		PublicKey: "public", ShortID: "short", ConnectionURI: "vless://" + uuid,
	}
	for _, m := range mutate {
		m(k)
	}
	if err := e.st.InsertKey(context.Background(), k); err != nil {
		e.t.Fatal(err)
	}
	return k
}

func transports(links any) []string {
	var out []string
	for _, l := range links.([]any) {
		out = append(out, l.(map[string]any)["transport"].(string))
	}
	return out
}

func TestKeyLimitBlocksCreateOverLimit(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com", withLimit(1))
	e.login("user@example.com")

	first := e.post("/keys", map[string]string{"name": "phone"})
	second := e.post("/keys", map[string]string{"name": "laptop"})

	if first.Status != 201 {
		t.Errorf("first: %d %s", first.Status, first.Raw)
	}
	if second.Status != 409 || second.Detail() != "Достигнут лимит ключей" {
		t.Errorf("second: %d %s", second.Status, second.Raw)
	}
}

func TestUserCannotSeeOtherUsersKeys(t *testing.T) {
	e := newEnv(t)
	owner := e.makeUser("owner@example.com")
	e.makeUser("viewer@example.com")
	key := e.insertKey(owner, "existing-uuid", "active")
	e.login("viewer@example.com")

	list := e.get("/keys")
	revoke := e.delete("/keys/" + itoa(key.ID))

	if list.Status != 200 || string(list.Raw) != "[]\n" {
		t.Errorf("список должен быть пустым массивом: %d %q", list.Status, list.Raw)
	}
	if revoke.Status != 404 || revoke.Detail() != "Ключ не найден" {
		t.Errorf("чужой ключ: %d %s", revoke.Status, revoke.Raw)
	}
}

func TestAdminCanListAllKeys(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser("admin@example.com", asAdmin)
	owner := e.makeUser("owner@example.com")
	e.insertKey(owner, "admin-visible-uuid", "active")
	e.login(admin.Email)

	resp := e.get("/admin/keys")

	if resp.Status != 200 || resp.List()[0]["uuid"] != "admin-visible-uuid" {
		t.Errorf("%d %s", resp.Status, resp.Raw)
	}
}

func TestAdminCanApproveSuspendRestoreAndUpdateLimit(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("pending@example.com", withStatus(store.UserPending))
	e.login("admin@example.com")
	id := itoa(user.ID)

	approve := e.post("/admin/users/"+id+"/approve", nil)
	limit := e.patch("/admin/users/"+id+"/limits", map[string]int{"key_limit": 8})
	suspend := e.post("/admin/users/"+id+"/suspend", nil)
	restore := e.post("/admin/users/"+id+"/restore", nil)

	for name, want := range map[string]struct {
		resp  response
		field string
		value any
	}{
		"approve": {approve, "status", "approved"},
		"limit":   {limit, "key_limit", float64(8)},
		"suspend": {suspend, "status", "suspended"},
		"restore": {restore, "status", "approved"},
	} {
		if want.resp.Status != 200 || want.resp.JSON()[want.field] != want.value {
			t.Errorf("%s: %d %s", name, want.resp.Status, want.resp.Raw)
		}
	}
}

func TestAuditLogWrittenForCreateAndRevoke(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")

	created := e.post("/keys", map[string]string{"name": "phone"})
	revoked := e.delete("/keys/" + itoa(int64(created.JSON()["id"].(float64))))

	if created.Status != 201 || revoked.Status != 200 || revoked.JSON()["message"] != "Ключ отозван" {
		t.Fatalf("%d %d %s", created.Status, revoked.Status, revoked.Raw)
	}
	if got := e.auditActions(); !reflect.DeepEqual(got, []string{"key.create", "key.revoke"}) {
		t.Errorf("аудит: %v", got)
	}
}

func TestAdminCannotSuspendSelf(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")

	resp := e.post("/admin/users/"+itoa(admin.ID)+"/suspend", nil)

	if resp.Status != 400 || resp.Detail() != "Нельзя заблокировать самого себя" {
		t.Errorf("%d %s", resp.Status, resp.Raw)
	}
}

func TestCreateKeyReturnsThreeLinksAndAddsToXray(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")

	resp := e.post("/keys", map[string]string{"name": "  Мой   телефон "})

	if resp.Status != 201 {
		t.Fatalf("%d %s", resp.Status, resp.Raw)
	}
	body := resp.JSON()
	if body["name"] != "Мой телефон" || body["type"] != "multi" || body["status"] != "active" {
		t.Errorf("тело: %v", body)
	}
	if got := transports(body["links"]); !reflect.DeepEqual(got, []string{"tcp", "grpc", "xhttp"}) {
		t.Errorf("transports = %v", got)
	}
	links := body["links"].([]any)
	if body["connection_uri"] != links[0].(map[string]any)["uri"] {
		t.Error("connection_uri должен совпадать с первой ссылкой")
	}
	if !e.inXray(body["uuid"].(string)) {
		t.Error("ключ не добавлен в Xray")
	}
	for _, field := range []string{"id", "owner_id", "uuid", "sni", "public_key", "short_id", "client_config_json", "created_at",
		"traffic_up", "traffic_down", "last_seen_at", "online", "owner_email"} {
		if _, ok := body[field]; !ok {
			t.Errorf("в ответе нет поля %s", field)
		}
	}
	if body["owner_email"] != nil || body["client_config_json"] != nil || body["online"] != false || body["sni"] != "vpn.example.com" {
		t.Errorf("значения полей: %v", body)
	}
	if _, err := time.Parse(time.RFC3339Nano, body["created_at"].(string)); err != nil || !strings.HasSuffix(body["created_at"].(string), "Z") {
		t.Errorf("created_at = %v", body["created_at"])
	}
}

func TestCreateKeyRejectsBadNames(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	tests := []struct {
		name  string
		value string
	}{
		{"пустое", ""},
		{"только пробелы", "   "},
		{"61 символ", strings.Repeat("x", 61)},
		{"61 кириллический символ", strings.Repeat("ы", 61)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.post("/keys", map[string]string{"name": tt.value})
			if resp.Status != 422 || resp.Detail() == "" {
				t.Errorf("%d %s", resp.Status, resp.Raw)
			}
		})
	}
	if ok := e.post("/keys", map[string]string{"name": strings.Repeat("ы", 60)}); ok.Status != 201 {
		t.Errorf("60 символов допустимо: %d %s", ok.Status, ok.Raw)
	}
}

func TestCreateKeyRollsBackWhenXrayFails(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	e.runner.FailTest = true

	resp := e.post("/keys", map[string]string{"name": "phone"})

	if resp.Status != 502 || resp.Detail() == "" {
		t.Errorf("%d %s", resp.Status, resp.Raw)
	}
	if keys := e.keysInDB(); len(keys) != 0 {
		t.Errorf("в БД остались ключи: %v", keys)
	}
	if got := e.auditActions(); len(got) != 0 {
		t.Errorf("аудит не должен писаться: %v", got)
	}
}

func TestCreateKeyRemovesFromXrayWhenDBFails(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	// ломаем запись в БД: таблица аудита недоступна, вставка ключа откатывается вместе с транзакцией
	if _, err := e.st.DB().Exec("ALTER TABLE audit_logs RENAME TO audit_logs_broken"); err != nil {
		t.Fatal(err)
	}

	resp := e.post("/keys", map[string]string{"name": "phone"})

	if resp.Status != 500 {
		t.Fatalf("%d %s", resp.Status, resp.Raw)
	}
	if keys := e.keysInDB(); len(keys) != 0 {
		t.Errorf("в БД остались ключи: %v", keys)
	}
	cfg := string(e.runner.Files[e.cfg.XrayConfigPath])
	for tag, users := range e.runner.APIUsers {
		if len(users) != 0 {
			t.Errorf("%s: ключ остался в API Xray: %v", tag, users)
		}
	}
	if strings.Count(cfg, `"email"`) != 0 {
		t.Error("ключ остался в конфиге Xray")
	}
}

func TestRevokeRemovesKeyFromXrayAndHidesLinks(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	key := e.post("/keys", map[string]string{"name": "phone"}).JSON()

	revoked := e.delete("/keys/" + itoa(int64(key["id"].(float64))))
	listed := e.get("/keys").List()

	if revoked.Status != 200 {
		t.Fatalf("%d %s", revoked.Status, revoked.Raw)
	}
	if e.inXray(key["uuid"].(string)) {
		t.Error("ключ остался в Xray")
	}
	if listed[0]["status"] != "revoked" || len(listed[0]["links"].([]any)) != 0 {
		t.Errorf("отозванный ключ: %v", listed[0])
	}
}

func TestSuspendDisablesKeysAndRestoreReturnsThem(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com")
	e.login(admin.Email)
	uid := itoa(user.ID)
	created := e.post("/admin/users/"+uid+"/keys", map[string]string{"name": "phone"}).JSON()
	revoked := e.post("/admin/users/"+uid+"/keys", map[string]string{"name": "old"}).JSON()
	e.delete("/admin/keys/" + itoa(int64(revoked["id"].(float64))))
	statuses := func() map[float64]string {
		out := map[float64]string{}
		for _, k := range e.get("/admin/users/" + uid + "/keys").List() {
			out[k["id"].(float64)] = k["status"].(string)
		}
		return out
	}

	suspend := e.post("/admin/users/"+uid+"/suspend", nil)
	afterSuspend := statuses()
	gone := !e.inXray(created["uuid"].(string))
	restore := e.post("/admin/users/"+uid+"/restore", nil)
	afterRestore := statuses()

	cid, rid := created["id"].(float64), revoked["id"].(float64)
	if suspend.Status != 200 || suspend.JSON()["status"] != "suspended" {
		t.Errorf("suspend: %d %s", suspend.Status, suspend.Raw)
	}
	if !reflect.DeepEqual(afterSuspend, map[float64]string{cid: "suspended", rid: "revoked"}) {
		t.Errorf("после блокировки: %v", afterSuspend)
	}
	if !gone {
		t.Error("ключ остался в Xray после блокировки")
	}
	if restore.Status != 200 || restore.JSON()["status"] != "approved" {
		t.Errorf("restore: %d %s", restore.Status, restore.Raw)
	}
	if !reflect.DeepEqual(afterRestore, map[float64]string{cid: "active", rid: "revoked"}) {
		t.Errorf("после восстановления: %v", afterRestore)
	}
	if !e.inXray(created["uuid"].(string)) || e.inXray(revoked["uuid"].(string)) {
		t.Error("состояние Xray после восстановления неверно")
	}
	entries, _ := e.st.ListAudit(context.Background(), 2)
	if entries[0].Action != "user.restore" || entries[0].Metadata != `{"keys_restored":1}` ||
		entries[1].Action != "user.suspend" || entries[1].Metadata != `{"keys_disabled":1}` {
		t.Errorf("аудит блокировки: %+v", entries)
	}
}

func TestAdminCannotCreateKeyForSuspendedUser(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com", withStatus(store.UserSuspended))
	e.login("admin@example.com")

	resp := e.post("/admin/users/"+itoa(user.ID)+"/keys", map[string]string{"name": "x"})

	if resp.Status != 409 || resp.Detail() != "Пользователь заблокирован" {
		t.Errorf("%d %s", resp.Status, resp.Raw)
	}
}

func TestAdminEndpointsErrors(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	e.login("admin@example.com")
	tests := []struct {
		name, method, path string
		body               any
		status             int
		detail             string
	}{
		{"нет пользователя", "POST", "/admin/users/999/approve", nil, 404, "Пользователь не найден"},
		{"нет пользователя для ключей", "GET", "/admin/users/999/keys", nil, 404, "Пользователь не найден"},
		{"нет ключа", "DELETE", "/admin/keys/999", nil, 404, "Ключ не найден"},
		{"плохой id", "POST", "/admin/users/abc/approve", nil, 422, ""},
		{"лимит > 100", "PATCH", "/admin/users/1/limits", map[string]int{"key_limit": 101}, 422, ""},
		{"лимит < 0", "PATCH", "/admin/users/1/limits", map[string]int{"key_limit": -1}, 422, ""},
		{"лимит не число", "PATCH", "/admin/users/1/limits", map[string]string{"key_limit": "много"}, 422, ""},
		{"нет лимита", "PATCH", "/admin/users/1/limits", map[string]string{}, 422, ""},
		{"аудит limit=0", "GET", "/admin/audit?limit=0", nil, 422, ""},
		{"аудит limit=1001", "GET", "/admin/audit?limit=1001", nil, 422, ""},
		{"аудит limit=abc", "GET", "/admin/audit?limit=abc", nil, 422, ""},
		{"неизвестный маршрут", "GET", "/nope", nil, 404, "Not Found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.do(tt.method, tt.path, tt.body)
			if resp.Status != tt.status {
				t.Fatalf("%d %s", resp.Status, resp.Raw)
			}
			if tt.detail != "" && resp.Detail() != tt.detail {
				t.Errorf("detail = %q", resp.Detail())
			}
			if tt.status == 422 && resp.Detail() == "" {
				t.Errorf("detail должен быть строкой: %s", resp.Raw)
			}
		})
	}
}

func TestAccessControl(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.makeUser("pending@example.com", withStatus(store.UserPending))
	tests := []struct {
		name, login, method, path string
		status                    int
		detail                    string
	}{
		{"аноним: ключи", "", "GET", "/keys", 401, "Не выполнен вход"},
		{"аноним: админка", "", "GET", "/admin/users", 401, "Не выполнен вход"},
		{"пользователь: админка", "user@example.com", "GET", "/admin/users", 403, "Нужны права администратора"},
		{"пользователь: импорт", "user@example.com", "POST", "/admin/keys/import", 403, "Нужны права администратора"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnv(t)
			env.makeUser("user@example.com")
			if tt.login != "" {
				env.login(tt.login)
			}
			resp := env.do(tt.method, tt.path, nil)
			if resp.Status != tt.status || resp.Detail() != tt.detail {
				t.Errorf("%d %s", resp.Status, resp.Raw)
			}
		})
	}
	t.Run("неподтверждённый с живой сессией", func(t *testing.T) {
		p := e.makeUser("late@example.com", withStatus(store.UserPending))
		_ = p
		token := pythonToken(t, testSecret, itoa(p.ID), time.Now().Add(time.Hour))
		e.client.Jar.SetCookies(mustURL(e.server.URL), []*httpCookie{{Name: "xray_access_token", Value: token, Path: "/"}})
		resp := e.get("/keys")
		if resp.Status != 403 || resp.Detail() != "Аккаунт не подтверждён" {
			t.Errorf("%d %s", resp.Status, resp.Raw)
		}
		if me := e.get("/auth/me"); me.Status != 200 {
			t.Errorf("/auth/me доступен и неподтверждённому: %d", me.Status)
		}
	})
}

func TestSubscriptionReturnsActiveLinks(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	first := e.post("/keys", map[string]string{"name": "phone"}).JSON()
	second := e.post("/keys", map[string]string{"name": "laptop"}).JSON()
	e.delete("/keys/" + itoa(int64(second["id"].(float64))))
	url := e.get("/keys/subscription").JSON()["url"].(string)
	token := url[strings.LastIndex(url, "/")+1:]

	resp := e.raw("GET", "/sub/"+token, nil)

	if !strings.HasPrefix(url, e.cfg.PublicBaseURL) || len(token) != 32 {
		t.Errorf("url = %s", url)
	}
	if resp.Status != 200 {
		t.Fatalf("%d %s", resp.Status, resp.Raw)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(resp.Raw))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, l := range first["links"].([]any) {
		want = append(want, l.(map[string]any)["uri"].(string))
	}
	if got := strings.Split(string(decoded), "\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("ссылки: %v", got)
	}
	title, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(resp.Header.Get("profile-title"), "base64:"))
	headers := map[string]string{
		"Profile-Update-Interval": "12",
		"Subscription-Userinfo":   "upload=0; download=0; total=0; expire=0",
		"Content-Disposition":     `attachment; filename="vpn.txt"`,
		"Cache-Control":           "no-store",
		"Content-Type":            "text/plain; charset=utf-8",
	}
	for name, want := range headers {
		if got := resp.Header.Get(name); got != want {
			t.Errorf("%s = %q, ожидалось %q", name, got, want)
		}
	}
	if string(title) != "Personal" || !strings.HasPrefix(resp.Header.Get("profile-title"), "base64:") {
		t.Errorf("profile-title = %q", resp.Header.Get("profile-title"))
	}
}

func TestSubscriptionRotateInvalidatesOldToken(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	last := func(r response) string {
		u := r.JSON()["url"].(string)
		return u[strings.LastIndex(u, "/")+1:]
	}
	old := last(e.get("/keys/subscription"))
	again := last(e.get("/keys/subscription"))

	fresh := last(e.post("/keys/subscription/rotate", nil))

	if old != again {
		t.Error("GET не должен менять токен")
	}
	if fresh == old {
		t.Error("токен не изменился")
	}
	if got := e.raw("GET", "/sub/"+old, nil).Status; got != 404 {
		t.Errorf("старый токен: %d", got)
	}
	if got := e.raw("GET", "/sub/"+fresh, nil).Status; got != 200 {
		t.Errorf("новый токен: %d", got)
	}
}

func TestSubscriptionUnknownToken(t *testing.T) {
	for _, token := range []string{"short", strings.Repeat("x", 32)} {
		t.Run(token, func(t *testing.T) {
			e := newEnv(t)
			resp := e.raw("GET", "/sub/"+token, nil)
			if resp.Status != 404 || resp.Detail() != "Not found" {
				t.Errorf("%d %s", resp.Status, resp.Raw)
			}
		})
	}
}

func TestSubscriptionOfSuspendedUserIsHidden(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser("user@example.com")
	e.login("user@example.com")
	url := e.get("/keys/subscription").JSON()["url"].(string)
	token := url[strings.LastIndex(url, "/")+1:]
	if err := e.st.SetUserStatus(context.Background(), user.ID, store.UserSuspended); err != nil {
		t.Fatal(err)
	}

	if got := e.raw("GET", "/sub/"+token, nil).Status; got != 404 {
		t.Errorf("status %d", got)
	}
}

func TestSubscriptionUserinfoSumsTraffic(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser("user@example.com")
	e.login("user@example.com")
	url := e.get("/keys/subscription").JSON()["url"].(string)
	e.insertKey(user, "k-1", "active", func(k *store.VpnKey) { k.TrafficUp, k.TrafficDown = 5, 7 })
	e.insertKey(user, "k-2", "active", func(k *store.VpnKey) { k.TrafficUp, k.TrafficDown = 1, 2 })
	e.insertKey(user, "k-3", "revoked", func(k *store.VpnKey) { k.TrafficUp, k.TrafficDown = 100, 100 })

	resp := e.raw("GET", "/sub/"+url[strings.LastIndex(url, "/")+1:], nil)

	if got := resp.Header.Get("subscription-userinfo"); got != "upload=6; download=9; total=0; expire=0" {
		t.Errorf("userinfo = %q", got)
	}
}

func TestImportServerKeys(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser("admin@example.com", asAdmin)
	e.login(admin.Email)
	existing := e.post("/admin/users/"+itoa(admin.ID)+"/keys", map[string]string{"name": "mine"}).JSON()
	cfg := e.runner.Files[e.cfg.XrayConfigPath]
	cfg = strings.Replace(cfg, `"clients": [`, `"clients": [
          {"id": "`+legacyUUID+`", "email": "first-user", "flow": "xtls-rprx-vision"},`, 1)
	e.runner.Files[e.cfg.XrayConfigPath] = cfg

	first := e.post("/admin/keys/import", nil)
	second := e.post("/admin/keys/import", nil)

	if !reflect.DeepEqual(first.JSON(), map[string]any{"imported": float64(1)}) || !reflect.DeepEqual(second.JSON(), map[string]any{"imported": float64(0)}) {
		t.Fatalf("first=%s second=%s", first.Raw, second.Raw)
	}
	var owner int64
	var name string
	if err := e.st.DB().QueryRow("SELECT owner_id, name FROM vpn_keys WHERE uuid = ?", legacyUUID).Scan(&owner, &name); err != nil {
		t.Fatal(err)
	}
	if owner != admin.ID || name != "Импортирован с сервера 33333333" {
		t.Errorf("owner=%d name=%q", owner, name)
	}
	if !e.inXray(legacyUUID) || !e.inXray(existing["uuid"].(string)) {
		t.Error("после импорта ключи должны быть во всех inbound")
	}
	actions := e.auditActions()
	if actions[len(actions)-1] != "admin.key.import" {
		t.Errorf("аудит: %v", actions)
	}
}

func TestAdminKeysIncludeOwnerEmail(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com")
	e.login("admin@example.com")
	e.post("/admin/users/"+itoa(user.ID)+"/keys", map[string]string{"name": "phone"})

	keys := e.get("/admin/keys").List()
	userKeys := e.get("/admin/users/" + itoa(user.ID) + "/keys").List()

	if keys[0]["owner_email"] != "user@example.com" || userKeys[0]["owner_email"] != "user@example.com" {
		t.Errorf("keys=%v userKeys=%v", keys[0]["owner_email"], userKeys[0]["owner_email"])
	}
}

func TestAdminKeysOrderNewestFirst(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser("admin@example.com", asAdmin)
	e.login(admin.Email)
	for _, name := range []string{"a", "b", "c"} {
		e.post("/admin/users/"+itoa(admin.ID)+"/keys", map[string]string{"name": name})
	}

	var names []string
	for _, k := range e.get("/admin/keys").List() {
		names = append(names, k["name"].(string))
	}

	if !reflect.DeepEqual(names, []string{"c", "b", "a"}) {
		t.Errorf("порядок: %v", names)
	}
}

func TestAdminAuditLog(t *testing.T) {
	e := newEnv(t)
	e.makeUser("admin@example.com", asAdmin)
	user := e.makeUser("user@example.com", withStatus(store.UserPending))
	e.login("admin@example.com")
	e.post("/admin/users/"+itoa(user.ID)+"/approve", nil)

	entries := e.get("/admin/audit").List()

	if entries[0]["action"] != "user.approve" || entries[0]["actor_email"] != "admin@example.com" {
		t.Errorf("запись: %v", entries[0])
	}
	if !reflect.DeepEqual(entries[0]["audit_metadata"], map[string]any{"keys_restored": float64(0)}) {
		t.Errorf("audit_metadata = %v", entries[0]["audit_metadata"])
	}
	if entries[0]["target_type"] != "user" || entries[0]["target_id"] != itoa(user.ID) {
		t.Errorf("цель: %v", entries[0])
	}
	if limited := e.get("/admin/audit?limit=1").List(); len(limited) != 1 {
		t.Errorf("limit=1 вернул %d записей", len(limited))
	}
}

func TestAuditIsAdminOnly(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")

	if got := e.get("/admin/audit").Status; got != 403 {
		t.Errorf("status %d", got)
	}
}

func TestStatsCollectAccumulatesAndMarksOnline(t *testing.T) {
	e := newEnv(t)
	e.makeUser("user@example.com")
	e.login("user@example.com")
	key := e.post("/keys", map[string]string{"name": "phone"}).JSON()
	id, keyUUID := int64(key["id"].(float64)), key["uuid"].(string)
	now := time.Now().UTC()
	ctx := context.Background()

	e.runner.Stats = map[string]int64{"user>>>" + keyUUID + ">>>traffic>>>uplink": 100, "user>>>" + keyUUID + ">>>traffic>>>downlink": 500}
	e.runner.Online = map[string]bool{keyUUID: true}
	updated, err := stats.CollectOnce(ctx, e.st, e.xray, now)
	if err != nil || updated != 1 {
		t.Fatalf("updated=%d err=%v", updated, err)
	}
	e.runner.Stats = map[string]int64{"user>>>" + keyUUID + ">>>traffic>>>downlink": 50}
	e.runner.Online = map[string]bool{}
	if _, err := stats.CollectOnce(ctx, e.st, e.xray, now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	listed := e.get("/keys").List()[0]

	if listed["traffic_up"] != float64(100) || listed["traffic_down"] != float64(550) || listed["online"] != true {
		t.Errorf("ключ: %v", listed)
	}
	if err := e.st.SetKeyLastSeen(ctx, id, now.Add(-time.Duration(e.cfg.OnlineWindowSeconds+1)*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := e.get("/keys").List()[0]["online"]; got != false {
		t.Errorf("online после окна = %v", got)
	}
}

func TestStatsCollectIgnoresUnknownAndFailsOnXrayError(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runner.Stats = map[string]int64{"user>>>nobody>>>traffic>>>uplink": 1}

	updated, err := stats.CollectOnce(ctx, e.st, e.xray, time.Now())
	if err != nil || updated != 0 {
		t.Errorf("updated=%d err=%v", updated, err)
	}
	e.runner.FailAPI = true
	if _, err := stats.CollectOnce(ctx, e.st, e.xray, time.Now()); err == nil {
		t.Error("ожидалась ошибка при недоступном API Xray")
	}
}

func TestSuspendedKeyIsNotOnline(t *testing.T) {
	e := newEnv(t)
	owner := e.makeUser("user@example.com")
	seen := time.Now().UTC()
	e.insertKey(owner, "u-1", store.KeySuspended, func(k *store.VpnKey) { k.Type, k.LastSeenAt = "multi", &seen })
	e.login("user@example.com")

	if got := e.get("/keys").List()[0]["online"]; got != false {
		t.Errorf("online = %v", got)
	}
}

func TestKeyOutWhenLinksUnavailable(t *testing.T) {
	e := newEnv(t)
	owner := e.makeUser("user@example.com")
	e.insertKey(owner, "u-1", store.KeyActive)
	e.login("user@example.com")
	delete(e.runner.Files, e.cfg.XrayConfigPath) // Xray недоступен: ссылки построить нельзя

	resp := e.get("/keys")

	if resp.Status != 200 {
		t.Fatalf("%d %s", resp.Status, resp.Raw)
	}
	if links := resp.List()[0]["links"].([]any); len(links) != 0 {
		t.Errorf("links = %v", links)
	}
}
