package service

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/domain/run"
)

// RunService реализует бизнес-логику запусков
type RunService struct {
	runRepo run.Repository
}

// NewRunService создает новый сервис запусков
func NewRunService(runRepo run.Repository) *RunService {
	return &RunService{
		runRepo: runRepo,
	}
}

// Create создает новый запуск
func (s *RunService) Create(name, description, config string) (*run.Run, error) {
	newRun, err := run.NewRun(name, description, config)
	if err != nil {
		return nil, err
	}

	if err := s.runRepo.Create(newRun); err != nil {
		return nil, err
	}

	return newRun, nil
}

// GetByID получает запуск по ID
func (s *RunService) GetByID(id int) (*run.Run, error) {
	return s.runRepo.GetByID(id)
}

// GetAll получает все запуски с пагинацией
func (s *RunService) GetAll(limit, offset int) ([]*run.Run, int, error) {
	runs, err := s.runRepo.GetAll(limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.runRepo.Count()
	if err != nil {
		return nil, 0, err
	}

	return runs, total, nil
}

// GetAllWithFilters получает запуски с фильтрацией и пагинацией
func (s *RunService) GetAllWithFilters(limit, offset int, searchText, status, dateFrom, dateTo string) ([]*run.Run, int, error) {
	runs, err := s.runRepo.GetAllWithFilters(limit, offset, searchText, status, dateFrom, dateTo)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.runRepo.CountWithFilters(searchText, status, dateFrom, dateTo)
	if err != nil {
		return nil, 0, err
	}

	return runs, total, nil
}

// GetAllWithFiltersAndSort получает запуски с фильтрацией, пагинацией и сортировкой
func (s *RunService) GetAllWithFiltersAndSort(limit, offset int, searchText, status, dateFrom, dateTo, sortBy, sortOrder string) ([]*run.Run, int, error) {
	runs, err := s.runRepo.GetAllWithFiltersAndSort(limit, offset, searchText, status, dateFrom, dateTo, sortBy, sortOrder)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.runRepo.CountWithFilters(searchText, status, dateFrom, dateTo)
	if err != nil {
		return nil, 0, err
	}

	return runs, total, nil
}

// Update обновляет запуск
func (s *RunService) Update(id int, name, description, config string) (*run.Run, error) {
	ru, err := s.runRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Валидируем новые данные
	if name == "" {
		return nil, run.ErrInvalidRunData
	}
	if config == "" {
		return nil, run.ErrInvalidRunData
	}

	// Обновляем поля
	ru.Name = name
	ru.Description = description
	ru.Config = config

	if err := s.runRepo.Update(ru); err != nil {
		return nil, err
	}

	return ru, nil
}

// UpdateStatus обновляет статус запуска
func (s *RunService) UpdateStatus(id int, status run.RunStatus, result string) (*run.Run, error) {
	ru, err := s.runRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Обновляем статус и результат
	ru.UpdateStatus(status)
	if result != "" {
		ru.Result = result
	}

	if err := s.runRepo.Update(ru); err != nil {
		return nil, err
	}

	return ru, nil
}

// UpdateTPSMetrics обновляет метрики TPS для запуска
func (s *RunService) UpdateTPSMetrics(id int, metrics run.TPSMetrics) (*run.Run, error) {
	ru, err := s.runRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Обновляем метрики TPS
	ru.UpdateTPSMetrics(metrics)

	if err := s.runRepo.Update(ru); err != nil {
		return nil, err
	}

	return ru, nil
}

// Delete удаляет запуск
func (s *RunService) Delete(id int) error {
	return s.runRepo.Delete(id)
}

// GetFilterOptions возвращает уникальные значения для фильтров
func (s *RunService) GetFilterOptions() (map[string][]string, error) {
	return s.runRepo.GetFilterOptions()
}
