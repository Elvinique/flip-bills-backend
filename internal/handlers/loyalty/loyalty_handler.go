package loyalty

import (
	"strconv"

	"github.com/flip-bills/backend/internal/middleware"
	loyaltysvc "github.com/flip-bills/backend/internal/services/loyalty"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	svc *loyaltysvc.Service
	log *zap.Logger
}

func NewHandler(svc *loyaltysvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// GET /api/v1/loyalty/balance
func (h *Handler) GetBalance(c *gin.Context) {
	bal, err := h.svc.GetBalance(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.InternalError(c, err.Error(), nil)
		return
	}
	response.OK(c, "loyalty balance retrieved", bal)
}

// GET /api/v1/loyalty/history?page=1&limit=20
func (h *Handler) GetHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	history, err := h.svc.GetHistory(c.Request.Context(), middleware.GetUserID(c), page, limit)
	if err != nil {
		response.InternalError(c, "could not retrieve points history", nil)
		return
	}
	response.OK(c, "points history retrieved", history)
}

// POST /api/v1/loyalty/redeem
func (h *Handler) RedeemPoints(c *gin.Context) {
	var req loyaltysvc.RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	result, err := h.svc.RedeemPoints(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "points redeemed successfully", result)
}
