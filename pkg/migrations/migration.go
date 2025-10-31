package migrations

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Migration представляет одну миграцию
type Migration struct {
	Version     int
	Name        string
	UpSQL       string
	DownSQL     string
	Description string
}

// Manager управляет миграциями базы данных
type Manager struct {
	db *sql.DB
}

// NewManager создает новый менеджер миграций
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// Init создает таблицу для отслеживания миграций
func (m *Manager) Init() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`

	_, err := m.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	return nil
}

// GetAppliedMigrations возвращает список примененных миграций
func (m *Manager) GetAppliedMigrations() (map[int]bool, error) {
	query := `SELECT version FROM schema_migrations ORDER BY version`
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		applied[version] = true
	}

	return applied, nil
}

// ApplyMigration применяет одну миграцию
func (m *Manager) ApplyMigration(migration *Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Выполняем SQL миграции
	if migration.UpSQL != "" {
		_, err = tx.Exec(migration.UpSQL)
		if err != nil {
			return fmt.Errorf("failed to execute migration %d: %w", migration.Version, err)
		}
	}

	// Записываем информацию о миграции
	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, description, applied_at)
		VALUES ($1, $2, $3, $4)
	`, migration.Version, migration.Name, migration.Description, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
	}

	return tx.Commit()
}

// RollbackMigration откатывает одну миграцию
func (m *Manager) RollbackMigration(migration *Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Выполняем SQL отката
	if migration.DownSQL != "" {
		_, err = tx.Exec(migration.DownSQL)
		if err != nil {
			return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
		}
	}

	// Удаляем запись о миграции
	_, err = tx.Exec(`DELETE FROM schema_migrations WHERE version = $1`, migration.Version)
	if err != nil {
		return fmt.Errorf("failed to remove migration record %d: %w", migration.Version, err)
	}

	return tx.Commit()
}

// GetMigrationVersion извлекает номер версии из имени файла
func GetMigrationVersion(filename string) (int, error) {
	base := filepath.Base(filename)
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid migration version in filename %s: %w", filename, err)
	}

	return version, nil
}

// GetMigrationName извлекает имя миграции из имени файла
func GetMigrationName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Убираем номер версии из начала
	parts := strings.Split(name, "_")
	if len(parts) > 1 {
		return strings.Join(parts[1:], "_")
	}

	return name
}
