"""Фоновый сбор трафика и онлайна из Xray в БД."""

from __future__ import annotations

import logging
import threading
from collections.abc import Callable
from datetime import UTC, datetime

from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.models import VpnKey
from app.services.xray_manager import XrayManager

logger = logging.getLogger(__name__)


def collect_once(db: Session, xray: XrayManager, now: datetime | None = None) -> int:
    """Добавляет трафик с прошлого сбора и отмечает время активности. Возвращает число обновлённых ключей."""
    now = now or datetime.now(UTC)
    traffic = xray.collect_traffic()
    online = xray.online_emails()
    active = set(online) | {email for email, stat in traffic.items() if stat.uplink or stat.downlink}
    emails = set(traffic) | active
    if not emails:
        return 0
    updated = 0
    for key in db.scalars(select(VpnKey).where(VpnKey.uuid.in_(emails))):
        stat = traffic.get(key.uuid)
        if stat:
            key.traffic_up += stat.uplink
            key.traffic_down += stat.downlink
        if key.uuid in active:
            key.last_seen_at = now
        updated += 1
    db.commit()
    return updated


class StatsCollector:
    def __init__(self, session_factory: Callable[[], Session], xray: XrayManager, interval: int) -> None:
        self._session_factory = session_factory
        self._xray = xray
        self._interval = interval
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._loop, name="xray-stats", daemon=True)

    def start(self) -> None:
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        self._thread.join(timeout=5)

    def _loop(self) -> None:
        while not self._stop.wait(self._interval):
            try:
                with self._session_factory() as db:
                    collect_once(db, self._xray)
            except HTTPException as exc:
                logger.warning("Сбор статистики Xray: %s", exc.detail)
            except Exception:
                logger.exception("Сбор статистики Xray завершился ошибкой")
