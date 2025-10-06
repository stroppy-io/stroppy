package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"stroppy-cloud-panel/internal/domain/run"
	"stroppy-cloud-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

// RunHandler обрабатывает HTTP запросы для запусков
type RunHandler struct {
	runService run.Service
}

// NewRunHandler создает новый обработчик запусков
func NewRunHandler(runService run.Service) *RunHandler {
	return &RunHandler{
		runService: runService,
	}
}

// CreateRunRequest представляет запрос на создание запуска
type CreateRunRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=1000"`
	Config      string `json:"config" binding:"required"`
}

// UpdateRunRequest представляет запрос на обновление запуска
type UpdateRunRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=1000"`
	Config      string `json:"config" binding:"required"`
}

// UpdateStatusRequest представляет запрос на обновление статуса
type UpdateStatusRequest struct {
	Status run.RunStatus `json:"status" binding:"required"`
	Result string        `json:"result,omitempty"`
}

// UpdateTPSMetricsRequest представляет запрос на обновление TPS метрик
type UpdateTPSMetricsRequest struct {
	Max     *float64 `json:"max,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Average *float64 `json:"average,omitempty"`
	P95     *float64 `json:"95p,omitempty"`
	P99     *float64 `json:"99p,omitempty"`
}

// RunResponse представляет запуск в ответе
type RunResponse struct {
	*run.Run
}

// RunListResponse представляет список запусков
type RunListResponse struct {
	Runs  []*RunResponse `json:"runs"`
	Total int            `json:"total"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
}

// FilterOptionsResponse представляет опции для фильтров
type FilterOptionsResponse struct {
	Statuses          []string `json:"statuses"`
	LoadTypes         []string `json:"load_types"`
	Databases         []string `json:"databases"`
	DeploymentSchemas []string `json:"deployment_schemas"`
	HardwareConfigs   []string `json:"hardware_configs"`
}

// CreateRun создает новый запуск
func (h *RunHandler) CreateRun(c *gin.Context) {
	// Проверяем аутентификацию, но не используем userID
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	var req CreateRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ru, err := h.runService.Create(req.Name, req.Description, req.Config)
	if err != nil {
		if errors.Is(err, run.ErrInvalidRunData) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid run data",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create run",
		})
		return
	}

	c.JSON(http.StatusCreated, &RunResponse{Run: ru})
}

// GetRun получает запуск по ID
func (h *RunHandler) GetRun(c *gin.Context) {
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	runID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	ru, err := h.runService.GetByID(runID)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Run not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get run",
		})
		return
	}

	c.JSON(http.StatusOK, &RunResponse{Run: ru})
}

// GetRuns получает список всех запусков
func (h *RunHandler) GetRuns(c *gin.Context) {
	log.Println("=== DEBUG: GetRuns handler called ===")

	_, ok := middleware.GetUserID(c)
	if !ok {
		log.Println("DEBUG: User not authenticated")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	log.Println("DEBUG: User authenticated, proceeding")

	// Параметры пагинации
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	// Параметры фильтрации
	searchText := c.Query("search")
	status := c.Query("status")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	// Параметры сортировки
	sortBy := c.Query("sort_by")
	sortOrder := c.Query("sort_order") // "asc" или "desc"

	log.Printf("DEBUG: Calling GetAll with limit=%d, offset=%d, search=%s, status=%s, sort_by=%s, sort_order=%s", limit, offset, searchText, status, sortBy, sortOrder)

	runs, total, err := h.runService.GetAllWithFiltersAndSort(limit, offset, searchText, status, dateFrom, dateTo, sortBy, sortOrder)
	if err != nil {
		log.Printf("ERROR: GetAll failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get runs",
			"details": err.Error(),
		})
		return
	}

	log.Printf("DEBUG: GetAll returned %d runs, total=%d", len(runs), total)

	runResponses := make([]*RunResponse, len(runs))
	for i, ru := range runs {
		runResponses[i] = &RunResponse{Run: ru}
	}

	c.JSON(http.StatusOK, &RunListResponse{
		Runs:  runResponses,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// GetFilterOptions возвращает уникальные значения для фильтров
func (h *RunHandler) GetFilterOptions(c *gin.Context) {
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	options, err := h.runService.GetFilterOptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get filter options",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, options)
}

// UpdateRun обновляет запуск
func (h *RunHandler) UpdateRun(c *gin.Context) {
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	runID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	var req UpdateRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ru, err := h.runService.Update(runID, req.Name, req.Description, req.Config)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Run not found",
			})
			return
		}
		if errors.Is(err, run.ErrInvalidRunData) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid run data",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update run",
		})
		return
	}

	c.JSON(http.StatusOK, &RunResponse{Run: ru})
}

// UpdateRunStatus обновляет статус запуска
func (h *RunHandler) UpdateRunStatus(c *gin.Context) {
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	runID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	ru, err := h.runService.UpdateStatus(runID, req.Status, req.Result)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Run not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update run status",
		})
		return
	}

	c.JSON(http.StatusOK, &RunResponse{Run: ru})
}

// UpdateRunTPSMetrics обновляет TPS метрики запуска
func (h *RunHandler) UpdateRunTPSMetrics(c *gin.Context) {
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	runID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	var req UpdateTPSMetricsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Преобразуем запрос в доменную модель
	metrics := run.TPSMetrics{
		Max:     req.Max,
		Min:     req.Min,
		Average: req.Average,
		P95:     req.P95,
		P99:     req.P99,
	}

	ru, err := h.runService.UpdateTPSMetrics(runID, metrics)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Run not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update TPS metrics",
		})
		return
	}

	c.JSON(http.StatusOK, &RunResponse{Run: ru})
}

// DeleteRun удаляет запуск
func (h *RunHandler) DeleteRun(c *gin.Context) {
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	runID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	err = h.runService.Delete(runID)
	if err != nil {
		if errors.Is(err, run.ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Run not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete run",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Run deleted successfully",
	})
}

// GetTopRuns получает топ запусков для публичного просмотра (без авторизации)
func (h *RunHandler) GetTopRuns(c *gin.Context) {
	// Параметры пагинации
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit < 1 || limit > 50 {
		limit = 10
	}

	// Получаем топ завершенных запусков, отсортированных по TPS (средний)
	runs, total, err := h.runService.GetAllWithFiltersAndSort(limit, 0, "", "completed", "", "", "tps_avg", "desc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get top runs",
			"details": err.Error(),
		})
		return
	}

	runResponses := make([]*RunResponse, len(runs))
	for i, ru := range runs {
		runResponses[i] = &RunResponse{Run: ru}
	}

	c.JSON(http.StatusOK, &RunListResponse{
		Runs:  runResponses,
		Total: total,
		Page:  1,
		Limit: limit,
	})
}
