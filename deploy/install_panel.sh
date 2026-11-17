#!/usr/bin/env bash
# ============================================================
# install_panel.sh — установка/обновление личного кабинета на ВХОДНОЙ (relay) сервер.
#
# Кабинет — один бинарник (Go) со встроенным интерфейсом. Соберите его на любой машине с Go и Node:
#   make -C server web build SITE_NAME=example VERSION=v2.0.0
# и скопируйте на сервер каталог deploy/ и бинарник (server/dist/xray-panel-linux-<arch>).
#
# Запуск на сервере от root из корня репозитория:
#   sudo ./deploy/install_panel.sh --panel-domain panel.example.com --vpn-domain example.com \
#        --admin-email admin@example.com --site-name example
#
# Что делает (повторный запуск безопасен — это обновление):
#   * системный пользователь xray-panel, каталоги, sudo-правило только на `systemctl restart xray`;
#   * бинарник в /usr/local/bin/xray-panel;
#   * /etc/xray-panel/panel.env (секреты, права 640) — только при первой установке;
#   * миграции SQLite, первый администратор;
#   * конфиг Xray: API и статистика на 127.0.0.1, XHTTP inbound, email = UUID, PROXY protocol к Caddy;
#   * Caddy: сайт-заглушка и кабинет на 127.0.0.1:8444 (снаружи доступны только через Xray REALITY на 443);
#   * systemd-сервис xray-panel (127.0.0.1:8000);
#   * если стоит прежняя Python-версия — удаляет её код (база и настройки сохраняются).
#
# Требования: Xray и Caddy уже настроены (generate_config.sh, setup-caddy.sh), DNS кабинета указывает на сервер.
# ============================================================
set -euo pipefail

PANEL_DOMAIN=""
VPN_DOMAIN=""
ADMIN_EMAIL=""
XHTTP_PORT="2053"
SITE_NAME=""
BINARY=""

while [ $# -gt 0 ]; do
    case "$1" in
        --panel-domain) PANEL_DOMAIN="$2"; shift 2 ;;
        --vpn-domain) VPN_DOMAIN="$2"; shift 2 ;;
        --admin-email) ADMIN_EMAIL="$2"; shift 2 ;;
        --xhttp-port) XHTTP_PORT="$2"; shift 2 ;;
        --site-name) SITE_NAME="$2"; shift 2 ;;
        --binary) BINARY="$2"; shift 2 ;;
        *) echo "Неизвестный аргумент: $1"; exit 1 ;;
    esac
done

if [ -z "$PANEL_DOMAIN" ] || [ -z "$VPN_DOMAIN" ]; then
    echo "Использование: $0 --panel-domain <домен кабинета> --vpn-domain <домен VPN> [--admin-email <email>]"
    echo "                 [--site-name <название>] [--xhttp-port 2053] [--binary <путь к бинарнику>]"
    exit 1
fi
[ "$(id -u)" -eq 0 ] || { echo "Нужно запускать от root"; exit 1; }

SRC_DIR="$(cd "$(dirname "$0")/.." && pwd)"
case "$(uname -m)" in
    x86_64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) echo "Неподдерживаемая архитектура: $(uname -m)"; exit 1 ;;
esac
BINARY="${BINARY:-$SRC_DIR/server/dist/xray-panel-linux-$ARCH}"
[ -f "$BINARY" ] || { echo "Нет бинарника $BINARY — соберите: make -C server web build"; exit 1; }

BIN_PATH=/usr/local/bin/xray-panel
DATA_DIR=/var/lib/xray-panel
ENV_DIR=/etc/xray-panel
ENV_FILE=$ENV_DIR/panel.env
XRAY_DIR=/usr/local/etc/xray
XRAY_CONFIG=$XRAY_DIR/config.json
CADDYFILE=/etc/caddy/Caddyfile
SERVICE_USER=xray-panel

log() { echo -e "\n==> $*"; }

[ -f "$XRAY_CONFIG" ] || { echo "Нет $XRAY_CONFIG — сначала настройте Xray (generate_config.sh)"; exit 1; }
command -v caddy >/dev/null || { echo "Caddy не установлен — сначала setup-caddy.sh"; exit 1; }
if ! command -v jq >/dev/null; then
    apt-get update -qq || true
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq jq >/dev/null
fi

log "Пользователь и каталоги"
id "$SERVICE_USER" >/dev/null 2>&1 || useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 750 "$DATA_DIR" "$DATA_DIR/backups"
install -d -o root -g "$SERVICE_USER" -m 750 "$ENV_DIR"
# Кабинет правит конфиг Xray атомарной заменой файла: нужна запись в каталог
chgrp "$SERVICE_USER" "$XRAY_DIR" && chmod 775 "$XRAY_DIR"
chgrp "$SERVICE_USER" "$XRAY_CONFIG" && chmod 664 "$XRAY_CONFIG"
cat > /etc/sudoers.d/xray-panel <<EOF
$SERVICE_USER ALL=(root) NOPASSWD: /usr/bin/systemctl restart xray
EOF
chmod 440 /etc/sudoers.d/xray-panel
visudo -cf /etc/sudoers.d/xray-panel >/dev/null

log "Бинарник"
install -m 755 "$BINARY" "$BIN_PATH.new"
"$BIN_PATH.new" version
mv "$BIN_PATH.new" "$BIN_PATH"

log "Настройки ($ENV_FILE)"
FIRST_INSTALL=0
ADMIN_PASSWORD=""
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
LISTEN_ADDR=127.0.0.1:8000

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

# Прежняя Python-версия: останавливаем и удаляем код; база и настройки остаются
if [ -d /opt/xray-panel ] || [ -d /var/www/xray-panel ]; then
    log "Удаление прежней Python-версии"
    systemctl stop xray-panel 2>/dev/null || true
    rm -rf /opt/xray-panel /var/www/xray-panel
fi

# Команда от имени пользователя кабинета. Лок — в каталоге данных: /run/xray-panel есть только у сервиса.
run_as_panel() {
    runuser -u "$SERVICE_USER" -- env XRAY_LOCK_PATH="$DATA_DIR/bootstrap.lock" \
        "$BIN_PATH" --env-file "$ENV_FILE" "$@"
}

log "База данных и администратор"
run_as_panel migrate
run_as_panel seed-admin

log "Конфиг Xray (API, статистика, XHTTP на порту $XHTTP_PORT)"
run_as_panel bootstrap-xray
XRAY_RESTART=0
# Реальный IP посетителей кабинета: Xray передаёт его Caddy по PROXY protocol
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

# Личный кабинет: бинарник отдаёт и интерфейс, и API, и подписки
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
    reverse_proxy 127.0.0.1:8000
}

# Порт 80: выпуск/продление сертификатов Let's Encrypt и редирект на HTTPS
http://$VPN_DOMAIN, http://$PANEL_DOMAIN {
    redir https://{host}{uri} permanent
}
EOF
chown root:caddy "$CADDYFILE" && chmod 640 "$CADDYFILE"
CADDY_CHECK=$(caddy validate --config "$CADDYFILE" --adapter caddyfile 2>&1) || { echo "$CADDY_CHECK"; exit 1; }

log "systemd"
cat > /etc/systemd/system/xray-panel.service <<EOF
[Unit]
Description=Personal cabinet (keys)
After=network-online.target xray.service
Wants=network-online.target

[Service]
User=$SERVICE_USER
Group=$SERVICE_USER
EnvironmentFile=$ENV_FILE
ExecStart=$BIN_PATH serve
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
[ "$XRAY_RESTART" = "1" ] && systemctl restart xray
systemctl restart xray-panel
sleep 2
systemctl is-active --quiet xray && echo "✅ Xray работает" || { echo "❌ Xray не запустился"; journalctl -u xray -n 20 --no-pager; exit 1; }
systemctl is-active --quiet caddy && echo "✅ Caddy работает" || { echo "❌ Caddy не запустился"; journalctl -u caddy -n 20 --no-pager; exit 1; }
systemctl is-active --quiet xray-panel && echo "✅ Кабинет работает" || { echo "❌ Кабинет не запустился"; journalctl -u xray-panel -n 30 --no-pager; exit 1; }
curl -fsS http://127.0.0.1:8000/api/health >/dev/null && echo "✅ API отвечает"

echo ""
echo "============================================"
echo "Кабинет: https://$PANEL_DOMAIN"
if [ "$FIRST_INSTALL" = "1" ]; then
    echo "Администратор: $ADMIN_EMAIL"
    echo "Пароль: $ADMIN_PASSWORD   (смените после первого входа; он также лежит в $ENV_FILE)"
fi
echo "Ключи, созданные ранее скриптами, можно подтянуть в кабинете: «Импортировать ключи с сервера»."
echo "============================================"
