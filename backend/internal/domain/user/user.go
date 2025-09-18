package user

import (
	"errors"
	"time"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserExists      = errors.New("user already exists")
	ErrInvalidUserData = errors.New("invalid user data")
)

// User представляет доменную модель пользователя
type User struct {
	ID           int       `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// NewUser создает нового пользователя с валидацией
func NewUser(username, passwordHash string) (*User, error) {
	if username == "" {
		return nil, ErrInvalidUserData
	}
	if passwordHash == "" {
		return nil, ErrInvalidUserData
	}

	now := time.Now()
	return &User{
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Repository определяет интерфейс для работы с пользователями
type Repository interface {
	Create(user *User) error
	GetByID(id int) (*User, error)
	GetByUsername(username string) (*User, error)
	Update(user *User) error
	Delete(id int) error
}

// Service определяет интерфейс бизнес-логики пользователей
type Service interface {
	Register(username, password string) (*User, error)
	Login(username, password string) (*User, string, error) // возвращает пользователя и JWT токен
	GetByID(id int) (*User, error)
}
