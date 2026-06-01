from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

from app.models import UserStatus
from tests.conftest import make_user


def test_register_creates_pending_user(client: TestClient, db: Session) -> None:
    response = client.post(
        "/auth/register",
        json={"email": "new@example.com", "password": "password123"},
    )

    assert response.status_code == 201
    assert response.json()["user"]["status"] == "pending"


def test_pending_user_cannot_login(client: TestClient, db: Session) -> None:
    make_user(db, email="pending@example.com", status=UserStatus.PENDING)

    response = client.post(
        "/auth/login",
        json={"email": "pending@example.com", "password": "password123"},
    )

    assert response.status_code == 403


def test_suspended_user_cannot_login(client: TestClient, db: Session) -> None:
    make_user(db, email="suspended@example.com", status=UserStatus.SUSPENDED)

    response = client.post(
        "/auth/login",
        json={"email": "suspended@example.com", "password": "password123"},
    )

    assert response.status_code == 403


def test_approved_user_can_login(client: TestClient, db: Session) -> None:
    make_user(db, email="ok@example.com")

    response = client.post(
        "/auth/login",
        json={"email": "ok@example.com", "password": "password123"},
    )

    assert response.status_code == 200
    assert response.json()["user"]["email"] == "ok@example.com"
    assert "xray_access_token" in response.cookies
