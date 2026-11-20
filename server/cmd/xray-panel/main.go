// Команда xray-panel — веб-панель управления ключами: один статический бинарник с вшитым фронтендом.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // часовой пояс из TZ работает и без /usr/share/zoneinfo

	"xraypanel/internal/api"
	"xraypanel/internal/backup"
	"xraypanel/internal/config"
	"xraypanel/internal/monitor"
	"xraypanel/internal/stats"
	"xraypanel/internal/store"
	"xraypanel/internal/telegram"
	"xraypanel/internal/web"
	"xraypanel/internal/xray"
)

// version задаётся при сборке: -ldflags "-X main.version=...".
var version = "dev"

const usage = `Использование: xray-panel [--env-file файл] [команда]

Команды:
  serve            запустить панель (по умолчанию; перед запуском выполняется migrate)
  migrate          создать или обновить схему базы
  seed-admin       создать администратора из ADMIN_EMAIL и ADMIN_PASSWORD
  bootstrap-xray   подготовить конфиг Xray (API, статистика, XHTTP, email = UUID)
  version          показать версию
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("ошибка", "error", err)
		os.Exit(1)
	}
}

// parseArgs выделяет --env-file и команду (флаг допустим до и после команды).
func parseArgs(args []string) (command, envFile string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch {
		case strings.HasPrefix(arg, "-") && name == "env-file":
			if !hasValue {
				if i+1 >= len(args) {
					return "", "", errors.New("--env-file требует путь к файлу")
				}
				i++
				value = args[i]
			}
			envFile = value
		case arg == "-h" || arg == "--help" || arg == "help":
			command = "help"
		case strings.HasPrefix(arg, "-"):
			return "", "", fmt.Errorf("неизвестный флаг %s", arg)
		case command == "":
			command = arg
		default:
			return "", "", fmt.Errorf("лишний аргумент %q", arg)
		}
	}
	if command == "" {
		command = "serve"
	}
	return command, envFile, nil
}

func run(args []string) error {
	command, envFile, err := parseArgs(args)
	if err != nil {
		return err
	}
	switch command {
	case "version":
		fmt.Println(version)
		return nil
	case "help":
		fmt.Print(usage)
		return nil
	case "serve", "migrate", "seed-admin", "bootstrap-xray":
	default:
		return fmt.Errorf("неизвестная команда %q\n\n%s", command, usage)
	}

	cfg, err := config.FromEnv(envFile)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch command {
	case "migrate":
		st, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		return migrate(ctx, st)
	case "seed-admin":
		st, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		if err := migrate(ctx, st); err != nil {
			return err
		}
		created, err := api.SeedAdmin(ctx, st, cfg.AdminEmail, cfg.AdminPassword)
		if err != nil {
			return err
		}
		if created {
			slog.Info("администратор создан", "email", strings.ToLower(cfg.AdminEmail))
		} else {
			slog.Info("администратор не создан: не заданы ADMIN_EMAIL/ADMIN_PASSWORD или он уже существует")
		}
		return nil
	case "bootstrap-xray":
		manager, err := xray.Build(cfg)
		if err != nil {
			return err
		}
		changed, err := manager.Bootstrap()
		if err != nil {
			return err
		}
		if changed {
			fmt.Println("Конфиг Xray обновлён, Xray перезапущен")
		} else {
			fmt.Println("Конфиг Xray уже готов")
		}
		return nil
	default:
		return serve(ctx, cfg)
	}
}

func openStore(cfg *config.Config) (*store.Store, error) {
	path, err := cfg.SQLitePath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

func migrate(ctx context.Context, st *store.Store) error {
	result, err := st.Migrate(ctx)
	if err != nil {
		return err
	}
	slog.Info("миграция", "результат", result)
	return nil
}

func serve(ctx context.Context, cfg *config.Config) error {
	if cfg.JWTSecret == config.DefaultJWTSecret {
		slog.Warn("JWT_SECRET имеет значение по умолчанию: задайте собственный секрет")
	}
	st, err := openStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := migrate(ctx, st); err != nil {
		return err
	}
	manager, err := xray.Build(cfg)
	if err != nil {
		return err
	}

	app := api.New(api.Deps{Config: cfg, Store: st, Xray: manager, Web: web.Handler(web.Dist())})
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}

	statsDone := make(chan struct{})
	statsCtx, cancelStats := context.WithCancel(ctx)
	defer cancelStats()
	if cfg.EffectiveXrayMode() != "dry_run" && cfg.StatsPollSeconds > 0 {
		go func() {
			defer close(statsDone)
			stats.Run(statsCtx, st, manager, time.Duration(cfg.StatsPollSeconds)*time.Second)
		}()
	} else {
		close(statsDone)
	}
	mon := newMonitor(cfg, manager)
	monitorDone := make(chan struct{})
	if mon != nil {
		mon.SetPersister(monitorStore{st})
		if err := mon.Restore(ctx); err != nil {
			slog.Warn("история мониторинга не загружена", "error", err)
		}
		app.SetStatusSource(mon)
		go func() {
			defer close(monitorDone)
			mon.Run(statsCtx)
		}()
	} else {
		close(monitorDone)
	}
	bot := startBot(statsCtx, cfg, app, mon)
	backupDone := startBackup(statsCtx, cfg, st, bot)

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("панель запущена", "addr", cfg.ListenAddr, "version", version, "xray_mode", cfg.EffectiveXrayMode())
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		cancelStats()
		<-statsDone
		<-monitorDone
		<-bot.done
		<-backupDone
		return err
	case <-ctx.Done():
	}
	slog.Info("остановка панели")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	cancelStats()
	<-statsDone
	<-monitorDone
	<-bot.done
	<-backupDone
	return err
}

// newMonitor собирает проверки цепочки и сервера; nil — мониторинг выключен (MONITOR_INTERVAL_SECONDS=0).
func newMonitor(cfg *config.Config, manager *xray.Manager) *monitor.Monitor {
	if cfg.MonitorIntervalSeconds <= 0 {
		return nil
	}
	interval := time.Duration(cfg.MonitorIntervalSeconds) * time.Second
	return monitor.New(monitor.DefaultChecks(monitorDeps(cfg, manager)), interval, cfg.MonitorFailThreshold)
}

// monitorDeps — откуда проверки берут данные; в dry_run Xray нет, остаются только диск и память.
func monitorDeps(cfg *config.Config, manager *xray.Manager) monitor.Deps {
	d := monitor.Deps{DataDir: "."}
	if path, err := cfg.SQLitePath(); err == nil && path != ":memory:" {
		d.DataDir = filepath.Dir(path)
	}
	if cfg.EffectiveXrayMode() == "dry_run" {
		return d
	}
	if cfg.XrayProbePort > 0 {
		d.ProbeAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.XrayProbePort))
	}
	d.ProbeURL = cfg.MonitorProbeURL
	d.ChainSlow = time.Duration(cfg.MonitorChainSlowMs) * time.Millisecond
	d.ExitSlow = time.Duration(cfg.MonitorExitSlowMs) * time.Millisecond
	d.ExitServer = manager.ExitServer
	d.XrayOnline = func() (int, error) {
		online, err := manager.OnlineEmails()
		return len(online), err
	}
	d.CertDomains = certDomains(cfg)
	if len(d.CertDomains) > 0 {
		d.CertAddr = net.JoinHostPort(cfg.XrayPublicHost, "443")
	}
	return d
}

// certDomains — домены с сертификатами Caddy: сайт-заглушка VPN и кабинет (IP-адреса пропускаются).
func certDomains(cfg *config.Config) []string {
	var out []string
	add := func(host string) {
		if host == "" || host == "localhost" || net.ParseIP(host) != nil {
			return
		}
		for _, d := range out {
			if d == host {
				return
			}
		}
		out = append(out, host)
	}
	add(cfg.XrayPublicHost)
	if u, err := url.Parse(cfg.PublicBaseURL); err == nil && u.Scheme == "https" {
		add(u.Hostname())
	}
	return out
}

// startBot запускает Telegram-бота, если задан TELEGRAM_BOT_TOKEN; канал закрывается после остановки.
// mon — источник тревог и кнопки «Состояние»; nil, если мониторинг выключен.
// runningBot — запущенный бот (bot == nil, если он выключен) и канал его завершения.
type runningBot struct {
	bot  *telegram.Bot
	done <-chan struct{}
}

func startBot(ctx context.Context, cfg *config.Config, app *api.App, mon *monitor.Monitor) runningBot {
	done := make(chan struct{})
	if cfg.TelegramBotToken == "" {
		close(done)
		return runningBot{done: done}
	}
	chats, err := cfg.TelegramAdminChats() // формат уже проверен в config.Load
	if err != nil {
		slog.Error("telegram", "error", err)
		close(done)
		return runningBot{done: done}
	}
	bot := telegram.New(telegram.NewClient(cfg.TelegramAPIURL, cfg.TelegramBotToken), chats, app)
	app.SetNotifier(bot)
	if mon != nil { // без проверки в интерфейс попал бы типизированный nil
		bot.SetStatusSource(mon)
		mon.SetNotifier(bot)
	}
	go func() {
		defer close(done)
		bot.Run(ctx)
	}()
	return runningBot{bot: bot, done: done}
}

// backupLabel — короткое имя сервера для названия файла копии: «kemsekov» из «kemsekov.ru»; для IP — пусто.
func backupLabel(host string) string {
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	label, _, _ := strings.Cut(host, ".")
	return label
}

// startBackup запускает ежедневную копию базы в Telegram; без бота, при BACKUP_HOUR=-1
// или базе в памяти копии нет.
func startBackup(ctx context.Context, cfg *config.Config, st *store.Store, rb runningBot) <-chan struct{} {
	done := make(chan struct{})
	path, err := cfg.SQLitePath()
	if rb.bot == nil || cfg.BackupHour < 0 || err != nil || path == ":memory:" {
		close(done)
		return done
	}
	b := backup.New(st, rb.bot, filepath.Dir(path), backupLabel(cfg.XrayPublicHost))
	rb.bot.SetBackup(b.RunOnce)
	go func() {
		defer close(done)
		b.Run(ctx, cfg.BackupHour)
	}()
	return done
}
