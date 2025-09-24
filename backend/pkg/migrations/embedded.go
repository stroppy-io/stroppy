package migrations

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var migrationFiles embed.FS

// GetEmbeddedMigrations возвращает все встроенные миграции
func GetEmbeddedMigrations() ([]*Migration, error) {
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to read migration files: %w", err)
	}

	var migrations []*Migration

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := GetMigrationVersion(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid migration file %s: %w", entry.Name(), err)
		}

		content, err := migrationFiles.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", err)
		}

		migration := &Migration{
			Version:     version,
			Name:        GetMigrationName(entry.Name()),
			UpSQL:       string(content),
			Description: fmt.Sprintf("Migration %d: %s", version, GetMigrationName(entry.Name())),
		}

		migrations = append(migrations, migration)
	}

	// Сортируем миграции по версии
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// GetMigrationVersionFromString извлекает номер версии из строки
func GetMigrationVersionFromString(filename string) (int, error) {
	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid migration version in filename %s: %w", filename, err)
	}

	return version, nil
}
