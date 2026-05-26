package operators

// ABCOperator adapts the ABC Transport API to the BusOperator interface.
// ABC Transport is one of Nigeria's major long-distance bus operators.
// Endpoint paths are implemented through the shared partner client and should be
// verified against the final ABC partner contract before production rollout.

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type ABCOperator struct {
	partner busPartnerClient
}

func NewABCOperator(apiKey, baseURL string, log *zap.Logger) *ABCOperator {
	return &ABCOperator{partner: newBusPartnerClient("ABC", "ABC Transport", apiKey, baseURL, log)}
}

func (a *ABCOperator) Code() string { return "ABC" }
func (a *ABCOperator) Name() string { return "ABC Transport" }

func (a *ABCOperator) Search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	return a.partner.search(ctx, req)
}

func (a *ABCOperator) Hold(ctx context.Context, vehicleRef, seatNumber string) (string, error) {
	return a.partner.hold(ctx, vehicleRef, seatNumber)
}

func (a *ABCOperator) Confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	return a.partner.confirm(ctx, holdRef, passenger)
}

func (a *ABCOperator) Cancel(ctx context.Context, ticketCode string) error {
	return a.partner.cancel(ctx, ticketCode)
}
