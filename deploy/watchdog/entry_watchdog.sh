#!/usr/bin/env bash
# Внешний сторож входного сервера. Запускается раз в минуту на ДРУГОМ сервере (обычно выходном):
# мониторинг кабинета живёт на самом входе и не сообщит, если вход упал целиком или пропала его сеть.
#
# Проверяет порт VPN на входе и адрес здоровья кабинета. После FAIL_THRESHOLD неудач подряд пишет
# в Telegram (TELEGRAM_CHAT_IDS — админ и группа тревог), при восстановлении — ещё раз.
# Настройки — в /etc/entry-watchdog.env (см. install_watchdog.sh), состояние — в STATE_DIR.
set -u

CONF="${WATCHDOG_CONF:-/etc/entry-watchdog.env}"
# shellcheck disable=SC1090
[ -f "$CONF" ] && . "$CONF"

: "${TELEGRAM_BOT_TOKEN:?нужен TELEGRAM_BOT_TOKEN}"
: "${TELEGRAM_CHAT_IDS:?нужен TELEGRAM_CHAT_IDS (через запятую)}"
: "${ENTRY_HOST:?нужен ENTRY_HOST — адрес входного сервера}"
ENTRY_PORT="${ENTRY_PORT:-443}"
PANEL_URL="${PANEL_URL:-}"
FAIL_THRESHOLD="${FAIL_THRESHOLD:-3}"
STATE_DIR="${STATE_DIR:-/var/lib/entry-watchdog}"
TELEGRAM_API="${TELEGRAM_API:-https://api.telegram.org}"
NOW="${WATCHDOG_NOW:-$(date +%s)}"

state="$STATE_DIR/state"
fails=0 alerted=0 since=0
mkdir -p "$STATE_DIR"
# shellcheck disable=SC1090
[ -f "$state" ] && . "$state"

send() {
  local chat
  IFS=',' read -ra chats <<<"$TELEGRAM_CHAT_IDS"
  for chat in "${chats[@]}"; do
    chat="${chat// /}"
    [ -n "$chat" ] || continue
    curl -s --max-time 15 -o /dev/null "$TELEGRAM_API/bot$TELEGRAM_BOT_TOKEN/sendMessage" \
      --data-urlencode "chat_id=$chat" --data-urlencode "text=$1" ||
      echo "entry-watchdog: сообщение в чат $chat не отправлено" >&2
  done
}

duration() {
  local mins=$(( ($1 + 30) / 60 ))
  if [ "$mins" -lt 60 ]; then
    echo "$mins мин"
  elif [ $((mins % 60)) -eq 0 ]; then
    echo "$((mins / 60)) ч"
  else
    echo "$((mins / 60)) ч $((mins % 60)) мин"
  fi
}

problems=()
if ! timeout 5 bash -c "</dev/tcp/$ENTRY_HOST/$ENTRY_PORT" 2>/dev/null; then
  problems+=("порт VPN $ENTRY_PORT не отвечает")
fi
if [ -n "$PANEL_URL" ]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$PANEL_URL" || true)
  if [ "$code" != "200" ]; then
    [ "$code" = "000" ] && problems+=("кабинет не отвечает") || problems+=("кабинет отвечает ошибкой $code")
  fi
fi

if [ ${#problems[@]} -gt 0 ]; then
  [ "$fails" -eq 0 ] && since="$NOW"
  fails=$((fails + 1))
  if [ "$fails" -ge "$FAIL_THRESHOLD" ] && [ "$alerted" -eq 0 ]; then
    detail=$(IFS=';'; echo "${problems[*]}" | sed 's/;/; /g')
    send "🔴 Входной сервер $ENTRY_HOST недоступен снаружи: $detail (проверка с другого сервера)"
    alerted=1
  fi
else
  if [ "$alerted" -eq 1 ]; then
    send "🟢 Входной сервер $ENTRY_HOST снова доступен снаружи (сбой длился $(duration $((NOW - since))))"
  fi
  fails=0 alerted=0 since=0
fi

printf 'fails=%d\nalerted=%d\nsince=%d\n' "$fails" "$alerted" "$since" >"$state.tmp" && mv "$state.tmp" "$state"
