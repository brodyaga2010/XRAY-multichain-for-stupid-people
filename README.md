1. Buy server in your friendly country(relay) and foreign country(exit)
2. Buy domain for your friendly country
3. In .env set your domain
4. Execute bash `setup-caddy.sh` on relay server
5. Execute `install_xray.sh` on relay and exit server
6. Execute `generate_config.sh` with exit and relay servers
7. Generate new clients with `add_user_relay` script

---

## Web Control Plane

Личный кабинет для пользователей и администратора: ключи (TCP, gRPC и XHTTP поверх REALITY), подписка,
трафик и онлайн, блокировка, журнал действий. Ключи добавляются в Xray через его API без перезапуска.

Кабинет — один бинарник на Go со встроенным интерфейсом (`server/`, `frontend/`). Ставится на relay-сервер
рядом с Xray:

```bash
make -C server web build SITE_NAME=example   # на машине с Go и Node
sudo ./deploy/install_panel.sh --panel-domain panel.example.com --vpn-domain example.com \
     --admin-email admin@example.com --site-name example    # на сервере
```

Подробнее: [`deploy/README.md`](deploy/README.md) — установка, [`docs/WEB_CONTROL_PLANE.md`](docs/WEB_CONTROL_PLANE.md) — устройство, API, разработка.
