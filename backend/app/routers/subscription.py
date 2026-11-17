import base64

from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import PlainTextResponse
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.config import settings
from app.database import get_db
from app.dependencies import get_xray_manager
from app.models import KeyStatus, User, UserStatus, VpnKey
from app.services.xray_manager import XrayManager

router = APIRouter(tags=["subscription"])


@router.get("/sub/{token}", response_class=PlainTextResponse)
def subscription(
    token: str,
    db: Session = Depends(get_db),
    xray: XrayManager = Depends(get_xray_manager),
) -> PlainTextResponse:
    """Подписка для клиентов (Hiddify, v2rayNG, v2rayN, Streisand, Happ): base64 со ссылками."""
    if len(token) < 16:
        raise HTTPException(status_code=404, detail="Not found")
    user = db.scalar(select(User).where(User.sub_token == token))
    if not user or user.status != UserStatus.APPROVED.value:
        raise HTTPException(status_code=404, detail="Not found")
    keys = list(
        db.scalars(
            select(VpnKey)
            .where(VpnKey.owner_id == user.id, VpnKey.status == KeyStatus.ACTIVE.value)
            .order_by(VpnKey.created_at, VpnKey.id)
        )
    )
    uris = [link.uri for key in keys for link in xray.links(key_uuid=key.uuid, name=key.name)]
    body = base64.b64encode("\n".join(uris).encode("utf-8")).decode("ascii")
    title = base64.b64encode(settings.subscription_title.encode("utf-8")).decode("ascii")
    upload = sum(key.traffic_up for key in keys)
    download = sum(key.traffic_down for key in keys)
    headers = {
        "profile-title": f"base64:{title}",
        "profile-update-interval": str(settings.subscription_update_hours),
        "subscription-userinfo": f"upload={upload}; download={download}; total=0; expire=0",
        "content-disposition": 'attachment; filename="vpn.txt"',
        "cache-control": "no-store",
    }
    return PlainTextResponse(body, headers=headers)
