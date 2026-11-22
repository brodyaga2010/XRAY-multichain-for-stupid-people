#!/usr/bin/env bash
# Тесты entry_watchdog.sh: curl подменяется (ответ кабинета и отправка в Telegram пишутся в файл),
# «живой» порт входа — локальный сервер, «мёртвый» — закрытый порт.
set -u
here="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'kill "$server_pid" 2>/dev/null; wait "$server_pid" 2>/dev/null; rm -rf "$work"' EXIT

mkdir -p "$work/bin"
cat >"$work/bin/curl" <<'EOF'
#!/usr/bin/env bash
# Поддельный curl: sendMessage — дописывает chat_id и текст в $FAKE_LOG; иначе — печатает $FAKE_HEALTH_CODE.
args="$*"
if [[ "$args" == *sendMessage* ]]; then
  chat="" text=""
  while [ $# -gt 0 ]; do
    if [ "$1" = "--data-urlencode" ]; then
      case "$2" in chat_id=*) chat="${2#chat_id=}" ;; text=*) text="${2#text=}" ;; esac
      shift
    fi
    shift
  done
  echo "$chat|$text" >>"$FAKE_LOG"
  exit 0
fi
printf '%s' "${FAKE_HEALTH_CODE:-200}"
EOF
chmod +x "$work/bin/curl"

open_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
python3 -m http.server "$open_port" --bind 127.0.0.1 >/dev/null 2>&1 &
server_pid=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do (exec 3<>"/dev/tcp/127.0.0.1/$open_port") 2>/dev/null && break; sleep 0.2; done
closed_port=1

failed=0
# run_case имя "шаги" "ожидаемые сообщения через ⏎"
# шаг: порт:код_кабинета (open/closed : 200/000/502), по минуте между шагами.
run_case() {
  local name="$1" steps="$2" want="$3" state="$work/state-$RANDOM" log="$work/log-$RANDOM" now=1000000 step port code
  : >"$log"
  for step in $steps; do
    port="${step%%:*}"; code="${step##*:}"
    [ "$port" = open ] && port=$open_port || port=$closed_port
    PATH="$work/bin:$PATH" WATCHDOG_CONF=/nonexistent STATE_DIR="$state" FAKE_LOG="$log" FAKE_HEALTH_CODE="$code" \
      WATCHDOG_NOW=$now TELEGRAM_BOT_TOKEN=t TELEGRAM_CHAT_IDS="111, -222" ENTRY_HOST=127.0.0.1 ENTRY_PORT="$port" \
      PANEL_URL=http://panel.test/health FAIL_THRESHOLD=3 bash "$here/entry_watchdog.sh"
    now=$((now + 60))
  done
  local got
  got=$(awk -F'|' '$1=="111"{if(n++)printf " ⏎ ";printf "%s",$2}' "$log")
  local group
  group=$(awk -F'|' '$1=="-222"{if(n++)printf " ⏎ ";printf "%s",$2}' "$log")
  if [ "$got" != "$want" ] || [ "$group" != "$want" ]; then
    echo "FAIL: $name"; echo "  админ:  $got"; echo "  группа: $group"; echo "  want:   $want"
    failed=1
  else
    echo "ok: $name"
  fi
}

run_case "всё работает — тишина" "open:200 open:200 open:200" ""
run_case "два сбоя ниже порога — тишина" "closed:200 closed:200 open:200" ""
run_case "вход недоступен — тревога и восстановление" "closed:000 closed:000 closed:000 closed:000 open:200" \
  "🔴 Входной сервер 127.0.0.1 недоступен снаружи: порт VPN $closed_port не отвечает; кабинет не отвечает (проверка с другого сервера) ⏎ 🟢 Входной сервер 127.0.0.1 снова доступен снаружи (сбой длился 4 мин)"
run_case "лежит только кабинет — ошибка с кодом" "open:502 open:502 open:502" \
  "🔴 Входной сервер 127.0.0.1 недоступен снаружи: кабинет отвечает ошибкой 502 (проверка с другого сервера)"
run_case "долгий сбой — одна тревога" "closed:200 closed:200 closed:200 closed:200 closed:200 closed:200" \
  "🔴 Входной сервер 127.0.0.1 недоступен снаружи: порт VPN $closed_port не отвечает (проверка с другого сервера)"

exit "$failed"
