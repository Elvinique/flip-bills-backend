package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── In-memory fake wallet repo ────────────────────────────────────────────────

type fakeWalletRepo struct {
	balance     int64
	txs         []*models.Transaction
	statusCalls map[string]models.TransactionStatus
	debitErr    error
	creditErr   error
}

func newFakeWalletRepo(balance int64) *fakeWalletRepo {
	return &fakeWalletRepo{
		balance:     balance,
		statusCalls: map[string]models.TransactionStatus{},
	}
}

func (f *fakeWalletRepo) DebitWithLock(_ context.Context, _ string, amount int64) (*models.Wallet, error) {
	if f.debitErr != nil {
		return nil, f.debitErr
	}
	if f.balance < amount {
		return nil, errors.New("insufficient funds")
	}
	f.balance -= amount
	return &models.Wallet{Balance: f.balance}, nil
}

func (f *fakeWalletRepo) CreditBalance(_ context.Context, _ uuid.UUID, amount int64) error {
	if f.creditErr != nil {
		return f.creditErr
	}
	f.balance += amount
	return nil
}

func (f *fakeWalletRepo) InsertTransaction(_ context.Context, tx *models.Transaction) error {
	f.txs = append(f.txs, tx)
	return nil
}

func (f *fakeWalletRepo) UpdateTransactionStatus(_ context.Context, ref string, status models.TransactionStatus, _ string) error {
	f.statusCalls[ref] = status
	return nil
}

func (f *fakeWalletRepo) ReverseDebitIfNeeded(_ context.Context, original *models.Transaction, reversal *models.Transaction) (bool, error) {
	// Mirror exactly what the real repo does: credit the balance and insert
	// the reversal record in one operation. No separate CreditBalance call.
	f.balance += original.Amount
	f.txs = append(f.txs, reversal)
	return true, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newEngine(repo *fakeWalletRepo) *Engine {
	return &Engine{walletRepo: repo, log: zap.NewNop(), timeout: 5 * time.Second}
}

func testTx(userID, walletID uuid.UUID, amount int64) *models.Transaction {
	return &models.Transaction{
		ID:        uuid.New(),
		UserID:    userID.String(),
		WalletID:  walletID,
		Reference: "TEST-" + uuid.NewString()[:8],
		Type:      models.TxTypeDebit,
		Category:  models.CategoryAirtime,
		Amount:    amount,
		Status:    models.TxProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────
// IMPORTANT: ExecuteWithFallback is called AFTER the caller has already debited
// the wallet. All tests start at post-debit balance to reflect this correctly.

func TestExecuteWithFallback_PrimarySucceeds(t *testing.T) {
	// Post-debit starting balance: 1,000,000 - 100,000 = 900,000.
	repo := newFakeWalletRepo(900_000)
	eng := newEngine(repo)
	tx := testTx(uuid.New(), uuid.New(), 100_000)

	extRef, err := eng.ExecuteWithFallback(context.Background(), tx,
		func(_ context.Context) (string, error) { return "FLW-EXT-001", nil },
		nil,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if extRef != "FLW-EXT-001" {
		t.Errorf("extRef = %q, want FLW-EXT-001", extRef)
	}
	if repo.statusCalls[tx.Reference] != models.TxSuccess {
		t.Errorf("status = %q, want success", repo.statusCalls[tx.Reference])
	}
	// Balance unchanged — primary succeeded, no reversal.
	if repo.balance != 900_000 {
		t.Errorf("balance = %d, want 900000 (unchanged after success)", repo.balance)
	}
}

func TestExecuteWithFallback_PrimaryFailsNoFallback_ReversesDebit(t *testing.T) {
	// Post-debit starting balance: 1,000,000 - 100,000 = 900,000.
	repo := newFakeWalletRepo(900_000)
	eng := newEngine(repo)
	tx := testTx(uuid.New(), uuid.New(), 100_000)

	_, err := eng.ExecuteWithFallback(context.Background(), tx,
		func(_ context.Context) (string, error) { return "", errors.New("provider timeout") },
		nil,
	)
	if err == nil {
		t.Fatal("expected error when primary fails with no fallback")
	}
	// Reversal restores 100,000 → back to original 1,000,000.
	if repo.balance != 1_000_000 {
		t.Errorf("balance after reversal = %d, want 1000000 (restored)", repo.balance)
	}
	hasReversal := false
	for _, stored := range repo.txs {
		if stored.Type == models.TxTypeReversal {
			hasReversal = true
			break
		}
	}
	if !hasReversal {
		t.Error("expected a reversal transaction record, found none")
	}
}

func TestExecuteWithFallback_PrimaryFailsFallbackSucceeds(t *testing.T) {
	// Post-debit starting balance: 1,000,000 - 100,000 = 900,000.
	repo := newFakeWalletRepo(900_000)
	eng := newEngine(repo)
	tx := testTx(uuid.New(), uuid.New(), 100_000)

	extRef, err := eng.ExecuteWithFallback(context.Background(), tx,
		func(_ context.Context) (string, error) { return "", errors.New("flutterwave down") },
		func(_ context.Context) (string, error) { return "MON-EXT-001", nil },
	)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if extRef != "MON-EXT-001" {
		t.Errorf("extRef = %q, want MON-EXT-001", extRef)
	}
	// Flow: start 900,000 → reversal +100,000 = 1,000,000 → re-debit -100,000 = 900,000.
	// Net: one debit taken, same as if primary had succeeded.
	if repo.balance != 900_000 {
		t.Errorf("balance = %d, want 900000 (one net debit)", repo.balance)
	}
}

func TestExecuteWithFallback_BothFail_FullRefund(t *testing.T) {
	// Post-debit starting balance: 1,000,000 - 100,000 = 900,000.
	repo := newFakeWalletRepo(900_000)
	eng := newEngine(repo)
	tx := testTx(uuid.New(), uuid.New(), 100_000)

	_, err := eng.ExecuteWithFallback(context.Background(), tx,
		func(_ context.Context) (string, error) { return "", errors.New("flutterwave down") },
		func(_ context.Context) (string, error) { return "", errors.New("monnify down") },
	)
	if err == nil {
		t.Fatal("expected error when both aggregators fail")
	}
	// Flow: 900,000 → reversal +100,000 = 1,000,000 → re-debit -100,000 = 900,000
	//       → fallback fails → reversal +100,000 = 1,000,000. Full refund.
	if repo.balance != 1_000_000 {
		t.Errorf("balance = %d, want 1000000 (full refund)", repo.balance)
	}
}

func TestExecuteWithFallback_PrimaryTimesOut(t *testing.T) {
	// Post-debit starting balance: 500,000 - 50,000 = 450,000.
	repo := newFakeWalletRepo(450_000)
	eng := &Engine{walletRepo: repo, log: zap.NewNop(), timeout: 50 * time.Millisecond}
	tx := testTx(uuid.New(), uuid.New(), 50_000)

	_, err := eng.ExecuteWithFallback(context.Background(), tx,
		func(ctx context.Context) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return "late-ref", nil
			}
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Reversal restores 50,000 → back to original 500,000.
	if repo.balance != 500_000 {
		t.Errorf("balance after timeout = %d, want 500000", repo.balance)
	}
}
