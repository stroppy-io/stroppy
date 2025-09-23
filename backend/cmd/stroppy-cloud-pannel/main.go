package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"stroppy-cloud-pannel/internal/handler"
	"stroppy-cloud-pannel/internal/middleware"
	"stroppy-cloud-pannel/internal/repository/sqlite"
	"stroppy-cloud-pannel/internal/service"
	"stroppy-cloud-pannel/pkg/auth"
	"stroppy-cloud-pannel/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("=== MAIN DEBUG: Application starting ===")
	log.Println("=== MAIN DEBUG: Application starting ===")

	// Настройки из переменных окружения
	dbPath := getEnv("DB_PATH", "./stroppy.db")
	jwtSecret := getEnv("JWT_SECRET", "your-secret-key-change-in-production")
	port := getEnv("PORT", "8080")
	staticDir := getEnv("STATIC_DIR", "./web")

	fmt.Printf("MAIN DEBUG: Config - dbPath=%s, port=%s\n", dbPath, port)
	log.Printf("MAIN DEBUG: Config - dbPath=%s, port=%s", dbPath, port)

	// Инициализация базы данных
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.InitSchema(db); err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Инициализация JWT менеджера
	jwtManager := auth.NewJWTManager(jwtSecret, 24*time.Hour)

	// Инициализация репозиториев
	userRepo := sqlite.NewUserRepository(db)
	runRepo := sqlite.NewRunRepository(db)

	// Инициализация сервисов
	userService := service.NewUserService(userRepo, jwtManager)
	runService := service.NewRunService(runRepo)

	// Инициализация обработчиков
	userHandler := handler.NewUserHandler(userService)
	runHandler := handler.NewRunHandler(runService)

	// Настройка Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Временно отключаем наши middleware для диагностики
	// r.Use(middleware.PanicRecovery())
	// r.Use(middleware.DetailedLogging())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Публичные маршруты
	api := r.Group("/api/v1")
	{
		// Аутентификация
		auth := api.Group("/auth")
		{
			auth.POST("/register", userHandler.Register)
			auth.POST("/login", userHandler.Login)
		}

		// Защищенные маршруты
		protected := api.Group("/")
		protected.Use(middleware.AuthMiddleware(jwtManager))
		{
			// Пользователи
			protected.GET("/profile", userHandler.GetProfile)

			// Запуски
			runs := protected.Group("/runs")
			{
				runs.POST("", runHandler.CreateRun)
				runs.GET("", runHandler.GetRuns)
				runs.GET("/filter-options", runHandler.GetFilterOptions)
				runs.GET("/:id", runHandler.GetRun)
				runs.PUT("/:id", runHandler.UpdateRun)
				runs.PUT("/:id/status", runHandler.UpdateRunStatus)
				runs.PUT("/:id/tps", runHandler.UpdateRunTPSMetrics)
				runs.DELETE("/:id", runHandler.DeleteRun)
			}
		}

		log.Println("MAIN DEBUG: Routes configured successfully")
	}

	// Healthcheck endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
		})
	})

	// Обслуживание статических файлов
	if _, err := os.Stat(staticDir); err == nil {
		log.Printf("Serving static files from %s", staticDir)

		// Обслуживание статических ресурсов (CSS, JS, изображения)
		r.Static("/assets", filepath.Join(staticDir, "assets"))
		r.StaticFile("/vite.svg", filepath.Join(staticDir, "vite.svg"))
		r.StaticFile("/favicon.ico", filepath.Join(staticDir, "favicon.ico"))

		// SPA маршрутизация - все остальные запросы отдают index.html
		r.NoRoute(func(c *gin.Context) {
			// Если запрос к API, возвращаем 404
			if len(c.Request.URL.Path) > 4 && c.Request.URL.Path[:5] == "/api/" {
				c.JSON(404, gin.H{"error": "API endpoint not found"})
				return
			}

			// Если запрос к health endpoint, возвращаем 404
			if c.Request.URL.Path == "/health" {
				c.JSON(404, gin.H{"error": "Endpoint not found"})
				return
			}

			// Для всех остальных запросов отдаем index.html (SPA)
			c.File(filepath.Join(staticDir, "index.html"))
		})
	} else {
		log.Printf("Static directory %s not found, serving API only", staticDir)

		// Если статических файлов нет, показываем информацию об API
		r.GET("/", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "Stroppy Cloud Panel API",
				"version": "1.0.0",
				"endpoints": gin.H{
					"health":  "/health",
					"api":     "/api/v1",
					"auth":    "/api/v1/auth",
					"runs":    "/api/v1/runs",
					"profile": "/api/v1/profile",
				},
			})
		})
	}

	log.Printf("Starting server on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// getEnv получает переменную окружения с значением по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
