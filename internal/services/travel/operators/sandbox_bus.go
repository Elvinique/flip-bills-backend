package operators

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/flip-bills/backend/internal/models"
)

type SandboxBusOperator struct {
	code  string
	name  string
	mu    sync.Mutex
	holds map[string]sandboxHold
}

type sandboxHold struct {
	VehicleRef string
	SeatNumber string
	ExpiresAt  time.Time
}

func NewSandboxBusOperator(code, name string) *SandboxBusOperator {
	return &SandboxBusOperator{
		code:  strings.ToUpper(code),
		name:  name,
		holds: make(map[string]sandboxHold),
	}
}

func (s *SandboxBusOperator) Code() string { return s.code }
func (s *SandboxBusOperator) Name() string { return s.name }

func (s *SandboxBusOperator) Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	departure, err := time.Parse("2006-01-02", req.DepartureDate)
	if err != nil {
		return nil, fmt.Errorf("invalid departure date: %w", err)
	}

	basePrice := int64(750000)
	vehicleClass := "executive"
	rating := 4.3
	if s.code == "ABC" {
		basePrice = 650000
		vehicleClass = "standard"
		rating = 4.0
	}

	morning := departure.Add(7 * time.Hour)
	afternoon := departure.Add(13 * time.Hour)

	return []models.BusSearchResult{
		s.result(req, morning, basePrice, vehicleClass, rating, "001"),
		s.result(req, afternoon, basePrice+100000, "vip", rating+0.1, "002"),
	}, nil
}

func (s *SandboxBusOperator) Hold(ctx context.Context, vehicleRef, seatNumber string) (string, error) {
	if strings.TrimSpace(vehicleRef) == "" || strings.TrimSpace(seatNumber) == "" {
		return "", fmt.Errorf("vehicle_ref and seat_number are required")
	}

	holdRef := fmt.Sprintf("%s-HOLD-%d", s.code, time.Now().UnixNano())
	s.mu.Lock()
	s.holds[holdRef] = sandboxHold{
		VehicleRef: vehicleRef,
		SeatNumber: seatNumber,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}
	s.mu.Unlock()
	return holdRef, nil
}

func (s *SandboxBusOperator) Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	s.mu.Lock()
	hold, ok := s.holds[holdRef]
	if ok {
		delete(s.holds, holdRef)
	}
	s.mu.Unlock()

	if !ok || time.Now().After(hold.ExpiresAt) {
		return "", fmt.Errorf("sandbox hold not found or expired")
	}
	return fmt.Sprintf("%s-TKT-%d", s.code, time.Now().UnixNano()), nil
}

func (s *SandboxBusOperator) Cancel(ctx context.Context, ticketCode string) error {
	s.mu.Lock()
	delete(s.holds, ticketCode)
	s.mu.Unlock()
	return nil
}

func (s *SandboxBusOperator) result(
	req BusSearchRequest,
	departure time.Time,
	priceKobo int64,
	vehicleClass string,
	rating float64,
	suffix string,
) models.BusSearchResult {
	return models.BusSearchResult{
		OperatorCode:   s.code,
		OperatorName:   s.name,
		Origin:         req.Origin,
		Destination:    req.Destination,
		DepartureTime:  departure,
		ArrivalTime:    departure.Add(7 * time.Hour),
		PriceKobo:      priceKobo,
		PriceNGN:       float64(priceKobo) / 100,
		SeatsAvailable: 18,
		VehicleRef:     fmt.Sprintf("%s-SBX-%s", s.code, suffix),
		VehicleClass:   vehicleClass,
		SeatLayout:     buildDefaultSeatLayout(14, 4),
		Rating:         rating,
	}
}
