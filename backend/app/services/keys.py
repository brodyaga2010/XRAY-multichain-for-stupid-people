"""Операции с ключами: БД и Xray меняются согласованно."""

from __future__ import annotations

import logging
import secrets
import uuid
from datetime import UTC, datetime, timedelta

from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.audit import write_audit
from app.config import settings
from app.models import KeyStatus, KeyType, User, VpnKey
from app.schemas import KeyLinkOut, VpnKeyOut
from app.services.xray_manager import XrayManager

logger = logging.getLogger(__name__)

IMPORTED_KEY_NAME = "Импортирован с сервера"


def provision_key(
    db: Session, xray: XrayManager, *, owner: User, name: str, actor: User, action: str
) -> VpnKey:
    key_uuid = str(uuid.uuid4())
    sni, public_key, short_id = xray.reality_params()
    links = xray.links(key_uuid=key_uuid, name=name)
    key = VpnKey(
        owner_id=owner.id,
        uuid=key_uuid,
        name=name,
        type=KeyType.MULTI.value,
        sni=sni,
        status=KeyStatus.ACTIVE.value,
        public_key=public_key,
        short_id=short_id,
        connection_uri=links[0].uri,
        client_config_json=None,
    )
    db.add(key)
    db.flush()
    xray.add_key(key_uuid)
    write_audit(
        db,
        actor=actor,
        action=action,
        target_type="vpn_key",
        target_id=key.id,
        metadata={"uuid": key_uuid, "owner_id": owner.id, "name": name},
    )
    try:
        db.commit()
    except Exception:
        db.rollback()
        _remove_quietly(xray, key_uuid)
        raise
    db.refresh(key)
    return key


def revoke_key(db: Session, xray: XrayManager, *, key: VpnKey, actor: User, action: str) -> None:
    if key.status == KeyStatus.REVOKED.value:
        return
    if key.status == KeyStatus.ACTIVE.value:
        xray.remove_key(key.uuid)
    key.status = KeyStatus.REVOKED.value
    write_audit(
        db,
        actor=actor,
        action=action,
        target_type="vpn_key",
        target_id=key.id,
        metadata={"uuid": key.uuid, "owner_id": key.owner_id},
    )
    db.commit()


def suspend_keys(db: Session, xray: XrayManager, user: User) -> int:
    """Отключает активные ключи пользователя в Xray (при блокировке)."""
    keys = _keys_with_status(db, user, KeyStatus.ACTIVE)
    for key in keys:
        xray.remove_key(key.uuid)
        key.status = KeyStatus.SUSPENDED.value
        db.flush()
    return len(keys)


def restore_keys(db: Session, xray: XrayManager, user: User) -> int:
    """Возвращает в Xray ключи, отключённые блокировкой."""
    keys = _keys_with_status(db, user, KeyStatus.SUSPENDED)
    for key in keys:
        xray.add_key(key.uuid)
        key.status = KeyStatus.ACTIVE.value
        db.flush()
    return len(keys)


def import_server_keys(db: Session, xray: XrayManager, *, admin: User) -> list[VpnKey]:
    """Ключи, которые есть в Xray, но не в БД (созданы скриптами), закрепляются за администратором."""
    xray.normalize()
    known = set(db.scalars(select(VpnKey.uuid)))
    sni, public_key, short_id = xray.reality_params()
    imported = []
    for key_uuid in xray.list_server_keys():
        if key_uuid in known:
            continue
        name = f"{IMPORTED_KEY_NAME} {key_uuid[:8]}"
        links = xray.links(key_uuid=key_uuid, name=name)
        key = VpnKey(
            owner_id=admin.id,
            uuid=key_uuid,
            name=name,
            type=KeyType.MULTI.value,
            sni=sni,
            status=KeyStatus.ACTIVE.value,
            public_key=public_key,
            short_id=short_id,
            connection_uri=links[0].uri,
        )
        db.add(key)
        imported.append(key)
    if imported:
        db.flush()
        write_audit(
            db,
            actor=admin,
            action="admin.key.import",
            target_type="vpn_key",
            target_id=",".join(str(key.id) for key in imported),
            metadata={"count": len(imported)},
        )
    db.commit()
    return imported


def ensure_sub_token(db: Session, user: User, *, rotate: bool = False) -> str:
    if rotate or not user.sub_token:
        user.sub_token = secrets.token_urlsafe(24)
        db.commit()
    return user.sub_token


def subscription_url(token: str) -> str:
    return f"{settings.public_base_url.rstrip('/')}/sub/{token}"


def is_online(key: VpnKey, now: datetime | None = None) -> bool:
    if key.status != KeyStatus.ACTIVE.value or key.last_seen_at is None:
        return False
    now = now or datetime.now(UTC)
    seen = key.last_seen_at if key.last_seen_at.tzinfo else key.last_seen_at.replace(tzinfo=UTC)
    return now - seen <= timedelta(seconds=settings.online_window_seconds)


def key_out(key: VpnKey, xray: XrayManager, *, owner_email: str | None = None) -> VpnKeyOut:
    links: list[KeyLinkOut] = []
    if key.status != KeyStatus.REVOKED.value:
        try:
            links = [
                KeyLinkOut(transport=link.transport, label=link.label, uri=link.uri)
                for link in xray.links(key_uuid=key.uuid, name=key.name)
            ]
        except HTTPException as exc:
            logger.warning("Не удалось построить ссылки ключа %s: %s", key.id, exc.detail)
    out = VpnKeyOut.model_validate(key)
    return out.model_copy(
        update={"links": links, "online": is_online(key), "owner_email": owner_email}
    )


def _keys_with_status(db: Session, user: User, key_status: KeyStatus) -> list[VpnKey]:
    return list(
        db.scalars(select(VpnKey).where(VpnKey.owner_id == user.id, VpnKey.status == key_status.value))
    )


def _remove_quietly(xray: XrayManager, key_uuid: str) -> None:
    try:
        xray.remove_key(key_uuid)
    except HTTPException:
        logger.exception("Ключ %s остался в Xray без записи в БД", key_uuid)
