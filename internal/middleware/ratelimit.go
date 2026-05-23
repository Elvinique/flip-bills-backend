package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit is a sliding-window rate limiter backed by Redis.
// limit = max requests; window = rolling time window.
func RateLimit(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Key by IP for public routes; by userID for authenticated routes.
		key := fmt.Sprintf("rl:%s", c.ClientIP())
		if uid, exists := c.Get(userIDKey); exists {
			key = fmt.Sprintf("rl:%s", uid)
		}

		ctx := context.Background()
		pipe := rdb.Pipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window)
		if _, err := pipe.Exec(ctx); err != nil {
			// If Redis is down, fail open (allow the request).
			c.Next()
			return
		}

		if incr.Val() > int64(limit) {
			c.Header("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "rate limit exceeded — please slow down",
			})
			return
		}
		c.Next()
	}
}
