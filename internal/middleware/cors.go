package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORSMiddleware создает middleware для обработки CORS запросов
// Разрешает все источники (*) для разработки
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Разрешаем все источники
		c.Header("Access-Control-Allow-Origin", "*")

		// Разрешаем все методы
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")

		// Разрешаем все заголовки
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")

		// Разрешаем кэширование preflight запросов на 12 часов
		c.Header("Access-Control-Max-Age", "43200")

		// Разрешаем отправку cookies и авторизационных заголовков
		c.Header("Access-Control-Allow-Credentials", "true")

		// Обрабатываем preflight запросы
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
