package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/stroppy-io/stroppy-cloud-panel/pkg/auth"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware создает middleware для аутентификации JWT
func AuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("=== AUTH DEBUG: %s %s ===", c.Request.Method, c.Request.URL.Path)

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			log.Println("AUTH DEBUG: No Authorization header")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header is required",
			})
			c.Abort()
			return
		}

		log.Printf("AUTH DEBUG: Auth header: %s...", authHeader[:min(50, len(authHeader))])

		// Проверяем формат Bearer token
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			log.Printf("AUTH DEBUG: Invalid header format, parts: %d", len(tokenParts))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header format",
			})
			c.Abort()
			return
		}

		tokenString := tokenParts[1]
		log.Printf("AUTH DEBUG: Token length: %d", len(tokenString))

		// Валидируем токен
		claims, err := jwtManager.ValidateToken(tokenString)
		if err != nil {
			log.Printf("AUTH DEBUG: Token validation failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
			})
			c.Abort()
			return
		}

		log.Printf("AUTH DEBUG: Token valid, userID=%d, username=%s", claims.UserID, claims.Username)

		// Сохраняем информацию о пользователе в контексте
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)

		c.Next()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetUserID извлекает ID пользователя из контекста Gin
func GetUserID(c *gin.Context) (int, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}

	id, ok := userID.(int)
	return id, ok
}

// GetUsername извлекает имя пользователя из контекста Gin
func GetUsername(c *gin.Context) (string, bool) {
	username, exists := c.Get("username")
	if !exists {
		return "", false
	}

	name, ok := username.(string)
	return name, ok
}
