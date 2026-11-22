#!/usr/bin/env bash
# Ставит внешний сторож входного сервера (entry_watchdog.sh) на ЭТОТ сервер — обычно выходной.
#
#   sudo TELEGRAM_BOT_TOKEN=... ./install_watchdog.sh --entry-host kemsekov.ru \
#        --panel-url https://panel.kemsekov.ru/health --chat-ids 111111111,-222222222
#
# Токен берётся из переменной окружения TELEGRAM_BOT_TOKEN (или спрашивается), чтобы не попасть в историю
# команд. Настройки пишутся в /etc/entry-watchdog.env (только root), проверка — systemd-таймер раз в минуту.
# Повторный запуск обновляет скрипт и настройки.
set -euo pipefail

entry_host="" entry_port=443 panel_url="" chat_ids="" threshold=3
while [ $# -gt 0 ]; do
  case "$1" in
    --entry-host) entry_host="$2"; shift 2 ;;
    --entry-port) entry_port="$2"; shift 2 ;;
    --panel-url) panel_url="$2"; shift 2 ;;
    --chat-ids) chat_ids="$2"; shift 2 ;;
    --threshold) threshold="$2"; shift 2 ;;
    *) echo "Неизвестный параметр: $1" >&2; exit 2 ;;
  esac
done

[ "$(id -u)" -eq 0 ] || { echo "Запустите от root" >&2; exit 1; }
[ -n "$entry_host" ] && [ -n "$chat_ids" ] || {
  echo "Нужны --entry-host и --chat-ids" >&2; exit 2
}
token="${TELEGRAM_BOT_TOKEN:-}"
if [ -z "$token" ]; then
  read -rsp "Токен Telegram-бота: " token; echo
fi
[ -n "$token" ] || { echo "Нужен токен бота" >&2; exit 2; }

here="$(cd "$(dirname "$0")" && pwd)"
install -m 0755 "$here/entry_watchdog.sh" /usr/local/bin/entry-watchdog

umask 077
cat >/etc/entry-watchdog.env <<EOF
TELEGRAM_BOT_TOKEN=$token
TELEGRAM_CHAT_IDS=$chat_ids
ENTRY_HOST=$entry_host
ENTRY_PORT=$entry_port
PANEL_URL=$panel_url
FAIL_THRESHOLD=$threshold
EOF
chmod 600 /etc/entry-watchdog.env

cat >/etc/systemd/system/entry-watchdog.service <<'EOF'
[Unit]
Description=Внешняя проверка входного VPN-сервера
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/entry-watchdog
EOF

cat >/etc/systemd/system/entry-watchdog.timer <<'EOF'
[Unit]
Description=Внешняя проверка входного VPN-сервера раз в минуту

[Timer]
OnBootSec=1min
OnUnitActiveSec=1min
AccuracySec=5s

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now entry-watchdog.timer
systemctl start entry-watchdog.service
echo "Сторож установлен: проверяет $entry_host:$entry_port${panel_url:+ и $panel_url} раз в минуту."
echo "Состояние: systemctl list-timers entry-watchdog.timer; журнал: journalctl -u entry-watchdog"
