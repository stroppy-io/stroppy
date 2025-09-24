package service

import (
	"errors"

	"stroppy-cloud-panel/internal/domain/user"
	"stroppy-cloud-panel/pkg/auth"
)

// UserService реализует бизнес-логику пользователей
type UserService struct {
	userRepo   user.Repository
	jwtManager *auth.JWTManager
}

// NewUserService создает новый сервис пользователей
func NewUserService(userRepo user.Repository, jwtManager *auth.JWTManager) *UserService {
	return &UserService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
	}
}

// Register регистрирует нового пользователя
func (s *UserService) Register(username, password string) (*user.User, error) {
	// Проверяем, что пользователь не существует
	existingUser, err := s.userRepo.GetByUsername(username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return nil, err
	}
	if existingUser != nil {
		return nil, user.ErrUserExists
	}

	// Хешируем пароль
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	// Создаем пользователя
	newUser, err := user.NewUser(username, passwordHash)
	if err != nil {
		return nil, err
	}

	// Сохраняем в базу данных
	if err := s.userRepo.Create(newUser); err != nil {
		return nil, err
	}

	return newUser, nil
}

// Login аутентифицирует пользователя и возвращает JWT токен
func (s *UserService) Login(username, password string) (*user.User, string, error) {
	// Получаем пользователя по имени
	u, err := s.userRepo.GetByUsername(username)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, "", user.ErrInvalidPassword
		}
		return nil, "", err
	}

	// Проверяем пароль
	if !auth.CheckPassword(password, u.PasswordHash) {
		return nil, "", user.ErrInvalidPassword
	}

	// Генерируем JWT токен
	token, err := s.jwtManager.GenerateToken(u.ID, u.Username)
	if err != nil {
		return nil, "", err
	}

	return u, token, nil
}

// GetByID получает пользователя по ID
func (s *UserService) GetByID(id int) (*user.User, error) {
	return s.userRepo.GetByID(id)
}
