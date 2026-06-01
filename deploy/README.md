# Xray Control Plane Deploy

## Local start

```bash
cd deploy
cp .env.example .env
mkdir -p secrets
printf "dry-run" > secrets/xray_ssh_key
docker compose up --build
```

Open `http://localhost:5173`.

The default `.env.example` uses `XRAY_DRY_RUN=true`, so key creation works without a real relay server.

## Production relay mode

1. Put a private SSH key with access to the relay server at `deploy/secrets/xray_ssh_key`.
2. Set `XRAY_DRY_RUN=false`.
3. Set `XRAY_RELAY_HOST`, `XRAY_RELAY_USER`, and `XRAY_CONFIG_PATH`.
4. Ensure the relay config has inbound tags:
   - `user-inbound` for TCP REALITY.
   - `user-inbound-xhttp` for XHTTP REALITY.
5. Run:

```bash
docker compose up --build -d
```
