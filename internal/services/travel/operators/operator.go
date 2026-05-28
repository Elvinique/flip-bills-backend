package operators

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
)

// ── Common Shared Structs ────────────────────────────────────────────────────

type BusSearchRequest struct {
	Origin        string
	Destination   string
	DepartureDate string // YYYY-MM-DD
}

type FlightSearchRequest struct {
	Origin        string
	Destination   string
	DepartureDate string
	CabinClass    string
	Adults        int
}

type PassengerInfo struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

// ── Interface Defitions ──────────────────────────────────────────────────────

type BusOperator interface {
	Code() string
	Name() string
	Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error)
	Hold(ctx context.Context, vehicleRef string, seatNumber string) (string, error)
	Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error)
	Cancel(ctx context.Context, holdRef string) error
}

type FlightOperator interface {
	Code() string
	Search(ctx context.Context, req FlightSearchRequest) ([]models.FlightSearchResult, error)
	PriceOffer(ctx context.Context, gdsRef string) (*models.FlightSearchResult, error)
	Book(ctx context.Context, gdsRef string, passenger PassengerInfo) (string, error)
}
