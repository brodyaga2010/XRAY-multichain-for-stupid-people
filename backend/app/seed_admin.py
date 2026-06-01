from sqlalchemy import select

from app.config import settings
from app.database import SessionLocal
from app.models import User, UserRole, UserStatus
from app.security import hash_password


def main() -> None:
    if not settings.admin_email or not settings.admin_password:
        return
    with SessionLocal() as db:
        existing = db.scalar(select(User).where(User.email == settings.admin_email.lower()))
        if existing:
            return
        db.add(
            User(
                email=settings.admin_email.lower(),
                password_hash=hash_password(settings.admin_password),
                role=UserRole.ADMIN.value,
                status=UserStatus.APPROVED.value,
                key_limit=100,
            )
        )
        db.commit()


if __name__ == "__main__":
    main()
