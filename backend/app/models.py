from datetime import datetime
from enum import StrEnum

from sqlalchemy import JSON, BigInteger, DateTime, ForeignKey, Integer, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.database import Base


class UserRole(StrEnum):
    USER = "user"
    ADMIN = "admin"


class UserStatus(StrEnum):
    PENDING = "pending"
    APPROVED = "approved"
    SUSPENDED = "suspended"


class KeyType(StrEnum):
    # Один UUID во всех inbound входного сервера: TCP, gRPC и XHTTP
    MULTI = "multi"
    # Устаревшие типы первой версии панели
    TCP_REALITY = "tcp_reality"
    XHTTP_REALITY = "xhttp_reality"


class KeyStatus(StrEnum):
    ACTIVE = "active"
    # Отключён вместе с владельцем (блокировка), вернётся при восстановлении
    SUSPENDED = "suspended"
    REVOKED = "revoked"


class User(Base):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    email: Mapped[str] = mapped_column(String(320), unique=True, index=True, nullable=False)
    password_hash: Mapped[str] = mapped_column(String(255), nullable=False)
    role: Mapped[str] = mapped_column(String(32), default=UserRole.USER.value, nullable=False)
    status: Mapped[str] = mapped_column(String(32), default=UserStatus.PENDING.value, nullable=False)
    key_limit: Mapped[int] = mapped_column(Integer, default=3, nullable=False)
    sub_token: Mapped[str | None] = mapped_column(String(64), unique=True, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now()
    )

    keys: Mapped[list["VpnKey"]] = relationship(back_populates="owner", cascade="all, delete-orphan")


class VpnKey(Base):
    __tablename__ = "vpn_keys"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    owner_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), index=True)
    uuid: Mapped[str] = mapped_column(String(64), unique=True, nullable=False)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    type: Mapped[str] = mapped_column(String(32), nullable=False)
    sni: Mapped[str] = mapped_column(String(255), nullable=False)
    status: Mapped[str] = mapped_column(String(32), default=KeyStatus.ACTIVE.value, nullable=False)
    public_key: Mapped[str] = mapped_column(String(255), nullable=False)
    short_id: Mapped[str] = mapped_column(String(64), nullable=False)
    connection_uri: Mapped[str] = mapped_column(Text, nullable=False)
    client_config_json: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    traffic_up: Mapped[int] = mapped_column(BigInteger, default=0, nullable=False)
    traffic_down: Mapped[int] = mapped_column(BigInteger, default=0, nullable=False)
    last_seen_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now()
    )

    owner: Mapped[User] = relationship(back_populates="keys")


class AuditLog(Base):
    __tablename__ = "audit_logs"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    actor_id: Mapped[int | None] = mapped_column(ForeignKey("users.id", ondelete="SET NULL"))
    action: Mapped[str] = mapped_column(String(120), nullable=False)
    target_type: Mapped[str] = mapped_column(String(80), nullable=False)
    target_id: Mapped[str] = mapped_column(String(80), nullable=False)
    audit_metadata: Mapped[dict] = mapped_column("metadata", JSON, default=dict, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
