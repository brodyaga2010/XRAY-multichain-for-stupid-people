from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.orm import Session, selectinload

from app.audit import write_audit
from app.database import get_db
from app.dependencies import get_xray_manager, require_admin
from app.models import KeyStatus, KeyType, User, UserStatus, VpnKey
from app.schemas import AdminKeyCreate, LimitUpdate, Message, UserOut, VpnKeyOut
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
) -> list[VpnKey]:
    user = db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    return list(
        db.scalars(
            select(VpnKey).where(VpnKey.owner_id == user_id).order_by(VpnKey.created_at.desc())
        )
    )


@router.get("/keys", response_model=list[VpnKeyOut])
def list_all_keys(
    db: Session = Depends(get_db),
    _: User = Depends(require_admin),
) -> list[VpnKey]:
    return list(db.scalars(select(VpnKey).order_by(VpnKey.created_at.desc(), VpnKey.id.desc())))


@router.post("/users/{user_id}/approve", response_model=UserOut)
def approve_user(
    user_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
) -> User:
    user = db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    user.status = UserStatus.APPROVED.value
    write_audit(db, actor=admin, action="user.approve", target_type="user", target_id=user.id)
    db.commit()
    db.refresh(user)
    return user


@router.post("/users/{user_id}/suspend", response_model=UserOut)
def suspend_user(
    user_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
) -> User:
    if user_id == admin.id:
        raise HTTPException(status_code=400, detail="Administrators cannot suspend their own account")
    user = db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    user.status = UserStatus.SUSPENDED.value
    write_audit(db, actor=admin, action="user.suspend", target_type="user", target_id=user.id)
    db.commit()
    db.refresh(user)
    return user


@router.post("/users/{user_id}/restore", response_model=UserOut)
def restore_user(
    user_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
) -> User:
    user = db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    user.status = UserStatus.APPROVED.value
    write_audit(db, actor=admin, action="user.restore", target_type="user", target_id=user.id)
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
    user = db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
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
) -> VpnKey:
    user = db.scalar(select(User).options(selectinload(User.keys)).where(User.id == user_id))
    if not user:
        raise HTTPException(status_code=404, detail="User not found")

    provisioned = xray.provision_key(name=payload.name, key_type=payload.type, sni=payload.sni)
    key = VpnKey(
        owner_id=user.id,
        uuid=provisioned.uuid,
        name=payload.name,
        type=payload.type.value,
        sni=provisioned.sni,
        status=KeyStatus.ACTIVE.value,
        public_key=provisioned.public_key,
        short_id=provisioned.short_id,
        connection_uri=provisioned.connection_uri,
        client_config_json=provisioned.client_config_json,
    )
    db.add(key)
    write_audit(
        db,
        actor=admin,
        action="admin.key.create",
        target_type="vpn_key",
        target_id=provisioned.uuid,
        metadata={"owner_id": user.id, "type": payload.type.value},
    )
    db.commit()
    db.refresh(key)
    return key


@router.delete("/keys/{key_id}", response_model=Message)
def revoke_any_key(
    key_id: int,
    db: Session = Depends(get_db),
    admin: User = Depends(require_admin),
    xray: XrayManager = Depends(get_xray_manager),
) -> Message:
    key = db.get(VpnKey, key_id)
    if not key:
        raise HTTPException(status_code=404, detail="Key not found")
    if key.status != KeyStatus.REVOKED.value:
        xray.revoke_key(key_uuid=key.uuid, key_type=KeyType(key.type))
        key.status = KeyStatus.REVOKED.value
        write_audit(
            db,
            actor=admin,
            action="admin.key.revoke",
            target_type="vpn_key",
            target_id=key.id,
            metadata={"uuid": key.uuid, "owner_id": key.owner_id},
        )
        db.commit()
    return Message(message="Key revoked")
