package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"stroppy-cloud-pannel/internal/domain/user"
)

// UserRepository реализует интерфейс user.Repository для PostgreSQL
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository создает новый репозиторий пользователей
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create создает нового пользователя в базе данных
func (r *UserRepository) Create(u *user.User) error {
	query := `
		INSERT INTO users (username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	err := r.db.QueryRow(query, u.Username, u.PasswordHash, u.CreatedAt, u.UpdatedAt).Scan(&u.ID)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID получает пользователя по ID
func (r *UserRepository) GetByID(id int) (*user.User, error) {
	query := `
		SELECT id, username, password_hash, created_at, updated_at
		FROM users WHERE id = $1
	`

	u := &user.User{}
	err := r.db.QueryRow(query, id).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, user.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return u, nil
}

// GetByUsername получает пользователя по имени пользователя
func (r *UserRepository) GetByUsername(username string) (*user.User, error) {
	query := `
		SELECT id, username, password_hash, created_at, updated_at
		FROM users WHERE username = $1
	`

	u := &user.User{}
	err := r.db.QueryRow(query, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, user.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return u, nil
}

// Update обновляет пользователя
func (r *UserRepository) Update(u *user.User) error {
	u.UpdatedAt = time.Now()

	query := `
		UPDATE users 
		SET username = $1, password_hash = $2, updated_at = $3
		WHERE id = $4
	`

	result, err := r.db.Exec(query, u.Username, u.PasswordHash, u.UpdatedAt, u.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return user.ErrUserNotFound
	}

	return nil
}

// Delete удаляет пользователя
func (r *UserRepository) Delete(id int) error {
	query := `DELETE FROM users WHERE id = $1`

	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return user.ErrUserNotFound
	}

	return nil
}
