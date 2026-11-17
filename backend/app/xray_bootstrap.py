"""Подготовка конфига Xray для панели: API, статистика, XHTTP, email = UUID.

Запуск на сервере: python -m app.xray_bootstrap
"""

from app.config import settings
from app.services.xray_manager import build_xray_manager


def main() -> None:
    manager = build_xray_manager(settings)
    changed = manager.bootstrap()
    print("Конфиг Xray обновлён, Xray перезапущен" if changed else "Конфиг Xray уже готов")


if __name__ == "__main__":
    main()
