package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// PanicRecovery middleware для перехвата и логирования panic
func PanicRecovery() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Получаем стек вызовов
				stack := debug.Stack()

				// Логируем panic с подробной информацией
				log.Printf("[PANIC] %s %s | Error: %v | Stack: %s",
					c.Request.Method, c.Request.RequestURI, err, string(stack))

				// Возвращаем ошибку клиенту
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Internal server error",
					"message": "An unexpected error occurred",
					"details": fmt.Sprintf("%v", err),
				})

				// Прерываем выполнение
				c.Abort()
			}
		}()

		// Продолжаем выполнение
		c.Next()
	})
}
