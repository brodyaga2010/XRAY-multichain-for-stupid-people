# Личный кабинет (web control plane) для Xray

Кабинет управляет пользователями и их ключами Xray на входном (relay) сервере. Это один бинарник на Go
со встроенным интерфейсом (React): на сервере не нужны ни Python, ни Node, ни Docker.

Установка на сервер описана в [`deploy/README.md`](../deploy/README.md).

## Как это работает

### Роли и статусы

- `admin` — видит всех пользователей и все ключи, подтверждает регистрации, блокирует, меняет лимиты,
  создаёт ключи за пользователя, смотрит журнал действий.
- `user` — видит и создаёт свои ключи в пределах лимита, получает ссылку-подписку.

Новый пользователь регистрируется по email и паролю и получает статус `pending`; войти он сможет после
подтверждения администратором. Статусы: `pending`, `approved`, `suspended`.

Блокировка (`suspend`) сразу убирает все активные ключи пользователя из Xray (ключи получают статус
`suspended`), восстановление (`restore`) возвращает их. Отозванные (`revoked`) ключи не возвращаются.

### Ключи

Ключ — один UUID, который добавляется во все пользовательские inbound relay-сервера. Для каждого ключа
кабинет выдаёт три ссылки `vless://` — пользователь выбирает ту, что лучше работает в его сети:

| Транспорт | Inbound tag | Порт |
|---|---|---|
| TCP + REALITY (xtls-rprx-vision) | `user-inbound` | 443 |
| gRPC + REALITY | `user-inbound-grpc` | 8443 |
| XHTTP + REALITY | `user-inbound-xhttp` | 2053 (`XRAY_XHTTP_PORT`) |

В Xray `email` клиента равен его UUID — по нему считаются трафик и онлайн.

Подписка: `https://<домен кабинета>/sub/<токен>` отдаёт все активные ссылки пользователя (base64),
с заголовками `profile-title`, `profile-update-interval` и `subscription-userinfo` (трафик).
Токен можно перевыпустить — старая ссылка перестанет работать.

### Управление Xray

Кабинет работает на том же сервере, что и Xray (`XRAY_MODE=local`), и не использует старые скрипты
`add_user_*.sh`. При создании или удалении ключа:

1. Под файловой блокировкой читается `/usr/local/etc/xray/config.json`, inbound ищутся по `tag`.
2. Новый конфиг пишется во временный `*.json` рядом и проверяется `xray run -test`.
3. Текущий конфиг копируется в `/var/lib/xray-panel/backups` (хранятся последние 30), новый
   атомарно заменяет старый.
4. Изменение применяется через API Xray (`127.0.0.1:10085`, команды `adu` и `rmu`) — **без перезапуска**,
   соединения остальных пользователей не рвутся.
5. Если API недоступно, кабинет перезапускает Xray (`XRAY_RESTART_COMMAND`, через sudo-правило только на
   `systemctl restart xray`).

Если проверка конфига не прошла, ключ не сохраняется, а пользователь получает ошибку.

Раз в минуту кабинет забирает статистику (`statsquery -reset`, `statsgetallonlineusers`): трафик
накапливается в базе, ключ считается онлайн, если был активен за последние 3 минуты.

Команда `xray-panel bootstrap-xray` готовит конфиг relay: включает API и статистику, добавляет XHTTP
inbound, приводит `email` клиентов к UUID. Ключи, созданные раньше скриптами, администратор подтягивает
кнопкой «Импортировать ключи с сервера».

## Структура

```text
server/                     бэкенд на Go
  cmd/xray-panel/           точка входа: serve, migrate, seed-admin, bootstrap-xray, version
  internal/api/             HTTP-обработчики, авторизация, CORS, журнал запросов
  internal/auth/            bcrypt и JWT (cookie)
  internal/config/          настройки из окружения и env-файла
  internal/ratelimit/       ограничение попыток входа
  internal/stats/           сбор трафика и онлайна
  internal/store/           SQLite: схема, миграции, запросы
  internal/web/             встроенный интерфейс (go:embed) и SPA-fallback
  internal/xray/            правка конфига, API Xray, ссылки vless://
  Makefile                  web / build / test
frontend/                   интерфейс на React + Vite
deploy/install_panel.sh     установка и обновление на relay-сервере
```

## Разработка

Нужны Go 1.26+ и Node 22+.

```bash
# API в режиме dry_run: Xray имитируется в памяти, база — server/panel.db
cp deploy/.env.example deploy/.env
cd server
go run ./cmd/xray-panel --env-file ../deploy/.env seed-admin
go run ./cmd/xray-panel --env-file ../deploy/.env serve
```

```bash
# интерфейс с горячей перезагрузкой; /api и /sub проксируются на 127.0.0.1:8000
cd frontend
npm ci
npm run dev
```

Открыть `http://localhost:5173`, войти как `ADMIN_EMAIL` / `ADMIN_PASSWORD` из `deploy/.env`.

### Сборка

```bash
cd server
make web SITE_NAME=example   # собрать интерфейс и вшить его в бинарник
make test                    # go vet + go test
make build VERSION=v2.0.0    # dist/xray-panel-linux-amd64 и dist/xray-panel-linux-arm64
make web-reset               # вернуть заглушку интерфейса перед коммитом
```

Бинарники статические (`CGO_ENABLED=0`, SQLite — `modernc.org/sqlite`), подходят для любого Linux.
`SITE_NAME` попадает в заголовок страницы («Личный кабинет example»); слов «VPN» в интерфейсе входа нет.

## Настройки

Читаются из окружения процесса; с флагом `--env-file` — ещё и из файла (окружение важнее).

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `DATABASE_URL` | `sqlite:///panel.db` | путь к SQLite: `sqlite:////абсолютный/путь.db` |
| `JWT_SECRET` | `change-me-in-production` | секрет подписи сессий, от 16 символов; своё значение обязательно |
| `ACCESS_TOKEN_MINUTES` | `1440` | время жизни сессии |
| `COOKIE_SECURE` | `false` | `true` на HTTPS |
| `CORS_ORIGINS` | `http://localhost:5173,…` | разрешённые origin через запятую |
| `PUBLIC_BASE_URL` | `http://localhost:5173` | адрес кабинета для ссылок подписки |
| `SUBSCRIPTION_TITLE` | `Personal` | название профиля в клиентах |
| `SUBSCRIPTION_UPDATE_HOURS` | `12` | как часто клиенты обновляют подписку |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD` | — | первый администратор (`seed-admin`) |
| `DEFAULT_KEY_LIMIT` | `3` | лимит ключей нового пользователя |
| `AUTH_RATE_LIMIT`, `AUTH_RATE_WINDOW_SECONDS` | `10`, `300` | попыток входа с одного IP за окно |
| `TRUSTED_PROXIES` | `127.0.0.1,::1` | от кого принимать `X-Forwarded-For` |
| `LISTEN_ADDR` | `127.0.0.1:8000` | адрес HTTP-сервера |
| `XRAY_MODE` | `dry_run` | `local` — Xray на этом сервере; `dry_run` — имитация |
| `XRAY_PUBLIC_HOST` | `127.0.0.1` | домен VPN в ссылках `vless://` |
| `XRAY_CONFIG_PATH` | `/usr/local/etc/xray/config.json` | конфиг Xray |
| `XRAY_BACKUP_DIR`, `XRAY_BACKUP_KEEP` | `/var/lib/xray-panel/backups`, `30` | резервные копии конфига |
| `XRAY_LOCK_PATH` | `/run/xray-panel/config.lock` | файловая блокировка правок |
| `XRAY_BINARY` | `xray` | путь к бинарнику Xray |
| `XRAY_API_SERVER` | `127.0.0.1:10085` | API Xray |
| `XRAY_RESTART_COMMAND` | `systemctl restart xray` | запасной вариант, если API недоступно |
| `XRAY_XHTTP_PORT` | `2053` | порт XHTTP inbound |
| `STATS_POLL_SECONDS`, `ONLINE_WINDOW_SECONDS` | `60`, `180` | сбор статистики и окно «онлайн» |

## API

Всё под `/api`, авторизация — httpOnly cookie с JWT.

```text
GET    /api/health
POST   /api/auth/register | /api/auth/login | /api/auth/logout | /api/auth/password
GET    /api/auth/me

GET    /api/keys                          свои ключи со ссылками, трафиком и онлайном
POST   /api/keys                          создать ключ {"name": "..."}
DELETE /api/keys/{key_id}                 отозвать свой ключ
GET    /api/keys/subscription             ссылка-подписка
POST   /api/keys/subscription/rotate      перевыпустить подписку

GET    /api/admin/users                   пользователи со счётчиками ключей и трафика
GET    /api/admin/users/{user_id}/keys
POST   /api/admin/users/{user_id}/approve | suspend | restore
PATCH  /api/admin/users/{user_id}/limits  {"key_limit": N}
POST   /api/admin/users/{user_id}/keys    создать ключ за пользователя
GET    /api/admin/keys                    все ключи с email владельца
DELETE /api/admin/keys/{key_id}
POST   /api/admin/keys/import             импорт ключей из конфига Xray
GET    /api/admin/audit                   журнал действий

GET    /sub/{token}                       подписка (без авторизации)
```

## База данных

SQLite, таблицы `users`, `vpn_keys`, `audit_logs` и служебная `alembic_version`. Схема совместима с
прежней Python-версией (ревизия `0002_panel_v2`), поэтому обновление не требует переноса данных:
`serve` при старте сам выполняет `migrate`. Пароли — bcrypt.

Резервная копия — просто файл: `sqlite3 /var/lib/xray-panel/panel.db ".backup /root/panel.db"`.

## Диагностика

- Логи кабинета: `journalctl -u xray-panel -f` (каждый запрос: метод, путь, код, время).
- Кабинет не стартует — чаще всего ошибка в `/etc/xray-panel/panel.env`: текст есть в логе.
- Ключ не создаётся (502) — конфиг не прошёл `xray run -test` или нет прав на запись в
  `/usr/local/etc/xray`; подробности в логе, прежний конфиг не тронут.
- Ключ создан, но не подключается — проверьте `xray api statsgetallonlineusers -s 127.0.0.1:10085`
  и что в `config.json` UUID есть во всех трёх inbound.
- Не сохраняется вход — на HTTPS должен быть `COOKIE_SECURE=true`, а открывать кабинет нужно по
  адресу из `PUBLIC_BASE_URL`.
