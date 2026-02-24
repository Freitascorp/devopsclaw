package fleet

import (
	"fmt"
	"log/slog"
	"path/filepath"
)

// StoreConfig holds the parameters needed to create a Store backend.
type StoreConfig struct {
	Backend    string          // "memory", "sqlite", "postgres"
	DataDir    string          // Base data directory (used for SQLite path default)
	SQLitePath string          // Explicit SQLite path (overrides DataDir default)
	Postgres   *PostgresConfig // PostgreSQL connection config
}

// NewStore creates the appropriate Store implementation based on config.
//
// Backends:
//   - "memory"   — in-process, non-durable (dev/test only)
//   - "sqlite"   — single-file durable store (single-node production)
//   - "postgres" — PostgreSQL durable store (multi-node HA production)
func NewStore(cfg StoreConfig, logger *slog.Logger) (Store, error) {
	switch cfg.Backend {
	case "", "memory":
		logger.Info("fleet store: using in-memory backend (non-durable)")
		return NewMemoryStore(), nil

	case "sqlite":
		dbPath := cfg.SQLitePath
		if dbPath == "" {
			if cfg.DataDir == "" {
				return nil, fmt.Errorf("sqlite store requires sqlite_path or data_dir")
			}
			dbPath = filepath.Join(cfg.DataDir, "fleet.db")
		}
		logger.Info("fleet store: using SQLite backend", "path", dbPath)
		return NewSQLiteStore(dbPath)

	case "postgres":
		if cfg.Postgres == nil {
			return nil, fmt.Errorf("postgres store requires postgres config")
		}
		logger.Info("fleet store: using PostgreSQL backend", "host", cfg.Postgres.Host, "database", cfg.Postgres.Database)
		return NewPostgresStore(*cfg.Postgres)

	default:
		return nil, fmt.Errorf("unknown fleet store backend: %q (supported: memory, sqlite, postgres)", cfg.Backend)
	}
}
