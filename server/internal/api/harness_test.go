package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"xraypanel/internal/api"
	"xraypanel/internal/auth"
	"xraypanel/internal/config"
	"xraypanel/internal/store"
	"xraypanel/internal/web"
	"xraypanel/internal/xray"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	auth.Cost = 4 // быстрые хеши в тестах; боевое значение (12) проверяется в пакете auth
	os.Exit(m.Run())
}

const testSecret = "test-secret-that-is-long-enough"

type env struct {
	t      *testing.T
	cfg    *config.Config
	st     *store.Store
	xray   *xray.Manager
	runner *xray.MemoryRunner
	app    *api.App
	server *httptest.Server
	client *http.Client
}

func newEnv(t *testing.T, extra ...map[string]string) *env {
	t.Helper()
	vars := map[string]string{
		"DATABASE_URL":     "sqlite:///" + filepath.Join(t.TempDir(), "panel.db"),
		"JWT_SECRET":       testSecret,
		"XRAY_MODE":        "dry_run",
		"XRAY_PUBLIC_HOST": "vpn.example.com",
	}
	for _, e := range extra {
		for k, v := range e {
			vars[k] = v
		}
	}
	cfg, err := config.Load(func(key string) (string, bool) { v, ok := vars[key]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	path, err := cfg.SQLitePath()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager, err := xray.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	staticFS := fstest.MapFS{
		"index.html":  {Data: []byte("<!doctype html><title>Panel</title>")},
		"assets/x.js": {Data: []byte("console.log(1)")},
	}
	app := api.New(api.Deps{Config: cfg, Store: st, Xray: manager, Web: web.Handler(staticFS)})
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	return &env{
		t: t, cfg: cfg, st: st, xray: manager, runner: manager.Runner().(*xray.MemoryRunner), app: app,
		server: server, client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

type response struct {
	Status  int
	Header  http.Header
	Raw     []byte
	Cookies []*http.Cookie
}

func (r response) JSON() map[string]any {
	var out map[string]any
	if err := json.Unmarshal(r.Raw, &out); err != nil {
		panic("не JSON-объект: " + string(r.Raw))
	}
	return out
}

func (r response) List() []map[string]any {
	var out []map[string]any
	if err := json.Unmarshal(r.Raw, &out); err != nil {
		panic("не JSON-массив: " + string(r.Raw))
	}
	return out
}

func (r response) Detail() string {
	d, _ := r.JSON()["detail"].(string)
	return d
}

// do выполняет запрос к API (путь без префикса /api).
func (e *env) do(method, path string, body any) response {
	e.t.Helper()
	return e.raw(method, "/api"+path, body)
}

func (e *env) raw(method, path string, body any) response {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return response{Status: resp.StatusCode, Header: resp.Header, Raw: raw, Cookies: resp.Cookies()}
}

func (e *env) get(path string) response             { e.t.Helper(); return e.do("GET", path, nil) }
func (e *env) post(path string, body any) response  { e.t.Helper(); return e.do("POST", path, body) }
func (e *env) delete(path string) response          { e.t.Helper(); return e.do("DELETE", path, nil) }
func (e *env) patch(path string, body any) response { e.t.Helper(); return e.do("PATCH", path, body) }

func (e *env) makeUser(email string, opts ...func(*store.User)) *store.User {
	e.t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		e.t.Fatal(err)
	}
	u := &store.User{Email: email, PasswordHash: hash, Role: store.RoleUser, Status: store.UserApproved, KeyLimit: 3}
	for _, o := range opts {
		o(u)
	}
	if err := e.st.CreateUser(context.Background(), u); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func asAdmin(u *store.User)                 { u.Role = store.RoleAdmin; u.KeyLimit = 100 }
func withStatus(s string) func(*store.User) { return func(u *store.User) { u.Status = s } }
func withLimit(n int) func(*store.User)     { return func(u *store.User) { u.KeyLimit = n } }

func (e *env) login(email string, password ...string) {
	e.t.Helper()
	pw := "password123"
	if len(password) > 0 {
		pw = password[0]
	}
	resp := e.post("/auth/login", map[string]string{"email": email, "password": pw})
	if resp.Status != 200 {
		e.t.Fatalf("вход %s: %d %s", email, resp.Status, resp.Raw)
	}
}

// inXray — ключ присутствует во всех трёх пользовательских inbound сохранённого конфига.
func (e *env) inXray(keyUUID string) bool {
	e.t.Helper()
	cfg, err := xray.ParseConfig([]byte(e.runner.Files[e.cfg.XrayConfigPath]))
	if err != nil {
		e.t.Fatal(err)
	}
	var tags []string
	for _, inbound := range xray.UserInbounds(cfg) {
		settings, _ := inbound["settings"].(map[string]any)
		clients, _ := settings["clients"].([]any)
		for _, c := range clients {
			if cm, _ := c.(map[string]any); cm["id"] == keyUUID {
				tags = append(tags, inbound["tag"].(string))
			}
		}
	}
	return strings.Join(tags, ",") == "user-inbound,user-inbound-grpc,user-inbound-xhttp"
}

func (e *env) keysInDB() []store.VpnKey {
	e.t.Helper()
	rows, err := e.st.DB().Query("SELECT id FROM vpn_keys")
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var keys []store.VpnKey
	for rows.Next() {
		var k store.VpnKey
		_ = rows.Scan(&k.ID)
		keys = append(keys, k)
	}
	return keys
}

func (e *env) auditActions() []string {
	e.t.Helper()
	entries, err := e.st.ListAudit(context.Background(), 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	var actions []string
	for i := len(entries) - 1; i >= 0; i-- {
		actions = append(actions, entries[i].Action)
	}
	return actions
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func stringsReader(s string) io.Reader { return strings.NewReader(s) }

type httpCookie = http.Cookie

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
