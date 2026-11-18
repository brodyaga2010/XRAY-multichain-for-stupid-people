// Команда xray-panel — веб-панель управления ключами: один статический бинарник с вшитым фронтендом.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // часовой пояс из TZ работает и без /usr/share/zoneinfo

	"xraypanel/internal/api"
	"xraypanel/internal/config"
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
	botDone := startBot(statsCtx, cfg, app)

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("панель запущена", "addr", cfg.ListenAddr, "version", version, "xray_mode", cfg.EffectiveXrayMode())
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		cancelStats()
		<-statsDone
		<-botDone
		return err
	case <-ctx.Done():
	}
	slog.Info("остановка панели")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	cancelStats()
	<-statsDone
	<-botDone
	return err
}

// startBot запускает Telegram-бота, если задан TELEGRAM_BOT_TOKEN; канал закрывается после остановки.
func startBot(ctx context.Context, cfg *config.Config, app *api.App) <-chan struct{} {
	done := make(chan struct{})
	if cfg.TelegramBotToken == "" {
		close(done)
		return done
	}
	chats, err := cfg.TelegramAdminChats() // формат уже проверен в config.Load
	if err != nil {
		slog.Error("telegram", "error", err)
		close(done)
		return done
	}
	bot := telegram.New(telegram.NewClient(cfg.TelegramAPIURL, cfg.TelegramBotToken), chats, app)
	app.SetNotifier(bot)
	go func() {
		defer close(done)
		bot.Run(ctx)
	}()
	return done
}
