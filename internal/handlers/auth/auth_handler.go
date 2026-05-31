package auth

import (
	"github.com/flip-bills/backend/internal/middleware"
	"github.com/flip-bills/backend/internal/models"
	authsvc "github.com/flip-bills/backend/internal/services/auth"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	svc *authsvc.Service
	log *zap.Logger
}

func NewHandler(svc *authsvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// POST /auth/register
func (h *Handler) Register(c *gin.Context) {
	var req authsvc.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	msg, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.Created(c, msg, nil)
}

// POST /auth/login
func (h *Handler) Login(c *gin.Context) {
	var req authsvc.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tokens, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}
	response.OK(c, "login successful", tokens)
}

// POST /auth/verify-phone
func (h *Handler) VerifyPhone(c *gin.Context) {
	var req authsvc.VerifyPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	if err := h.svc.VerifyPhone(c.Request.Context(), req); err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "phone verified successfully", nil)
}

// POST /auth/resend-otp
func (h *Handler) ResendOTP(c *gin.Context) {
	var body struct {
		Phone   string `json:"phone"   binding:"required"`
		Purpose string `json:"purpose" binding:"required,oneof=phone_verify pin_reset tx_auth"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	_ = h.svc.ResendOTP(c.Request.Context(), body.Phone, models.OTPPurpose(body.Purpose))
	// Always return success to prevent phone enumeration.
	response.OK(c, "if that number is registered, a code has been sent", nil)
}

// POST /auth/set-pin  [protected]
func (h *Handler) SetPIN(c *gin.Context) {
	var body struct {
		Phone      string `json:"phone"`
		PIN        string `json:"pin"        binding:"required,len=6"`
		ConfirmPIN string `json:"confirm_pin" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	// Try JWT first, fall back to phone lookup
	userID := middleware.GetUserID(c)
	if userID == "" && body.Phone != "" {
		u, err := h.svc.GetUserByPhone(c.Request.Context(), body.Phone)
		if err != nil {
			response.UnprocessableEntity(c, "user not found", nil)
			return
		}
		userID = u.ID.String()
	}
	req := authsvc.SetPINRequest{PIN: body.PIN, ConfirmPIN: body.ConfirmPIN}
	if err := h.svc.SetPIN(c.Request.Context(), userID, req); err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "transaction PIN set successfully", nil)
}

// POST /auth/kyc/upgrade  [protected]
func (h *Handler) UpgradeKYC(c *gin.Context) {
	var req authsvc.KYCUpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	if req.BVN == "" && req.NIN == "" {
		response.BadRequest(c, "provide either bvn or nin", nil)
		return
	}
	newTier, err := h.svc.UpgradeKYC(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "KYC upgraded successfully", gin.H{"kyc_tier": newTier})
}

// POST /auth/refresh
func (h *Handler) RefreshToken(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "refresh_token is required", err.Error())
		return
	}
	tokens, err := h.svc.RefreshToken(c.Request.Context(), body.RefreshToken)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}
	response.OK(c, "token refreshed", tokens)
}
