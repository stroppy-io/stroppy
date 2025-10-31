package middleware

import (
	"bytes"
	"io"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// responseBodyWriter обёртка для записи тела ответа
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// DetailedLogging middleware для подробного логирования запросов и ответов
func DetailedLogging() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		start := time.Now()

		// Читаем тело запроса
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Создаём обёртку для записи ответа
		w := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = w

		// Логируем начало запроса
		log.Printf("[REQUEST] %s %s | Headers: %v | Body: %s",
			c.Request.Method, c.Request.RequestURI, c.Request.Header, string(requestBody))

		// Выполняем запрос
		c.Next()

		// Логируем результат
		duration := time.Since(start)
		statusCode := c.Writer.Status()
		responseBody := w.body.String()

		log.Printf("[RESPONSE] %s %s | Status: %d | Duration: %v | Body: %s",
			c.Request.Method, c.Request.RequestURI, statusCode, duration, responseBody)

		// Если есть ошибки, логируем их
		if len(c.Errors) > 0 {
			log.Printf("[ERRORS] %s %s | Errors: %v", c.Request.Method, c.Request.RequestURI, c.Errors)
		}
	})
}
