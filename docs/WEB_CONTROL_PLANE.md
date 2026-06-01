# Web Control Plane для Xray VPN

Документ описывает новую систему управления пользователями и VPN-ключами в этом репозитории.

## Что добавлено

Проект теперь состоит из двух частей:

- Legacy shell-скрипты в корне репозитория: установка Xray, генерация relay/exit конфигов, ручное добавление пользователей.
- Новый web control plane:
  - `backend/` - FastAPI backend на Python.
  - `frontend/` - React + Vite frontend.
  - `deploy/` - Docker Compose для Postgres, backend и frontend.

Control plane управляет пользовательскими ключами Xray через relay-сервер. Пользовательские доступы добавляются и удаляются из `/usr/local/etc/xray/config.json` на relay через SSH.

## Как это работает

### Роли

Есть две роли:

- `admin` - администратор сервиса.
- `user` - обычный пользователь.

Новый пользователь регистрируется через email/password и получает статус `pending`. Он не может войти в приложение, пока администратор не подтвердит его.

Статусы пользователей:

- `pending` - ожидает подтверждения.
- `approved` - может входить и управлять ключами.
- `suspended` - заблокирован.

### Ключи

В v1 поддерживаются два типа пользовательских ключей:

- `tcp_reality` - VLESS REALITY TCP, relay inbound `user-inbound`, порт `443`.
- `xhttp_reality` - VLESS REALITY XHTTP, relay inbound `user-inbound-xhttp`, порт `80`.

Для каждого ключа backend хранит в Postgres:

- UUID ключа.
- владельца;
- имя ключа;
- тип;
- SNI;
- статус `active` или `revoked`;
- VLESS connection URI;
- XHTTP JSON client config, если ключ XHTTP.

### Управление Xray

Backend не вызывает старые `add_user_*.sh` скрипты. Вместо этого он использует `XrayManager`:

1. Подключается к relay-серверу по SSH.
2. Читает `/usr/local/etc/xray/config.json`.
3. Находит inbound по `tag`, а не по индексу массива:
   - `user-inbound`;
   - `user-inbound-xhttp`.
4. Добавляет или удаляет UUID в `settings.clients`.
5. Загружает временный конфиг на relay.
6. Проверяет его командой:

```bash
xray run -test -config /tmp/xray-control-config.tmp
```

7. Делает backup текущего конфига.
8. Заменяет config.
9. Перезапускает Xray:

```bash
systemctl restart xray
systemctl is-active --quiet xray
```

Если проверка конфига или рестарт не проходит, backend возвращает ошибку и не записывает ключ в базу как успешно созданный.

## Структура проекта

```text
backend/
  app/
    routers/          FastAPI routes: auth, keys, admin
    services/         XrayManager
    models.py         SQLAlchemy models
    schemas.py        Pydantic schemas
    security.py       password hashing + JWT cookies
    seed_admin.py     создание первого администратора
  alembic/            миграции БД
  tests/              backend tests

frontend/
  src/
    App.tsx           основной dashboard
    api.ts            typed API client
    styles.css        UI стили

deploy/
  docker-compose.yml
  .env.example
  README.md
```

## Локальный запуск в dry-run режиме

Dry-run режим нужен для разработки без реального VPN-сервера. Backend будет создавать ключи в памяти mock Xray manager, а не ходить на relay.

### Требования

- Docker Desktop.
- Свободные порты:
  - `5173` для frontend;
  - `8000` для backend.

Postgres наружу не публикуется, поэтому порт `5432` на хосте не нужен.

### Команды запуска

Из корня репозитория:

```bash
cd deploy
cp .env.example .env
mkdir -p secrets
printf "dry-run" > secrets/xray_ssh_key
docker compose up --build
```

На Windows PowerShell:

```powershell
cd deploy
Copy-Item .env.example .env
New-Item -ItemType Directory -Force secrets
Set-Content -NoNewline -Path secrets\xray_ssh_key -Value "dry-run"
docker compose up --build
```

После запуска:

- frontend: `http://localhost:5173`
- backend healthcheck: `http://localhost:8000/health`
- OpenAPI docs: `http://localhost:8000/docs`

### Логин первого администратора

По умолчанию из `deploy/.env.example`:

```text
admin@example.com
change-me-admin-password
```

Перед реальным использованием пароль обязательно поменять.

## Основные сценарии

### Регистрация пользователя

1. Пользователь открывает `http://localhost:5173`.
2. Выбирает `Register`.
3. Вводит email и пароль.
4. Backend создает пользователя со статусом `pending`.
5. Пользователь видит экран ожидания подтверждения.

### Подтверждение пользователя администратором

1. Admin входит в приложение.
2. Открывает раздел `Admin`.
3. Видит пользователя со статусом `pending`.
4. Нажимает `Approve`.
5. Пользователь получает статус `approved` и может войти.

### Лимиты ключей

У каждого пользователя есть `key_limit`.

По умолчанию:

```text
DEFAULT_KEY_LIMIT=3
```

Администратор может менять лимит в разделе `Admin`.

Обычный пользователь не может создать больше активных ключей, чем разрешено лимитом.

### Dashboard и поиск

Для администратора доступна отдельная вкладка `Dashboard`. Она показывает:

- количество активных ключей;
- пользователей в ожидании подтверждения;
- приостановленных пользователей;
- суммарную емкость по лимитам ключей;
- recent users;
- общий список ключей.

Вкладки `Users` и `Keys` имеют локальный поиск:

- `Search users` ищет по email, роли, статусу и лимиту;
- `Search keys` ищет по имени ключа, UUID, SNI, типу и статусу.

Frontend адаптирован под мобильные устройства:

- на desktop используется sidebar;
- на телефонах используется bottom navigation;
- таблицы превращаются в карточки с touch-friendly actions;
- анимации отключаются автоматически при `prefers-reduced-motion`.

### Создание ключа пользователем

1. Пользователь открывает раздел `Keys`.
2. Вводит имя ключа.
3. Выбирает тип:
   - `TCP REALITY`;
   - `XHTTP REALITY`.
4. Опционально вводит SNI.
5. Нажимает `Create key`.

Если SNI не указан, backend использует первый SNI из `serverNames` соответствующего inbound в Xray config.

Если SNI указан, но его нет в `serverNames`, backend вернет ошибку.

### Отзыв ключа

При отзыве ключа:

1. Backend удаляет UUID из Xray config на relay.
2. Проверяет новый config.
3. Перезапускает Xray.
4. В базе ключ получает статус `revoked`.

Удаление сделано идемпотентным на уровне Xray: если UUID уже отсутствует, config все равно приводится к нужному состоянию.

## Подготовка relay-сервера

Backend ожидает, что на relay уже установлен Xray и существует рабочий config.

Минимальные требования к relay:

- Linux server с systemd.
- Xray установлен как сервис `xray`.
- SSH-доступ по приватному ключу.
- Пользователь SSH имеет права:
  - читать `/usr/local/etc/xray/config.json`;
  - писать туда новый config;
  - запускать `xray run -test`;
  - выполнять `systemctl restart xray`;
  - выполнять `systemctl is-active xray`.

Проще всего использовать `root`, но для production лучше создать отдельного пользователя с ограниченным `sudo`.

### Обязательные inbound tags

Relay config должен содержать:

```json
{
  "tag": "user-inbound"
}
```

для TCP REALITY и:

```json
{
  "tag": "user-inbound-xhttp"
}
```

для XHTTP REALITY.

Backend ищет inbounds именно по этим tag.

### Генерация relay config legacy-скриптом

Скрипт `generate_config.sh` был поправлен под web control plane:

- закрыт heredoc relay-конфига;
- исправлен невалидный JSON в XHTTP inbound;
- `serverName` теперь пишется строкой;
- XHTTP link использует порт `80`;
- relay inbounds имеют нужные tags.

Порядок legacy setup:

```bash
./install_xray.sh <exit_ip> root "<exit_pass>"
./install_xray.sh <relay_ip> root "<relay_pass>"
./generate_config.sh <exit_ip> root "<exit_pass>" <relay_ip> root "<relay_pass>"
```

После этого можно подключать backend к relay.

## Production настройка

### 1. Создать SSH key для backend

На машине администратора:

```bash
ssh-keygen -t ed25519 -f xray_control_plane_ed25519 -C "xray-control-plane"
```

Публичный ключ добавить на relay:

```bash
ssh-copy-id -i xray_control_plane_ed25519.pub root@<relay_ip>
```

Или вручную добавить `.pub` ключ в:

```text
/root/.ssh/authorized_keys
```

Приватный ключ положить в:

```text
deploy/secrets/xray_ssh_key
```

### 2. Настроить `deploy/.env`

Скопировать пример:

```bash
cd deploy
cp .env.example .env
```

Минимально изменить:

```env
JWT_SECRET=long-random-production-secret
COOKIE_SECURE=true
CORS_ORIGINS=https://your-domain.example

ADMIN_EMAIL=admin@your-domain.example
ADMIN_PASSWORD=very-strong-password

XRAY_RELAY_HOST=<relay_ip_or_dns>
XRAY_RELAY_PORT=22
XRAY_RELAY_USER=root
XRAY_SSH_PRIVATE_KEY_PATH=/run/secrets/xray_ssh_key
XRAY_CONFIG_PATH=/usr/local/etc/xray/config.json
XRAY_DRY_RUN=false
```

Важно:

- `JWT_SECRET` должен быть длинным случайным секретом.
- `ADMIN_PASSWORD` после первого входа лучше заменить механизмом смены пароля в будущей версии или пересоздать admin вручную.
- `COOKIE_SECURE=true` требует HTTPS.

### 3. Запустить production stack

```bash
cd deploy
docker compose up --build -d
```

Проверить:

```bash
docker compose ps
docker compose logs backend
docker compose logs frontend
```

Healthcheck:

```bash
curl http://localhost:8000/health
```

### 4. HTTPS и reverse proxy

Для production рекомендуется ставить reverse proxy перед frontend/backend:

- Caddy;
- nginx;
- Traefik.

Типовая схема:

```text
https://vpn-panel.example.com      -> frontend:5173
https://vpn-panel.example.com/api  -> backend:8000
```

Если frontend и backend находятся за одним доменом, cookies работают проще и безопаснее.

### 5. Firewall

На сервере с panel:

- открыть `80/tcp` и `443/tcp` для reverse proxy;
- не открывать Postgres наружу;
- backend port `8000` лучше держать только во внутренней Docker network или за reverse proxy.

На relay:

- открыть VPN порты:
  - `443/tcp` для TCP REALITY;
  - `80/tcp` для XHTTP REALITY, если используется;
- открыть `22/tcp` только для trusted IP, если возможно.

## Переменные окружения backend

| Переменная | Назначение |
|---|---|
| `DATABASE_URL` | SQLAlchemy URL Postgres |
| `JWT_SECRET` | секрет подписи JWT |
| `COOKIE_SECURE` | secure cookies, должно быть `true` на HTTPS |
| `CORS_ORIGINS` | разрешенные origins frontend |
| `ADMIN_EMAIL` | email seed admin |
| `ADMIN_PASSWORD` | пароль seed admin |
| `DEFAULT_KEY_LIMIT` | лимит ключей по умолчанию |
| `XRAY_RELAY_HOST` | адрес relay |
| `XRAY_RELAY_PORT` | SSH port relay |
| `XRAY_RELAY_USER` | SSH user |
| `XRAY_SSH_PRIVATE_KEY_PATH` | путь к private key внутри контейнера |
| `XRAY_CONFIG_PATH` | путь к Xray config на relay |
| `XRAY_DRY_RUN` | mock режим без реального relay |

## API

Backend поднимает OpenAPI документацию:

```text
http://localhost:8000/docs
```

Основные endpoints:

```text
POST   /auth/register
POST   /auth/login
POST   /auth/logout
GET    /auth/me

GET    /keys
POST   /keys
DELETE /keys/{key_id}

GET    /admin/users
GET    /admin/users/{user_id}/keys
GET    /admin/keys
POST   /admin/users/{user_id}/approve
POST   /admin/users/{user_id}/suspend
POST   /admin/users/{user_id}/restore
PATCH  /admin/users/{user_id}/limits
POST   /admin/users/{user_id}/keys
DELETE /admin/keys/{key_id}
```

## База данных

Используются таблицы:

- `users`
- `vpn_keys`
- `audit_logs`

Миграции Alembic находятся в:

```text
backend/alembic/
```

При старте backend container выполняет:

```bash
alembic upgrade head
python -m app.seed_admin
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

## Тесты и проверки

Backend:

```bash
cd backend
py -m pip install -e ".[dev]"
py -m pytest
py -m ruff check .
```

Frontend:

```bash
cd frontend
npm install
npm run build
npm audit --audit-level=moderate
```

Docker:

```bash
docker compose -f deploy/docker-compose.yml config
docker compose -f deploy/docker-compose.yml build
```

## Что важно доделать перед серьезным production

Текущая версия уже дает рабочий control plane, но перед коммерческим production стоит добавить:

1. Смену пароля пользователем и админом.
2. Refresh token или короткоживущий access token с refresh-cookie.
3. Rate limiting для `/auth/login` и `/auth/register`.
4. CSRF protection для cookie-based auth.
5. Более строгую password policy.
6. Email verification или invite-only регистрацию.
7. UI для просмотра audit logs.
8. Backup policy для Postgres.
9. Monitoring backend/frontend/Postgres.
10. Отдельного non-root SSH пользователя на relay с ограниченным sudo.
11. Проверку, что `xray run -test` и `systemctl restart xray` разрешены только нужному пользователю.
12. Ротацию `JWT_SECRET` и SSH ключей по регламенту.
13. Nginx/Caddy HTTPS reverse proxy.
14. CI pipeline: backend tests, frontend build, docker build.

## Приостановка и восстановление доступа

Кнопка `Suspend` переводит пользователя в статус `suspended`. Такой пользователь не может
авторизоваться в приложении, но его запись и ключи остаются в базе.

Кнопка `Restore` переводит пользователя обратно в `approved`. После этого пользователь снова
может войти в приложение. Ключи пользователя не пересоздаются автоматически.

## Быстрый troubleshooting

### Backend не стартует

Проверить:

```bash
docker compose logs backend
```

Частые причины:

- неправильный `DATABASE_URL`;
- Postgres еще не healthy;
- слабый или пустой `JWT_SECRET`;
- ошибка миграции.

### Ключ не создается

Проверить:

```bash
docker compose logs backend
```

Если `XRAY_DRY_RUN=false`, частые причины:

- backend не может подключиться по SSH;
- private key неверный или не имеет прав;
- relay config не содержит нужных inbound tags;
- `xray run -test` падает;
- `systemctl restart xray` требует sudo/root;
- SNI не входит в `serverNames`.

### Пользователь не может войти

Проверить статус пользователя в admin UI:

- `pending` - нужно нажать `Approve`;
- `suspended` - пользователь заблокирован;
- `approved` - можно входить.

### Cookies не работают в production

Проверить:

- frontend/backend на одном домене или корректно настроен CORS;
- `CORS_ORIGINS` содержит точный origin frontend;
- `COOKIE_SECURE=true` используется только с HTTPS;
- reverse proxy прокидывает headers корректно.
