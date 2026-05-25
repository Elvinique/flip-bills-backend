package utilities

import (
	"errors"
	"strings"

	"github.com/flip-bills/backend/internal/middleware"
	utilitysvc "github.com/flip-bills/backend/internal/services/utilities"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	svc *utilitysvc.Service
	log *zap.Logger
}

func NewHandler(svc *utilitysvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) BuyAirtime(c *gin.Context) {
	var req utilitysvc.AirtimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tx, err := h.svc.PurchaseAirtime(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "airtime purchase successful", tx)
}

func (h *Handler) GetCatalog(c *gin.Context) {
	response.OK(c, "VAS catalog retrieved", h.svc.GetCatalog(c.Request.Context()))
}

func (h *Handler) BuyData(c *gin.Context) {
	var req utilitysvc.DataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tx, err := h.svc.PurchaseData(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "data purchase successful", tx)
}

func (h *Handler) PayElectricity(c *gin.Context) {
	var req utilitysvc.ElectricityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tx, err := h.svc.PayElectricity(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "electricity payment successful", tx)
}

func (h *Handler) FundBetting(c *gin.Context) {
	var req utilitysvc.BettingFundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tx, err := h.svc.FundBettingWallet(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		var challenge *utilitysvc.RiskChallengeError
		if errors.As(err, &challenge) {
			response.UnprocessableEntity(c, "extra confirmation required", challenge)
			return
		}
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "betting wallet funded", tx)
}

func (h *Handler) GetTransaction(c *gin.Context) {
	ref := strings.TrimSpace(c.Param("reference"))
	if ref == "" {
		response.BadRequest(c, "reference is required", nil)
		return
	}

	tx, err := h.svc.GetTransaction(c.Request.Context(), middleware.GetUserID(c), ref)
	if err != nil {
		response.NotFound(c, "transaction not found")
		return
	}
	response.OK(c, "transaction retrieved", tx)
}
