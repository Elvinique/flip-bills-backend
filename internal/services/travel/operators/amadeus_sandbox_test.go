package operators

import (
	"context"
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestAmadeusSandboxContract(t *testing.T) {
	if os.Getenv("AMADEUS_VALIDATE_SANDBOX") != "true" {
		t.Skip("set AMADEUS_VALIDATE_SANDBOX=true with sandbox credentials to run")
	}

	clientID := os.Getenv("AMADEUS_CLIENT_ID")
	secret := os.Getenv("AMADEUS_CLIENT_SECRET")
	baseURL := os.Getenv("AMADEUS_BASE_URL")
	if baseURL == "" {
		baseURL = "https://test.api.amadeus.com"
	}
	if clientID == "" || secret == "" {
		t.Fatalf("AMADEUS_CLIENT_ID and AMADEUS_CLIENT_SECRET are required")
	}

	origin := envDefault("AMADEUS_SANDBOX_ORIGIN", "LOS")
	destination := envDefault("AMADEUS_SANDBOX_DESTINATION", "ABV")
	departureDate := os.Getenv("AMADEUS_SANDBOX_DEPARTURE_DATE")
	if departureDate == "" {
		t.Fatalf("AMADEUS_SANDBOX_DEPARTURE_DATE is required; use an available Amadeus test-data date")
	}

	op := NewAmadeusOperator(clientID, secret, baseURL, zap.NewNop())
	results, err := op.Search(context.Background(), FlightSearchRequest{
		Origin:        origin,
		Destination:   destination,
		DepartureDate: departureDate,
		CabinClass:    "economy",
		Adults:        1,
	})
	if err != nil {
		t.Fatalf("Amadeus search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("Amadeus search returned no offers")
	}
	if results[0].GDSRef == "" {
		t.Fatalf("Amadeus offer missing gds_ref")
	}
	if results[0].PriceKobo <= 0 {
		t.Fatalf("Amadeus offer has invalid price_kobo: %d", results[0].PriceKobo)
	}

	priced, err := op.PriceOffer(context.Background(), results[0].GDSRef)
	if err != nil {
		t.Fatalf("Amadeus pricing failed: %v", err)
	}
	if priced.PriceKobo <= 0 {
		t.Fatalf("Amadeus priced offer has invalid price_kobo: %d", priced.PriceKobo)
	}

	if os.Getenv("AMADEUS_VALIDATE_BOOKING") != "true" {
		t.Log("set AMADEUS_VALIDATE_BOOKING=true to validate flight order creation")
		return
	}

	orderID, err := op.Book(context.Background(), results[0].GDSRef, PassengerInfo{
		FullName: "Flip Bills Sandbox",
		Phone:    "+2348012345678",
		Email:    "sandbox@flipbills.local",
	})
	if err != nil {
		t.Fatalf("Amadeus booking failed: %v", err)
	}
	if orderID == "" {
		t.Fatalf("Amadeus booking returned empty order ID")
	}
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
