package operators

// ABCOperator adapts the ABC Transport API to the BusOperator interface.
// ABC Transport is one of Nigeria's major long-distance bus operators.
// Partner API access: https://abctransport.com.ng/partners

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type ABCOperator struct {
	apiKey  string
	baseURL string
	log     *zap.Logger
}

func NewABCOperator(apiKey, baseURL string, log *zap.Logger) *ABCOperator {
	return &ABCOperator{apiKey: apiKey, baseURL: baseURL, log: log}
}

func (a *ABCOperator) Code() string { return "ABC" }
func (a *ABCOperator) Name() string { return "ABC Transport" }

func (a *ABCOperator) Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	// TODO: implement ABC Transport REST call.
	a.log.Info("ABC search called", zap.String("route", req.Origin+"→"+req.Destination))

	departure, _ := time.Parse("2006-01-02", req.DepartureDate)
	departure = departure.Add(8 * time.Hour) // 08:00 departure

	return []models.BusSearchResult{
		{
			OperatorCode:   "ABC",
			OperatorName:   "ABC Transport",
			Origin:         req.Origin,
			Destination:    req.Destination,
			DepartureTime:  departure,
			ArrivalTime:    departure.Add(8 * time.Hour),
			PriceKobo:      650000, // ₦6,500
			PriceNGN:       6500,
			SeatsAvailable: 7,
			VehicleRef:     "ABC-BUS-0017",
			VehicleClass:   "standard",
			Rating:         4.0,
			SeatLayout:     buildDefaultSeatLayout(14, 4),
		},
	}, nil
}

func (a *ABCOperator) Hold(ctx context.Context, vehicleRef, seatNumber string) (string, error) {
	return fmt.Sprintf("ABC-HOLD-%d", time.Now().UnixMilli()), nil
}

func (a *ABCOperator) Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	return fmt.Sprintf("ABC-TKT-%d", time.Now().UnixMilli()), nil
}

func (a *ABCOperator) Cancel(ctx context.Context, ticketCode string) error {
	return nil
}
