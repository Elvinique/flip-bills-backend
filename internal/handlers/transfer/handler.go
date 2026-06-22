package transfer

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/flip-bills/backend/internal/services/transfer"
)

type Handler struct {
	svc *transfer.Service
}

func NewHandler(svc *transfer.Service) *Handler {
	return &Handler{svc: svc}
}

type BankTransferRequest struct {
	BankCode      string `json:"bank_code" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required"`
	AccountName   string `json:"account_name" binding:"required"`
	AmountKobo    int64  `json:"amount_kobo" binding:"required,gt=0"`
	Narration     string `json:"narration"`
	Pin           string `json:"pin" binding:"required"` // Should be verified via Auth service middleware
}

func (h *Handler) InitiateBankTransfer(c *gin.Context) {
	var req BankTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters"})
		return
	}

	// Retrieve authenticated user info from Gin context
	userID := c.GetString("user_id")
	walletID := c.GetString("wallet_id")

	// Fixed fee of 50 NGN (5000 kobo) for transfers
	var feeKobo int64 = 5000

	resp, err := h.svc.InitiateBankTransfer(c.Request.Context(), userID, walletID, req.BankCode, req.AccountNumber, req.AccountName, req.Narration, req.AmountKobo, feeKobo)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Transfer initiated successfully",
		"data":    resp,
	})
}

func (h *Handler) GetBanks(c *gin.Context) {
	banks, err := h.svc.GetSupportedBanks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch banks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Banks retrieved successfully",
		"data":    banks,
	})
}

type ResolveAccountRequest struct {
	AccountNumber string `form:"account_number" binding:"required"`
	BankCode      string `form:"bank_code" binding:"required"`
}

func (h *Handler) ResolveAccount(c *gin.Context) {
	var req ResolveAccountRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing account_number or bank_code"})
		return
	}

	info, err := h.svc.ResolveAccount(c.Request.Context(), req.AccountNumber, req.BankCode)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account could not be resolved"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Account resolved successfully",
		"data":    info,
	})
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	transferGroup := router.Group("/transfers")
	{
		transferGroup.POST("/bank", h.InitiateBankTransfer)
		transferGroup.GET("/banks", h.GetBanks)
		transferGroup.GET("/resolve-account", h.ResolveAccount)
	}
}
