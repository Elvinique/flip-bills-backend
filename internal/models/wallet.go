package models

import (
	"time"

	"github.com/google/uuid"
)

// WalletCurrency — for future multi-currency expansion.
type WalletCurrency string

const (
	NGN WalletCurrency = "NGN"
)

// Wallet is the user's primary stored-value account.
// Stored in PostgreSQL for full ACID compliance.
type Wallet struct {
	ID            uuid.UUID      `db:"id"            json:"id"`
	UserID        uuid.UUID      `db:"user_id"       json:"user_id"`
	Balance       int64          `db:"balance"       json:"balance"`         // stored in kobo (1 NGN = 100 kobo)
	LedgerBalance int64          `db:"ledger_balance" json:"ledger_balance"` // pending/in-flight
	Currency      WalletCurrency `db:"currency"      json:"currency"`
	DailySpend    int64          `db:"daily_spend"   json:"daily_spend"` // resets at midnight
	DailyLimit    int64          `db:"daily_limit"   json:"daily_limit"` // set by KYC tier
	CreatedAt     time.Time      `db:"created_at"    json:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"    json:"updated_at"`
}

// TransactionStatus tracks the lifecycle of every money movement.
type TransactionStatus string

const (
	TxPending    TransactionStatus = "pending"
	TxProcessing TransactionStatus = "processing"
	TxSuccess    TransactionStatus = "success"
	TxFailed     TransactionStatus = "failed"
	TxReversed   TransactionStatus = "reversed"
)

// TransactionType classifies the nature of the debit/credit.
type TransactionType string

const (
	TxTypeDebit    TransactionType = "debit"
	TxTypeCredit   TransactionType = "credit"
	TxTypeReversal TransactionType = "reversal"
)

// ServiceCategory maps to the PRD service verticals.
type ServiceCategory string

const (
	CategoryAirtime     ServiceCategory = "airtime"
	CategoryData        ServiceCategory = "data"
	CategoryElectricity ServiceCategory = "electricity"
	CategoryCableTV     ServiceCategory = "cable_tv"
	CategoryBetting     ServiceCategory = "betting"
	CategoryBusTravel   ServiceCategory = "bus_travel"
	CategoryFlight      ServiceCategory = "flight"
	CategoryWalletFund  ServiceCategory = "wallet_funding"
	CategoryTransfer    ServiceCategory = "transfer"
)

// Transaction is the immutable audit ledger — never updated, only inserted.
type Transaction struct {
	ID             uuid.UUID         `json:"id"`
	UserID         string            `json:"user_id"`
	WalletID       uuid.UUID         `json:"wallet_id"`
	Reference      string            `json:"reference"`
	ExternalRef    string            `json:"external_ref,omitempty"`
	Type           TransactionType   `json:"type"`
	Category       ServiceCategory   `json:"category"`
	Amount         int64             `json:"amount"`
	CommissionKobo int64             `json:"commission_kobo"` // Platform cut
	Fee            int64             `json:"fee"`             // Required by wallet_repo
	BalanceBefore  int64             `json:"balance_before"`
	BalanceAfter   int64             `json:"balance_after"`
	Status         TransactionStatus `json:"status"`
	Provider       string            `json:"provider"`
	Narration      string            `json:"narration"`
	Meta           []byte            `json:"meta,omitempty"`
	ReversedTxID   *uuid.UUID        `json:"reversed_tx_id,omitempty"` // Required by wallet_repo
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}
