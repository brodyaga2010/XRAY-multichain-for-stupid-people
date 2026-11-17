"""Чистые функции над config.json входного (relay) сервера Xray.

Ключ = один UUID, который присутствует во всех пользовательских inbound (TCP, gRPC, XHTTP).
В Xray у клиента email = UUID, по нему считаются трафик и онлайн.
"""

from __future__ import annotations

import copy
from dataclasses import dataclass
from typing import Any
from urllib.parse import quote, urlencode


@dataclass(frozen=True)
class Transport:
    key: str
    tag: str
    network: str
    flow: str
    label: str


TRANSPORTS: tuple[Transport, ...] = (
    Transport("tcp", "user-inbound", "tcp", "xtls-rprx-vision", "TCP"),
    Transport("grpc", "user-inbound-grpc", "grpc", "", "gRPC"),
    Transport("xhttp", "user-inbound-xhttp", "xhttp", "", "XHTTP"),
)
TRANSPORT_BY_TAG = {transport.tag: transport for transport in TRANSPORTS}
PRIMARY_TAG = TRANSPORTS[0].tag

DEFAULT_GRPC_SERVICE = "xray_grpc_service"
DEFAULT_XHTTP_PATH = "/download/updates"
EXIT_OUTBOUND_TAG = "to-exit"


class XrayConfigError(ValueError):
    pass


@dataclass(frozen=True)
class TransportProfile:
    transport: Transport
    port: int
    grpc_service: str | None = None
    xhttp_path: str | None = None


@dataclass(frozen=True)
class ServerProfile:
    sni: str
    short_id: str
    private_key: str
    transports: tuple[TransportProfile, ...]


@dataclass(frozen=True)
class KeyLink:
    transport: str
    label: str
    uri: str


def user_inbounds(config: dict[str, Any]) -> list[dict[str, Any]]:
    """Пользовательские inbound в порядке TRANSPORTS (отсутствующие пропускаются)."""
    by_tag = {inbound.get("tag"): inbound for inbound in config.get("inbounds", [])}
    return [by_tag[t.tag] for t in TRANSPORTS if t.tag in by_tag]


def _clients(inbound: dict[str, Any]) -> list[dict[str, Any]]:
    return inbound.setdefault("settings", {}).setdefault("clients", [])


def client_entry(inbound: dict[str, Any], key_uuid: str) -> dict[str, Any]:
    transport = TRANSPORT_BY_TAG[inbound["tag"]]
    return {"id": key_uuid, "email": key_uuid, "flow": transport.flow}


def list_client_ids(config: dict[str, Any]) -> list[str]:
    """Все UUID из пользовательских inbound без повторов, в порядке появления."""
    seen: dict[str, None] = {}
    for inbound in user_inbounds(config):
        for client in _clients(inbound):
            if client.get("id"):
                seen.setdefault(client["id"], None)
    return list(seen)


def add_client(config: dict[str, Any], key_uuid: str) -> list[str]:
    """Добавляет UUID во все пользовательские inbound. Возвращает теги, куда реально добавлен."""
    inbounds = user_inbounds(config)
    if not inbounds:
        raise XrayConfigError("В конфиге Xray нет пользовательских inbound")
    added: list[str] = []
    for inbound in inbounds:
        clients = _clients(inbound)
        if any(client.get("id") == key_uuid for client in clients):
            continue
        clients.append(client_entry(inbound, key_uuid))
        added.append(inbound["tag"])
    return added


def remove_client(config: dict[str, Any], key_uuid: str) -> list[tuple[str, str]]:
    """Удаляет UUID из всех пользовательских inbound. Возвращает пары (tag, email) удалённых."""
    removed: list[tuple[str, str]] = []
    for inbound in user_inbounds(config):
        clients = _clients(inbound)
        keep = []
        for client in clients:
            if client.get("id") == key_uuid:
                removed.append((inbound["tag"], client.get("email") or ""))
            else:
                keep.append(client)
        inbound["settings"]["clients"] = keep
    return removed


def normalize_clients(config: dict[str, Any]) -> bool:
    """email = UUID у всех клиентов, правильный flow, каждый UUID во всех inbound.

    Возвращает True, если конфиг изменился (нужен перезапуск Xray).
    """
    before = copy.deepcopy(config)
    ids = list_client_ids(config)
    for inbound in user_inbounds(config):
        inbound["settings"]["clients"] = [client_entry(inbound, key_uuid) for key_uuid in ids]
    return before != config


def ensure_panel_features(
    config: dict[str, Any], *, api_listen: str, xhttp_port: int | None
) -> bool:
    """Включает API/статистику Xray и (опционально) XHTTP inbound. Идемпотентно.

    Возвращает True, если конфиг изменился.
    """
    before = copy.deepcopy(config)

    config["api"] = {"tag": "api", "listen": api_listen, "services": ["HandlerService", "StatsService"]}
    config.setdefault("stats", {})
    levels = config.setdefault("policy", {}).setdefault("levels", {})
    level0 = levels.setdefault("0", {})
    level0.update({"statsUserUplink": True, "statsUserDownlink": True, "statsUserOnline": True})

    by_tag = {inbound.get("tag"): inbound for inbound in config.get("inbounds", [])}
    primary = by_tag.get(PRIMARY_TAG)
    if primary is None:
        raise XrayConfigError(f"Не найден inbound {PRIMARY_TAG}")

    xhttp_tag = TRANSPORTS[2].tag
    if xhttp_port and xhttp_tag not in by_tag:
        reality = copy.deepcopy(primary["streamSettings"]["realitySettings"])
        config["inbounds"].append(
            {
                "listen": "0.0.0.0",
                "port": xhttp_port,
                "protocol": "vless",
                "tag": xhttp_tag,
                "settings": {"clients": [], "decryption": "none"},
                "streamSettings": {
                    "network": "xhttp",
                    "security": "reality",
                    "realitySettings": reality,
                    "xhttpSettings": {"path": DEFAULT_XHTTP_PATH, "mode": "auto"},
                },
                "sniffing": {"enabled": True, "destOverride": ["http", "tls", "quic"]},
            }
        )

    present = [inbound["tag"] for inbound in user_inbounds(config)]
    rules = config.setdefault("routing", {}).setdefault("rules", [])
    exit_rule = next((rule for rule in rules if rule.get("outboundTag") == EXIT_OUTBOUND_TAG), None)
    if exit_rule is not None:
        tags = exit_rule.setdefault("inboundTag", [])
        for tag in present:
            if tag not in tags:
                tags.append(tag)

    normalize_clients(config)
    return before != config


def server_profile(config: dict[str, Any]) -> ServerProfile:
    inbounds = user_inbounds(config)
    if not inbounds or inbounds[0]["tag"] != PRIMARY_TAG:
        raise XrayConfigError(f"Не найден inbound {PRIMARY_TAG}")
    reality = inbounds[0].get("streamSettings", {}).get("realitySettings", {})
    names = [name for name in reality.get("serverNames", []) if name]
    short_ids = [sid for sid in reality.get("shortIds", []) if sid]
    if not names:
        raise XrayConfigError("В realitySettings.serverNames нет непустого имени")
    if not short_ids:
        raise XrayConfigError("В realitySettings.shortIds нет значения")
    if not reality.get("privateKey"):
        raise XrayConfigError("В realitySettings нет privateKey")

    profiles = []
    for inbound in inbounds:
        transport = TRANSPORT_BY_TAG[inbound["tag"]]
        stream = inbound.get("streamSettings", {})
        profiles.append(
            TransportProfile(
                transport=transport,
                port=int(inbound["port"]),
                grpc_service=stream.get("grpcSettings", {}).get("serviceName", DEFAULT_GRPC_SERVICE)
                if transport.network == "grpc"
                else None,
                xhttp_path=stream.get("xhttpSettings", {}).get("path", DEFAULT_XHTTP_PATH)
                if transport.network == "xhttp"
                else None,
            )
        )
    return ServerProfile(
        sni=names[0], short_id=short_ids[0], private_key=reality["privateKey"], transports=tuple(profiles)
    )


def build_links(
    profile: ServerProfile, *, host: str, public_key: str, key_uuid: str, name: str
) -> list[KeyLink]:
    links = []
    for item in profile.transports:
        params: dict[str, str] = {"encryption": "none"}
        if item.transport.flow:
            params["flow"] = item.transport.flow
        params.update(
            {
                "security": "reality",
                "sni": profile.sni,
                "fp": "chrome",
                "pbk": public_key,
                "sid": profile.short_id,
                "type": item.transport.network,
            }
        )
        if item.grpc_service is not None:
            params.update({"serviceName": item.grpc_service, "mode": "gun"})
        if item.xhttp_path is not None:
            params.update({"path": item.xhttp_path, "mode": "auto"})
        fragment = quote(f"{name} {item.transport.label}", safe="")
        uri = f"vless://{key_uuid}@{host}:{item.port}?{urlencode(params, quote_via=quote)}#{fragment}"
        links.append(KeyLink(transport=item.transport.key, label=item.transport.label, uri=uri))
    return links


def parse_public_key(x25519_output: str) -> str:
    """Публичный ключ из вывода `xray x25519 -i` (старый и новый формат)."""
    for line in x25519_output.splitlines():
        head, _, value = line.partition(":")
        if "publickey" in head.replace(" ", "").lower() and value.strip():
            return value.strip()
    raise XrayConfigError("Не удалось получить публичный ключ из вывода xray x25519")
