package middleware

import (
	"strings"

	"github.com/flip-bills/backend/pkg/jwt"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
)

const userIDKey = "userID"
const kycTierKey = "kycTier"

// Auth validates the Bearer token on every protected route.
func Auth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			response.Unauthorized(c, "authorization header is required")
			c.Abort()
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			response.Unauthorized(c, "authorization header format must be: Bearer <token>")
			c.Abort()
			return
		}

		claims, err := jwtManager.Validate(parts[1])
		if err != nil {
			response.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		c.Set(userIDKey, claims.UserID)
		c.Set(kycTierKey, claims.KYCTier)
		c.Next()
	}
}

// RequireKYC enforces a minimum KYC tier for sensitive operations.
// Example: wallet transfers above ₦50k require KYCTierOne.
func RequireKYC(minTier int) gin.HandlerFunc {
	return func(c *gin.Context) {
		tier, exists := c.Get(kycTierKey)
		if !exists {
			response.Unauthorized(c, "unauthenticated")
			c.Abort()
			return
		}
		if tier.(int) < minTier {
			response.Forbidden(c, "KYC upgrade required for this operation")
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetUserID is a convenience helper for handlers.
func GetUserID(c *gin.Context) string {
	v, _ := c.Get(userIDKey)
	return v.(string)
}
