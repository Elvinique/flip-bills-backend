package operators

import (
	"context"
	"os"
	"testing"

	"github.com/flip-bills/backend/internal/models"
)

func TestGIGMSandboxContract(t *testing.T) {
	if os.Getenv("GIGM_VALIDATE_SANDBOX") != "true" {
		t.Skip("set GIGM_VALIDATE_SANDBOX=true with sandbox credentials to run")
	}

	op := NewGIGMOperatorWithConfig(
		os.Getenv("GIGM_API_KEY"),
		sandboxBusConfig("GIGM"),
		nil,
	)
	validateBusOperatorContract(t, op, sandboxBusSearch("GIGM"))
}

func TestABCSandboxContract(t *testing.T) {
	if os.Getenv("ABC_VALIDATE_SANDBOX") != "true" {
		t.Skip("set ABC_VALIDATE_SANDBOX=true with sandbox credentials to run")
	}

	op := NewABCOperatorWithConfig(
		os.Getenv("ABC_API_KEY"),
		sandboxBusConfig("ABC"),
		nil,
	)
	validateBusOperatorContract(t, op, sandboxBusSearch("ABC"))
}

func sandboxBusConfig(prefix string) BusPartnerConfig {
	return BusPartnerConfig{
		BaseURL:             os.Getenv(prefix + "_BASE_URL"),
		SearchPath:          os.Getenv(prefix + "_SEARCH_PATH"),
		HoldPath:            os.Getenv(prefix + "_HOLD_PATH"),
		ConfirmPath:         os.Getenv(prefix + "_CONFIRM_PATH"),
		CancelPath:          os.Getenv(prefix + "_CANCEL_PATH"),
		AuthHeader:          os.Getenv(prefix + "_AUTH_HEADER"),
		AuthScheme:          os.Getenv(prefix + "_AUTH_SCHEME"),
		SecondaryAuthHeader: os.Getenv(prefix + "_SECONDARY_AUTH_HEADER"),
	}
}

func sandboxBusSearch(prefix string) BusSearchRequest {
	return BusSearchRequest{
		Origin:        os.Getenv(prefix + "_SANDBOX_ORIGIN"),
		Destination:   os.Getenv(prefix + "_SANDBOX_DESTINATION"),
		DepartureDate: os.Getenv(prefix + "_SANDBOX_DEPARTURE_DATE"),
	}
}

func validateBusOperatorContract(t *testing.T, op BusOperator, req BusSearchRequest) {
	t.Helper()
	if req.Origin == "" || req.Destination == "" || req.DepartureDate == "" {
		t.Fatalf("sandbox origin, destination, and departure date are required")
	}

	results, err := op.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("%s search failed: %v", op.Code(), err)
	}
	if len(results) == 0 {
		t.Fatalf("%s search returned no trips", op.Code())
	}

	selected := results[0]
	if selected.VehicleRef == "" {
		t.Fatalf("%s search result missing vehicle_ref", op.Code())
	}
	if selected.PriceKobo <= 0 {
		t.Fatalf("%s search result has invalid price_kobo: %d", op.Code(), selected.PriceKobo)
	}

	seatNumber := firstAvailableSeat(selected.SeatLayout)
	if seatNumber == "" {
		t.Skipf("%s search returned no available seat to validate hold/confirm", op.Code())
	}

	holdRef, err := op.Hold(context.Background(), selected.VehicleRef, seatNumber)
	if err != nil {
		t.Fatalf("%s hold failed: %v", op.Code(), err)
	}
	if holdRef == "" {
		t.Fatalf("%s hold returned empty hold reference", op.Code())
	}

	ticketCode, err := op.Confirm(context.Background(), holdRef, PassengerInfo{
		FullName: "Flip Bills Sandbox",
		Phone:    "+2348012345678",
		Email:    "sandbox@flipbills.local",
	})
	if err != nil {
		t.Fatalf("%s confirm failed: %v", op.Code(), err)
	}
	if ticketCode == "" {
		t.Fatalf("%s confirm returned empty ticket code", op.Code())
	}

	if err := op.Cancel(context.Background(), ticketCode); err != nil {
		t.Fatalf("%s cancel failed: %v", op.Code(), err)
	}
}

func firstAvailableSeat(layout []models.SeatRow) string {
	for _, row := range layout {
		for _, seat := range row.Seats {
			if seat.Available {
				return seat.Number
			}
		}
	}
	return ""
}
