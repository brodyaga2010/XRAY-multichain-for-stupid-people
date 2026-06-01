from functools import lru_cache

from fastapi import Cookie, Depends, HTTPException, status
from sqlalchemy.orm import Session

from app.config import settings
from app.database import get_db
from app.models import User, UserRole, UserStatus
from app.security import decode_access_token
from app.services.xray_manager import DryRunXrayManager, XrayManager


@lru_cache
def get_xray_manager() -> XrayManager:
    if settings.xray_dry_run:
        return DryRunXrayManager(settings)
    return XrayManager(settings)


def get_current_user(
    db: Session = Depends(get_db),
    token: str | None = Cookie(default=None, alias=settings.cookie_name),
) -> User:
    if not token:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Not authenticated")
    user = db.get(User, decode_access_token(token))
    if not user:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="User not found")
    return user


def require_approved_user(user: User = Depends(get_current_user)) -> User:
    if user.status != UserStatus.APPROVED.value:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Account is not approved")
    return user


def require_admin(user: User = Depends(require_approved_user)) -> User:
    if user.role != UserRole.ADMIN.value:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Admin role required")
    return user
