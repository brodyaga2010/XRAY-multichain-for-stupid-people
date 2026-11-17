from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.audit import write_audit
from app.database import get_db
from app.dependencies import get_xray_manager, require_admin
from app.models import AuditLog, User, UserStatus, VpnKey
from app.schemas import (
    AdminKeyCreate,
    AuditLogOut,
    ImportResult,
    LimitUpdate,
    Message,
    UserOut,
    VpnKeyOut,
)
from app.services import keys as key_service
from app.services.xray_manager import XrayManager

router = APIRouter(prefix="/admin", tags=["admin"])


@router.get("/users", response_model=list[UserOut])
def list_users(
    db: Session = Depends(get_db),
    _: User = Depends(require_admin),
) -> list[User]:
    return list(db.scalars(select(User).order_by(User.created_at.desc(), User.id.desc())))


@router.get("/users/{user_id}/keys", response_model=list[VpnKeyOut])
def list_user_keys(
    user_id: int,
    db: Session = Depends(get_db),
    _: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> list[VpnKeyOut]:
    user = _get_user(db, user_id)
    keys = db.scalars(
        select(VpnKey).where(VpnKey.owner_id == user_id).order_by(VpnKey.created_at.desc(), VpnKey.id.desc())
    )
    return [key_service.key_out(key, xray, owner_email=user.email) for key in keys]


@router.get("/keys", response_model=list[VpnKeyOut])
def list_all_keys(
    db: Session = Depends(get_db),
    _: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> list[VpnKeyOut]:
    rows = db.execute(
        select(VpnKey, User.email)
        .join(User, User.id == VpnKey.owner_id)
        .order_by(VpnKey.created_at.desc(), VpnKey.id.desc())
    )
    return [key_service.key_out(key, xray, owner_email=email) for key, email in rows]


@router.post("/keys/import", response_model=ImportResult)
def import_keys(
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> ImportResult:
    imported = key_service.import_server_keys(db, xray, admin=admin)
    return ImportResult(imported=len(imported))


@router.post("/users/{user_id}/approve", response_model=UserOut)
def approve_user(
    user_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> User:
    return _activate(db, xray, admin, user_id, action="user.approve")


@router.post("/users/{user_id}/restore", response_model=UserOut)
def restore_user(
    user_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> User:
    return _activate(db, xray, admin, user_id, action="user.restore")


@router.post("/users/{user_id}/suspend", response_model=UserOut)
def suspend_user(
    user_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> User:
    if user_id == admin.id:
        raise HTTPException(status_code=400, detail="Нельзя заблокировать самого себя")
    user = _get_user(db, user_id)
    disabled = key_service.suspend_keys(db, xray, user)
    user.status = UserStatus.SUSPENDED.value
    write_audit(
        db, actor=admin, action="user.suspend", target_type="user", target_id=user.id,
        metadata={"keys_disabled": disabled},
    )
    db.commit()
    db.refresh(user)
    return user


@router.patch("/users/{user_id}/limits", response_model=UserOut)
def update_limits(
    user_id: int,
    payload: LimitUpdate,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
) -> User:
    user = _get_user(db, user_id)
    user.key_limit = payload.key_limit
    write_audit(
        db,
        actor=admin,
        action="user.limit_update",
        target_type="user",
        target_id=user.id,
        metadata={"key_limit": payload.key_limit},
    )
    db.commit()
    db.refresh(user)
    return user


@router.post("/users/{user_id}/keys", response_model=VpnKeyOut, status_code=status.HTTP_201_CREATED)
def create_key_for_user(
    user_id: int,
    payload: AdminKeyCreate,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> VpnKeyOut:
    user = _get_user(db, user_id)
    if user.status == UserStatus.SUSPENDED.value:
        raise HTTPException(status_code=409, detail="Пользователь заблокирован")
    key = key_service.provision_key(
        db, xray, owner=user, name=payload.name, actor=admin, action="admin.key.create"
    )
    return key_service.key_out(key, xray, owner_email=user.email)


@router.delete("/keys/{key_id}", response_model=Message)
def revoke_any_key(
    key_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> Message:
    key = db.get(VpnKey, key_id)
    if not key:
        raise HTTPException(status_code=404, detail="Ключ не найден")
    key_service.revoke_key(db, xray, key=key, actor=admin, action="admin.key.revoke")
    return Message(message="Ключ отозван")


@router.get("/audit", response_model=list[AuditLogOut])
def audit_log(
    limit: int = Query(default=200, ge=1, le=1000),
    db: Session = Depends(get_db),
    _: User = Depends(require_admin),
) -> list[AuditLogOut]:
    rows = db.execute(
        select(AuditLog, User.email)
        .outerjoin(User, User.id == AuditLog.actor_id)
        .order_by(AuditLog.created_at.desc(), AuditLog.id.desc())
        .limit(limit)
    )
    return [
        AuditLogOut.model_validate(entry).model_copy(update={"actor_email": email}) for entry, email in rows
    ]


def _get_user(db: Session, user_id: int) -> User:
    user = db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="Пользователь не найден")
    return user


def _activate(db: Session, xray: XrayManager, admin: User, user_id: int, *, action: str) -> User:
    user = _get_user(db, user_id)
    restored = key_service.restore_keys(db, xray, user)
    user.status = UserStatus.APPROVED.value
    write_audit(
        db, actor=admin, action=action, target_type="user", target_id=user.id,
        metadata={"keys_restored": restored},
    )
    db.commit()
    db.refresh(user)
    return user
