package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// NewPostgresDB создает новое соединение с PostgreSQL базой данных
func NewPostgresDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}
