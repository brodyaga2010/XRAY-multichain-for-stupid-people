import base64
import json
from datetime import UTC, datetime, timedelta

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.config import settings
from app.models import KeyStatus, UserRole, UserStatus, VpnKey
from app.services import stats
from app.services.xray_manager import XrayManager
from app.services.xray_runner import MemoryRunner
from tests.conftest import make_user

LEGACY_UUID = "33333333-3333-3333-3333-333333333333"


def login(client: TestClient, email: str, password: str = "password123") -> None:
    response = client.post("/auth/login", json={"email": email, "password": password})
    assert response.status_code == 200, response.text


def in_xray(runner: MemoryRunner, key_uuid: str) -> bool:
    config = json.loads(runner.files[settings.xray_config_path])
    tags = [
        inbound["tag"]
        for inbound in config["inbounds"]
        if any(client["id"] == key_uuid for client in inbound["settings"]["clients"])
    ]
    return tags == ["user-inbound", "user-inbound-grpc", "user-inbound-xhttp"]


def test_create_key_returns_three_links_and_adds_to_xray(
    client: TestClient, db: Session, runner: MemoryRunner
) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")

    response = client.post("/keys", json={"name": "  Мой   телефон "})

    assert response.status_code == 201
    body = response.json()
    assert body["name"] == "Мой телефон"
    assert body["type"] == "multi"
    assert [link["transport"] for link in body["links"]] == ["tcp", "grpc", "xhttp"]
    assert body["connection_uri"] == body["links"][0]["uri"]
    assert in_xray(runner, body["uuid"])


@pytest.mark.parametrize("name", ["", "   ", "x" * 61])
def test_create_key_rejects_bad_names(client: TestClient, db: Session, name: str) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")

    assert client.post("/keys", json={"name": name}).status_code == 422


def test_create_key_rolls_back_when_xray_fails(client: TestClient, db: Session, runner: MemoryRunner) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")
    runner.fail_test = True

    response = client.post("/keys", json={"name": "phone"})

    assert response.status_code == 502
    db.rollback()
    assert db.scalars(select(VpnKey)).all() == []


def test_revoke_removes_key_from_xray_and_hides_links(
    client: TestClient, db: Session, runner: MemoryRunner
) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")
    key = client.post("/keys", json={"name": "phone"}).json()

    revoked = client.delete(f"/keys/{key['id']}")
    listed = client.get("/keys").json()

    assert revoked.status_code == 200
    assert not in_xray(runner, key["uuid"])
    assert listed[0]["status"] == "revoked"
    assert listed[0]["links"] == []


def test_suspend_disables_keys_and_restore_returns_them(
    client: TestClient, db: Session, runner: MemoryRunner, xray: XrayManager
) -> None:
    admin = make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    user = make_user(db, email="user@example.com")
    login(client, admin.email)
    created = client.post(f"/admin/users/{user.id}/keys", json={"name": "phone"}).json()
    revoked = client.post(f"/admin/users/{user.id}/keys", json={"name": "old"}).json()
    client.delete(f"/admin/keys/{revoked['id']}")

    suspend = client.post(f"/admin/users/{user.id}/suspend")
    after_suspend = {key["id"]: key["status"] for key in client.get(f"/admin/users/{user.id}/keys").json()}
    gone = not in_xray(runner, created["uuid"])
    restore = client.post(f"/admin/users/{user.id}/restore")
    after_restore = {key["id"]: key["status"] for key in client.get(f"/admin/users/{user.id}/keys").json()}

    assert suspend.status_code == 200 and suspend.json()["status"] == "suspended"
    assert after_suspend == {created["id"]: "suspended", revoked["id"]: "revoked"}
    assert gone
    assert restore.status_code == 200
    assert after_restore == {created["id"]: "active", revoked["id"]: "revoked"}
    assert in_xray(runner, created["uuid"])
    assert not in_xray(runner, revoked["uuid"])


def test_admin_cannot_create_key_for_suspended_user(client: TestClient, db: Session) -> None:
    make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    user = make_user(db, email="user@example.com", status=UserStatus.SUSPENDED)
    login(client, "admin@example.com")

    assert client.post(f"/admin/users/{user.id}/keys", json={"name": "x"}).status_code == 409


def test_subscription_returns_active_links(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")
    first = client.post("/keys", json={"name": "phone"}).json()
    second = client.post("/keys", json={"name": "laptop"}).json()
    client.delete(f"/keys/{second['id']}")
    url = client.get("/keys/subscription").json()["url"]
    token = url.rsplit("/", 1)[1]

    response = client.get(f"/sub/{token}")

    assert url.startswith(settings.public_base_url)
    assert response.status_code == 200
    uris = base64.b64decode(response.text).decode().splitlines()
    assert uris == [link["uri"] for link in first["links"]]
    assert response.headers["profile-update-interval"] == str(settings.subscription_update_hours)
    assert "upload=0; download=0" in response.headers["subscription-userinfo"]


def test_subscription_rotate_invalidates_old_token(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")
    old = client.get("/keys/subscription").json()["url"].rsplit("/", 1)[1]

    new = client.post("/keys/subscription/rotate").json()["url"].rsplit("/", 1)[1]

    assert new != old
    assert client.get(f"/sub/{old}").status_code == 404
    assert client.get(f"/sub/{new}").status_code == 200


@pytest.mark.parametrize("token", ["short", "x" * 32])
def test_subscription_unknown_token(client: TestClient, token: str) -> None:
    assert client.get(f"/sub/{token}").status_code == 404


def test_subscription_of_suspended_user_is_hidden(client: TestClient, db: Session) -> None:
    user = make_user(db, email="user@example.com")
    login(client, "user@example.com")
    token = client.get("/keys/subscription").json()["url"].rsplit("/", 1)[1]
    user.status = UserStatus.SUSPENDED.value
    db.commit()

    assert client.get(f"/sub/{token}").status_code == 404


def test_import_server_keys(client: TestClient, db: Session, runner: MemoryRunner) -> None:
    admin = make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    login(client, admin.email)
    existing = client.post(f"/admin/users/{admin.id}/keys", json={"name": "mine"}).json()
    config = json.loads(runner.files[settings.xray_config_path])
    config["inbounds"][0]["settings"]["clients"].append(
        {"id": LEGACY_UUID, "email": "first-user", "flow": "xtls-rprx-vision"}
    )
    runner.files[settings.xray_config_path] = json.dumps(config)

    first = client.post("/admin/keys/import")
    second = client.post("/admin/keys/import")

    assert first.json() == {"imported": 1}
    assert second.json() == {"imported": 0}
    imported = db.scalar(select(VpnKey).where(VpnKey.uuid == LEGACY_UUID))
    assert imported is not None and imported.owner_id == admin.id
    assert in_xray(runner, LEGACY_UUID)
    assert in_xray(runner, existing["uuid"])


def test_admin_keys_include_owner_email(client: TestClient, db: Session) -> None:
    make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    user = make_user(db, email="user@example.com")
    login(client, "admin@example.com")
    client.post(f"/admin/users/{user.id}/keys", json={"name": "phone"})

    keys = client.get("/admin/keys").json()

    assert keys[0]["owner_email"] == "user@example.com"


def test_admin_audit_log(client: TestClient, db: Session) -> None:
    make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    user = make_user(db, email="user@example.com", status=UserStatus.PENDING)
    login(client, "admin@example.com")
    client.post(f"/admin/users/{user.id}/approve")

    entries = client.get("/admin/audit").json()

    assert entries[0]["action"] == "user.approve"
    assert entries[0]["actor_email"] == "admin@example.com"


def test_audit_is_admin_only(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")

    assert client.get("/admin/audit").status_code == 403


def test_change_password(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")

    wrong = client.post("/auth/password", json={"current_password": "nope", "new_password": "newpassword1"})
    ok = client.post("/auth/password", json={"current_password": "password123", "new_password": "newpassword1"})

    assert wrong.status_code == 400
    assert ok.status_code == 200
    login(client, "user@example.com", "newpassword1")


def test_login_rate_limited(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com")
    codes = [
        client.post("/auth/login", json={"email": "user@example.com", "password": "bad"}).status_code
        for _ in range(settings.auth_rate_limit + 1)
    ]

    assert codes[:-1] == [401] * settings.auth_rate_limit
    assert codes[-1] == 429


def test_stats_collect_accumulates_and_marks_online(
    client: TestClient, db: Session, runner: MemoryRunner, xray: XrayManager
) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")
    key = client.post("/keys", json={"name": "phone"}).json()
    now = datetime.now(UTC)

    runner.stats = {f"user>>>{key['uuid']}>>>traffic>>>uplink": 100, f"user>>>{key['uuid']}>>>traffic>>>downlink": 500}
    runner.online = {key["uuid"]}
    stats.collect_once(db, xray, now=now)
    runner.stats = {f"user>>>{key['uuid']}>>>traffic>>>downlink": 50}
    runner.online = set()
    stats.collect_once(db, xray, now=now + timedelta(seconds=10))
    listed = client.get("/keys").json()[0]

    assert listed["traffic_up"] == 100
    assert listed["traffic_down"] == 550
    assert listed["online"] is True
    stored = db.get(VpnKey, key["id"])
    stored.last_seen_at = now - timedelta(seconds=settings.online_window_seconds + 1)
    db.commit()
    assert client.get("/keys").json()[0]["online"] is False


def test_suspended_key_is_not_online(client: TestClient, db: Session) -> None:
    owner = make_user(db, email="user@example.com")
    db.add(
        VpnKey(
            owner_id=owner.id, uuid="u-1", name="k", type="multi", sni="s", status=KeyStatus.SUSPENDED.value,
            public_key="p", short_id="s", connection_uri="vless://u-1", last_seen_at=datetime.now(UTC),
        )
    )
    db.commit()
    login(client, "user@example.com")

    assert client.get("/keys").json()[0]["online"] is False
