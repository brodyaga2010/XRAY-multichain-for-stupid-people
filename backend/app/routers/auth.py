from fastapi import APIRouter, Depends, HTTPException, Response, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.audit import write_audit
from app.config import settings
from app.database import get_db
from app.dependencies import get_current_user
from app.models import User, UserStatus
from app.ratelimit import limit_auth
from app.schemas import AuthSession, Message, PasswordChange, UserCreate, UserLogin, UserOut
from app.security import (
    clear_auth_cookie,
    create_access_token,
    hash_password,
    set_auth_cookie,
    verify_password,
)

router = APIRouter(prefix="/auth", tags=["auth"])


@router.post(
    "/register",
    response_model=AuthSession,
    status_code=status.HTTP_201_CREATED,
    dependencies=[Depends(limit_auth)],
)
def register(payload: UserCreate, db: Session = Depends(get_db)) -> AuthSession:
    existing = db.scalar(select(User).where(User.email == payload.email.lower()))
    if existing:
        raise HTTPException(status_code=409, detail="Этот email уже зарегистрирован")

    user = User(
        email=payload.email.lower(),
        password_hash=hash_password(payload.password),
        status=UserStatus.PENDING.value,
        key_limit=settings.default_key_limit,
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return AuthSession(user=UserOut.model_validate(user))


@router.post("/login", response_model=AuthSession, dependencies=[Depends(limit_auth)])
def login(payload: UserLogin, response: Response, db: Session = Depends(get_db)) -> AuthSession:
    user = db.scalar(select(User).where(User.email == payload.email.lower()))
    if not user or not verify_password(payload.password, user.password_hash):
        raise HTTPException(status_code=401, detail="Неверный email или пароль")
    if user.status != UserStatus.APPROVED.value:
        raise HTTPException(status_code=403, detail="Аккаунт ещё не подтверждён администратором")

    token = create_access_token(user.id)
    set_auth_cookie(response, token)
    return AuthSession(user=UserOut.model_validate(user))


@router.post("/logout", response_model=Message)
def logout(response: Response) -> Message:
    clear_auth_cookie(response)
    return Message(message="Вы вышли")


@router.get("/me", response_model=AuthSession)
def me(user: User = Depends(get_current_user)) -> AuthSession:
    return AuthSession(user=UserOut.model_validate(user))


@router.post("/password", response_model=Message, dependencies=[Depends(limit_auth)])
def change_password(
    payload: PasswordChange,
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> Message:
    if not verify_password(payload.current_password, user.password_hash):
        raise HTTPException(status_code=400, detail="Текущий пароль указан неверно")
    user.password_hash = hash_password(payload.new_password)
    write_audit(db, actor=user, action="user.password_change", target_type="user", target_id=user.id)
    db.commit()
    return Message(message="Пароль изменён")
