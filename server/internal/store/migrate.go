package store

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	revisionInitial = "0001_initial"
	revisionPanelV2 = "0002_panel_v2"
	revisionMonitor = "0003_monitor_history"
)

// Схема, идентичная результату alembic 0001 + 0002 (как её создаёт SQLAlchemy на SQLite).
var schemaStatements = []string{
	`CREATE TABLE users (
	id INTEGER NOT NULL,
	email VARCHAR(320) NOT NULL,
	password_hash VARCHAR(255) NOT NULL,
	role VARCHAR(32) DEFAULT 'user' NOT NULL,
	status VARCHAR(32) DEFAULT 'pending' NOT NULL,
	key_limit INTEGER DEFAULT '3' NOT NULL,
	created_at DATETIME DEFAULT (CURRENT_TIMESTAMP) NOT NULL,
	updated_at DATETIME DEFAULT (CURRENT_TIMESTAMP) NOT NULL,
	sub_token VARCHAR(64),
	PRIMARY KEY (id),
	CONSTRAINT uq_users_sub_token UNIQUE (sub_token),
	UNIQUE (email)
)`,
	`CREATE INDEX ix_users_email ON users (email)`,
	`CREATE TABLE vpn_keys (
	id INTEGER NOT NULL,
	owner_id INTEGER NOT NULL,
	uuid VARCHAR(64) NOT NULL,
	name VARCHAR(120) NOT NULL,
	type VARCHAR(32) NOT NULL,
	sni VARCHAR(255) NOT NULL,
	status VARCHAR(32) DEFAULT 'active' NOT NULL,
	public_key VARCHAR(255) NOT NULL,
	short_id VARCHAR(64) NOT NULL,
	connection_uri TEXT NOT NULL,
	client_config_json JSON,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	traffic_up BIGINT DEFAULT '0' NOT NULL,
	traffic_down BIGINT DEFAULT '0' NOT NULL,
	last_seen_at DATETIME,
	PRIMARY KEY (id),
	UNIQUE (uuid),
	FOREIGN KEY(owner_id) REFERENCES users (id) ON DELETE CASCADE
)`,
	`CREATE INDEX ix_vpn_keys_owner_id ON vpn_keys (owner_id)`,
	`CREATE TABLE audit_logs (
	id INTEGER NOT NULL,
	actor_id INTEGER,
	action VARCHAR(120) NOT NULL,
	target_type VARCHAR(80) NOT NULL,
	target_id VARCHAR(80) NOT NULL,
	metadata JSON DEFAULT '{}' NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY(actor_id) REFERENCES users (id) ON DELETE SET NULL
)`,
	`CREATE TABLE alembic_version (
	version_num VARCHAR(32) NOT NULL,
	CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num)
)`,
}

// Изменения 0002 для базы, созданной миграцией 0001.
var upgradeTo0002 = []string{
	`ALTER TABLE users ADD COLUMN sub_token VARCHAR(64)`,
	`CREATE UNIQUE INDEX uq_users_sub_token ON users (sub_token)`,
	`ALTER TABLE vpn_keys ADD COLUMN traffic_up BIGINT DEFAULT '0' NOT NULL`,
	`ALTER TABLE vpn_keys ADD COLUMN traffic_down BIGINT DEFAULT '0' NOT NULL`,
	`ALTER TABLE vpn_keys ADD COLUMN last_seen_at DATETIME`,
}

// migration — шаг схемы from → to.
type migration struct {
	from, to string
	stmts    []string
}

// Цепочка миграций по порядку; последняя to — актуальная версия схемы.
var migrations = []migration{
	{revisionInitial, revisionPanelV2, upgradeTo0002},
	{revisionPanelV2, revisionMonitor, upgradeTo0003},
}

// LatestRevision — актуальная версия схемы.
var LatestRevision = migrations[len(migrations)-1].to

// 0003: история мониторинга переживает перезапуск кабинета.
var upgradeTo0003 = []string{
	`CREATE TABLE monitor_points (
	at DATETIME NOT NULL,
	ms FLOAT,
	status VARCHAR(16) NOT NULL,
	PRIMARY KEY (at)
)`,
	`CREATE TABLE monitor_incidents (
	id INTEGER NOT NULL,
	check_id VARCHAR(32) NOT NULL,
	name VARCHAR(128) NOT NULL,
	status VARCHAR(16) NOT NULL,
	detail TEXT DEFAULT '' NOT NULL,
	started_at DATETIME NOT NULL,
	ended_at DATETIME,
	PRIMARY KEY (id)
)`,
	`CREATE INDEX ix_monitor_incidents_started_at ON monitor_incidents (started_at)`,
}

// Migrate приводит базу к последней версии. Идемпотентна.
// Нет таблиц — создаёт схему 0002 и применяет следующие шаги; известная версия — догоняет до последней.
func (s *Store) Migrate(ctx context.Context) (string, error) {
	var result string
	err := s.Tx(ctx, func(q *Queries) error {
		current, created, err := currentRevision(ctx, q.db)
		if err != nil {
			return err
		}
		start := current
		for _, m := range migrations {
			if m.from != current {
				continue
			}
			for _, stmt := range m.stmts {
				if _, err := q.db.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("применение %s: %w", m.to, err)
				}
			}
			current = m.to
		}
		if current != LatestRevision {
			return fmt.Errorf("неизвестная версия схемы %q", start)
		}
		if _, err := q.db.ExecContext(ctx, `UPDATE alembic_version SET version_num = ?`, current); err != nil {
			return err
		}
		switch {
		case created:
			result = "схема создана (" + current + ")"
		case start == current:
			result = "схема актуальна (" + current + ")"
		default:
			result = "схема обновлена " + start + " -> " + current
		}
		return nil
	})
	return result, err
}

// currentRevision — версия схемы; пустая база получает схему 0002 (created = true).
func currentRevision(ctx context.Context, db DBTX) (string, bool, error) {
	hasVersion, err := tableExists(ctx, db, "alembic_version")
	if err != nil {
		return "", false, err
	}
	if !hasVersion {
		hasUsers, err := tableExists(ctx, db, "users")
		if err != nil {
			return "", false, err
		}
		if hasUsers {
			return "", false, fmt.Errorf("таблица users есть, а alembic_version нет: схема неизвестной версии, миграция отменена")
		}
		for _, stmt := range schemaStatements {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return "", false, fmt.Errorf("создание схемы: %w", err)
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO alembic_version (version_num) VALUES (?)`, revisionPanelV2); err != nil {
			return "", false, err
		}
		return revisionPanelV2, true, nil
	}
	var versions []string
	rows, err := db.QueryContext(ctx, `SELECT version_num FROM alembic_version`)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "", false, err
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(versions) != 1 {
		return "", false, fmt.Errorf("alembic_version содержит %d записей, ожидается одна", len(versions))
	}
	return versions[0], false, nil
}

func tableExists(ctx context.Context, db DBTX, name string) (bool, error) {
	var found string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
