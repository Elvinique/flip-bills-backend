package dispatcher

import (
	"github.com/flip-bills/backend/internal/middleware"
	dispatchersvc "github.com/flip-bills/backend/internal/services/dispatcher"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	svc *dispatchersvc.Service
	log *zap.Logger
}

func NewHandler(svc *dispatchersvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// POST /webhooks/dispatcher
// Called by the operator's Terminal Dispatcher portal when a disruption occurs.
func (h *Handler) HandleEvent(c *gin.Context) {
	var req dispatchersvc.DispatchEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	result, err := h.svc.HandleDispatchEvent(c.Request.Context(), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "dispatch event processed", result)
}

// POST /api/v1/travel/bookings/:id/reschedule  [protected]
// Passenger selects a new vehicle and departure time after a disruption.
func (h *Handler) Reschedule(c *gin.Context) {
	bookingID := c.Param("id")
	var req dispatchersvc.RescheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	newBooking, err := h.svc.RescheduleBooking(
		c.Request.Context(),
		middleware.GetUserID(c),
		bookingID,
		req,
	)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "booking rescheduled successfully", newBooking)
}

// POST /api/v1/travel/bookings/:id/refund  [protected]
// Passenger claims an instant wallet refund after a disruption.
func (h *Handler) Refund(c *gin.Context) {
	bookingID := c.Param("id")
	tx, err := h.svc.RefundBooking(
		c.Request.Context(),
		middleware.GetUserID(c),
		bookingID,
	)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.OK(c, "refund credited to your wallet", tx)
}
