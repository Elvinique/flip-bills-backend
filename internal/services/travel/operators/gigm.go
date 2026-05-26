package operators

// GIGMOperator adapts the GIGM REST API to the BusOperator interface.
// GIGM (God Is Good Motors) is Nigeria's largest formal inter-state bus operator.
// Endpoint paths are implemented through the shared partner client and should be
// verified against the final GIGM partner contract before production rollout.

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type GIGMOperator struct {
	partner busPartnerClient
}

func NewGIGMOperator(apiKey, baseURL string, log *zap.Logger) *GIGMOperator {
	return &GIGMOperator{partner: newBusPartnerClient("GIGM", "GIGM Transport", apiKey, baseURL, log)}
}

func (g *GIGMOperator) Code() string { return "GIGM" }
func (g *GIGMOperator) Name() string { return "GIGM Transport" }

func (g *GIGMOperator) Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	return g.partner.search(ctx, req)
}

func (g *GIGMOperator) Hold(ctx context.Context, vehicleRef, seatNumber string) (string, error) {
	return g.partner.hold(ctx, vehicleRef, seatNumber)
}

func (g *GIGMOperator) Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	return g.partner.confirm(ctx, holdRef, passenger)
}

func (g *GIGMOperator) Cancel(ctx context.Context, ticketCode string) error {
	return g.partner.cancel(ctx, ticketCode)
}
