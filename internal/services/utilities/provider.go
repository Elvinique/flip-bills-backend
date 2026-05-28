package utilities

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
)

// BillPurchaseParams defines a provider-agnostic contract for triggering a bill purchase.
type BillPurchaseParams struct {
	Category   models.ServiceCategory
	Reference  string
	CustomerID string
	Amount     int64
	Meta       map[string]interface{}
}

// UnifiedBillResponse normalizes provider-specific responses into a common super-app contract.
type UnifiedBillResponse struct {
	ExternalReference string
	RechargeToken     string
	Status            string
	RawMessage        []byte
}

// BillProvider defines the common behavior required from any integrated aggregator.
type BillProvider interface {
	PurchaseBill(ctx context.Context, params BillPurchaseParams) (*UnifiedBillResponse, error)
	CheckBillStatus(ctx context.Context, reference string) (*UnifiedBillResponse, error)
}
