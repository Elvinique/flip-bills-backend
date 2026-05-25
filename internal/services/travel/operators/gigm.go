package operators

// GIGMOperator adapts the GIGM REST API to the BusOperator interface.
// GIGM (God Is Good Motors) is Nigeria's largest formal inter-state bus operator.
// Replace the stub HTTP calls below with the real GIGM partner API credentials
// obtained from: https://partners.gigm.com

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type GIGMOperator struct {
	apiKey  string
	baseURL string
	client  *http.Client
	log     *zap.Logger
}

func NewGIGMOperator(apiKey, baseURL string, log *zap.Logger) *GIGMOperator {
	return &GIGMOperator{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
		log:     log,
	}
}

func (g *GIGMOperator) Code() string { return "GIGM" }
func (g *GIGMOperator) Name() string { return "GIGM Transport" }

func (g *GIGMOperator) Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	endpoint := fmt.Sprintf("%s/trips/search", g.baseURL)
	body, _ := json.Marshal(map[string]string{
		"from": req.Origin,
		"to":   req.Destination,
		"date": req.DepartureDate,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("GIGM search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GIGM returned status %d", resp.StatusCode)
	}

	// TODO: decode actual GIGM response schema and map to models.BusSearchResult.
	// The stub below returns a synthetic result for local development / testing.
	g.log.Info("GIGM search called", zap.String("route", req.Origin+"→"+req.Destination))

	departure, _ := time.Parse("2006-01-02", req.DepartureDate)
	departure = departure.Add(7 * time.Hour) // 07:00 departure

	return []models.BusSearchResult{
		{
			OperatorCode:   "GIGM",
			OperatorName:   "GIGM Transport",
			Origin:         req.Origin,
			Destination:    req.Destination,
			DepartureTime:  departure,
			ArrivalTime:    departure.Add(7 * time.Hour),
			PriceKobo:      750000, // ₦7,500
			PriceNGN:       7500,
			SeatsAvailable: 12,
			VehicleRef:     "GIGM-BUS-0042",
			VehicleClass:   "executive",
			Rating:         4.3,
			SeatLayout:     buildDefaultSeatLayout(14, 4),
		},
	}, nil
}

func (g *GIGMOperator) Hold(ctx context.Context, vehicleRef, seatNumber string) (string, error) {
	// TODO: call GIGM seat-hold endpoint.
	g.log.Info("GIGM hold seat", zap.String("vehicle", vehicleRef), zap.String("seat", seatNumber))
	return fmt.Sprintf("GIGM-HOLD-%d", time.Now().UnixMilli()), nil
}

func (g *GIGMOperator) Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	// TODO: call GIGM booking confirmation endpoint.
	g.log.Info("GIGM confirm booking", zap.String("hold_ref", holdRef))
	return fmt.Sprintf("GIGM-TKT-%d", time.Now().UnixMilli()), nil
}

func (g *GIGMOperator) Cancel(ctx context.Context, ticketCode string) error {
	g.log.Info("GIGM cancel", zap.String("ticket", ticketCode))
	return nil
}
