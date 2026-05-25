package utilities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
)

func TestConfirmFlutterwaveDeliveryFallsBackToExternalReference(t *testing.T) {
	provider := &fakeBillProvider{
		statusErrors: map[string]error{
			"FB-123": fmt.Errorf("not found"),
		},
	}
	service := &Service{bills: provider}

	status, err := service.confirmFlutterwaveDelivery(context.Background(), "FB-123", "BP123")
	if err != nil {
		t.Fatalf("confirmFlutterwaveDelivery returned error: %v", err)
	}
	if status.Data.CustomerReference != "BP123" {
		t.Fatalf("status customer reference = %q, want BP123", status.Data.CustomerReference)
	}

	if provider.statusCalls["FB-123"] != 1 {
		t.Fatalf("internal reference status calls = %d, want 1", provider.statusCalls["FB-123"])
	}
	if provider.statusCalls["BP123"] != 1 {
		t.Fatalf("external reference status calls = %d, want 1", provider.statusCalls["BP123"])
	}
}

func TestBuildBillReceiptMeta(t *testing.T) {
	meta := buildBillReceiptMeta(
		map[string]interface{}{
			"phone":   "08031234567",
			"network": "MTN",
		},
		&FlutterwaveBillResponse{
			Status: "success",
			Data: FlutterwaveBillData{
				Reference:     "BP123",
				RechargeToken: "1234-5678",
			},
		},
		&FlutterwaveBillStatusResponse{
			Status: "success",
			Data: FlutterwaveBillStatusData{
				CustomerReference: "BP123",
				Product:           "AIRTIME",
			},
		},
	)
	if len(meta) == 0 {
		t.Fatal("expected receipt metadata")
	}

	var got map[string]interface{}
	if err := json.Unmarshal(meta, &got); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if got["phone"] != "08031234567" {
		t.Fatalf("phone = %v, want 08031234567", got["phone"])
	}
	bill, ok := got["flutterwave_bill"].(map[string]interface{})
	if !ok {
		t.Fatalf("flutterwave_bill missing from metadata: %#v", got)
	}
	if bill["payment_response"] == nil || bill["status_response"] == nil || bill["confirmed_at"] == nil {
		t.Fatalf("incomplete flutterwave_bill metadata: %#v", bill)
	}
}

func TestExtractBillToken(t *testing.T) {
	fromPayment := extractBillToken(
		&FlutterwaveBillResponse{Data: FlutterwaveBillData{RechargeToken: "1234-5678"}},
		nil,
	)
	if fromPayment != "1234-5678" {
		t.Fatalf("payment token = %q, want 1234-5678", fromPayment)
	}

	fromStatus := extractBillToken(nil, &FlutterwaveBillStatusResponse{
		Data: FlutterwaveBillStatusData{
			Extra: json.RawMessage(`{"details":{"meter_token":"9999-0000"}}`),
		},
	})
	if fromStatus != "9999-0000" {
		t.Fatalf("status token = %q, want 9999-0000", fromStatus)
	}
}

func TestVASReference(t *testing.T) {
	ref, idempotent := vasReference("12345678-90ab-cdef", " checkout 001 ")
	if !idempotent {
		t.Fatal("expected idempotent reference")
	}
	if ref != "FB-12345678-checkout001" {
		t.Fatalf("ref = %q, want FB-12345678-checkout001", ref)
	}

	ref, idempotent = vasReference("12345678-90ab-cdef", "")
	if idempotent {
		t.Fatal("expected generated reference to be non-idempotent")
	}
	if !strings.HasPrefix(ref, "FB-") {
		t.Fatalf("generated ref = %q, want FB- prefix", ref)
	}
}

func TestGetCatalog(t *testing.T) {
	catalog := (&Service{}).GetCatalog(context.Background())
	if len(catalog.AirtimeNetworks) != 4 {
		t.Fatalf("airtime networks = %d, want 4", len(catalog.AirtimeNetworks))
	}
	if len(catalog.ElectricityDiscos) == 0 || len(catalog.BettingProviders) == 0 || len(catalog.DataPlans) == 0 {
		t.Fatalf("catalog is incomplete: %#v", catalog)
	}
}

func TestGetCatalogUsesLiveProvider(t *testing.T) {
	service := &Service{bills: &fakeCatalogProvider{
		catalog: &FlutterwaveCatalog{
			Categories: []FlutterwaveBillCategory{
				{CodeRaw: "AIRTIME", Name: "Airtime"},
				{CodeRaw: "MOBILEDATA", Name: "Mobile Data"},
				{CodeRaw: "UTILITYBILLS", Name: "Electricity"},
				{CodeRaw: "BETTING", Name: "Betting"},
			},
			Billers: map[string][]FlutterwaveBiller{
				"AIRTIME":      {{Name: "MTN", BillerCode: "BIL099"}},
				"MOBILEDATA":   {{Name: "MTN Data", BillerCode: "BIL100"}},
				"UTILITYBILLS": {{Name: "AEDC", BillerCode: "BIL200"}},
				"BETTING":      {{Name: "Bet9ja", BillerCode: "BIL300"}},
			},
			Items: map[string][]FlutterwaveBillItem{
				"BIL100": {{Name: "1GB", ItemCode: "MD1", Amount: json.RawMessage(`500`), Validity: "30 days"}},
			},
		},
	}}

	catalog := service.GetCatalog(context.Background())
	if catalog.Source != "flutterwave" {
		t.Fatalf("source = %q, want flutterwave", catalog.Source)
	}
	if catalog.AirtimeNetworks[0].BillerCode != "BIL099" {
		t.Fatalf("airtime biller code = %q, want BIL099", catalog.AirtimeNetworks[0].BillerCode)
	}
	if catalog.DataPlans[0].Amount != 50000 {
		t.Fatalf("data amount = %d, want 50000", catalog.DataPlans[0].Amount)
	}
}

func TestRequiresBettingRiskChallenge(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		stats  *postgres.CategorySpendStats
		want   bool
	}{
		{
			name:   "new user high value top-up",
			amount: 5_000_000,
			stats:  &postgres.CategorySpendStats{},
			want:   true,
		},
		{
			name:   "new user normal top-up",
			amount: 500_000,
			stats:  &postgres.CategorySpendStats{},
			want:   false,
		},
		{
			name:   "mature user triples average",
			amount: 3_000_000,
			stats:  &postgres.CategorySpendStats{Count: 4, Avg: 1_000_000, Max: 1_500_000},
			want:   true,
		},
		{
			name:   "mature user stays close to average",
			amount: 1_200_000,
			stats:  &postgres.CategorySpendStats{Count: 4, Avg: 1_000_000, Max: 1_500_000},
			want:   false,
		},
		{
			name:   "thin history needs high value and doubled max",
			amount: 4_000_000,
			stats:  &postgres.CategorySpendStats{Count: 2, Avg: 1_000_000, Max: 1_500_000},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiresBettingRiskChallenge(tt.amount, tt.stats); got != tt.want {
				t.Fatalf("requiresBettingRiskChallenge() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeBillProvider struct {
	statusErrors map[string]error
	statusCalls  map[string]int
}

type fakeCatalogProvider struct {
	fakeBillProvider
	catalog *FlutterwaveCatalog
	err     error
}

func (f *fakeCatalogProvider) FetchCatalog(context.Context) (*FlutterwaveCatalog, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.catalog, nil
}

func (f *fakeBillProvider) PurchaseBill(context.Context, FlutterwaveBillRequest) (*FlutterwaveBillResponse, error) {
	return nil, nil
}

func (f *fakeBillProvider) CheckBillStatus(_ context.Context, reference string) (*FlutterwaveBillStatusResponse, error) {
	if f.statusCalls == nil {
		f.statusCalls = make(map[string]int)
	}
	f.statusCalls[reference]++
	if err := f.statusErrors[reference]; err != nil {
		return nil, err
	}
	return &FlutterwaveBillStatusResponse{
		Status: "success",
		Data: FlutterwaveBillStatusData{
			CustomerReference: reference,
			Product:           string(models.CategoryAirtime),
		},
	}, nil
}
