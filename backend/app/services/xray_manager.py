from __future__ import annotations

import json
import shlex
import uuid
from dataclasses import dataclass
from typing import Any

import paramiko
from fastapi import HTTPException, status

from app.config import Settings
from app.models import KeyType


@dataclass(frozen=True)
class ProvisionedKey:
    uuid: str
    sni: str
    public_key: str
    short_id: str
    connection_uri: str
    client_config_json: dict[str, Any] | None


class XrayManager:
    tcp_tag = "user-inbound"
    xhttp_tag = "user-inbound-xhttp"

    def __init__(self, config: Settings) -> None:
        self.config = config

    def provision_key(self, *, name: str, key_type: KeyType, sni: str | None) -> ProvisionedKey:
        config = self._read_config()
        inbound = self._find_inbound(config, key_type)
        use_sni = self._resolve_sni(inbound, sni)
        key_uuid = str(uuid.uuid4())
        flow = "xtls-rprx-vision" if key_type == KeyType.TCP_REALITY else ""

        inbound.setdefault("settings", {}).setdefault("clients", []).append(
            {"id": key_uuid, "flow": flow}
        )
        self._apply_config(config)

        public_key = self._get_public_key()
        short_id = self._get_short_id(config)
        connection_uri = self._build_connection_uri(
            key_uuid=key_uuid,
            name=name,
            key_type=key_type,
            sni=use_sni,
            public_key=public_key,
            short_id=short_id,
        )
        client_config = None
        if key_type == KeyType.XHTTP_REALITY:
            client_config = self._build_xhttp_client_config(
                key_uuid=key_uuid,
                sni=use_sni,
                public_key=public_key,
                short_id=short_id,
            )

        return ProvisionedKey(
            uuid=key_uuid,
            sni=use_sni,
            public_key=public_key,
            short_id=short_id,
            connection_uri=connection_uri,
            client_config_json=client_config,
        )

    def revoke_key(self, *, key_uuid: str, key_type: KeyType) -> None:
        config = self._read_config()
        inbound = self._find_inbound(config, key_type)
        clients = inbound.setdefault("settings", {}).setdefault("clients", [])
        inbound["settings"]["clients"] = [client for client in clients if client.get("id") != key_uuid]
        self._apply_config(config)

    def list_sni(self, *, key_type: KeyType = KeyType.TCP_REALITY) -> list[str]:
        config = self._read_config()
        inbound = self._find_inbound(config, key_type)
        return inbound.get("streamSettings", {}).get("realitySettings", {}).get("serverNames", [])

    def _connect(self) -> paramiko.SSHClient:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(
            hostname=self.config.xray_relay_host,
            port=self.config.xray_relay_port,
            username=self.config.xray_relay_user,
            key_filename=self.config.xray_ssh_private_key_path,
            timeout=15,
        )
        return client

    def _exec(self, command: str) -> str:
        with self._connect() as client:
            _, stdout, stderr = client.exec_command(command)
            exit_code = stdout.channel.recv_exit_status()
            out = stdout.read().decode("utf-8").strip()
            err = stderr.read().decode("utf-8").strip()
            if exit_code != 0:
                raise HTTPException(
                    status_code=status.HTTP_502_BAD_GATEWAY,
                    detail=f"Xray command failed: {err or out or command}",
                )
            return out

    def _read_config(self) -> dict[str, Any]:
        raw = self._exec(f"cat {shlex.quote(self.config.xray_config_path)}")
        try:
            return json.loads(raw)
        except json.JSONDecodeError as exc:
            raise HTTPException(status_code=502, detail="Remote Xray config is invalid JSON") from exc

    def _apply_config(self, config: dict[str, Any]) -> None:
        tmp_path = "/tmp/xray-control-config.tmp"
        config_path = shlex.quote(self.config.xray_config_path)
        quoted_tmp_path = shlex.quote(tmp_path)
        payload = json.dumps(config, indent=2, ensure_ascii=False)
        with self._connect() as client:
            sftp = client.open_sftp()
            with sftp.file(tmp_path, "wb") as remote_file:
                remote_file.write(payload.encode("utf-8"))
            sftp.close()
            command = (
                f"xray run -test -config {quoted_tmp_path} >/dev/null 2>&1 "
                f"&& cp {config_path} "
                f"{config_path}.bak.$(date +%Y%m%d%H%M%S) "
                f"&& mv {quoted_tmp_path} {config_path} "
                "&& systemctl restart xray "
                "&& sleep 2 "
                "&& systemctl is-active --quiet xray"
            )
            _, stdout, stderr = client.exec_command(command)
            exit_code = stdout.channel.recv_exit_status()
            if exit_code != 0:
                err = stderr.read().decode("utf-8").strip()
                raise HTTPException(status_code=502, detail=f"Failed to apply Xray config: {err}")

    def _get_public_key(self) -> str:
        command = (
            f"PRIV=$(jq -r '.inbounds[] | select(.tag == \"{self.tcp_tag}\") "
            f"| .streamSettings.realitySettings.privateKey' "
            f"{shlex.quote(self.config.xray_config_path)}); "
            "echo \"$PRIV\" | xargs -I {} xray x25519 -i {} | awk '/PublicKey/ {print $3}'"
        )
        return self._exec(command)

    def _get_short_id(self, config: dict[str, Any]) -> str:
        inbound = self._find_inbound(config, KeyType.TCP_REALITY)
        short_ids = inbound.get("streamSettings", {}).get("realitySettings", {}).get("shortIds", [])
        if not short_ids:
            raise HTTPException(status_code=502, detail="Xray REALITY shortId not found")
        return short_ids[0]

    def _find_inbound(self, config: dict[str, Any], key_type: KeyType) -> dict[str, Any]:
        tag = self.tcp_tag if key_type == KeyType.TCP_REALITY else self.xhttp_tag
        for inbound in config.get("inbounds", []):
            if inbound.get("tag") == tag:
                return inbound
        raise HTTPException(status_code=502, detail=f"Xray inbound not found: {tag}")

    def _resolve_sni(self, inbound: dict[str, Any], requested_sni: str | None) -> str:
        names = inbound.get("streamSettings", {}).get("realitySettings", {}).get("serverNames", [])
        if not names:
            raise HTTPException(status_code=502, detail="Xray serverNames list is empty")
        if requested_sni and requested_sni in names:
            return requested_sni
        if requested_sni and requested_sni not in names:
            raise HTTPException(status_code=400, detail="SNI is not allowed")
        return names[0]

    def _build_connection_uri(
        self,
        *,
        key_uuid: str,
        name: str,
        key_type: KeyType,
        sni: str,
        public_key: str,
        short_id: str,
    ) -> str:
        port = 443 if key_type == KeyType.TCP_REALITY else 80
        flow = "&flow=xtls-rprx-vision" if key_type == KeyType.TCP_REALITY else ""
        return (
            f"vless://{key_uuid}@{self.config.xray_relay_host}:{port}"
            f"?encryption=none{flow}&security=reality&sni={sni}"
            f"&fp=chrome&pbk={public_key}&sid={short_id}#{name}"
        )

    def _build_xhttp_client_config(
        self, *, key_uuid: str, sni: str, public_key: str, short_id: str
    ) -> dict[str, Any]:
        return {
            "log": {"loglevel": "warning"},
            "inbounds": [
                {
                    "listen": "127.0.0.1",
                    "port": 10808,
                    "protocol": "socks",
                    "settings": {"auth": "noauth", "udp": True},
                    "sniffing": {"enabled": True, "destOverride": ["http", "tls"]},
                }
            ],
            "outbounds": [
                {
                    "protocol": "vless",
                    "settings": {
                        "vnext": [
                            {
                                "address": self.config.xray_relay_host,
                                "port": 80,
                                "users": [{"id": key_uuid, "encryption": "none"}],
                            }
                        ]
                    },
                    "streamSettings": {
                        "network": "xhttp",
                        "security": "reality",
                        "realitySettings": {
                            "serverName": sni,
                            "fingerprint": "chrome",
                            "publicKey": public_key,
                            "shortId": short_id,
                        },
                        "xhttpSettings": {
                            "mode": "stream-up",
                            "host": sni,
                            "path": "/download/updates",
                            "headers": {
                                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
                            },
                            "xPaddingBytes": "100-500",
                            "xmux": {
                                "maxConcurrency": 128,
                                "hMaxRequestTimes": 1000,
                                "hMaxReusableSecs": 3600,
                            },
                        },
                    },
                }
            ],
        }


class DryRunXrayManager(XrayManager):
    def __init__(self, config: Settings) -> None:
        super().__init__(config)
        self._config = {
            "inbounds": [
                {
                    "tag": self.tcp_tag,
                    "settings": {"clients": []},
                    "streamSettings": {
                        "realitySettings": {
                            "serverNames": ["www.yahoo.com", "www.google.com"],
                            "privateKey": "dry-run-private",
                            "shortIds": ["dryrun01"],
                        }
                    },
                },
                {
                    "tag": self.xhttp_tag,
                    "settings": {"clients": []},
                    "streamSettings": {
                        "realitySettings": {
                            "serverNames": ["www.yahoo.com", "www.google.com"],
                            "privateKey": "dry-run-private",
                            "shortIds": ["dryrun01"],
                        }
                    },
                },
            ]
        }

    def _read_config(self) -> dict[str, Any]:
        return json.loads(json.dumps(self._config))

    def _apply_config(self, config: dict[str, Any]) -> None:
        self._config = config

    def _get_public_key(self) -> str:
        return "dry-run-public-key"
