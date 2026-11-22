package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedStubIsServed(t *testing.T) {
	h := Handler(Dist())

	for _, path := range []string{"/", "/any/spa/route", "/index.html"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Личный кабинет</title>") {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestContentTypes(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":        {Data: []byte("x")},
		"assets/app.js":     {Data: []byte("x")},
		"assets/app.css":    {Data: []byte("x")},
		"assets/font.woff2": {Data: []byte("x")},
		"favicon.svg":       {Data: []byte("x")},
		"unknown.zzz":       {Data: []byte("x")},
	}
	tests := []struct{ path, want string }{
		{"/assets/app.js", "text/javascript; charset=utf-8"},
		{"/assets/app.css", "text/css; charset=utf-8"},
		{"/assets/font.woff2", "font/woff2"},
		{"/favicon.svg", "image/svg+xml"},
		{"/unknown.zzz", "application/octet-stream"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		Handler(fsys).ServeHTTP(rec, httptest.NewRequest("GET", tt.path, nil))
		if got := rec.Header().Get("Content-Type"); got != tt.want {
			t.Errorf("%s: Content-Type = %q, ожидалось %q", tt.path, got, tt.want)
		}
	}
	rec := httptest.NewRecorder()
	Handler(fsys).ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.svg", nil))
	if rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestPathTraversalIsContained(t *testing.T) {
	fsys := fstest.MapFS{"index.html": {Data: []byte("index")}}
	rec := httptest.NewRecorder()

	Handler(fsys).ServeHTTP(rec, httptest.NewRequest("GET", "/../../etc/passwd.txt", nil))

	if rec.Code != 404 {
		t.Errorf("status %d", rec.Code)
	}
}

func TestMissingIndexGives404(t *testing.T) {
	rec := httptest.NewRecorder()

	Handler(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 404 {
		t.Errorf("status %d", rec.Code)
	}
}
