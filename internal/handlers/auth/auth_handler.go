package auth

import (
	authsvc "github.com/flip-bills/backend/internal/services/auth"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler exposes auth HTTP endpoints.
type Handler struct {
	svc *authsvc.Service
	log *zap.Logger
}

func NewHandler(svc *authsvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register godoc
// @Summary      Register a new user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body authsvc.RegisterRequest true "Registration payload"
// @Success      201  {object} authsvc.TokenPair
// @Router       /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req authsvc.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	tokens, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.Created(c, "registration successful", tokens)
}

// Login godoc
// @Summary      Authenticate user and get tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body authsvc.LoginRequest true "Login payload"
// @Success      200  {object} authsvc.TokenPair
// @Router       /auth/login [post]
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
