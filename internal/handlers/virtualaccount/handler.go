package virtualaccount

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/flip-bills/backend/internal/services/virtualaccount"
)

type Handler struct {
	svc *virtualaccount.Service
}

func NewHandler(svc *virtualaccount.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetVirtualAccount(c *gin.Context) {
	userID := c.GetString("user_id")

	va, err := h.svc.GetUserVirtualAccount(c.Request.Context(), userID)
	if err != nil {
		// Assuming error means not found for simplicity
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual account not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Virtual account retrieved successfully",
		"data":    va,
	})
}

func (h *Handler) CreateVirtualAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	email := c.GetString("email")
	fullName := c.GetString("full_name")
	phone := c.GetString("phone_number")
	
	// Usually BVN is collected during KYC. For now, we pass empty if not available
	// or assume it's fetched from the user profile service.
	bvn := "" 

	va, err := h.svc.CreateVirtualAccount(c.Request.Context(), userID, fullName, email, phone, bvn)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Failed to create virtual account"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Virtual account created successfully",
		"data":    va,
	})
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	vaGroup := router.Group("/wallet")
	{
		vaGroup.GET("/virtual-account", h.GetVirtualAccount)
		vaGroup.POST("/virtual-account", h.CreateVirtualAccount)
	}
}
