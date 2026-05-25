package operators

// Operator is the interface every bus/flight partner adapter must implement.
// This is the Adapter Pattern — new operators (ABC Transport, Peace Mass Transit,
// Azman Air, etc.) are added by writing a new struct that satisfies this interface
// without touching any service or handler code.

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
)

// BusOperator abstracts a single inter-state bus company API.
type BusOperator interface {
	// Code returns the unique operator identifier (e.g. "GIGM").
	Code() string
	// Name returns the display name (e.g. "GIGM Transport").
	Name() string
	// Search queries live inventory for a given route and date.
	Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error)
	// Hold temporarily reserves a seat while the user completes payment.
	Hold(ctx context.Context, vehicleRef, seatNumber string) (holdRef string, err error)
	// Confirm converts a hold into a confirmed booking after wallet debit.
	Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (ticketCode string, err error)
	// Cancel releases a hold or voids a confirmed ticket.
	Cancel(ctx context.Context, ticketCode string) error
}

// FlightOperator abstracts a GDS connection (Amadeus / Travelport).
type FlightOperator interface {
	Code() string
	Name() string
	Search(ctx context.Context, req FlightSearchRequest) ([]models.FlightSearchResult, error)
	PriceOffer(ctx context.Context, gdsRef string) (*models.FlightSearchResult, error)
	Book(ctx context.Context, gdsRef string, passenger PassengerInfo) (ticketCode string, err error)
	Cancel(ctx context.Context, ticketCode string) error
}

// ── Shared request / info types ───────────────────────────────────────────────

type BusSearchRequest struct {
	Origin        string
	Destination   string
	DepartureDate string // "YYYY-MM-DD"
}

type FlightSearchRequest struct {
	Origin        string // IATA code
	Destination   string // IATA code
	DepartureDate string // "YYYY-MM-DD"
	CabinClass    string // "economy" | "business"
	Adults        int
}

type PassengerInfo struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	NIN      string `json:"nin"`
}
