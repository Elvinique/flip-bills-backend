package operators

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
)

type TravuBusAdapter struct {
	apiKey  string
	baseURL string
	opCode  string
	opName  string
}

func NewTravuBusAdapter(apiKey, baseURL, opCode, opName string) *TravuBusAdapter {
	return &TravuBusAdapter{
		apiKey:  apiKey,
		baseURL: baseURL,
		opCode:  opCode,
		opName:  opName,
	}
}

func NewSandboxBusOperator(opCode, opName string) BusOperator {
	return NewTravuBusAdapter("mock_key", "https://mock.travu.africa", opCode, opName)
}

func (t *TravuBusAdapter) Code() string { return t.opCode }
func (t *TravuBusAdapter) Name() string { return t.opName }

func (t *TravuBusAdapter) Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	departureTime, _ := time.Parse("2006-01-02 15:04", req.DepartureDate+" 08:00")
	arrivalTime := departureTime.Add(6 * time.Hour)

	return []models.BusSearchResult{
		{
			OperatorCode:   t.opCode,
			OperatorName:   t.opName,
			Origin:         req.Origin,
			Destination:    req.Destination,
			VehicleRef:     fmt.Sprintf("VEH-%s-01", t.opCode),
			VehicleClass:   "Executive Premium",
			PriceKobo:      3500000, // ₦35,000
			PriceNGN:       35000.0,
			SeatsAvailable: 12,
			DepartureTime:  departureTime,
			ArrivalTime:    arrivalTime,
			Rating:         4.7,
			SeatLayout: []models.SeatRow{
				{
					Seats: []models.Seat{
						{Number: "1A", Available: true},
						{Number: "1B", Available: false},
					},
				},
				{
					Seats: []models.Seat{
						{Number: "2A", Available: true},
						{Number: "2B", Available: true},
					},
				},
			},
		},
	}, nil
}

func (t *TravuBusAdapter) Hold(ctx context.Context, vehicleRef string, seatNumber string) (string, error) {
	return fmt.Sprintf("HOLD-REF-%s-%d", t.opCode, time.Now().UnixMilli()%10000), nil
}

func (t *TravuBusAdapter) Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	return fmt.Sprintf("TKT-%s-%d", t.opCode, time.Now().UnixMilli()%100000), nil
}

func (t *TravuBusAdapter) Cancel(ctx context.Context, holdRef string) error {
	return nil
}
