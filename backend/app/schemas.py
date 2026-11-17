from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict, EmailStr, Field, field_validator

from app.models import KeyStatus, KeyType, UserRole, UserStatus


class UserCreate(BaseModel):
    email: EmailStr
    password: str = Field(min_length=8, max_length=128)


class UserLogin(BaseModel):
    email: EmailStr
    password: str


class UserOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    email: EmailStr
    role: UserRole
    status: UserStatus
    key_limit: int
    created_at: datetime


class AuthSession(BaseModel):
    user: UserOut


class KeyCreate(BaseModel):
    """Ключ создаётся сразу для всех видов подключения; type/sni первой версии игнорируются."""

    name: str = Field(min_length=1, max_length=60)

    @field_validator("name")
    @classmethod
    def strip_name(cls, value: str) -> str:
        value = " ".join(value.split())
        if not value:
            raise ValueError("Название не может быть пустым")
        return value


class AdminKeyCreate(KeyCreate):
    pass


class KeyLinkOut(BaseModel):
    transport: str
    label: str
    uri: str


class VpnKeyOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    owner_id: int
    uuid: str
    name: str
    type: KeyType
    sni: str
    status: KeyStatus
    public_key: str
    short_id: str
    connection_uri: str
    client_config_json: dict[str, Any] | None
    created_at: datetime
    traffic_up: int = 0
    traffic_down: int = 0
    last_seen_at: datetime | None = None
    links: list[KeyLinkOut] = []
    online: bool = False
    owner_email: str | None = None


class LimitUpdate(BaseModel):
    key_limit: int = Field(ge=0, le=100)


class Message(BaseModel):
    message: str


class PasswordChange(BaseModel):
    current_password: str
    new_password: str = Field(min_length=8, max_length=128)


class SubscriptionOut(BaseModel):
    url: str


class ImportResult(BaseModel):
    imported: int


class AuditLogOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    actor_id: int | None
    actor_email: str | None = None
    action: str
    target_type: str
    target_id: str
    audit_metadata: dict[str, Any]
    created_at: datetime
