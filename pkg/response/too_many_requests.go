package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TooManyRequests writes a 429 response.
// Add this to your existing response.go file — it's a separate file
// so you don't have to touch the original.
func TooManyRequests(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, APIResponse{
		Success: false,
		Message: message,
	})
}
