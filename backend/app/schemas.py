from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict, EmailStr, Field

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
    name: str = Field(min_length=1, max_length=120)
    type: KeyType
    sni: str | None = Field(default=None, max_length=255)


class AdminKeyCreate(KeyCreate):
    owner_id: int | None = None


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


class LimitUpdate(BaseModel):
    key_limit: int = Field(ge=0, le=100)


class Message(BaseModel):
    message: str
