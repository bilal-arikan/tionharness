// Package db manages the SQLite connection and schema migrations.
// It uses modernc.org/sqlite (pure Go, no CGO) for easy cross-compilation.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps the SQL connection with SwarmGo-specific helpers.
type DB struct {
	*sql.DB
}

// Open opens (and creates if missing) the SQLite database at path,
// applies pragmas for sane concurrent behaviour, and runs migrations.
func Open(path string) (*DB, error) {
	// WAL mode + busy timeout improve concurrent read/write from goroutines.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	d := &DB{DB: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}
