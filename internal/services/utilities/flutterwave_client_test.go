package utilities

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/flip-bills/backend/internal/models"
)

func TestFlutterwaveClientPurchaseBill(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotPayload map[string]interface{}

	client := NewFlutterwaveClient("FLWSECK_TEST", "https://api.flutterwave.test/v3")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		return jsonResponse(http.StatusOK, `{
            "status": "success",
            "message": "Bill payment successful",
            "data": {
                "reference": "BP123",
                "tx_ref": "CF-FLYAPI-123"
            }
		}`), nil
	})}

	resp, err := client.PurchaseBill(context.Background(), BillPurchaseParams{
		Category:   models.CategoryAirtime,
		Reference:  "FB-123",
		CustomerID: "08031234567",
		Amount:     50000,
		Meta: map[string]interface{}{
			"network": "MTN",
		},
	})
	if err != nil {
		t.Fatalf("PurchaseBill returned error: %v", err)
	}

	if gotPath != "/v3/bills" {
		t.Fatalf("path = %q, want /v3/bills", gotPath)
	}
	if gotAuth != "Bearer FLWSECK_TEST" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotPayload["country"] != "NG" || gotPayload["type"] != "AIRTIME" {
		t.Fatalf("unexpected payload: %#v", gotPayload)
	}
	if gotPayload["amount"] != float64(500) {
		t.Fatalf("amount = %#v, want 500", gotPayload["amount"])
	}
	if gotPayload["customer_id"] != "08031234567" || gotPayload["reference"] != "FB-123" {
		t.Fatalf("unexpected customer/reference payload: %#v", gotPayload)
	}
	if resp.ExternalReference != "BP123" {
		t.Fatalf("response reference = %q, want BP123", resp.ExternalReference)
	}
}

func TestFlutterwaveClientCheckBillStatus(t *testing.T) {
	var gotPath string
	client := NewFlutterwaveClient("FLWSECK_TEST", "https://api.flutterwave.test/v3")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return jsonResponse(http.StatusOK, `{
            "status": "success",
            "message": "Bill status fetch successful",
            "data": {
                "flw_ref": "BP123",
                "tx_ref": "CF-FLYAPI-123",
                "product": "AIRTIME"
            }
		}`), nil
	})}

	resp, err := client.CheckBillStatus(context.Background(), "BP123")
	if err != nil {
		t.Fatalf("CheckBillStatus returned error: %v", err)
	}
	if gotPath != "/v3/bills/BP123" {
		t.Fatalf("path = %q, want /v3/bills/BP123", gotPath)
	}
	if resp.ExternalReference != "BP123" {
		t.Fatalf("flw_ref = %q, want BP123", resp.ExternalReference)
	}
	if resp.Status != "success" {
		t.Fatalf("status = %q, want success", resp.Status)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}
