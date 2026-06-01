from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.audit import write_audit
from app.database import get_db
from app.dependencies import get_xray_manager, require_approved_user
from app.models import KeyStatus, KeyType, User, VpnKey
from app.schemas import KeyCreate, Message, VpnKeyOut
from app.services.xray_manager import XrayManager

router = APIRouter(prefix="/keys", tags=["keys"])


@router.get("", response_model=list[VpnKeyOut])
def list_keys(
    db: Session = Depends(get_db),
    current_user: User = Depends(require_approved_user),
) -> list[VpnKey]:
    return list(
        db.scalars(
            select(VpnKey)
            .where(VpnKey.owner_id == current_user.id)
            .order_by(VpnKey.created_at.desc(), VpnKey.id.desc())
        )
    )


@router.post("", response_model=VpnKeyOut, status_code=status.HTTP_201_CREATED)
def create_key(
    payload: KeyCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(require_approved_user),
    xray: XrayManager = Depends(get_xray_manager),
) -> VpnKey:
    active_count = db.scalar(
        select(func.count(VpnKey.id)).where(
            VpnKey.owner_id == current_user.id,
            VpnKey.status == KeyStatus.ACTIVE.value,
        )
    )
    if active_count is not None and active_count >= current_user.key_limit:
        raise HTTPException(status_code=409, detail="Key limit reached")

    provisioned = xray.provision_key(name=payload.name, key_type=payload.type, sni=payload.sni)
    key = VpnKey(
        owner_id=current_user.id,
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
        actor=current_user,
        action="key.create",
        target_type="vpn_key",
        target_id=provisioned.uuid,
        metadata={"type": payload.type.value},
    )
    db.commit()
    db.refresh(key)
    return key


@router.delete("/{key_id}", response_model=Message)
def revoke_key(
    key_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(require_approved_user),
    xray: XrayManager = Depends(get_xray_manager),
) -> Message:
    key = db.get(VpnKey, key_id)
    if not key or key.owner_id != current_user.id:
        raise HTTPException(status_code=404, detail="Key not found")

    if key.status != KeyStatus.REVOKED.value:
        xray.revoke_key(key_uuid=key.uuid, key_type=KeyType(key.type))
        key.status = KeyStatus.REVOKED.value
        write_audit(
            db,
            actor=current_user,
            action="key.revoke",
            target_type="vpn_key",
            target_id=key.id,
            metadata={"uuid": key.uuid},
        )
        db.commit()

    return Message(message="Key revoked")
