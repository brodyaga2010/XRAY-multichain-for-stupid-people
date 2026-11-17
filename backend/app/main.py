from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.config import settings
from app.database import SessionLocal
from app.dependencies import get_xray_manager
from app.routers import admin, auth, keys, subscription
from app.services.stats import StatsCollector


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    collector = None
    if settings.effective_xray_mode != "dry_run" and settings.stats_poll_seconds > 0:
        collector = StatsCollector(SessionLocal, get_xray_manager(), settings.stats_poll_seconds)
        collector.start()
    try:
        yield
    finally:
        if collector:
            collector.stop()


# Без /docs и /openapi.json: снаружи API не должно рассказывать, что это за сервис
app = FastAPI(title="Panel", version="0.2.0", lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None)

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origin_list,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(auth.router)
app.include_router(keys.router)
app.include_router(admin.router)
app.include_router(subscription.router)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}
