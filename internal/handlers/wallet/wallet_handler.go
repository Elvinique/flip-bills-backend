package wallet

import (
	"strconv"

	"github.com/flip-bills/backend/internal/middleware"
	walletsvc "github.com/flip-bills/backend/internal/services/wallet"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	svc *walletsvc.Service
	log *zap.Logger
}
// POST /wallet/initialize-funding
func (h *Handler) InitializeFunding(c *gin.Context) {
	var req walletsvc.InitializeFundingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}

	res, err := h.svc.InitializeFunding(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.InternalError(c, err.Error(), nil)
		return
	}

	response.OK(c, "payment funding initialized successfully", res)
}

func NewHandler(svc *walletsvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// GET /wallet/balance
func (h *Handler) GetBalance(c *gin.Context) {
	bal, err := h.svc.GetBalance(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.InternalError(c, err.Error(), nil)
		return
	}
	response.OK(c, "wallet balance retrieved", bal)
}

// GET /wallet/transactions?page=1&limit=20
func (h *Handler) GetTransactions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.svc.GetTransactions(c.Request.Context(), middleware.GetUserID(c), page, limit)
	if err != nil {
		response.InternalError(c, "could not retrieve transactions", nil)
		return
	}
	response.OK(c, "transactions retrieved", result)
}

// POST /wallet/fund  — called after successful payment gateway webhook verification
func (h *Handler) FundWallet(c *gin.Context) {
	var req walletsvc.FundWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tx, err := h.svc.FundWallet(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "wallet funded successfully", tx)
}
