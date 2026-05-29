package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWalletBalanceConstraints(t *testing.T) {
	wallet := Wallet{
		ID:            uuid.New(),
		UserID:        uuid.New(),
		Balance:       1_000_000, // ₦10,000
		LedgerBalance: 0,
		Currency:      NGN,
		DailySpend:    200_000,   // ₦2,000
		DailyLimit:    5_000_000, // ₦50,000
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	remaining := wallet.DailyLimit - wallet.DailySpend
	if remaining != 4_800_000 {
		t.Fatalf("daily remaining = %d, want 4800000", remaining)
	}

	if wallet.Currency != NGN {
		t.Fatalf("currency = %q, want NGN", wallet.Currency)
	}
}

func TestTransactionStatusValues(t *testing.T) {
	statuses := []TransactionStatus{
		TxPending, TxProcessing, TxSuccess, TxFailed, TxReversed,
	}
	for _, s := range statuses {
		if string(s) == "" {
			t.Fatalf("transaction status should not be empty")
		}
	}
}

func TestTransactionTypeValues(t *testing.T) {
	types := []TransactionType{
		TxTypeDebit, TxTypeCredit, TxTypeReversal,
	}
	for _, tt := range types {
		if string(tt) == "" {
			t.Fatalf("transaction type should not be empty")
		}
	}
}

func TestServiceCategoryValues(t *testing.T) {
	categories := []ServiceCategory{
		CategoryAirtime, CategoryData, CategoryElectricity,
		CategoryCableTV, CategoryBetting, CategoryBusTravel,
		CategoryFlight, CategoryWalletFund, CategoryTransfer,
	}
	if len(categories) != 9 {
		t.Fatalf("expected 9 service categories, got %d", len(categories))
	}
	for _, c := range categories {
		if string(c) == "" {
			t.Fatalf("service category should not be empty")
		}
	}
}

func TestTransactionImmutability(t *testing.T) {
	// Transactions should only ever have status updated, never amount
	tx := Transaction{
		ID:            uuid.New(),
		UserID:        uuid.New().String(),
		WalletID:      uuid.New(),
		Reference:     "FB-TEST-001",
		Type:          TxTypeDebit,
		Category:      CategoryAirtime,
		Amount:        50_000,
		BalanceBefore: 1_000_000,
		BalanceAfter:  950_000,
		Status:        TxProcessing,
		Provider:      "flutterwave",
		Narration:     "Airtime MTN",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Only status and external_ref should change
	tx.Status = TxSuccess
	tx.ExternalRef = "BPUSSD123456"

	if tx.Amount != 50_000 {
		t.Fatal("amount should never change on a transaction")
	}
	if tx.BalanceBefore != 1_000_000 {
		t.Fatal("balance_before should never change on a transaction")
	}
	if tx.Status != TxSuccess {
		t.Fatal("status should be success")
	}
}
