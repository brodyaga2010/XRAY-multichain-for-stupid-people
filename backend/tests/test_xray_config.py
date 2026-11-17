import copy
from urllib.parse import parse_qs, unquote, urlsplit

import pytest

from app.services import xray_config
from app.services.xray_config import XrayConfigError
from app.services.xray_manager import dry_run_config

UUID_A = "11111111-1111-1111-1111-111111111111"
UUID_B = "22222222-2222-2222-2222-222222222222"


def legacy_relay_config() -> dict:
    """Конфиг, как его создаёт generate_config.sh: TCP + gRPC, разные email у одного UUID."""
    reality = {
        "dest": "127.0.0.1:8444",
        "serverNames": ["vpn.example.com", ""],
        "privateKey": "priv",
        "shortIds": ["abcd"],
    }
    return {
        "inbounds": [
            {
                "tag": "user-inbound",
                "port": 443,
                "protocol": "vless",
                "settings": {"clients": [{"id": UUID_A, "flow": "xtls-rprx-vision", "email": "first"}]},
                "streamSettings": {"network": "tcp", "security": "reality", "realitySettings": reality},
            },
            {
                "tag": "user-inbound-grpc",
                "port": 8443,
                "protocol": "vless",
                "settings": {"clients": [{"id": UUID_A, "flow": "", "email": "first-grpc"}]},
                "streamSettings": {
                    "network": "grpc",
                    "security": "reality",
                    "realitySettings": copy.deepcopy(reality),
                    "grpcSettings": {"serviceName": "svc"},
                },
            },
        ],
        "outbounds": [{"protocol": "vless", "tag": "to-exit"}, {"protocol": "freedom", "tag": "direct"}],
        "routing": {
            "rules": [{"type": "field", "inboundTag": ["user-inbound", "user-inbound-grpc"], "outboundTag": "to-exit"}]
        },
    }


def clients_by_tag(config: dict) -> dict[str, list[dict]]:
    return {inbound["tag"]: inbound["settings"]["clients"] for inbound in config["inbounds"] if "tag" in inbound}


def test_ensure_panel_features_adds_api_stats_xhttp_and_routing() -> None:
    config = legacy_relay_config()

    changed = xray_config.ensure_panel_features(config, api_listen="127.0.0.1:10085", xhttp_port=2053)

    assert changed is True
    assert config["api"]["listen"] == "127.0.0.1:10085"
    assert config["policy"]["levels"]["0"]["statsUserOnline"] is True
    xhttp = next(inbound for inbound in config["inbounds"] if inbound["tag"] == "user-inbound-xhttp")
    assert xhttp["port"] == 2053
    assert xhttp["streamSettings"]["realitySettings"]["privateKey"] == "priv"
    assert "user-inbound-xhttp" in config["routing"]["rules"][0]["inboundTag"]
    # один UUID во всех inbound, email = UUID, flow по виду подключения
    assert clients_by_tag(config) == {
        "user-inbound": [{"id": UUID_A, "email": UUID_A, "flow": "xtls-rprx-vision"}],
        "user-inbound-grpc": [{"id": UUID_A, "email": UUID_A, "flow": ""}],
        "user-inbound-xhttp": [{"id": UUID_A, "email": UUID_A, "flow": ""}],
    }


def test_ensure_panel_features_is_idempotent() -> None:
    config = legacy_relay_config()
    xray_config.ensure_panel_features(config, api_listen="127.0.0.1:10085", xhttp_port=2053)
    snapshot = copy.deepcopy(config)

    changed = xray_config.ensure_panel_features(config, api_listen="127.0.0.1:10085", xhttp_port=2053)

    assert changed is False
    assert config == snapshot


def test_ensure_panel_features_without_xhttp() -> None:
    config = legacy_relay_config()

    xray_config.ensure_panel_features(config, api_listen="127.0.0.1:10085", xhttp_port=None)

    assert [inbound["tag"] for inbound in config["inbounds"]] == ["user-inbound", "user-inbound-grpc"]


def test_ensure_panel_features_requires_primary_inbound() -> None:
    config = legacy_relay_config()
    config["inbounds"] = config["inbounds"][1:]

    with pytest.raises(XrayConfigError):
        xray_config.ensure_panel_features(config, api_listen="127.0.0.1:10085", xhttp_port=2053)


def test_add_and_remove_client_in_all_inbounds() -> None:
    config = dry_run_config()

    added = xray_config.add_client(config, UUID_B)
    added_again = xray_config.add_client(config, UUID_B)
    removed = xray_config.remove_client(config, UUID_B)
    removed_again = xray_config.remove_client(config, UUID_B)

    assert added == ["user-inbound", "user-inbound-grpc", "user-inbound-xhttp"]
    assert added_again == []
    assert removed == [(tag, UUID_B) for tag in added]
    assert removed_again == []
    assert all(clients == [] for clients in clients_by_tag(config).values())


def test_add_client_without_user_inbounds_fails() -> None:
    with pytest.raises(XrayConfigError):
        xray_config.add_client({"inbounds": []}, UUID_A)


def test_list_client_ids_unique_in_order() -> None:
    config = dry_run_config()
    xray_config.add_client(config, UUID_B)
    xray_config.add_client(config, UUID_A)

    assert xray_config.list_client_ids(config) == [UUID_B, UUID_A]


@pytest.mark.parametrize(
    ("server_names", "short_ids", "private_key", "error"),
    [
        (["", "vpn.example.com"], ["abcd"], "priv", None),
        ([""], ["abcd"], "priv", "serverNames"),
        (["vpn.example.com"], [], "priv", "shortIds"),
        (["vpn.example.com"], ["abcd"], "", "privateKey"),
    ],
)
def test_server_profile_validation(server_names, short_ids, private_key, error) -> None:
    config = legacy_relay_config()
    reality = config["inbounds"][0]["streamSettings"]["realitySettings"]
    reality.update({"serverNames": server_names, "shortIds": short_ids, "privateKey": private_key})

    if error:
        with pytest.raises(XrayConfigError, match=error):
            xray_config.server_profile(config)
    else:
        profile = xray_config.server_profile(config)
        assert profile.sni == "vpn.example.com"
        assert [(item.transport.key, item.port) for item in profile.transports] == [("tcp", 443), ("grpc", 8443)]


def test_build_links_for_all_transports_with_encoded_name() -> None:
    profile = xray_config.server_profile(dry_run_config())

    links = xray_config.build_links(
        profile, host="vpn.example.com", public_key="PUB", key_uuid=UUID_A, name="Мой телефон"
    )

    assert [link.transport for link in links] == ["tcp", "grpc", "xhttp"]
    parsed = [urlsplit(link.uri) for link in links]
    assert [item.port for item in parsed] == [443, 8443, 2053]
    assert all(item.username == UUID_A and item.hostname == "vpn.example.com" for item in parsed)
    assert unquote(parsed[0].fragment) == "Мой телефон TCP"
    assert " " not in links[0].uri

    tcp, grpc, xhttp = (parse_qs(item.query) for item in parsed)
    assert tcp["flow"] == ["xtls-rprx-vision"] and tcp["type"] == ["tcp"]
    assert tcp["sni"] == ["vpn.example.com"] and tcp["pbk"] == ["PUB"] and tcp["sid"] == ["dryrun01"]
    assert "flow" not in grpc and grpc["serviceName"] == ["xray_grpc_service"] and grpc["mode"] == ["gun"]
    assert xhttp["type"] == ["xhttp"] and xhttp["path"] == ["/download/updates"]


@pytest.mark.parametrize(
    ("output", "expected"),
    [
        ("PrivateKey: a\nPassword (PublicKey): NEWFORMAT\nHash32: z\n", "NEWFORMAT"),
        ("Private key: a\nPublic key: OLDFORMAT\n", "OLDFORMAT"),
        ("PrivateKey: a\n", None),
        ("PrivateKey: a\nPublicKey: MIDFORMAT\n", "MIDFORMAT"),
    ],
)
def test_parse_public_key(output: str, expected: str | None) -> None:
    if expected is None:
        with pytest.raises(XrayConfigError):
            xray_config.parse_public_key(output)
    else:
        assert xray_config.parse_public_key(output) == expected
