#!/usr/bin/env bash
# Выкатка кабинета на входной сервер одной командой: сборка → копирование → установка → проверка.
#
#   make -C server deploy VERSION=v2.8.0
#
# Настройки — в deploy/deploy.env (не попадает в git, пример — deploy/deploy.env.example).
# Вход на сервер — только по SSH-ключу.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f deploy/deploy.env ]]; then
  # shellcheck source=/dev/null
  source deploy/deploy.env
fi
: "${DEPLOY_HOST:?задайте DEPLOY_HOST (например root@203.0.113.10) в deploy/deploy.env}"
: "${PANEL_DOMAIN:?задайте PANEL_DOMAIN в deploy/deploy.env}"
: "${VPN_DOMAIN:?задайте VPN_DOMAIN в deploy/deploy.env}"
SITE_NAME="${SITE_NAME:-}"
DEPLOY_SSH_KEY="${DEPLOY_SSH_KEY:-$HOME/.ssh/id_ed25519}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
REMOTE_DIR=/root/panel-src

ssh_remote() {
  ssh -o BatchMode=yes -o IdentitiesOnly=yes -o ConnectTimeout=15 -i "$DEPLOY_SSH_KEY" "$DEPLOY_HOST" "$@"
}

# Сборка подменяет заглушку фронтенда и tsbuildinfo — возвращаем их при любом исходе.
cleanup() {
  make -s -C server web-reset >/dev/null 2>&1 || true
  git checkout -- frontend/tsconfig.tsbuildinfo 2>/dev/null || true
}
trap cleanup EXIT

echo "→ проверяю доступ к $DEPLOY_HOST"
ssh_remote true

echo "→ собираю $VERSION"
make -s -C server web SITE_NAME="$SITE_NAME" >/dev/null
make -s -C server build-amd64 VERSION="$VERSION"

echo "→ копирую и устанавливаю"
COPYFILE_DISABLE=1 tar czf - deploy/install_panel.sh deploy/README.md server/dist/xray-panel-linux-amd64 |
  ssh_remote "rm -rf $REMOTE_DIR && mkdir -p $REMOTE_DIR && tar xzf - -C $REMOTE_DIR && cd $REMOTE_DIR && \
    ./deploy/install_panel.sh --panel-domain '$PANEL_DOMAIN' --vpn-domain '$VPN_DOMAIN' --site-name '$SITE_NAME'" |
  tail -n 4

echo "→ проверяю"
running="$(ssh_remote "journalctl -u xray-panel --since '-2 min' --no-pager | grep -o 'version=[^ ]*' | tail -n 1")"
code="$(curl -s -o /dev/null -w '%{http_code}' "https://$PANEL_DOMAIN/")"
if [[ "$running" != "version=$VERSION" || "$code" != "200" ]]; then
  echo "✗ что-то не так: запущено «$running», сайт ответил $code" >&2
  exit 1
fi
echo "✓ $VERSION работает на https://$PANEL_DOMAIN"
