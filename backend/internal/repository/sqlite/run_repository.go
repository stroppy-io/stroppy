package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"stroppy-cloud-pannel/internal/domain/run"
)

// RunRepository реализует интерфейс run.Repository для SQLite
type RunRepository struct {
	db *sql.DB
}

// NewRunRepository создает новый репозиторий запусков
func NewRunRepository(db *sql.DB) *RunRepository {
	return &RunRepository{db: db}
}

// Create создает новый запуск в базе данных
func (r *RunRepository) Create(ru *run.Run) error {
	query := `
		INSERT INTO runs (name, description, status, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Exec(query,
		ru.Name, ru.Description, ru.Status, ru.Config,
		ru.CreatedAt, ru.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create run: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get run id: %w", err)
	}

	ru.ID = int(id)
	return nil
}

// GetByID получает запуск по ID
func (r *RunRepository) GetByID(id int) (*run.Run, error) {
	query := `
		SELECT id, name, description, status, config, result,
			   created_at, updated_at, started_at, completed_at
		FROM runs WHERE id = ?
	`

	ru := &run.Run{}
	var startedAt, completedAt sql.NullTime
	var result sql.NullString

	err := r.db.QueryRow(query, id).Scan(
		&ru.ID, &ru.Name, &ru.Description, &ru.Status,
		&ru.Config, &result, &ru.CreatedAt, &ru.UpdatedAt,
		&startedAt, &completedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, run.ErrRunNotFound
		}
		return nil, fmt.Errorf("failed to get run by id: %w", err)
	}

	if result.Valid {
		ru.Result = result.String
	}
	if startedAt.Valid {
		ru.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		ru.CompletedAt = &completedAt.Time
	}

	return ru, nil
}

// GetAll получает все запуски с пагинацией
func (r *RunRepository) GetAll(limit, offset int) ([]*run.Run, error) {
	query := `
		SELECT id, name, description, status, config, result,
			   created_at, updated_at, started_at, completed_at
		FROM runs 
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get all runs: %w", err)
	}
	defer rows.Close()

	var runs []*run.Run
	for rows.Next() {
		ru := &run.Run{}
		var startedAt, completedAt sql.NullTime
		var result sql.NullString

		err := rows.Scan(
			&ru.ID, &ru.Name, &ru.Description, &ru.Status,
			&ru.Config, &result, &ru.CreatedAt, &ru.UpdatedAt,
			&startedAt, &completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}

		if result.Valid {
			ru.Result = result.String
		}
		if startedAt.Valid {
			ru.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			ru.CompletedAt = &completedAt.Time
		}

		runs = append(runs, ru)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runs: %w", err)
	}

	return runs, nil
}

// Update обновляет запуск
func (r *RunRepository) Update(ru *run.Run) error {
	ru.UpdatedAt = time.Now()

	query := `
		UPDATE runs 
		SET name = ?, description = ?, status = ?, config = ?, result = ?,
			updated_at = ?, started_at = ?, completed_at = ?
		WHERE id = ?
	`

	result, err := r.db.Exec(query,
		ru.Name, ru.Description, ru.Status, ru.Config, ru.Result,
		ru.UpdatedAt, ru.StartedAt, ru.CompletedAt, ru.ID)
	if err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return run.ErrRunNotFound
	}

	return nil
}

// Delete удаляет запуск
func (r *RunRepository) Delete(id int) error {
	query := `DELETE FROM runs WHERE id = ?`

	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return run.ErrRunNotFound
	}

	return nil
}

// Count возвращает общее количество запусков
func (r *RunRepository) Count() (int, error) {
	query := `SELECT COUNT(*) FROM runs`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count runs: %w", err)
	}

	return count, nil
}

// GetAllWithFilters получает все запуски с фильтрацией и пагинацией
func (r *RunRepository) GetAllWithFilters(limit, offset int, searchText, status, dateFrom, dateTo string) ([]*run.Run, error) {
	query := `
		SELECT id, name, description, status, config, result,
			   created_at, updated_at, started_at, completed_at
		FROM runs 
		WHERE 1=1
	`
	args := []interface{}{}

	// Добавляем фильтр по тексту
	if searchText != "" {
		query += ` AND (name LIKE ? OR description LIKE ?)`
		searchPattern := "%" + searchText + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Добавляем фильтр по статусу
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	// Добавляем фильтр по дате
	if dateFrom != "" {
		query += ` AND created_at >= ?`
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		query += ` AND created_at <= ?`
		args = append(args, dateTo)
	}

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered runs: %w", err)
	}
	defer rows.Close()

	var runs []*run.Run
	for rows.Next() {
		ru := &run.Run{}
		var startedAt, completedAt sql.NullTime
		var result sql.NullString

		err := rows.Scan(
			&ru.ID, &ru.Name, &ru.Description, &ru.Status,
			&ru.Config, &result, &ru.CreatedAt, &ru.UpdatedAt,
			&startedAt, &completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}

		if result.Valid {
			ru.Result = result.String
		}
		if startedAt.Valid {
			ru.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			ru.CompletedAt = &completedAt.Time
		}

		runs = append(runs, ru)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runs: %w", err)
	}

	return runs, nil
}

// CountWithFilters возвращает количество запусков с учетом фильтров
func (r *RunRepository) CountWithFilters(searchText, status, dateFrom, dateTo string) (int, error) {
	query := `SELECT COUNT(*) FROM runs WHERE 1=1`
	args := []interface{}{}

	// Добавляем фильтр по тексту
	if searchText != "" {
		query += ` AND (name LIKE ? OR description LIKE ?)`
		searchPattern := "%" + searchText + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Добавляем фильтр по статусу
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	// Добавляем фильтр по дате
	if dateFrom != "" {
		query += ` AND created_at >= ?`
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		query += ` AND created_at <= ?`
		args = append(args, dateTo)
	}

	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count filtered runs: %w", err)
	}

	return count, nil
}
