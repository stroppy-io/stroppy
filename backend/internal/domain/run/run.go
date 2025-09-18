package run

import (
	"errors"
	"time"
)

var (
	ErrRunNotFound    = errors.New("run not found")
	ErrInvalidRunData = errors.New("invalid run data")
	ErrUnauthorized   = errors.New("unauthorized access to run")
)

// RunStatus представляет статус запуска
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
	StatusCancelled RunStatus = "cancelled"
)

// Run представляет доменную модель запуска
type Run struct {
	ID          int        `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	Status      RunStatus  `json:"status" db:"status"`
	Config      string     `json:"config" db:"config"`           // JSON конфигурация
	Result      string     `json:"result,omitempty" db:"result"` // JSON результат
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}

// NewRun создает новый запуск с валидацией
func NewRun(name, description, config string) (*Run, error) {
	if name == "" {
		return nil, ErrInvalidRunData
	}
	if config == "" {
		return nil, ErrInvalidRunData
	}

	now := time.Now()
	return &Run{
		Name:        name,
		Description: description,
		Status:      StatusPending,
		Config:      config,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// UpdateStatus обновляет статус запуска с соответствующими временными метками
func (r *Run) UpdateStatus(status RunStatus) {
	r.Status = status
	r.UpdatedAt = time.Now()

	switch status {
	case StatusRunning:
		if r.StartedAt == nil {
			now := time.Now()
			r.StartedAt = &now
		}
	case StatusCompleted, StatusFailed, StatusCancelled:
		if r.CompletedAt == nil {
			now := time.Now()
			r.CompletedAt = &now
		}
	}
}

// Repository определяет интерфейс для работы с запусками
type Repository interface {
	Create(run *Run) error
	GetByID(id int) (*Run, error)
	GetAll(limit, offset int) ([]*Run, error)
	GetAllWithFilters(limit, offset int, searchText, status, dateFrom, dateTo string) ([]*Run, error)
	Update(run *Run) error
	Delete(id int) error
	Count() (int, error)
	CountWithFilters(searchText, status, dateFrom, dateTo string) (int, error)
}

// Service определяет интерфейс бизнес-логики запусков
type Service interface {
	Create(name, description, config string) (*Run, error)
	GetByID(id int) (*Run, error)
	GetAll(limit, offset int) ([]*Run, int, error)
	GetAllWithFilters(limit, offset int, searchText, status, dateFrom, dateTo string) ([]*Run, int, error)
	Update(id int, name, description, config string) (*Run, error)
	UpdateStatus(id int, status RunStatus, result string) (*Run, error)
	Delete(id int) error
}
