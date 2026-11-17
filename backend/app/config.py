from functools import lru_cache
from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    database_url: str = "postgresql+psycopg://xray:xray@postgres:5432/xray_control"
    jwt_secret: str = Field(default="change-me-in-production", min_length=16)
    jwt_algorithm: str = "HS256"
    access_token_minutes: int = 60 * 24
    cookie_name: str = "xray_access_token"
    cookie_secure: bool = False
    cookie_domain: str | None = None
    cors_origins: str = "http://localhost:5173,http://localhost:8080"

    default_key_limit: int = 3
    admin_email: str | None = None
    admin_password: str | None = None

    # Публичный адрес панели: из него строится ссылка-подписка
    public_base_url: str = "http://localhost:5173"
    subscription_title: str = "Personal"
    subscription_update_hours: int = 12

    # Ограничение попыток входа/регистрации с одного IP
    auth_rate_limit: int = 10
    auth_rate_window_seconds: int = 300
    # Заголовок X-Forwarded-For учитывается только от этих адресов (Caddy на том же сервере)
    trusted_proxies: str = "127.0.0.1,::1"

    # Управление Xray: local — панель на входном сервере, ssh — на отдельной машине,
    # dry_run — без реального сервера (локальная разработка)
    xray_mode: Literal["local", "ssh", "dry_run"] = "dry_run"
    xray_dry_run: bool = False  # устаревший флаг, эквивалент xray_mode=dry_run
    xray_config_path: str = "/usr/local/etc/xray/config.json"
    xray_backup_dir: str = "/var/lib/xray-panel/backups"
    xray_backup_keep: int = 30
    xray_lock_path: str = "/run/xray-panel/config.lock"
    xray_binary: str = "xray"
    xray_api_server: str = "127.0.0.1:10085"
    xray_restart_command: str = "systemctl restart xray"
    xray_xhttp_port: int = 2053
    # Адрес, который попадает в ссылки клиентов (домен входного сервера)
    xray_public_host: str = "127.0.0.1"
    stats_poll_seconds: int = 60
    online_window_seconds: int = 180

    xray_relay_host: str = "127.0.0.1"
    xray_relay_port: int = 22
    xray_relay_user: str = "root"
    xray_ssh_private_key_path: str = "/run/secrets/xray_ssh_key"
    xray_ssh_known_hosts: str | None = None

    @property
    def cors_origin_list(self) -> list[str]:
        return [origin.strip() for origin in self.cors_origins.split(",") if origin.strip()]

    @property
    def trusted_proxy_list(self) -> set[str]:
        return {item.strip() for item in self.trusted_proxies.split(",") if item.strip()}

    @property
    def effective_xray_mode(self) -> str:
        return "dry_run" if self.xray_dry_run else self.xray_mode


@lru_cache
def get_settings() -> Settings:
    return Settings()


settings = get_settings()
