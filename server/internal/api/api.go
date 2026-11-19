// Package api — HTTP API панели: маршруты, обработчики, middleware.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"xraypanel/internal/auth"
	"xraypanel/internal/config"
	"xraypanel/internal/ratelimit"
	"xraypanel/internal/store"
	"xraypanel/internal/xray"
)

// Deps — зависимости API.
type Deps struct {
	Config *config.Config
	Store  *store.Store
	Xray   *xray.Manager
	// Web отдаёт вшитый фронтенд; nil — только API.
	Web http.Handler
}

// App — HTTP-приложение.
type App struct {
	cfg       *config.Config
	st        *store.Store
	xray      *xray.Manager
	web       http.Handler
	limiter   *ratelimit.Limiter
	cookie    auth.CookieOptions
	trusted   map[string]struct{}
	origins   map[string]struct{}
	anyOrigin bool
	log       *slog.Logger
	notifier  RegistrationNotifier
	status    StatusSource
}

// New создаёт приложение.
func New(d Deps) *App {
	cfg := d.Config
	a := &App{
		cfg:     cfg,
		st:      d.Store,
		xray:    d.Xray,
		web:     d.Web,
		limiter: ratelimit.New(cfg.AuthRateLimit, time.Duration(cfg.AuthRateWindowSeconds)*time.Second),
		cookie: auth.CookieOptions{
			Name:   cfg.CookieName,
			Domain: cfg.CookieDomain,
			Secure: cfg.CookieSecure,
			MaxAge: time.Duration(cfg.AccessTokenMinutes) * time.Minute,
		},
		trusted: cfg.TrustedProxyList(),
		origins: map[string]struct{}{},
		log:     slog.Default(),
	}
	for _, origin := range cfg.CORSOriginList() {
		if origin == "*" {
			a.anyOrigin = true
		}
		a.origins[origin] = struct{}{}
	}
	return a
}

// Limiter — ограничитель попыток входа (нужен тестам).
func (a *App) Limiter() *ratelimit.Limiter { return a.limiter }

// Handler собирает маршрутизатор. API доступно под /api, подписка — на /sub/{token}.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h handlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.Handle(method+" /api"+path, a.handle(h))
	}

	mux.Handle("GET /health", a.handle(health))
	api("GET /health", health)
	mux.Handle("GET /sub/{token}", a.handle(a.subscription))

	api("POST /auth/register", a.register)
	api("POST /auth/login", a.login)
	api("POST /auth/logout", a.logout)
	api("GET /auth/me", a.me)
	api("POST /auth/password", a.changePassword)

	api("GET /keys", a.listKeys)
	api("POST /keys", a.createKey)
	api("GET /keys/subscription", a.getSubscription)
	api("POST /keys/subscription/rotate", a.rotateSubscription)
	api("DELETE /keys/{key_id}", a.revokeOwnKey)

	api("GET /admin/users", a.adminListUsers)
	api("GET /admin/users/{user_id}/keys", a.adminListUserKeys)
	api("GET /admin/keys", a.adminListKeys)
	api("POST /admin/keys/import", a.adminImportKeys)
	api("POST /admin/users/{user_id}/approve", a.adminApprove)
	api("POST /admin/users/{user_id}/restore", a.adminRestore)
	api("POST /admin/users/{user_id}/suspend", a.adminSuspend)
	api("PATCH /admin/users/{user_id}/limits", a.adminUpdateLimits)
	api("POST /admin/users/{user_id}/keys", a.adminCreateKey)
	api("DELETE /admin/keys/{key_id}", a.adminRevokeKey)
	api("GET /admin/audit", a.adminAudit)
	api("GET /admin/status", a.adminStatus)

	mux.Handle("/api/", a.handle(func(http.ResponseWriter, *http.Request) error {
		return httpErr(http.StatusNotFound, "Not Found")
	}))
	if a.web != nil {
		mux.Handle("/", a.web)
	}
	return a.recoverer(a.cors(a.accessLog(mux)))
}

func health(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	return nil
}

// ---------- ошибки и JSON ----------

type apiError struct {
	Status int
	Detail string
}

func (e *apiError) Error() string { return e.Detail }

func httpErr(status int, detail string) error { return &apiError{Status: status, Detail: detail} }

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (a *App) handle(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		var ae *apiError
		var gw *xray.GatewayError
		switch {
		case errors.As(err, &ae):
			writeJSON(w, ae.Status, map[string]string{"detail": ae.Detail})
		case errors.As(err, &gw):
			a.log.Warn("Ошибка Xray", "path", logPath(r), "error", gw.Detail)
			writeJSON(w, http.StatusBadGateway, map[string]string{"detail": gw.Detail})
		default:
			a.log.Error("Внутренняя ошибка", "method", r.Method, "path", logPath(r), "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Internal Server Error"})
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// decodeBody разбирает JSON-тело запроса; при ошибке — 422.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return httpErr(http.StatusUnprocessableEntity, "Некорректное тело запроса")
	}
	return nil
}

// logPath скрывает токен подписки в логах.
func logPath(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/sub/") {
		return "/sub/***"
	}
	return r.URL.Path
}

// ---------- middleware ----------

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *App) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/sub/") {
			a.log.Info("http", "method", r.Method, "path", logPath(r), "status", sw.status,
				"ms", time.Since(start).Milliseconds())
		}
	})
}

func (a *App) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
				a.log.Error("panic в обработчике", "method", r.Method, "path", logPath(r), "panic", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Internal Server Error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors повторяет поведение CORSMiddleware прежней версии (разрешённые origin, credentials).
func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		_, listed := a.origins[origin]
		allowed := listed || a.anyOrigin
		h := w.Header()
		h.Add("Vary", "Origin")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			if !allowed {
				http.Error(w, "Disallowed CORS origin", http.StatusBadRequest)
				return
			}
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Methods", "DELETE, GET, HEAD, OPTIONS, PATCH, POST, PUT")
			h.Set("Access-Control-Max-Age", "600")
			if requested := r.Header.Get("Access-Control-Request-Headers"); requested != "" {
				h.Set("Access-Control-Allow-Headers", requested)
				h.Add("Vary", "Access-Control-Request-Headers")
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
		if allowed {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- текущий пользователь ----------

func (a *App) currentUser(r *http.Request) (*store.User, error) {
	cookie, err := r.Cookie(a.cfg.CookieName)
	if err != nil || cookie.Value == "" {
		return nil, httpErr(http.StatusUnauthorized, "Не выполнен вход")
	}
	id, err := auth.ParseToken(a.cfg.JWTSecret, cookie.Value)
	if err != nil {
		return nil, httpErr(http.StatusUnauthorized, "Invalid session")
	}
	user, err := a.st.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpErr(http.StatusUnauthorized, "Пользователь не найден")
	}
	return user, err
}

func (a *App) approvedUser(r *http.Request) (*store.User, error) {
	user, err := a.currentUser(r)
	if err != nil {
		return nil, err
	}
	if user.Status != store.UserApproved {
		return nil, httpErr(http.StatusForbidden, "Аккаунт не подтверждён")
	}
	return user, nil
}

func (a *App) adminUser(r *http.Request) (*store.User, error) {
	user, err := a.approvedUser(r)
	if err != nil {
		return nil, err
	}
	if user.Role != store.RoleAdmin {
		return nil, httpErr(http.StatusForbidden, "Нужны права администратора")
	}
	return user, nil
}

// limitAuth ограничивает попытки входа/регистрации/смены пароля с одного IP.
func (a *App) limitAuth(r *http.Request) error {
	key := strings.TrimPrefix(r.URL.Path, "/api") + ":" + ratelimit.ClientIP(r, a.trusted)
	if !a.limiter.Hit(key) {
		return httpErr(http.StatusTooManyRequests, "Слишком много попыток, попробуйте через несколько минут")
	}
	return nil
}

// detached — контекст без отмены: операции над Xray и БД не должны обрываться на полпути
// из-за закрытого клиентом соединения.
func detached(r *http.Request) context.Context { return context.WithoutCancel(r.Context()) }
