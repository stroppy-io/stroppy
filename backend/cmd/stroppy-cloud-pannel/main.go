package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"stroppy-cloud-pannel/internal/handler"
	"stroppy-cloud-pannel/internal/middleware"
	"stroppy-cloud-pannel/internal/repository/postgres"
	"stroppy-cloud-pannel/internal/service"
	"stroppy-cloud-pannel/pkg/auth"
	"stroppy-cloud-pannel/pkg/database"
	"stroppy-cloud-pannel/pkg/migrations"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("=== MAIN DEBUG: Application starting ===")
	log.Println("=== MAIN DEBUG: Application starting ===")

	// Настройки из переменных окружения
	jwtSecret := getEnv("JWT_SECRET", "your-secret-key-change-in-production")
	port := getEnv("PORT", "8080")
	staticDir := getEnv("STATIC_DIR", "./web")

	// Конфигурация базы данных
	dbConfig := database.NewConfigFromEnv()
	dbDSN := dbConfig.DSN()

	fmt.Printf("MAIN DEBUG: Config - dbHost=%s, dbPort=%s, dbName=%s, port=%s\n", dbConfig.Host, dbConfig.Port, dbConfig.DBName, port)
	log.Printf("MAIN DEBUG: Config - dbHost=%s, dbPort=%s, dbName=%s, port=%s", dbConfig.Host, dbConfig.Port, dbConfig.DBName, port)

	// Инициализация базы данных
	db, err := database.NewPostgresDB(dbDSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Применение миграций
	if err := migrations.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Инициализация JWT менеджера
	jwtManager := auth.NewJWTManager(jwtSecret, 24*time.Hour)

	// Инициализация репозиториев
	userRepo := postgres.NewUserRepository(db)
	runRepo := postgres.NewRunRepository(db)

	// Инициализация сервисов
	userService := service.NewUserService(userRepo, jwtManager)
	runService := service.NewRunService(runRepo)

	// Инициализация обработчиков
	userHandler := handler.NewUserHandler(userService)
	runHandler := handler.NewRunHandler(runService)
	healthHandler := handler.NewHealthHandler(db)

	// Настройка Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Временно отключаем наши middleware для диагностики
	// r.Use(middleware.PanicRecovery())
	// r.Use(middleware.DetailedLogging())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// CORS middleware
	r.Use(middleware.CORSMiddleware())

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

	// Health check endpoints для Kubernetes проб
	healthz := r.Group("/healthz")
	{
		healthz.GET("/liveness", healthHandler.Liveness)
		healthz.HEAD("/liveness", healthHandler.Liveness)
		healthz.GET("/readiness", healthHandler.Readiness)
		healthz.HEAD("/readiness", healthHandler.Readiness)
		healthz.GET("/startup", healthHandler.Startup)
		healthz.HEAD("/startup", healthHandler.Startup)
	}

	// Общий health endpoint
	r.GET("/health", healthHandler.Health)

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

			// Если запрос к health endpoints, возвращаем 404
			if c.Request.URL.Path == "/health" ||
				(len(c.Request.URL.Path) > 7 && c.Request.URL.Path[:8] == "/healthz") {
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
					"health":    "/health",
					"healthz":   "/healthz",
					"liveness":  "/healthz/liveness",
					"readiness": "/healthz/readiness",
					"startup":   "/healthz/startup",
					"api":       "/api/v1",
					"auth":      "/api/v1/auth",
					"runs":      "/api/v1/runs",
					"profile":   "/api/v1/profile",
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
