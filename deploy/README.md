# Xray Control Plane Deploy

## Установка на входной (relay) сервер — рекомендуемый вариант

Панель ставится на тот же сервер, где работает Xray-relay, и управляет им локально
(без SSH-паролей и ключей). Docker не нужен: бэкенд — systemd-сервис, база — SQLite,
фронтенд — статика, которую раздаёт Caddy.

Требования: Xray и Caddy уже настроены скриптами `generate_config.sh` и `setup-caddy.sh`,
A-запись домена панели указывает на сервер.

```bash
# на сервере, из корня репозитория
sudo ./deploy/install_panel.sh \
  --panel-domain panel.example.com \
  --vpn-domain example.com \
  --admin-email admin@example.com \
  --site-name example
```

Повторный запуск того же скрипта — обновление (настройки и база сохраняются).

Что получится:

- `https://panel.example.com` — личный кабинет; `https://example.com` — сайт-заглушка.
  Оба сайта слушают только `127.0.0.1:8444` и доступны снаружи через Xray REALITY на 443
  (реальный IP посетителя передаётся по PROXY protocol).
- Ключ = UUID во всех inbound: TCP 443, gRPC 8443, XHTTP 2053 (порт меняется `--xhttp-port`).
- Ключи добавляются и удаляются через API Xray (`127.0.0.1:10085`) без перезапуска —
  соединения других пользователей не рвутся. Конфиг сохраняется с проверкой `xray run -test`,
  резервные копии — `/var/lib/xray-panel/backups`.
- Блокировка пользователя сразу отключает его ключи, восстановление — возвращает.
- Трафик и онлайн по ключам собираются раз в минуту.
- Настройки и секреты: `/etc/xray-panel/panel.env` (пароль первого администратора печатается
  при первой установке и лежит там же — смените его в разделе «Профиль»).
- Логи: `journalctl -u xray-panel`.

Ключи, созданные раньше скриптами, подтягиваются кнопкой «Импортировать ключи с сервера».

## Local start (разработка, dry-run)

```bash
cd deploy
cp .env.example .env
mkdir -p secrets
printf "dry-run" > secrets/xray_ssh_key
docker compose up --build
```

Open `http://localhost:5173`.

`XRAY_MODE=dry_run` (по умолчанию) — ключи создаются без реального сервера, в памяти.

## Панель на отдельной машине (SSH)

`XRAY_MODE=ssh`, `XRAY_RELAY_HOST`, `XRAY_RELAY_USER`, приватный ключ в `deploy/secrets/xray_ssh_key`,
`XRAY_SSH_KNOWN_HOSTS` — файл known_hosts для проверки ключа сервера. Конфиг relay готовится командой
`python -m app.xray_bootstrap` (API, статистика, XHTTP).
