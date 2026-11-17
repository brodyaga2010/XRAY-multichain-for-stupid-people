import json

import pytest
from fastapi import HTTPException

from app.config import settings
from app.services.xray_manager import XrayManager, build_xray_manager
from app.services.xray_runner import MemoryRunner

UUID_A = "11111111-1111-1111-1111-111111111111"
CONFIG = settings.xray_config_path
RESTART = settings.xray_restart_command.split()


@pytest.fixture()
def manager() -> XrayManager:
    return build_xray_manager(settings)


def runner_of(manager: XrayManager) -> MemoryRunner:
    assert isinstance(manager.runner, MemoryRunner)
    return manager.runner


def saved_clients(runner: MemoryRunner) -> dict[str, list[str]]:
    config = json.loads(runner.files[CONFIG])
    return {
        inbound["tag"]: [client["id"] for client in inbound["settings"]["clients"]]
        for inbound in config["inbounds"]
    }


def test_add_key_uses_api_without_restart(manager: XrayManager) -> None:
    runner = runner_of(manager)

    manager.add_key(UUID_A)

    assert all(ids == [UUID_A] for ids in saved_clients(runner).values())
    assert runner.api_users == {tag: {UUID_A} for tag in saved_clients(runner)}
    assert RESTART not in runner.commands
    # временный файл для adu удалён
    assert not [name for name in runner.files if "adu." in name]


def test_add_key_falls_back_to_restart_when_api_fails(manager: XrayManager) -> None:
    runner = runner_of(manager)
    runner.fail_api = True

    manager.add_key(UUID_A)

    assert all(ids == [UUID_A] for ids in saved_clients(runner).values())
    assert RESTART in runner.commands


def test_add_existing_key_is_noop(manager: XrayManager) -> None:
    runner = runner_of(manager)
    manager.add_key(UUID_A)
    commands_before = len(runner.commands)

    manager.add_key(UUID_A)

    assert len(runner.commands) == commands_before


def test_invalid_config_is_not_saved(manager: XrayManager) -> None:
    runner = runner_of(manager)
    original = runner.files[CONFIG]
    runner.fail_test = True

    with pytest.raises(HTTPException) as exc:
        manager.add_key(UUID_A)

    assert exc.value.status_code == 502
    assert runner.files[CONFIG] == original
    assert not [name for name in runner.files if "panel-tmp" in name]
    assert runner.api_users == {}


def test_remove_key_from_all_inbounds(manager: XrayManager) -> None:
    runner = runner_of(manager)
    manager.add_key(UUID_A)

    manager.remove_key(UUID_A)

    assert all(ids == [] for ids in saved_clients(runner).values())
    assert all(users == set() for users in runner.api_users.values())
    assert RESTART not in runner.commands


def test_remove_key_restarts_when_user_missing_in_api(manager: XrayManager) -> None:
    runner = runner_of(manager)
    manager.add_key(UUID_A)
    runner.api_users.clear()  # например, Xray перезапустили со старым конфигом

    manager.remove_key(UUID_A)

    assert RESTART in runner.commands


def test_backups_are_rotated(manager: XrayManager) -> None:
    runner = runner_of(manager)
    keep = settings.xray_backup_keep

    for index in range(keep + 5):
        manager.add_key(f"00000000-0000-0000-0000-{index:012d}")

    backups = runner.list_dir(settings.xray_backup_dir)
    assert len(backups) == keep


def test_links_and_reality_params(manager: XrayManager) -> None:
    links = manager.links(key_uuid=UUID_A, name="phone")

    assert [link.transport for link in links] == ["tcp", "grpc", "xhttp"]
    assert manager.reality_params() == ("vpn.example.com", "dry-run-public-key", "dryrun01")


def test_collect_traffic_and_online(manager: XrayManager) -> None:
    runner = runner_of(manager)
    runner.stats = {
        f"user>>>{UUID_A}>>>traffic>>>uplink": 10,
        f"user>>>{UUID_A}>>>traffic>>>downlink": 20,
        "inbound>>>user-inbound>>>traffic>>>uplink": 99,
    }
    runner.online = {UUID_A}

    traffic = manager.collect_traffic()
    online = manager.online_emails()

    assert traffic[UUID_A].uplink == 10 and traffic[UUID_A].downlink == 20
    assert len(traffic) == 1
    assert online == {UUID_A}
    assert manager.collect_traffic() == {}  # счётчики сброшены


def test_import_normalize_restarts_once(manager: XrayManager) -> None:
    runner = runner_of(manager)
    config = json.loads(runner.files[CONFIG])
    config["inbounds"][0]["settings"]["clients"] = [{"id": UUID_A, "email": "legacy", "flow": "xtls-rprx-vision"}]
    runner.files[CONFIG] = json.dumps(config)

    assert manager.normalize() is True
    assert manager.normalize() is False
    assert runner.commands.count(RESTART) == 1
    assert all(ids == [UUID_A] for ids in saved_clients(runner).values())
