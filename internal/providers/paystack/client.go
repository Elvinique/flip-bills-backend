package paystack

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
	defaultBaseURL = "https://api.paystack.co"
	healthEndpoint = "/bank"
)

// Client implements providers.PaymentProvider for Paystack.
// Paystack is preferred for:
//   - Card payments (highest success rate in Nigeria)
//   - Bank transfers (Paystack Transfer API)
//   - Account resolution (bank name enquiry)
//   - Virtual account provisioning (Dedicated Virtual Accounts)
type Client struct {
	secretKey  string
	baseURL    string
	httpClient *http.Client
}

func New(secretKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		secretKey: secretKey,
		baseURL:   baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Name() string { return "paystack" }

// ─── Payment Initialization ───────────────────────────────────────────────────

func (c *Client) InitializePayment(ctx context.Context, req providers.PaymentRequest) (*providers.PaymentResponse, error) {
	body := map[string]any{
		"reference": req.Reference,
		"amount":    req.AmountKobo,
		"email":     req.Email,
		"callback_url": req.CallbackURL,
		"metadata":  req.Metadata,
		"currency":  "NGN",
	}
	resp, err := c.post(ctx, "/transaction/initialize", body)
	if err != nil {
		return nil, fmt.Errorf("paystack: InitializePayment: %w", err)
	}
	data := resp["data"].(map[string]any)
	return &providers.PaymentResponse{
		AuthorizationURL: strVal(data, "authorization_url"),
		Reference:        strVal(data, "reference"),
		AccessCode:       strVal(data, "access_code"),
		ProviderName:     "paystack",
	}, nil
}

// ─── Verify Transaction ───────────────────────────────────────────────────────

func (c *Client) VerifyTransaction(ctx context.Context, reference string) (*providers.VerificationResponse, error) {
	resp, err := c.get(ctx, "/transaction/verify/"+reference)
	if err != nil {
		return nil, fmt.Errorf("paystack: VerifyTransaction: %w", err)
	}
	data := resp["data"].(map[string]any)
	amountF, _ := data["amount"].(float64)
	status := strVal(data, "status")
	var paidAt time.Time
	if s := strVal(data, "paid_at"); s != "" {
		paidAt, _ = time.Parse(time.RFC3339, s)
	}
	return &providers.VerificationResponse{
		Reference:   reference,
		AmountKobo:  int64(amountF),
		Status:      status,
		Channel:     strVal(data, "channel"),
		PaidAt:      paidAt,
		ProviderRef: strVal(data, "id"),
	}, nil
}

// ─── Virtual Account ──────────────────────────────────────────────────────────

func (c *Client) CreateVirtualAccount(ctx context.Context, req providers.VirtualAccountRequest) (*providers.VirtualAccountResponse, error) {
	// Step 1: create a Paystack customer
	custBody := map[string]any{
		"email":      req.Email,
		"first_name": req.FullName,
		"phone":      req.PhoneNumber,
	}
	custResp, err := c.post(ctx, "/customer", custBody)
	if err != nil {
		return nil, fmt.Errorf("paystack: CreateVirtualAccount (customer): %w", err)
	}
	custData := custResp["data"].(map[string]any)
	customerCode := strVal(custData, "customer_code")

	// Step 2: assign a dedicated virtual account
	dvaBody := map[string]any{
		"customer":     customerCode,
		"preferred_bank": "test-bank", // "wema-bank" or "titan-paystack" in production
	}
	dvaResp, err := c.post(ctx, "/dedicated_account", dvaBody)
	if err != nil {
		return nil, fmt.Errorf("paystack: CreateVirtualAccount (dva): %w", err)
	}
	dvaData := dvaResp["data"].(map[string]any)
	bankMap, _ := dvaData["bank"].(map[string]any)
	return &providers.VirtualAccountResponse{
		AccountNumber: strVal(dvaData, "account_number"),
		AccountName:   strVal(dvaData, "account_name"),
		BankName:      strVal(bankMap, "name"),
		ProviderRef:   strVal(dvaData, "id"),
	}, nil
}

// ─── Bank Transfer ────────────────────────────────────────────────────────────

func (c *Client) Transfer(ctx context.Context, req providers.TransferRequest) (*providers.TransferResponse, error) {
	// Step 1: Create a transfer recipient
	recipBody := map[string]any{
		"type":           "nuban",
		"name":           req.AccountName,
		"account_number": req.AccountNumber,
		"bank_code":      req.BankCode,
		"currency":       "NGN",
	}
	recipResp, err := c.post(ctx, "/transferrecipient", recipBody)
	if err != nil {
		return nil, fmt.Errorf("paystack: Transfer (recipient): %w", err)
	}
	recipData := recipResp["data"].(map[string]any)
	recipCode := strVal(recipData, "recipient_code")

	// Step 2: Initiate transfer
	xferBody := map[string]any{
		"source":    "balance",
		"amount":    req.AmountKobo,
		"recipient": recipCode,
		"reason":    req.Narration,
		"reference": req.Reference,
	}
	xferResp, err := c.post(ctx, "/transfer", xferBody)
	if err != nil {
		return nil, fmt.Errorf("paystack: Transfer (initiate): %w", err)
	}
	xferData := xferResp["data"].(map[string]any)
	return &providers.TransferResponse{
		Reference:   req.Reference,
		Status:      strVal(xferData, "status"),
		ProviderRef: strVal(xferData, "id"),
	}, nil
}

// ─── Bill Purchase (delegated to Flutterwave/OPay) ───────────────────────────

// Paystack does not natively handle VAS bills; return ErrNotSupported
// so the ProviderRouter falls back to OPay/Flutterwave.
func (c *Client) PurchaseBill(_ context.Context, _ providers.BillRequest) (*providers.BillResponse, error) {
	return nil, fmt.Errorf("paystack: PurchaseBill not supported — use OPay or Flutterwave")
}

// ─── Bank List ────────────────────────────────────────────────────────────────

func (c *Client) GetBanks(ctx context.Context) ([]providers.BankInfo, error) {
	resp, err := c.get(ctx, "/bank")
	if err != nil {
		return nil, fmt.Errorf("paystack: GetBanks: %w", err)
	}
	list, _ := resp["data"].([]any)
	banks := make([]providers.BankInfo, 0, len(list))
	for _, b := range list {
		bm := b.(map[string]any)
		banks = append(banks, providers.BankInfo{
			Name: strVal(bm, "name"),
			Code: strVal(bm, "code"),
		})
	}
	return banks, nil
}

// ─── Account Resolution ───────────────────────────────────────────────────────

func (c *Client) ResolveAccount(ctx context.Context, accountNumber, bankCode string) (*providers.AccountInfo, error) {
	url := fmt.Sprintf("/bank/resolve?account_number=%s&bank_code=%s", accountNumber, bankCode)
	resp, err := c.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("paystack: ResolveAccount: %w", err)
	}
	data := resp["data"].(map[string]any)
	return &providers.AccountInfo{
		AccountName:   strVal(data, "account_name"),
		AccountNumber: strVal(data, "account_number"),
	}, nil
}

// ─── Health Check ─────────────────────────────────────────────────────────────

func (c *Client) HealthCheck(ctx context.Context) bool {
	_, err := c.get(ctx, healthEndpoint)
	return err == nil
}

// ─── Webhook Signature Verification ──────────────────────────────────────────

// VerifyWebhookSignature validates an incoming Paystack webhook using
// HMAC-SHA512. Call this BEFORE processing any webhook event.
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
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) get(ctx context.Context, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
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
		return nil, fmt.Errorf("paystack: decode error: %w", err)
	}
	if status, _ := result["status"].(bool); !status {
		msg, _ := result["message"].(string)
		return nil, fmt.Errorf("paystack API error: %s", msg)
	}
	return result, nil
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
