from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.database import get_db
from app.dependencies import get_xray_manager, require_approved_user
from app.models import KeyStatus, User, VpnKey
from app.schemas import KeyCreate, Message, SubscriptionOut, VpnKeyOut
from app.services import keys as key_service
from app.services.xray_manager import XrayManager

router = APIRouter(prefix="/keys", tags=["keys"])


@router.get("", response_model=list[VpnKeyOut])
def list_keys(
    db: Session = Depends(get_db),
    current_user: User = Depends(require_approved_user),
    xray: XrayManager = Depends(get_xray_manager),
) -> list[VpnKeyOut]:
    keys = db.scalars(
        select(VpnKey)
        .where(VpnKey.owner_id == current_user.id)
        .order_by(VpnKey.created_at.desc(), VpnKey.id.desc())
    )
    return [key_service.key_out(key, xray) for key in keys]


@router.post("", response_model=VpnKeyOut, status_code=status.HTTP_201_CREATED)
def create_key(
    payload: KeyCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(require_approved_user),
    xray: XrayManager = Depends(get_xray_manager),
) -> VpnKeyOut:
    active_count = db.scalar(
        select(func.count(VpnKey.id)).where(
            VpnKey.owner_id == current_user.id,
            VpnKey.status == KeyStatus.ACTIVE.value,
        )
    )
    if active_count is not None and active_count >= current_user.key_limit:
        raise HTTPException(status_code=409, detail="Достигнут лимит ключей")

    key = key_service.provision_key(
        db, xray, owner=current_user, name=payload.name, actor=current_user, action="key.create"
    )
    return key_service.key_out(key, xray)


@router.delete("/{key_id}", response_model=Message)
def revoke_key(
    key_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(require_approved_user),
    xray: XrayManager = Depends(get_xray_manager),
) -> Message:
    key = db.get(VpnKey, key_id)
    if not key or key.owner_id != current_user.id:
        raise HTTPException(status_code=404, detail="Ключ не найден")
    key_service.revoke_key(db, xray, key=key, actor=current_user, action="key.revoke")
    return Message(message="Ключ отозван")


@router.get("/subscription", response_model=SubscriptionOut)
def get_subscription(
    db: Session = Depends(get_db), current_user: User = Depends(require_approved_user)
) -> SubscriptionOut:
    return SubscriptionOut(url=key_service.subscription_url(key_service.ensure_sub_token(db, current_user)))


@router.post("/subscription/rotate", response_model=SubscriptionOut)
def rotate_subscription(
    db: Session = Depends(get_db), current_user: User = Depends(require_approved_user)
) -> SubscriptionOut:
    token = key_service.ensure_sub_token(db, current_user, rotate=True)
    return SubscriptionOut(url=key_service.subscription_url(token))
