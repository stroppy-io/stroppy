package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthHandler обрабатывает health check запросы
type HealthHandler struct {
	db *sql.DB
}

// NewHealthHandler создает новый экземпляр HealthHandler
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{
		db: db,
	}
}

// HealthResponse представляет ответ health check
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime,omitempty"`
	Details   gin.H     `json:"details,omitempty"`
}

// Liveness проверяет, что приложение работает (не зависло)
// GET /healthz/liveness
func (h *HealthHandler) Liveness(c *gin.Context) {
	// Простая проверка, что приложение не зависло
	// В реальном приложении здесь может быть проверка критических компонентов
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
	}

	// Поддержка HEAD запросов для health checks
	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}

	c.JSON(http.StatusOK, response)
}

// Readiness проверяет, что приложение готово принимать трафик
// GET /healthz/readiness
func (h *HealthHandler) Readiness(c *gin.Context) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Details:   gin.H{},
	}

	// Проверка подключения к базе данных
	if err := h.checkDatabase(); err != nil {
		response.Status = "not ready"
		response.Details["database"] = gin.H{
			"status": "error",
			"error":  err.Error(),
		}

		// Поддержка HEAD запросов для health checks
		if c.Request.Method == "HEAD" {
			c.Status(http.StatusServiceUnavailable)
			return
		}

		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	response.Details["database"] = gin.H{
		"status": "ok",
	}

	// Поддержка HEAD запросов для health checks
	if c.Request.Method == "HEAD" {
		c.Status(http.StatusOK)
		return
	}

	c.JSON(http.StatusOK, response)
}

// Health общий health check endpoint
// GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Details:   gin.H{},
	}

	// Проверка базы данных
	dbStatus := "ok"
	if err := h.checkDatabase(); err != nil {
		dbStatus = "error"
		response.Details["database_error"] = err.Error()
	}

	response.Details["database"] = gin.H{
		"status": dbStatus,
	}

	// Если есть проблемы с БД, возвращаем 503
	if dbStatus == "error" {
		response.Status = "degraded"
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

// checkDatabase проверяет подключение к базе данных
func (h *HealthHandler) checkDatabase() error {
	if h.db == nil {
		return errors.New("database connection is nil")
	}

	// Ping базы данных
	if err := h.db.Ping(); err != nil {
		return err
	}

	return nil
}

// Startup проверяет, что приложение запустилось
// GET /healthz/startup
func (h *HealthHandler) Startup(c *gin.Context) {
	// Проверка, что все критические компоненты инициализированы
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Details:   gin.H{},
	}

	// Проверка базы данных
	if err := h.checkDatabase(); err != nil {
		response.Status = "not ready"
		response.Details["database"] = gin.H{
			"status": "error",
			"error":  err.Error(),
		}
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	response.Details["database"] = gin.H{
		"status": "ok",
	}

	c.JSON(http.StatusOK, response)
}
