package operators

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestAmadeusSearchPriceBookCancel(t *testing.T) {
	var (
		pricingCalled bool
		bookingCalled bool
		cancelCalled  bool
	)

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := http.StatusOK
		var payload interface{}
		switch r.URL.Path {
		case "/v1/security/oauth2/token":
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %s", r.Method)
			}
			payload = map[string]interface{}{
				"access_token": "token-123",
				"expires_in":   1800,
			}
		case "/v2/shopping/flight-offers":
			if r.Method != http.MethodGet {
				t.Fatalf("search method = %s", r.Method)
			}
			if got := r.URL.Query().Get("originLocationCode"); got != "LOS" {
				t.Fatalf("originLocationCode = %q", got)
			}
			if got := r.URL.Query().Get("destinationLocationCode"); got != "ABV" {
				t.Fatalf("destinationLocationCode = %q", got)
			}
			assertBearer(t, r)
			payload = amadeusOfferResponse("OFF-1")
		case "/v1/shopping/flight-offers/pricing":
			pricingCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("pricing method = %s", r.Method)
			}
			assertBearer(t, r)
			payload = map[string]interface{}{
				"data": map[string]interface{}{
					"flightOffers": []interface{}{amadeusOffer("OFF-1", "62000.00")},
				},
				"dictionaries": map[string]interface{}{
					"carriers": map[string]string{"P4": "Air Peace"},
				},
			}
		case "/v1/booking/flight-orders":
			bookingCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("booking method = %s", r.Method)
			}
			assertBearer(t, r)
			payload = map[string]interface{}{
				"data": map[string]interface{}{
					"id": "ORDER-1",
				},
			}
		case "/v1/booking/flight-orders/ORDER-1":
			cancelCalled = true
			if r.Method != http.MethodDelete {
				t.Fatalf("cancel method = %s", r.Method)
			}
			assertBearer(t, r)
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
		default:
			status = http.StatusNotFound
			payload = map[string]interface{}{"error": "not found"}
		}
		return jsonResponse(status, payload), nil
	})}

	op := NewAmadeusOperator("client-id", "client-secret", "https://amadeus.test", zap.NewNop())
	op.client = httpClient

	results, err := op.Search(context.Background(), FlightSearchRequest{
		Origin:        "LOS",
		Destination:   "ABV",
		DepartureDate: "2026-06-01",
		CabinClass:    "economy",
		Adults:        1,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].GDSRef != "OFF-1" {
		t.Fatalf("GDS ref = %q", results[0].GDSRef)
	}
	if results[0].PriceKobo != 5_500_000 {
		t.Fatalf("search price kobo = %d", results[0].PriceKobo)
	}

	priced, err := op.PriceOffer(context.Background(), "OFF-1")
	if err != nil {
		t.Fatalf("PriceOffer returned error: %v", err)
	}
	if priced.PriceKobo != 6_200_000 {
		t.Fatalf("priced kobo = %d", priced.PriceKobo)
	}

	orderID, err := op.Book(context.Background(), "OFF-1", PassengerInfo{
		FullName: "Ada Okonkwo",
		Phone:    "+2348012345678",
		Email:    "ada@example.com",
	})
	if err != nil {
		t.Fatalf("Book returned error: %v", err)
	}
	if orderID != "ORDER-1" {
		t.Fatalf("order ID = %q", orderID)
	}

	if err := op.Cancel(context.Background(), "ORDER-1"); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if !pricingCalled || !bookingCalled || !cancelCalled {
		t.Fatalf("expected pricing, booking, and cancel endpoints to be called")
	}
}

func assertBearer(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer token-123") {
		t.Fatalf("Authorization header = %q", got)
	}
}

func amadeusOfferResponse(id string) map[string]interface{} {
	return map[string]interface{}{
		"data": []interface{}{amadeusOffer(id, "55000.00")},
		"dictionaries": map[string]interface{}{
			"carriers": map[string]string{"P4": "Air Peace"},
		},
	}
}

func amadeusOffer(id string, price string) map[string]interface{} {
	return map[string]interface{}{
		"type":                  "flight-offer",
		"id":                    id,
		"numberOfBookableSeats": 4,
		"itineraries": []map[string]interface{}{
			{
				"duration": "PT1H15M",
				"segments": []map[string]interface{}{
					{
						"departure": map[string]string{
							"iataCode": "LOS",
							"at":       "2026-06-01T09:00:00",
						},
						"arrival": map[string]string{
							"iataCode": "ABV",
							"at":       "2026-06-01T10:15:00",
						},
						"carrierCode": "P4",
						"number":      "7201",
					},
				},
			},
		},
		"price": map[string]string{
			"currency":   "NGN",
			"total":      price,
			"grandTotal": price,
		},
		"travelerPricings": []map[string]interface{}{
			{
				"fareDetailsBySegment": []map[string]string{
					{"cabin": "ECONOMY"},
				},
			},
		},
	}
}
