"""Управление пользователями Xray на входном (relay) сервере.

Изменения применяются через API Xray (без перезапуска — соединения других пользователей не рвутся)
и одновременно сохраняются в config.json, чтобы пережить перезагрузку. Если API недоступно,
Xray перезапускается с новым конфигом.
"""

from __future__ import annotations

import json
import logging
import shlex
import time
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

from fastapi import HTTPException, status

from app.config import Settings
from app.services import xray_config
from app.services.xray_config import KeyLink, ServerProfile, XrayConfigError
from app.services.xray_runner import (
    CommandRunner,
    LocalRunner,
    MemoryRunner,
    SshRunner,
    XrayCommandError,
)

logger = logging.getLogger(__name__)

PROFILE_TTL_SECONDS = 60


@dataclass(frozen=True)
class TrafficStat:
    uplink: int = 0
    downlink: int = 0


class XrayManager:
    def __init__(self, config: Settings, runner: CommandRunner) -> None:
        self.config = config
        self.runner = runner
        self._profile: tuple[float, ServerProfile, str] | None = None

    # ---------- ключи ----------

    def add_key(self, key_uuid: str) -> None:
        """Добавляет UUID во все пользовательские inbound."""
        with self.runner.lock():
            config = self._read_config()
            try:
                added = xray_config.add_client(config, key_uuid)
            except XrayConfigError as exc:
                raise _bad_gateway(str(exc)) from exc
            if not added:
                return
            self._save_config(config)
            inbounds = {inbound["tag"]: inbound for inbound in xray_config.user_inbounds(config)}
            payload = {
                "inbounds": [
                    {
                        "tag": tag,
                        "port": inbounds[tag]["port"],
                        "protocol": "vless",
                        "settings": {
                            "clients": [xray_config.client_entry(inbounds[tag], key_uuid)],
                            "decryption": "none",
                        },
                    }
                    for tag in added
                ]
            }
            if not self._api_add(payload, expected=len(added)):
                self._restart()

    def remove_key(self, key_uuid: str) -> None:
        """Удаляет UUID из всех пользовательских inbound."""
        with self.runner.lock():
            config = self._read_config()
            removed = xray_config.remove_client(config, key_uuid)
            if not removed:
                return
            self._save_config(config)
            ok = True
            for tag, email in removed:
                ok = self._api_remove(tag, email or key_uuid) and ok
            if not ok:
                self._restart()

    def list_server_keys(self) -> list[str]:
        """UUID всех ключей, которые есть в конфиге Xray."""
        return xray_config.list_client_ids(self._read_config())

    def normalize(self) -> bool:
        """email = UUID и каждый ключ во всех inbound. Перезапускает Xray, если что-то поменялось."""
        with self.runner.lock():
            config = self._read_config()
            if not xray_config.normalize_clients(config):
                return False
            self._save_config(config)
            self._restart()
            return True

    def bootstrap(self) -> bool:
        """Включает API/статистику и XHTTP inbound. Перезапускает Xray, если конфиг изменился."""
        with self.runner.lock():
            config = self._read_config()
            try:
                changed = xray_config.ensure_panel_features(
                    config,
                    api_listen=self.config.xray_api_server,
                    xhttp_port=self.config.xray_xhttp_port or None,
                )
            except XrayConfigError as exc:
                raise _bad_gateway(str(exc)) from exc
            if changed:
                self._save_config(config)
                self._restart()
            return changed

    # ---------- ссылки ----------

    def links(self, *, key_uuid: str, name: str) -> list[KeyLink]:
        profile, public_key = self._server_profile()
        return xray_config.build_links(
            profile,
            host=self.config.xray_public_host,
            public_key=public_key,
            key_uuid=key_uuid,
            name=name,
        )

    def reality_params(self) -> tuple[str, str, str]:
        """(sni, public_key, short_id) — сохраняются в БД вместе с ключом."""
        profile, public_key = self._server_profile()
        return profile.sni, public_key, profile.short_id

    # ---------- статистика ----------

    def collect_traffic(self) -> dict[str, TrafficStat]:
        """Трафик по email с момента прошлого вызова (счётчики Xray сбрасываются)."""
        raw = self._api(["statsquery", "-pattern", "user>>>", "-reset"])
        data = json.loads(raw or "{}")
        result: dict[str, dict[str, int]] = {}
        for item in data.get("stat", []) or []:
            parts = item.get("name", "").split(">>>")
            if len(parts) != 4 or parts[0] != "user" or parts[2] != "traffic":
                continue
            result.setdefault(parts[1], {"uplink": 0, "downlink": 0})[parts[3]] = int(item.get("value", 0))
        return {email: TrafficStat(**values) for email, values in result.items()}

    def online_emails(self) -> set[str]:
        raw = self._api(["statsgetallonlineusers"])
        data = json.loads(raw or "{}")
        emails = set()
        for name in data.get("users", []) or []:
            parts = name.split(">>>")
            if len(parts) == 3 and parts[0] == "user":
                emails.add(parts[1])
        return emails

    # ---------- внутреннее ----------

    def _read_config(self) -> dict[str, Any]:
        try:
            raw = self.runner.read_text(self.config.xray_config_path)
        except (XrayCommandError, OSError) as exc:
            raise _bad_gateway(f"Не удалось прочитать конфиг Xray: {exc}") from exc
        try:
            return json.loads(raw)
        except json.JSONDecodeError as exc:
            raise _bad_gateway("Конфиг Xray — некорректный JSON") from exc

    def _save_config(self, config: dict[str, Any]) -> None:
        path = self.config.xray_config_path
        # Xray определяет формат по расширению — временный файл обязан быть .json
        tmp_path = f"{path.removesuffix('.json')}.panel-tmp.json"
        payload = json.dumps(config, indent=2, ensure_ascii=False) + "\n"
        try:
            self.runner.write_text(tmp_path, payload)
            self.runner.run([self.config.xray_binary, "run", "-test", "-config", tmp_path])
        except (XrayCommandError, OSError) as exc:
            self._safe_remove(tmp_path)
            raise _bad_gateway(f"Новый конфиг Xray не прошёл проверку: {exc}") from exc
        try:
            self._backup(path)
            self.runner.replace(tmp_path, path)
        except (XrayCommandError, OSError) as exc:
            raise _bad_gateway(f"Не удалось сохранить конфиг Xray: {exc}") from exc
        self._profile = None

    def _backup(self, path: str) -> None:
        backup_dir = self.config.xray_backup_dir.rstrip("/")
        stamp = datetime.now(UTC).strftime("%Y%m%d%H%M%S%f")
        self.runner.copy(path, f"{backup_dir}/config.{stamp}.json")
        backups = [name for name in self.runner.list_dir(backup_dir) if name.startswith("config.")]
        for name in backups[: max(0, len(backups) - self.config.xray_backup_keep)]:
            self._safe_remove(f"{backup_dir}/{name}")

    def _safe_remove(self, path: str) -> None:
        try:
            self.runner.remove(path)
        except (XrayCommandError, OSError):
            logger.warning("Не удалось удалить %s", path)

    def _restart(self) -> None:
        try:
            self.runner.run(shlex.split(self.config.xray_restart_command), timeout=30)
        except XrayCommandError as exc:
            raise _bad_gateway(f"Не удалось перезапустить Xray: {exc}") from exc

    def _api(self, args: list[str]) -> str:
        try:
            return self.runner.run(
                [self.config.xray_binary, "api", args[0], f"--server={self.config.xray_api_server}", *args[1:]]
            )
        except XrayCommandError as exc:
            raise _bad_gateway(f"API Xray недоступно: {exc}") from exc

    def _api_add(self, payload: dict[str, Any], *, expected: int) -> bool:
        tmp_path = f"{self.config.xray_backup_dir.rstrip('/')}/adu.{time.time_ns()}.json"
        try:
            self.runner.write_text(tmp_path, json.dumps(payload))
            out = self._api(["adu", tmp_path])
        except (HTTPException, XrayCommandError, OSError) as exc:
            logger.warning("adu не сработал, будет перезапуск Xray: %s", exc)
            return False
        finally:
            self._safe_remove(tmp_path)
        return f"Added {expected} user(s)" in out

    def _api_remove(self, tag: str, email: str) -> bool:
        try:
            out = self._api(["rmu", f"-tag={tag}", email])
        except HTTPException as exc:
            logger.warning("rmu не сработал, будет перезапуск Xray: %s", exc.detail)
            return False
        return "Removed 1 user(s)" in out

    def _server_profile(self) -> tuple[ServerProfile, str]:
        now = time.monotonic()
        if self._profile and now - self._profile[0] < PROFILE_TTL_SECONDS:
            return self._profile[1], self._profile[2]
        config = self._read_config()
        try:
            profile = xray_config.server_profile(config)
            out = self.runner.run([self.config.xray_binary, "x25519", "-i", profile.private_key])
            public_key = xray_config.parse_public_key(out)
        except (XrayConfigError, XrayCommandError) as exc:
            raise _bad_gateway(str(exc)) from exc
        self._profile = (now, profile, public_key)
        return profile, public_key


def _bad_gateway(detail: str) -> HTTPException:
    return HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail=detail)


def dry_run_config() -> dict[str, Any]:
    """Конфиг входного сервера для локальной разработки (как после настройки на реальном)."""
    reality = {
        "dest": "127.0.0.1:8444",
        "serverNames": ["vpn.example.com", ""],
        "privateKey": "dry-run-private",
        "shortIds": ["dryrun01"],
    }
    config: dict[str, Any] = {
        "inbounds": [
            {
                "tag": "user-inbound",
                "port": 443,
                "protocol": "vless",
                "settings": {"clients": [], "decryption": "none"},
                "streamSettings": {"network": "tcp", "security": "reality", "realitySettings": reality},
            },
            {
                "tag": "user-inbound-grpc",
                "port": 8443,
                "protocol": "vless",
                "settings": {"clients": [], "decryption": "none"},
                "streamSettings": {
                    "network": "grpc",
                    "security": "reality",
                    "realitySettings": dict(reality),
                    "grpcSettings": {"serviceName": "xray_grpc_service"},
                },
            },
        ],
        "outbounds": [{"protocol": "freedom", "tag": "to-exit"}],
        "routing": {"rules": [{"type": "field", "inboundTag": ["user-inbound", "user-inbound-grpc"], "outboundTag": "to-exit"}]},
    }
    xray_config.ensure_panel_features(config, api_listen="127.0.0.1:10085", xhttp_port=2053)
    return config


def build_xray_manager(config: Settings) -> XrayManager:
    mode = config.effective_xray_mode
    if mode == "local":
        return XrayManager(config, LocalRunner(config.xray_lock_path))
    if mode == "ssh":
        runner = SshRunner(
            host=config.xray_relay_host,
            port=config.xray_relay_port,
            user=config.xray_relay_user,
            key_path=config.xray_ssh_private_key_path,
            known_hosts=config.xray_ssh_known_hosts,
        )
        return XrayManager(config, runner)
    memory = MemoryRunner({config.xray_config_path: json.dumps(dry_run_config())})
    return XrayManager(config, memory)
