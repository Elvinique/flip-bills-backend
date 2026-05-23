package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse is the unified envelope for every HTTP response.
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func OK(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Message: message, Data: data})
}

func Created(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{Success: true, Message: message, Data: data})
}

func BadRequest(c *gin.Context, message string, err interface{}) {
	c.JSON(http.StatusBadRequest, APIResponse{Success: false, Message: message, Error: err})
}

func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Message: message})
}

func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, APIResponse{Success: false, Message: message})
}

func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, APIResponse{Success: false, Message: message})
}

func InternalError(c *gin.Context, message string, err interface{}) {
	c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Message: message, Error: err})
}

func UnprocessableEntity(c *gin.Context, message string, err interface{}) {
	c.JSON(http.StatusUnprocessableEntity, APIResponse{Success: false, Message: message, Error: err})
}
