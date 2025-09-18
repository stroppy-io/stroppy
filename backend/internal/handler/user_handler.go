package handler

import (
	"errors"
	"net/http"

	"stroppy-cloud-pannel/internal/domain/user"

	"github.com/gin-gonic/gin"
)

// UserHandler обрабатывает HTTP запросы для пользователей
type UserHandler struct {
	userService user.Service
}

// NewUserHandler создает новый обработчик пользователей
func NewUserHandler(userService user.Service) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// RegisterRequest представляет запрос на регистрацию
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
}

// LoginRequest представляет запрос на вход
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AuthResponse представляет ответ аутентификации
type AuthResponse struct {
	User  *UserResponse `json:"user"`
	Token string        `json:"token"`
}

// UserResponse представляет пользователя в ответе
type UserResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// Register регистрирует нового пользователя
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	u, err := h.userService.Register(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, user.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "User already exists",
			})
			return
		}
		if errors.Is(err, user.ErrInvalidUserData) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid user data",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to register user",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully",
		"user": &UserResponse{
			ID:       u.ID,
			Username: u.Username,
		},
	})
}

// Login аутентифицирует пользователя
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	u, token, err := h.userService.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, user.ErrInvalidPassword) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid username or password",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to login",
		})
		return
	}

	c.JSON(http.StatusOK, &AuthResponse{
		User: &UserResponse{
			ID:       u.ID,
			Username: u.Username,
		},
		Token: token,
	})
}

// GetProfile получает профиль текущего пользователя
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	u, err := h.userService.GetByID(userID.(int))
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get user profile",
		})
		return
	}

	c.JSON(http.StatusOK, &UserResponse{
		ID:       u.ID,
		Username: u.Username,
	})
}
