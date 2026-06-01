import os
from collections.abc import Generator

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import Session, sessionmaker
from sqlalchemy.pool import StaticPool

os.environ.setdefault("DATABASE_URL", "sqlite+pysqlite:///:memory:")
os.environ.setdefault("JWT_SECRET", "test-secret-that-is-long-enough")
os.environ.setdefault("XRAY_DRY_RUN", "true")

from app.database import Base, get_db  # noqa: E402
from app.dependencies import get_xray_manager  # noqa: E402
from app.main import app  # noqa: E402
from app.models import User, UserRole, UserStatus  # noqa: E402
from app.security import hash_password  # noqa: E402
from app.services.xray_manager import DryRunXrayManager  # noqa: E402
from app.config import settings  # noqa: E402


engine = create_engine(
    "sqlite+pysqlite:///:memory:",
    connect_args={"check_same_thread": False},
    poolclass=StaticPool,
)
TestingSessionLocal = sessionmaker(bind=engine, autoflush=False, autocommit=False, expire_on_commit=False)


@pytest.fixture()
def db() -> Generator[Session, None, None]:
    Base.metadata.drop_all(bind=engine)
    Base.metadata.create_all(bind=engine)
    session = TestingSessionLocal()
    try:
        yield session
    finally:
        session.close()


@pytest.fixture()
def client(db: Session) -> Generator[TestClient, None, None]:
    def override_db() -> Generator[Session, None, None]:
        yield db

    manager = DryRunXrayManager(settings)
    app.dependency_overrides[get_db] = override_db
    app.dependency_overrides[get_xray_manager] = lambda: manager
    with TestClient(app) as test_client:
        yield test_client
    app.dependency_overrides.clear()


def make_user(
    db: Session,
    *,
    email: str,
    password: str = "password123",
    role: UserRole = UserRole.USER,
    status: UserStatus = UserStatus.APPROVED,
    key_limit: int = 3,
) -> User:
    user = User(
        email=email,
        password_hash=hash_password(password),
        role=role.value,
        status=status.value,
        key_limit=key_limit,
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user
