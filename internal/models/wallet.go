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
	ID           uuid.UUID      `db:"id"            json:"id"`
	UserID       uuid.UUID      `db:"user_id"       json:"user_id"`
	Balance      int64          `db:"balance"       json:"balance"`  // stored in kobo (1 NGN = 100 kobo)
	LedgerBalance int64         `db:"ledger_balance" json:"ledger_balance"` // pending/in-flight
	Currency     WalletCurrency `db:"currency"      json:"currency"`
	DailySpend   int64          `db:"daily_spend"   json:"daily_spend"`  // resets at midnight
	DailyLimit   int64          `db:"daily_limit"   json:"daily_limit"`  // set by KYC tier
	CreatedAt    time.Time      `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"    json:"updated_at"`
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
	TxTypeDebit       TransactionType = "debit"
	TxTypeCredit      TransactionType = "credit"
	TxTypeReversal    TransactionType = "reversal"
)

// ServiceCategory maps to the PRD service verticals.
type ServiceCategory string

const (
	CategoryAirtime    ServiceCategory = "airtime"
	CategoryData       ServiceCategory = "data"
	CategoryElectricity ServiceCategory = "electricity"
	CategoryCableTV    ServiceCategory = "cable_tv"
	CategoryBetting    ServiceCategory = "betting"
	CategoryBusTravel  ServiceCategory = "bus_travel"
	CategoryFlight     ServiceCategory = "flight"
	CategoryWalletFund ServiceCategory = "wallet_funding"
	CategoryTransfer   ServiceCategory = "transfer"
)

// Transaction is the immutable audit ledger — never updated, only inserted.
type Transaction struct {
	ID              uuid.UUID         `db:"id"               json:"id"`
	UserID          uuid.UUID         `db:"user_id"          json:"user_id"`
	WalletID        uuid.UUID         `db:"wallet_id"        json:"wallet_id"`
	Reference       string            `db:"reference"        json:"reference"`        // internal idempotency key
	ExternalRef     string            `db:"external_ref"     json:"external_ref"`     // partner/aggregator ref
	Type            TransactionType   `db:"type"             json:"type"`
	Category        ServiceCategory   `db:"category"         json:"category"`
	Amount          int64             `db:"amount"           json:"amount"`            // in kobo
	Fee             int64             `db:"fee"              json:"fee"`               // in kobo
	BalanceBefore   int64             `db:"balance_before"   json:"balance_before"`
	BalanceAfter    int64             `db:"balance_after"    json:"balance_after"`
	Status          TransactionStatus `db:"status"           json:"status"`
	Provider        string            `db:"provider"         json:"provider"`          // flutterwave|monnify|interswitch
	Narration       string            `db:"narration"        json:"narration"`
	Meta            []byte            `db:"meta"             json:"meta,omitempty"`    // JSONB for biller-specific data
	ReversedTxID    *uuid.UUID        `db:"reversed_tx_id"   json:"reversed_tx_id,omitempty"`
	CreatedAt       time.Time         `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time         `db:"updated_at"       json:"updated_at"`
}
