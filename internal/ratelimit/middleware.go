package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"digital-wallet/internal/auth"

	"github.com/gin-gonic/gin"
)

func Middleware(
	limiter *Limiter,
	limit int,
	window time.Duration,
	keyPrefix string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, err := buildRateLimitKey(c, keyPrefix)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "could not determine rate limit key",
			})
			return
		}

		result, err := limiter.Allow(
			c.Request.Context(),
			key,
			limit,
			window,
		)
		if err != nil {
		
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "rate limiting temporarily unavailable",
			})
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

		if !result.Allowed {
			retrySeconds := int(result.RetryAfter.Seconds())
			if result.RetryAfter > 0 && retrySeconds == 0 {
				retrySeconds = 1
			}

			c.Header("Retry-After", strconv.Itoa(retrySeconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}

func buildRateLimitKey(
	c *gin.Context,
	prefix string,
) (string, error) {
	if prefix == "" {
		return "", net.InvalidAddrError("rate limit prefix is empty")
	}

	if userID, ok := auth.UserIDFromContext(c); ok {
		return prefix + ":user:" + strconv.FormatInt(userID, 10), nil
	}


	ip := strings.TrimSpace(c.ClientIP())
	if net.ParseIP(ip) == nil {
		return "", net.InvalidAddrError("invalid client IP")
	}

	return prefix + ":ip:" + ip, nil
}
