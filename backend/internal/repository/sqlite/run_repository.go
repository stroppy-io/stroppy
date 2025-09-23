package sqlite

import (
	"database/sql"
	"encoding/json"
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
		INSERT INTO runs (name, description, status, config, tps_max, tps_min, tps_average, tps_95p, tps_99p, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Exec(query,
		ru.Name, ru.Description, ru.Status, ru.Config,
		ru.TPSMetrics.Max, ru.TPSMetrics.Min, ru.TPSMetrics.Average, ru.TPSMetrics.P95, ru.TPSMetrics.P99,
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
			   tps_max, tps_min, tps_average, tps_95p, tps_99p,
			   created_at, updated_at, started_at, completed_at
		FROM runs WHERE id = ?
	`

	ru := &run.Run{}
	var startedAt, completedAt sql.NullTime
	var result sql.NullString
	var tpsMax, tpsMin, tpsAverage, tps95p, tps99p sql.NullFloat64

	err := r.db.QueryRow(query, id).Scan(
		&ru.ID, &ru.Name, &ru.Description, &ru.Status,
		&ru.Config, &result, &tpsMax, &tpsMin, &tpsAverage, &tps95p, &tps99p,
		&ru.CreatedAt, &ru.UpdatedAt, &startedAt, &completedAt,
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

	// Заполняем TPS метрики
	if tpsMax.Valid {
		ru.TPSMetrics.Max = &tpsMax.Float64
	}
	if tpsMin.Valid {
		ru.TPSMetrics.Min = &tpsMin.Float64
	}
	if tpsAverage.Valid {
		ru.TPSMetrics.Average = &tpsAverage.Float64
	}
	if tps95p.Valid {
		ru.TPSMetrics.P95 = &tps95p.Float64
	}
	if tps99p.Valid {
		ru.TPSMetrics.P99 = &tps99p.Float64
	}

	return ru, nil
}

// GetAll получает все запуски с пагинацией
func (r *RunRepository) GetAll(limit, offset int) ([]*run.Run, error) {
	query := `
		SELECT id, name, description, status, config, result,
			   tps_max, tps_min, tps_average, tps_95p, tps_99p,
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
		var tpsMax, tpsMin, tpsAverage, tps95p, tps99p sql.NullFloat64

		err := rows.Scan(
			&ru.ID, &ru.Name, &ru.Description, &ru.Status,
			&ru.Config, &result, &tpsMax, &tpsMin, &tpsAverage, &tps95p, &tps99p,
			&ru.CreatedAt, &ru.UpdatedAt, &startedAt, &completedAt,
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

		// Заполняем TPS метрики
		if tpsMax.Valid {
			ru.TPSMetrics.Max = &tpsMax.Float64
		}
		if tpsMin.Valid {
			ru.TPSMetrics.Min = &tpsMin.Float64
		}
		if tpsAverage.Valid {
			ru.TPSMetrics.Average = &tpsAverage.Float64
		}
		if tps95p.Valid {
			ru.TPSMetrics.P95 = &tps95p.Float64
		}
		if tps99p.Valid {
			ru.TPSMetrics.P99 = &tps99p.Float64
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
			tps_max = ?, tps_min = ?, tps_average = ?, tps_95p = ?, tps_99p = ?,
			updated_at = ?, started_at = ?, completed_at = ?
		WHERE id = ?
	`

	result, err := r.db.Exec(query,
		ru.Name, ru.Description, ru.Status, ru.Config, ru.Result,
		ru.TPSMetrics.Max, ru.TPSMetrics.Min, ru.TPSMetrics.Average, ru.TPSMetrics.P95, ru.TPSMetrics.P99,
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
			   tps_max, tps_min, tps_average, tps_95p, tps_99p,
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
		var tpsMax, tpsMin, tpsAverage, tps95p, tps99p sql.NullFloat64

		err := rows.Scan(
			&ru.ID, &ru.Name, &ru.Description, &ru.Status,
			&ru.Config, &result, &tpsMax, &tpsMin, &tpsAverage, &tps95p, &tps99p,
			&ru.CreatedAt, &ru.UpdatedAt, &startedAt, &completedAt,
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

		// Заполняем TPS метрики
		if tpsMax.Valid {
			ru.TPSMetrics.Max = &tpsMax.Float64
		}
		if tpsMin.Valid {
			ru.TPSMetrics.Min = &tpsMin.Float64
		}
		if tpsAverage.Valid {
			ru.TPSMetrics.Average = &tpsAverage.Float64
		}
		if tps95p.Valid {
			ru.TPSMetrics.P95 = &tps95p.Float64
		}
		if tps99p.Valid {
			ru.TPSMetrics.P99 = &tps99p.Float64
		}

		runs = append(runs, ru)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runs: %w", err)
	}

	return runs, nil
}

// GetAllWithFiltersAndSort получает все запуски с фильтрацией, пагинацией и сортировкой
func (r *RunRepository) GetAllWithFiltersAndSort(limit, offset int, searchText, status, dateFrom, dateTo, sortBy, sortOrder string) ([]*run.Run, error) {
	query := `
		SELECT id, name, description, status, config, result,
			   tps_max, tps_min, tps_average, tps_95p, tps_99p,
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

	// Добавляем сортировку
	orderBy := "created_at DESC" // по умолчанию
	if sortBy != "" {
		// Валидируем поле для сортировки
		validSortFields := map[string]string{
			"id":         "id",
			"name":       "name",
			"status":     "status",
			"created_at": "created_at",
			"updated_at": "updated_at",
			"tps_avg":    "tps_average",
			"tps_max":    "tps_max",
			"tps_min":    "tps_min",
		}

		if field, exists := validSortFields[sortBy]; exists {
			order := "ASC"
			if sortOrder == "desc" {
				order = "DESC"
			}
			orderBy = fmt.Sprintf("%s %s", field, order)
		}
	}

	query += fmt.Sprintf(` ORDER BY %s LIMIT ? OFFSET ?`, orderBy)
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered and sorted runs: %w", err)
	}
	defer rows.Close()

	var runs []*run.Run
	for rows.Next() {
		ru := &run.Run{}
		var startedAt, completedAt sql.NullTime
		var result sql.NullString
		var tpsMax, tpsMin, tpsAverage, tps95p, tps99p sql.NullFloat64

		err := rows.Scan(
			&ru.ID, &ru.Name, &ru.Description, &ru.Status,
			&ru.Config, &result, &tpsMax, &tpsMin, &tpsAverage, &tps95p, &tps99p,
			&ru.CreatedAt, &ru.UpdatedAt, &startedAt, &completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}

		// Заполняем опциональные поля
		if startedAt.Valid {
			ru.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			ru.CompletedAt = &completedAt.Time
		}
		if result.Valid {
			ru.Result = result.String
		}

		// Заполняем TPS метрики
		if tpsMax.Valid {
			ru.TPSMetrics.Max = &tpsMax.Float64
		}
		if tpsMin.Valid {
			ru.TPSMetrics.Min = &tpsMin.Float64
		}
		if tpsAverage.Valid {
			ru.TPSMetrics.Average = &tpsAverage.Float64
		}
		if tps95p.Valid {
			ru.TPSMetrics.P95 = &tps95p.Float64
		}
		if tps99p.Valid {
			ru.TPSMetrics.P99 = &tps99p.Float64
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

// GetFilterOptions возвращает уникальные значения для фильтров из config JSON
func (r *RunRepository) GetFilterOptions() (map[string][]string, error) {
	// Получаем все config'и и статусы из базы данных
	query := `SELECT config, status FROM runs WHERE config IS NOT NULL AND config != ''`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query configs: %w", err)
	}
	defer rows.Close()

	// Множества для хранения уникальных значений
	statuses := make(map[string]bool)
	loadTypes := make(map[string]bool)
	databases := make(map[string]bool)
	deploymentSchemas := make(map[string]bool)
	hardwareConfigs := make(map[string]bool)

	for rows.Next() {
		var configJSON, status string
		if err := rows.Scan(&configJSON, &status); err != nil {
			continue // Пропускаем некорректные записи
		}

		// Добавляем статус
		if status != "" {
			statuses[status] = true
		}

		// Парсим JSON config
		config, err := parseConfigJSON(configJSON)
		if err != nil {
			continue // Пропускаем некорректные JSON
		}

		// Добавляем значения в множества
		if config.LoadType != "" {
			loadTypes[config.LoadType] = true
		}
		if config.Database != "" {
			databases[config.Database] = true
		}
		if config.DeploymentSchema != "" {
			deploymentSchemas[config.DeploymentSchema] = true
		}
		if config.HardwareConfig != "" {
			hardwareConfigs[config.HardwareConfig] = true
		}
	}

	// Преобразуем множества в слайсы
	result := map[string][]string{
		"statuses":           mapKeysToSlice(statuses),
		"load_types":         mapKeysToSlice(loadTypes),
		"databases":          mapKeysToSlice(databases),
		"deployment_schemas": mapKeysToSlice(deploymentSchemas),
		"hardware_configs":   mapKeysToSlice(hardwareConfigs),
	}

	return result, nil
}

// ConfigData представляет структуру config JSON
type ConfigData struct {
	LoadType         string `json:"load_type"`
	Database         string `json:"database"`
	DeploymentSchema string `json:"deployment_schema"`
	HardwareConfig   string `json:"hardware_config"`
}

// parseConfigJSON парсит JSON config и извлекает нужные поля
func parseConfigJSON(configJSON string) (*ConfigData, error) {
	var config ConfigData
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}
	return &config, nil
}

// mapKeysToSlice преобразует ключи map в слайс строк
func mapKeysToSlice(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
