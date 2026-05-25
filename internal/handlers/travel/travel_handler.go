package travel

import (
	"github.com/flip-bills/backend/internal/middleware"
	travelsvc "github.com/flip-bills/backend/internal/services/travel"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	svc *travelsvc.Service
	log *zap.Logger
}

func NewHandler(svc *travelsvc.Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// GET /travel/bus/search?origin=Lagos&destination=Abuja&departure_date=2026-06-01
func (h *Handler) SearchBus(c *gin.Context) {
	req := travelsvc.BusSearchRequest{
		Origin:        c.Query("origin"),
		Destination:   c.Query("destination"),
		DepartureDate: c.Query("departure_date"),
	}
	if req.Origin == "" || req.Destination == "" || req.DepartureDate == "" {
		response.BadRequest(c, "origin, destination, and departure_date are required", nil)
		return
	}
	results, err := h.svc.SearchBus(c.Request.Context(), req)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, "bus routes found", results)
}

// GET /travel/flight/search?origin=LOS&destination=ABV&departure_date=2026-06-01&cabin_class=economy
func (h *Handler) SearchFlights(c *gin.Context) {
	req := travelsvc.FlightSearchRequest{
		Origin:        c.Query("origin"),
		Destination:   c.Query("destination"),
		DepartureDate: c.Query("departure_date"),
		CabinClass:    c.DefaultQuery("cabin_class", "economy"),
		Adults:        1,
	}
	if req.Origin == "" || req.Destination == "" || req.DepartureDate == "" {
		response.BadRequest(c, "origin, destination, and departure_date are required", nil)
		return
	}
	results, err := h.svc.SearchFlights(c.Request.Context(), req)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, "flights found", results)
}

// POST /travel/bus/book
func (h *Handler) BookBus(c *gin.Context) {
	var req travelsvc.BusBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	result, err := h.svc.BookBus(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.Created(c, "bus ticket booked successfully", result)
}

// POST /travel/flight/book
func (h *Handler) BookFlight(c *gin.Context) {
	var req travelsvc.FlightBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "validation failed", err.Error())
		return
	}
	result, err := h.svc.BookFlight(c.Request.Context(), middleware.GetUserID(c), req)
	if err != nil {
		response.UnprocessableEntity(c, err.Error(), nil)
		return
	}
	response.Created(c, "flight booked successfully", result)
}

// GET /travel/bookings
func (h *Handler) GetMyBookings(c *gin.Context) {
	bookings, err := h.svc.GetMyBookings(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.InternalError(c, "could not retrieve bookings", nil)
		return
	}
	response.OK(c, "bookings retrieved", bookings)
}

// GET /travel/bookings/:id
func (h *Handler) GetBooking(c *gin.Context) {
	booking, err := h.svc.GetBooking(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.NotFound(c, "booking not found")
		return
	}
	response.OK(c, "booking retrieved", booking)
}
