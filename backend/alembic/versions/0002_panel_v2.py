"""panel v2: подписка, трафик и онлайн по ключам

Revision ID: 0002_panel_v2
Revises: 0001_initial
Create Date: 2026-11-18
"""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "0002_panel_v2"
down_revision: Union[str, None] = "0001_initial"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    with op.batch_alter_table("users") as batch:
        batch.add_column(sa.Column("sub_token", sa.String(length=64), nullable=True))
        batch.create_unique_constraint("uq_users_sub_token", ["sub_token"])
    with op.batch_alter_table("vpn_keys") as batch:
        batch.add_column(sa.Column("traffic_up", sa.BigInteger(), nullable=False, server_default="0"))
        batch.add_column(sa.Column("traffic_down", sa.BigInteger(), nullable=False, server_default="0"))
        batch.add_column(sa.Column("last_seen_at", sa.DateTime(timezone=True), nullable=True))


def downgrade() -> None:
    with op.batch_alter_table("vpn_keys") as batch:
        batch.drop_column("last_seen_at")
        batch.drop_column("traffic_down")
        batch.drop_column("traffic_up")
    with op.batch_alter_table("users") as batch:
        batch.drop_constraint("uq_users_sub_token", type_="unique")
        batch.drop_column("sub_token")
