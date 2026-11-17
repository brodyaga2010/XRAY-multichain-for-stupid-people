package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/api/health", "/health"} {
		resp := e.raw("GET", path, nil)
		if resp.Status != 200 || strings.TrimSpace(string(resp.Raw)) != `{"status":"ok"}` {
			t.Errorf("%s: %d %s", path, resp.Status, resp.Raw)
		}
		if resp.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s: Content-Type %q", path, resp.Header.Get("Content-Type"))
		}
	}
}

func TestStaticFrontend(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name, method, path string
		status             int
		contentType        string
		body               string
		cache              string
	}{
		{"корень — index.html", "GET", "/", 200, "text/html; charset=utf-8", "<title>Panel</title>", "no-cache"},
		{"маршрут SPA — index.html", "GET", "/some/route", 200, "text/html; charset=utf-8", "<title>Panel</title>", "no-cache"},
		{"глубокий маршрут SPA", "GET", "/admin/users/12", 200, "text/html; charset=utf-8", "<title>Panel</title>", "no-cache"},
		{"js с долгим кэшем", "GET", "/assets/x.js", 200, "text/javascript; charset=utf-8", "console.log(1)", "public, max-age=31536000, immutable"},
		{"HEAD без тела", "HEAD", "/", 200, "text/html; charset=utf-8", "", "no-cache"},
		{"нет такого файла", "GET", "/assets/missing.js", 404, "", "", ""},
		{"POST на статику", "POST", "/", 405, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := e.raw(tt.method, tt.path, nil)
			if resp.Status != tt.status {
				t.Fatalf("status %d", resp.Status)
			}
			if tt.contentType != "" && resp.Header.Get("Content-Type") != tt.contentType {
				t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
			}
			if tt.cache != "" && resp.Header.Get("Cache-Control") != tt.cache {
				t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
			}
			if tt.status == 200 && !strings.Contains(string(resp.Raw), tt.body) {
				t.Errorf("тело: %q", resp.Raw)
			}
			if got := resp.Header.Get("Server"); got != "" {
				t.Errorf("заголовок Server не должен выставляться: %q", got)
			}
		})
	}
}

func TestStaticResponsesDoNotMentionInternals(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/", "/x/y", "/assets/x.js", "/api/unknown"} {
		resp := e.raw("GET", path, nil)
		all := strings.ToLower(string(resp.Raw))
		for name, values := range resp.Header {
			all += " " + strings.ToLower(name+strings.Join(values, ""))
		}
		for _, word := range []string{"vpn", "xray"} {
			if strings.Contains(all, word) {
				t.Errorf("%s: в ответе встречается %q", path, word)
			}
		}
	}
}

func TestUnknownAPIPathIsJSON404NotSPA(t *testing.T) {
	e := newEnv(t)

	resp := e.raw("GET", "/api/nothing/here", nil)

	if resp.Status != 404 || resp.Detail() != "Not Found" {
		t.Errorf("%d %s", resp.Status, resp.Raw)
	}
}

func TestSubscriptionIsServedWithoutAPIPrefixOnly(t *testing.T) {
	e := newEnv(t)

	if got := e.raw("GET", "/sub/"+strings.Repeat("x", 32), nil); got.Status != 404 || got.Detail() != "Not found" {
		t.Errorf("/sub: %d %s", got.Status, got.Raw)
	}
	if got := e.raw("GET", "/api/sub/"+strings.Repeat("x", 32), nil); got.Detail() != "Not Found" {
		t.Errorf("/api/sub не должен быть маршрутом подписки: %d %s", got.Status, got.Raw)
	}
}

func TestCORS(t *testing.T) {
	e := newEnv(t)
	do := func(method, origin string, extra map[string]string) *http.Response {
		req, _ := http.NewRequest(method, e.server.URL+"/api/health", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	allowed := do("GET", "http://localhost:5173", nil)
	denied := do("GET", "http://evil.example.com", nil)
	preflight := do("OPTIONS", "http://localhost:8080", map[string]string{
		"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "content-type",
	})
	badPreflight := do("OPTIONS", "http://evil.example.com", map[string]string{"Access-Control-Request-Method": "POST"})
	noOrigin := do("GET", "", nil)

	if allowed.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" || allowed.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("разрешённый origin: %v", allowed.Header)
	}
	if denied.StatusCode != 200 || denied.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("чужой origin: %d %v", denied.StatusCode, denied.Header)
	}
	if preflight.StatusCode != 200 || preflight.Header.Get("Access-Control-Allow-Origin") != "http://localhost:8080" ||
		preflight.Header.Get("Access-Control-Allow-Headers") != "content-type" ||
		!strings.Contains(preflight.Header.Get("Access-Control-Allow-Methods"), "PATCH") {
		t.Errorf("preflight: %d %v", preflight.StatusCode, preflight.Header)
	}
	if badPreflight.StatusCode != 400 {
		t.Errorf("preflight с чужого origin: %d", badPreflight.StatusCode)
	}
	if noOrigin.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("без Origin заголовков CORS быть не должно: %v", noOrigin.Header)
	}
}
