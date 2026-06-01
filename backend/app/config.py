from functools import lru_cache

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

    xray_relay_host: str = "127.0.0.1"
    xray_relay_port: int = 22
    xray_relay_user: str = "root"
    xray_ssh_private_key_path: str = "/run/secrets/xray_ssh_key"
    xray_config_path: str = "/usr/local/etc/xray/config.json"
    xray_dry_run: bool = False

    @property
    def cors_origin_list(self) -> list[str]:
        return [origin.strip() for origin in self.cors_origins.split(",") if origin.strip()]


@lru_cache
def get_settings() -> Settings:
    return Settings()


settings = get_settings()
