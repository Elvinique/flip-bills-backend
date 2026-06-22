package providers

import (
	"context"
	"time"
)

// ─── Payment Request/Response Types ──────────────────────────────────────────

type PaymentRequest struct {
	Reference   string
	AmountKobo  int64
	Email       string
	PhoneNumber string
	FullName    string
	CallbackURL string
	Currency    string // defaults to NGN
	Metadata    map[string]any
}

type PaymentResponse struct {
	AuthorizationURL string
	Reference        string
	AccessCode       string
	ProviderName     string
}

type VerificationResponse struct {
	Reference   string
	AmountKobo  int64
	Status      string // "success" | "failed" | "pending"
	Channel     string
	PaidAt      time.Time
	ProviderRef string
}

type VirtualAccountRequest struct {
	UserID      string
	FullName    string
	Email       string
	PhoneNumber string
	BVN         string
}

type VirtualAccountResponse struct {
	AccountNumber string
	AccountName   string
	BankName      string
	ProviderRef   string
}

type TransferRequest struct {
	Reference     string
	AccountNumber string
	BankCode      string
	AccountName   string
	AmountKobo    int64
	Narration     string
}

type TransferResponse struct {
	Reference   string
	Status      string
	ProviderRef string
}

type BillRequest struct {
	Reference   string
	ServiceType string // airtime | data | electricity | betting | tv_cable
	ServiceCode string
	Customer    string
	AmountKobo  int64
	PhoneNumber string
	Extras      map[string]any
}

type BillResponse struct {
	Reference   string
	Status      string
	Token       string // electricity token
	ProviderRef string
}

type BankInfo struct {
	Name string
	Code string
}

type AccountInfo struct {
	AccountName   string
	AccountNumber string
}

// ─── PaymentProvider Interface ────────────────────────────────────────────────

// PaymentProvider is the universal abstraction for all Nigerian payment
// aggregators. Implement this interface for each provider (Paystack,
// Flutterwave, Monnify, OPay) to allow the ProviderRouter to route
// requests transparently with automatic failover.
type PaymentProvider interface {
	// Name returns a short identifier e.g. "paystack", "flutterwave"
	Name() string

	// InitializePayment creates a payment session and returns the checkout URL.
	InitializePayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)

	// VerifyTransaction confirms a payment by reference. ALWAYS call this
	// on the server via webhook — never trust frontend callbacks.
	VerifyTransaction(ctx context.Context, reference string) (*VerificationResponse, error)

	// CreateVirtualAccount provisions a dedicated bank account for a user.
	// Credits auto-route to the user's ledger wallet when funds arrive.
	CreateVirtualAccount(ctx context.Context, req VirtualAccountRequest) (*VirtualAccountResponse, error)

	// Transfer debits a provider balance and sends funds to a bank account.
	Transfer(ctx context.Context, req TransferRequest) (*TransferResponse, error)

	// PurchaseBill executes a VAS transaction (airtime, data, electricity, TV, betting).
	PurchaseBill(ctx context.Context, req BillRequest) (*BillResponse, error)

	// GetBanks returns the list of supported banks.
	GetBanks(ctx context.Context) ([]BankInfo, error)

	// ResolveAccount looks up the account name for a given account number + bank code.
	ResolveAccount(ctx context.Context, accountNumber, bankCode string) (*AccountInfo, error)

	// HealthCheck pings the provider API. The circuit breaker uses this
	// to determine if a provider is available before routing to it.
	HealthCheck(ctx context.Context) bool
}
