#!/usr/bin/env bash
# ============================================================
# install_panel.sh — установка/обновление веб-панели на ВХОДНОЙ (relay) сервер.
#
# Запускать на самом сервере от root из корня репозитория:
#   sudo ./deploy/install_panel.sh --panel-domain panel.example.com --vpn-domain example.com \
#        --admin-email admin@example.com --site-name example
#
# Что делает (повторный запуск безопасен — это обновление):
#   * ставит python3-venv, nodejs/npm, sqlite3, jq;
#   * создаёт системного пользователя xray-panel, каталоги, sudo-правило только на `systemctl restart xray`;
#   * собирает бэкенд (venv) и фронтенд (статика в /var/www/xray-panel);
#   * создаёт /etc/xray-panel/panel.env (секреты, права 600) — только при первой установке;
#   * миграции БД (SQLite), первый администратор;
#   * дополняет конфиг Xray: API и статистика на 127.0.0.1, XHTTP inbound, email = UUID, PROXY protocol к Caddy;
#   * Caddy: сайт-заглушка и панель на 127.0.0.1:8444 (снаружи доступны только через Xray REALITY на 443);
#   * systemd-сервис xray-panel (uvicorn на 127.0.0.1:8000).
#
# Требования: Xray и Caddy уже настроены (generate_config.sh, setup-caddy.sh), DNS панели указывает на сервер.
# ============================================================
set -euo pipefail

PANEL_DOMAIN=""
VPN_DOMAIN=""
ADMIN_EMAIL=""
XHTTP_PORT="2053"
SITE_NAME=""

while [ $# -gt 0 ]; do
    case "$1" in
        --panel-domain) PANEL_DOMAIN="$2"; shift 2 ;;
        --vpn-domain) VPN_DOMAIN="$2"; shift 2 ;;
        --admin-email) ADMIN_EMAIL="$2"; shift 2 ;;
        --xhttp-port) XHTTP_PORT="$2"; shift 2 ;;
        --site-name) SITE_NAME="$2"; shift 2 ;;
        *) echo "Неизвестный аргумент: $1"; exit 1 ;;
    esac
done

if [ -z "$PANEL_DOMAIN" ] || [ -z "$VPN_DOMAIN" ]; then
    echo "Использование: $0 --panel-domain <домен панели> --vpn-domain <домен VPN> [--admin-email <email>] [--site-name <название кабинета>] [--xhttp-port 2053]"
    exit 1
fi
[ "$(id -u)" -eq 0 ] || { echo "Нужно запускать от root"; exit 1; }

SRC_DIR="$(cd "$(dirname "$0")/.." && pwd)"
APP_DIR=/opt/xray-panel
DATA_DIR=/var/lib/xray-panel
ENV_DIR=/etc/xray-panel
ENV_FILE=$ENV_DIR/panel.env
WEB_DIR=/var/www/xray-panel
XRAY_DIR=/usr/local/etc/xray
XRAY_CONFIG=$XRAY_DIR/config.json
CADDYFILE=/etc/caddy/Caddyfile
SERVICE_USER=xray-panel

log() { echo -e "\n==> $*"; }

[ -f "$XRAY_CONFIG" ] || { echo "Нет $XRAY_CONFIG — сначала настройте Xray (generate_config.sh)"; exit 1; }
command -v caddy >/dev/null || { echo "Caddy не установлен — сначала setup-caddy.sh"; exit 1; }

log "Пакеты"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq || true
apt-get install -y -qq python3-venv nodejs npm sqlite3 jq >/dev/null

log "Пользователь и каталоги"
id "$SERVICE_USER" >/dev/null 2>&1 || useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 750 "$DATA_DIR" "$DATA_DIR/backups"
install -d -o root -g root -m 755 "$APP_DIR" "$WEB_DIR"
install -d -o root -g "$SERVICE_USER" -m 750 "$ENV_DIR"
# Панель правит конфиг Xray атомарной заменой файла: нужна запись в каталог
chgrp "$SERVICE_USER" "$XRAY_DIR" && chmod 775 "$XRAY_DIR"
chgrp "$SERVICE_USER" "$XRAY_CONFIG" && chmod 664 "$XRAY_CONFIG"
cat > /etc/sudoers.d/xray-panel <<EOF
$SERVICE_USER ALL=(root) NOPASSWD: /usr/bin/systemctl restart xray
EOF
chmod 440 /etc/sudoers.d/xray-panel
visudo -cf /etc/sudoers.d/xray-panel >/dev/null

log "Бэкенд"
rm -rf "$APP_DIR/backend.new" && cp -r "$SRC_DIR/backend" "$APP_DIR/backend.new"
rm -rf "$APP_DIR/backend.new/.venv" "$APP_DIR/backend.new/.pytest_cache" "$APP_DIR/backend.new/.ruff_cache"
rm -rf "$APP_DIR/backend.old" && { [ -d "$APP_DIR/backend" ] && mv "$APP_DIR/backend" "$APP_DIR/backend.old" || true; }
mv "$APP_DIR/backend.new" "$APP_DIR/backend"
[ -d "$APP_DIR/venv" ] || python3 -m venv "$APP_DIR/venv"
"$APP_DIR/venv/bin/pip" install -q --upgrade pip
"$APP_DIR/venv/bin/pip" install -q "$APP_DIR/backend"

log "Фронтенд"
BUILD_DIR=$(mktemp -d)
cp -r "$SRC_DIR/frontend/." "$BUILD_DIR/"
rm -rf "$BUILD_DIR/node_modules" "$BUILD_DIR/dist"
(cd "$BUILD_DIR" && npm ci --no-audit --no-fund --loglevel=error && VITE_API_BASE=/api VITE_SITE_NAME="$SITE_NAME" npm run build >/dev/null)
rm -rf "$WEB_DIR.new" && cp -r "$BUILD_DIR/dist" "$WEB_DIR.new"
rm -rf "$WEB_DIR" && mv "$WEB_DIR.new" "$WEB_DIR"
chmod -R a+rX "$WEB_DIR"
rm -rf "$BUILD_DIR"

log "Настройки панели ($ENV_FILE)"
FIRST_INSTALL=0
if [ ! -f "$ENV_FILE" ]; then
    FIRST_INSTALL=1
    [ -n "$ADMIN_EMAIL" ] || { echo "При первой установке нужен --admin-email"; exit 1; }
    ADMIN_PASSWORD=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)
    cat > "$ENV_FILE" <<EOF
DATABASE_URL=sqlite:///$DATA_DIR/panel.db
JWT_SECRET=$(openssl rand -hex 32)
COOKIE_SECURE=true
CORS_ORIGINS=https://$PANEL_DOMAIN
PUBLIC_BASE_URL=https://$PANEL_DOMAIN
SUBSCRIPTION_TITLE="${SITE_NAME:-Personal}"
ADMIN_EMAIL=$ADMIN_EMAIL
ADMIN_PASSWORD=$ADMIN_PASSWORD
DEFAULT_KEY_LIMIT=3

XRAY_MODE=local
XRAY_PUBLIC_HOST=$VPN_DOMAIN
XRAY_CONFIG_PATH=$XRAY_CONFIG
XRAY_BACKUP_DIR=$DATA_DIR/backups
XRAY_LOCK_PATH=/run/xray-panel/config.lock
XRAY_API_SERVER=127.0.0.1:10085
XRAY_XHTTP_PORT=$XHTTP_PORT
XRAY_RESTART_COMMAND="sudo -n /usr/bin/systemctl restart xray"
EOF
    chown root:"$SERVICE_USER" "$ENV_FILE"
    chmod 640 "$ENV_FILE"
fi

# Команда от имени пользователя панели с настройками из $ENV_FILE (значения с пробелами — в кавычках)
run_as_panel() {
    runuser -u "$SERVICE_USER" -- bash -c \
        'set -a; . "$0"; set +a; export XRAY_LOCK_PATH="$1"; shift; exec "$@"' \
        "$ENV_FILE" "$DATA_DIR/bootstrap.lock" "$@"
}

log "База данных и администратор"
(cd "$APP_DIR/backend" && run_as_panel "$APP_DIR/venv/bin/alembic" upgrade head)
(cd "$APP_DIR/backend" && run_as_panel "$APP_DIR/venv/bin/python" -m app.seed_admin)

log "Конфиг Xray (API, статистика, XHTTP на порту $XHTTP_PORT)"
(cd "$APP_DIR/backend" && run_as_panel "$APP_DIR/venv/bin/python" -m app.xray_bootstrap)
# Реальный IP посетителей панели: Xray передаёт его Caddy по PROXY protocol
if [ "$(jq '[.inbounds[] | select(.streamSettings.realitySettings.dest == "127.0.0.1:8444") | .streamSettings.realitySettings.xver // 0] | min' "$XRAY_CONFIG")" != "1" ]; then
    TMP=$(mktemp --suffix=.json)
    jq '(.inbounds[] | select(.streamSettings.realitySettings.dest == "127.0.0.1:8444") | .streamSettings.realitySettings.xver) = 1' "$XRAY_CONFIG" > "$TMP"
    xray run -test -config "$TMP" >/dev/null
    cp "$XRAY_CONFIG" "$DATA_DIR/backups/config.before-xver.json"
    cat "$TMP" > "$XRAY_CONFIG" && rm -f "$TMP"
    XRAY_RESTART=1
fi

log "Caddy"
SITE_ROOT=/var/www/caddy_site
cp "$CADDYFILE" "$DATA_DIR/backups/Caddyfile.$(date +%Y%m%d%H%M%S)"
cat > "$CADDYFILE" <<EOF
{
    default_sni $VPN_DOMAIN
    servers 127.0.0.1:8444 {
        listener_wrappers {
            proxy_protocol {
                allow 127.0.0.1/32
            }
            tls
        }
    }
}

# Сайт-заглушка: его видят все, кто открывает домен VPN без ключа
$VPN_DOMAIN:8444 {
    bind 127.0.0.1
    root * $SITE_ROOT
    file_server
}

# Панель управления ключами
$PANEL_DOMAIN:8444 {
    bind 127.0.0.1
    encode zstd gzip
    header {
        -Server
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy no-referrer
        X-Robots-Tag "noindex, nofollow"
        Strict-Transport-Security "max-age=31536000"
    }
    handle_path /api/* {
        reverse_proxy 127.0.0.1:8000
    }
    handle /sub/* {
        reverse_proxy 127.0.0.1:8000
    }
    handle {
        root * $WEB_DIR
        try_files {path} /index.html
        file_server
    }
}

# Порт 80: выпуск/продление сертификатов Let's Encrypt и редирект на HTTPS
http://$VPN_DOMAIN, http://$PANEL_DOMAIN {
    redir https://{host}{uri} permanent
}
EOF
chown root:caddy "$CADDYFILE" && chmod 640 "$CADDYFILE"
caddy validate --config "$CADDYFILE" --adapter caddyfile >/dev/null

log "systemd"
cat > /etc/systemd/system/xray-panel.service <<EOF
[Unit]
Description=Xray VPN panel (backend)
After=network-online.target xray.service
Wants=network-online.target

[Service]
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$APP_DIR/backend
EnvironmentFile=$ENV_FILE
ExecStart=$APP_DIR/venv/bin/uvicorn app.main:app --host 127.0.0.1 --port 8000 --no-server-header
RuntimeDirectory=xray-panel
Restart=on-failure
RestartSec=3
PrivateTmp=true
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable -q xray-panel
systemctl restart caddy
[ "${XRAY_RESTART:-0}" = "1" ] && systemctl restart xray
systemctl restart xray-panel
sleep 3
systemctl is-active --quiet xray && echo "✅ Xray работает" || { echo "❌ Xray не запустился"; journalctl -u xray -n 20 --no-pager; exit 1; }
systemctl is-active --quiet caddy && echo "✅ Caddy работает" || { echo "❌ Caddy не запустился"; journalctl -u caddy -n 20 --no-pager; exit 1; }
systemctl is-active --quiet xray-panel && echo "✅ Панель работает" || { echo "❌ Панель не запустилась"; journalctl -u xray-panel -n 30 --no-pager; exit 1; }
curl -fsS http://127.0.0.1:8000/health >/dev/null && echo "✅ API отвечает"
rm -rf "$APP_DIR/backend.old"

echo ""
echo "============================================"
echo "Панель: https://$PANEL_DOMAIN"
if [ "$FIRST_INSTALL" = "1" ]; then
    echo "Администратор: $ADMIN_EMAIL"
    echo "Пароль: $ADMIN_PASSWORD   (смените после первого входа; он также лежит в $ENV_FILE)"
fi
echo "Ключи, созданные ранее скриптами, можно подтянуть в панели: «Импортировать ключи с сервера»."
echo "============================================"
