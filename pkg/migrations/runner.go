package migrations

import (
	"database/sql"
	"fmt"
	"log"
)

// RunMigrations применяет все непримененные миграции
func RunMigrations(db *sql.DB) error {
	log.Println("Starting database migrations...")

	manager := NewManager(db)

	// Инициализируем таблицу миграций
	if err := manager.Init(); err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}

	// Получаем список примененных миграций
	applied, err := manager.GetAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Получаем все доступные миграции
	migrations, err := GetEmbeddedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get embedded migrations: %w", err)
	}

	// Применяем непримененные миграции
	appliedCount := 0
	for _, migration := range migrations {
		if applied[migration.Version] {
			log.Printf("Migration %d (%s) already applied, skipping", migration.Version, migration.Name)
			continue
		}

		log.Printf("Applying migration %d: %s", migration.Version, migration.Name)
		if err := manager.ApplyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}

		appliedCount++
		log.Printf("Migration %d (%s) applied successfully", migration.Version, migration.Name)
	}

	if appliedCount == 0 {
		log.Println("No new migrations to apply")
	} else {
		log.Printf("Applied %d migrations successfully", appliedCount)
	}

	return nil
}

// CheckMigrationsStatus проверяет статус миграций
func CheckMigrationsStatus(db *sql.DB) error {
	manager := NewManager(db)

	// Инициализируем таблицу миграций
	if err := manager.Init(); err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}

	// Получаем список примененных миграций
	applied, err := manager.GetAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Получаем все доступные миграции
	migrations, err := GetEmbeddedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get embedded migrations: %w", err)
	}

	log.Println("Migration status:")
	for _, migration := range migrations {
		status := "PENDING"
		if applied[migration.Version] {
			status = "APPLIED"
		}
		log.Printf("  %d: %s - %s", migration.Version, migration.Name, status)
	}

	return nil
}
