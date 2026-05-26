package operators

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"go.uber.org/zap"
)

func TestBusPartnerClientSearchHoldConfirmCancel(t *testing.T) {
	var cancelCalled bool
	client := newBusPartnerClient("TEST", "Test Transport", "test-key", BusPartnerConfig{BaseURL: "https://partner.test"}, zap.NewNop())
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization header = %q", got)
		}
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Fatalf("X-API-Key header = %q", got)
		}

		status := http.StatusOK
		var payload interface{}
		switch r.URL.Path {
		case "/trips/search":
			payload = map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"id":              "BUS-42",
						"origin":          "Lagos",
						"destination":     "Abuja",
						"departure_time":  "2026-06-01T07:00:00Z",
						"arrival_time":    "2026-06-01T15:00:00Z",
						"price":           12500,
						"seats_available": 2,
						"vehicle_class":   "executive",
						"rating":          4.5,
						"seats": []map[string]interface{}{
							{"number": "1A", "available": true, "class": "executive"},
							{"number": "1B", "available": false, "class": "executive"},
						},
					},
				},
			}
		case "/seats/hold":
			payload = map[string]interface{}{
				"data": map[string]interface{}{"hold_ref": "HOLD-42"},
			}
		case "/bookings/confirm":
			payload = map[string]interface{}{
				"data": map[string]interface{}{"ticket_code": "TICKET-42"},
			}
		case "/bookings/cancel":
			cancelCalled = true
			payload = map[string]interface{}{"ok": true}
		default:
			status = http.StatusNotFound
			payload = map[string]interface{}{"error": "not found"}
		}

		return jsonResponse(status, payload), nil
	})}

	results, err := client.search(context.Background(), BusSearchRequest{
		Origin:        "Lagos",
		Destination:   "Abuja",
		DepartureDate: "2026-06-01",
	})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].VehicleRef != "BUS-42" {
		t.Fatalf("vehicle ref = %q", results[0].VehicleRef)
	}
	if results[0].PriceKobo != 1_250_000 {
		t.Fatalf("price kobo = %d", results[0].PriceKobo)
	}
	if results[0].SeatLayout[0].Seats[1].Available {
		t.Fatalf("expected seat 1B to be unavailable")
	}

	holdRef, err := client.hold(context.Background(), "BUS-42", "1A")
	if err != nil {
		t.Fatalf("hold returned error: %v", err)
	}
	if holdRef != "HOLD-42" {
		t.Fatalf("hold ref = %q", holdRef)
	}

	ticket, err := client.confirm(context.Background(), holdRef, PassengerInfo{FullName: "Ada Okonkwo"})
	if err != nil {
		t.Fatalf("confirm returned error: %v", err)
	}
	if ticket != "TICKET-42" {
		t.Fatalf("ticket = %q", ticket)
	}

	if err := client.cancel(context.Background(), ticket); err != nil {
		t.Fatalf("cancel returned error: %v", err)
	}
	if !cancelCalled {
		t.Fatalf("cancel endpoint was not called")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, payload interface{}) *http.Response {
	var body bytes.Buffer
	_ = json.NewEncoder(&body).Encode(payload)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(&body),
	}
}
