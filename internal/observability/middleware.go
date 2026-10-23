package observability

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

func RequestLoger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set("request_id", requestID)
		c.Header(RequestIDHeader, requestID)

		c.Next()
		duration := time.Since(start)
		status := c.Writer.Status()

		logger.Info("http_request",
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			slog.String("route", c.FullPath()),
			slog.Int("status", status),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		)
	}
}
