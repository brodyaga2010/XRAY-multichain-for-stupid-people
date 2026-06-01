from fastapi.testclient import TestClient
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.models import AuditLog, UserRole, UserStatus, VpnKey
from tests.conftest import make_user


def login(client: TestClient, email: str, password: str = "password123") -> None:
    response = client.post("/auth/login", json={"email": email, "password": password})
    assert response.status_code == 200


def test_key_limit_blocks_create_over_limit(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com", key_limit=1)
    login(client, "user@example.com")

    first = client.post("/keys", json={"name": "phone", "type": "tcp_reality"})
    second = client.post("/keys", json={"name": "laptop", "type": "tcp_reality"})

    assert first.status_code == 201
    assert second.status_code == 409


def test_user_cannot_see_other_users_keys(client: TestClient, db: Session) -> None:
    owner = make_user(db, email="owner@example.com")
    make_user(db, email="viewer@example.com")
    db.add(
        VpnKey(
            owner_id=owner.id,
            uuid="existing-uuid",
            name="owner key",
            type="tcp_reality",
            sni="www.yahoo.com",
            status="active",
            public_key="public",
            short_id="short",
            connection_uri="vless://existing-uuid",
        )
    )
    db.commit()
    login(client, "viewer@example.com")

    response = client.get("/keys")

    assert response.status_code == 200
    assert response.json() == []


def test_admin_can_list_all_keys(client: TestClient, db: Session) -> None:
    admin = make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    owner = make_user(db, email="owner@example.com")
    db.add(
        VpnKey(
            owner_id=owner.id,
            uuid="admin-visible-uuid",
            name="owner key",
            type="tcp_reality",
            sni="www.yahoo.com",
            status="active",
            public_key="public",
            short_id="short",
            connection_uri="vless://admin-visible-uuid",
        )
    )
    db.commit()
    login(client, admin.email)

    response = client.get("/admin/keys")

    assert response.status_code == 200
    assert response.json()[0]["uuid"] == "admin-visible-uuid"


def test_admin_can_approve_suspend_restore_and_update_limit(client: TestClient, db: Session) -> None:
    make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    user = make_user(db, email="pending@example.com", status=UserStatus.PENDING)
    login(client, "admin@example.com")

    approve = client.post(f"/admin/users/{user.id}/approve")
    limit = client.patch(f"/admin/users/{user.id}/limits", json={"key_limit": 8})
    suspend = client.post(f"/admin/users/{user.id}/suspend")
    restore = client.post(f"/admin/users/{user.id}/restore")

    assert approve.status_code == 200
    assert approve.json()["status"] == "approved"
    assert limit.status_code == 200
    assert limit.json()["key_limit"] == 8
    assert suspend.status_code == 200
    assert suspend.json()["status"] == "suspended"
    assert restore.status_code == 200
    assert restore.json()["status"] == "approved"


def test_audit_log_written_for_create_and_revoke(client: TestClient, db: Session) -> None:
    make_user(db, email="user@example.com")
    login(client, "user@example.com")

    created = client.post("/keys", json={"name": "phone", "type": "tcp_reality"})
    key_id = created.json()["id"]
    revoked = client.delete(f"/keys/{key_id}")

    actions = [row.action for row in db.scalars(select(AuditLog).order_by(AuditLog.id))]
    assert created.status_code == 201
    assert revoked.status_code == 200
    assert "key.create" in actions
    assert "key.revoke" in actions


def test_admin_cannot_suspend_self(client: TestClient, db: Session) -> None:
    admin = make_user(db, email="admin@example.com", role=UserRole.ADMIN, key_limit=100)
    login(client, "admin@example.com")

    response = client.post(f"/admin/users/{admin.id}/suspend")

    assert response.status_code == 400
