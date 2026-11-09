package middleware

import (
	"crypto/rand"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := c.Writer.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = rand.Text()
		}
		c.Header("X-Request-ID", requestID)
		c.Next()
		logger.Info("http request",
			"request_id", requestID,
			"method", c.Request.Method,
			"route", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}
}
