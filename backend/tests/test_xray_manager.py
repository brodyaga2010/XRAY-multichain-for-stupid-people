from app.config import settings
from app.models import KeyType
from app.services.xray_manager import DryRunXrayManager


def test_xray_manager_add_and_remove_by_tag() -> None:
    manager = DryRunXrayManager(settings)

    provisioned = manager.provision_key(name="phone", key_type=KeyType.TCP_REALITY, sni=None)
    config = manager._read_config()
    tcp_clients = config["inbounds"][0]["settings"]["clients"]
    xhttp_clients = config["inbounds"][1]["settings"]["clients"]

    assert tcp_clients[0]["id"] == provisioned.uuid
    assert xhttp_clients == []

    manager.revoke_key(key_uuid=provisioned.uuid, key_type=KeyType.TCP_REALITY)

    assert manager._read_config()["inbounds"][0]["settings"]["clients"] == []


def test_xray_manager_xhttp_uses_xhttp_inbound() -> None:
    manager = DryRunXrayManager(settings)

    provisioned = manager.provision_key(name="laptop", key_type=KeyType.XHTTP_REALITY, sni="www.google.com")
    config = manager._read_config()

    assert config["inbounds"][1]["settings"]["clients"][0]["id"] == provisioned.uuid
    assert provisioned.client_config_json is not None
    assert provisioned.sni == "www.google.com"
