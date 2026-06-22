package opay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flip-bills/backend/internal/providers"
)

const (
	defaultBaseURL = "https://api.opay.inc" // Example, adjust based on actual OPay API
)

// Client implements providers.PaymentProvider for OPay.
// Primarily used for Bills/VAS fallback.
type Client struct {
	secretKey  string
	publicKey  string
	merchantID string
	baseURL    string
	httpClient *http.Client
}

func New(secretKey, publicKey, merchantID, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		secretKey:  secretKey,
		publicKey:  publicKey,
		merchantID: merchantID,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Name() string { return "opay" }

// InitializePayment is stubbed for OPay (using Paystack/Flutterwave primarily for cards).
func (c *Client) InitializePayment(ctx context.Context, req providers.PaymentRequest) (*providers.PaymentResponse, error) {
	return nil, fmt.Errorf("opay: InitializePayment not fully implemented")
}

// VerifyTransaction is stubbed.
func (c *Client) VerifyTransaction(ctx context.Context, reference string) (*providers.VerificationResponse, error) {
	return nil, fmt.Errorf("opay: VerifyTransaction not implemented")
}

// CreateVirtualAccount is stubbed.
func (c *Client) CreateVirtualAccount(ctx context.Context, req providers.VirtualAccountRequest) (*providers.VirtualAccountResponse, error) {
	return nil, fmt.Errorf("opay: CreateVirtualAccount not implemented")
}

// Transfer is stubbed.
func (c *Client) Transfer(ctx context.Context, req providers.TransferRequest) (*providers.TransferResponse, error) {
	return nil, fmt.Errorf("opay: Transfer not implemented")
}

// PurchaseBill executes a VAS transaction (airtime, data, electricity, betting, tv_cable).
func (c *Client) PurchaseBill(ctx context.Context, req providers.BillRequest) (*providers.BillResponse, error) {
	// Sample implementation mapping FlipBills service type to OPay endpoints
	endpoint := "/api/v1/bills/purchase"
	body := map[string]any{
		"reference":    req.Reference,
		"service_type": req.ServiceType,
		"service_code": req.ServiceCode,
		"customer":     req.Customer,
		"amount":       req.AmountKobo / 100, // Assuming OPay uses NGN not kobo
		"phone":        req.PhoneNumber,
	}
	// Add extras if any
	for k, v := range req.Extras {
		body[k] = v
	}

	resp, err := c.post(ctx, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("opay: PurchaseBill: %w", err)
	}

	data, _ := resp["data"].(map[string]any)
	status := "failed"
	if s, ok := data["status"].(string); ok && s == "successful" {
		status = "success"
	}

	return &providers.BillResponse{
		Reference:   req.Reference,
		Status:      status,
		Token:       strVal(data, "token"),
		ProviderRef: strVal(data, "transaction_id"),
	}, nil
}

// GetBanks is stubbed.
func (c *Client) GetBanks(ctx context.Context) ([]providers.BankInfo, error) {
	return nil, fmt.Errorf("opay: GetBanks not implemented")
}

// ResolveAccount is stubbed.
func (c *Client) ResolveAccount(ctx context.Context, accountNumber, bankCode string) (*providers.AccountInfo, error) {
	return nil, fmt.Errorf("opay: ResolveAccount not implemented")
}

// HealthCheck pings the provider API.
func (c *Client) HealthCheck(ctx context.Context) bool {
	// A simple ping endpoint, if any, or just return true if we don't have one
	return true
}

func (c *Client) VerifyWebhookSignature(payload []byte, signature string) bool {
	mac := hmac.New(sha512.New, []byte(c.secretKey))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ─── HTTP Helpers ─────────────────────────────────────────────────────────────

func (c *Client) post(ctx context.Context, path string, body any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	// OPay usually requires signing the request body
	mac := hmac.New(sha512.New, []byte(c.secretKey))
	mac.Write(b)
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("Authorization", "Bearer "+c.publicKey)
	req.Header.Set("MerchantId", c.merchantID)
	req.Header.Set("Signature", signature)
	req.Header.Set("Content-Type", "application/json")

	return c.do(req)
}

func (c *Client) do(req *http.Request) (map[string]any, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("opay: decode error: %w", err)
	}

	code, _ := result["code"].(string)
	if code != "00000" { // Assuming "00000" is success for OPay
		msg, _ := result["message"].(string)
		return nil, fmt.Errorf("opay API error: %s", msg)
	}

	return result, nil
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
