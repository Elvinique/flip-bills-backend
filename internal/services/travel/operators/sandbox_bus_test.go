package operators

import (
	"context"
	"testing"
)

func TestSandboxBusOperatorSearchHoldConfirm(t *testing.T) {
	op := NewSandboxBusOperator("GIGM", "GIGM Transport Sandbox")

	results, err := op.Search(context.Background(), BusSearchRequest{
		Origin:        "Lagos",
		Destination:   "Abuja",
		DepartureDate: "2026-06-01",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 sandbox trips, got %d", len(results))
	}
	if results[0].OperatorCode != "GIGM" {
		t.Fatalf("operator code = %q", results[0].OperatorCode)
	}
	if results[0].VehicleRef == "" || results[0].PriceKobo <= 0 {
		t.Fatalf("invalid sandbox result: %+v", results[0])
	}

	holdRef, err := op.Hold(context.Background(), results[0].VehicleRef, "1A")
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}
	if holdRef == "" {
		t.Fatalf("hold ref is empty")
	}

	ticket, err := op.Confirm(context.Background(), holdRef, PassengerInfo{FullName: "Ada Okonkwo"})
	if err != nil {
		t.Fatalf("Confirm returned error: %v", err)
	}
	if ticket == "" {
		t.Fatalf("ticket code is empty")
	}
}
