package wallet

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── In-memory fakes ───────────────────────────────────────────────────────────

type fakeWalletStore struct {
	wallet    *models.Wallet
	txs       []*models.Transaction
	creditErr error
}

func newFakeWalletStore(balance int64) *fakeWalletStore {
	return &fakeWalletStore{
		wallet: &models.Wallet{
			ID:         uuid.New(),
			UserID:     uuid.New(),
			Balance:    balance,
			Currency:   models.NGN,
			DailyLimit: 5_000_000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
	}
}

func (f *fakeWalletStore) FindByUserID(_ context.Context, _ string) (*models.Wallet, error) {
	return f.wallet, nil
}
func (f *fakeWalletStore) ListTransactions(_ context.Context, _ string, limit, offset int) ([]*models.Transaction, int64, error) {
	total := int64(len(f.txs))
	end := offset + limit
	if end > len(f.txs) {
		end = len(f.txs)
	}
	if offset > len(f.txs) {
		return nil, total, nil
	}
	return f.txs[offset:end], total, nil
}
func (f *fakeWalletStore) CreditWithTransaction(_ context.Context, _ uuid.UUID, tx *models.Transaction) error {
	if f.creditErr != nil {
		return f.creditErr
	}
	f.wallet.Balance += tx.Amount
	f.txs = append(f.txs, tx)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeUserStore struct{ user *models.User }

func newFakeUserStore(kycTier models.KYCTier) *fakeUserStore {
	return &fakeUserStore{user: &models.User{ID: uuid.New(), KYCTier: kycTier, IsActive: true}}
}
func (f *fakeUserStore) FindByID(_ context.Context, _ string) (*models.User, error) {
	return f.user, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeLoyalty struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeLoyalty) AwardPoints(_ context.Context, _ string, _ uuid.UUID, _ models.ServiceCategory, _ int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
}

// ── helper ────────────────────────────────────────────────────────────────────

func nopLog() *zap.Logger { return zap.NewNop() }

func newTestSvc(walletBalance int64, kycTier models.KYCTier) (*Service, *fakeWalletStore, *fakeLoyalty) {
	ws := newFakeWalletStore(walletBalance)
	us := newFakeUserStore(kycTier)
	loy := &fakeLoyalty{}
	svc := &Service{walletRepo: ws, userRepo: us, loyaltySvc: loy, log: nopLog()}
	return svc, ws, loy
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestGetBalance_ReturnsCorrectValues(t *testing.T) {
	svc, _, _ := newTestSvc(500_000, models.KYCTierOne)
	resp, err := svc.GetBalance(context.Background(), uuid.NewString())
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if resp.Balance != 5000.0 {
		t.Errorf("Balance = %.2f, want 5000.00", resp.Balance)
	}
	if resp.KYCTier != int(models.KYCTierOne) {
		t.Errorf("KYCTier = %d, want 1", resp.KYCTier)
	}
	if resp.DailyRemaining != resp.DailyLimit {
		t.Error("DailyRemaining should equal DailyLimit when nothing spent")
	}
}

func TestGetBalance_DailyRemainingClampsToZero(t *testing.T) {
	svc, ws, _ := newTestSvc(0, models.KYCTierUnverified)
	ws.wallet.DailySpend = ws.wallet.DailyLimit + 100

	resp, err := svc.GetBalance(context.Background(), uuid.NewString())
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if resp.DailyRemaining != 0 {
		t.Errorf("DailyRemaining = %.2f, want 0 when overspent", resp.DailyRemaining)
	}
}

func TestFundWallet_Success(t *testing.T) {
	svc, ws, loy := newTestSvc(100_000, models.KYCTierUnverified)

	tx, err := svc.FundWallet(context.Background(), uuid.NewString(), FundWalletRequest{
		Amount: 500_000, PaymentRef: "FLW-REF-001", Provider: "flutterwave",
	})
	if err != nil {
		t.Fatalf("FundWallet error: %v", err)
	}
	if tx.Amount != 500_000 {
		t.Errorf("Amount = %d, want 500000", tx.Amount)
	}
	if tx.Type != models.TxTypeCredit {
		t.Errorf("Type = %q, want credit", tx.Type)
	}
	if tx.Status != models.TxSuccess {
		t.Errorf("Status = %q, want success", tx.Status)
	}
	if tx.ExternalRef != "FLW-REF-001" {
		t.Errorf("ExternalRef = %q, want FLW-REF-001", tx.ExternalRef)
	}
	if ws.wallet.Balance != 600_000 {
		t.Errorf("wallet balance = %d, want 600000", ws.wallet.Balance)
	}
	// Loyalty goroutine — give it a moment.
	time.Sleep(50 * time.Millisecond)
	loy.mu.Lock(); calls := loy.calls; loy.mu.Unlock()
	if calls != 1 {
		t.Errorf("loyalty AwardPoints calls = %d, want 1", calls)
	}
}

func TestFundWallet_AtomicLedgerEntry(t *testing.T) {
	svc, ws, _ := newTestSvc(0, models.KYCTierUnverified)

	_, err := svc.FundWallet(context.Background(), uuid.NewString(), FundWalletRequest{
		Amount: 200_000, PaymentRef: "REF-ATOMIC", Provider: "monnify",
	})
	if err != nil {
		t.Fatalf("FundWallet error: %v", err)
	}
	// CreditWithTransaction stores both credit and ledger entry atomically.
	if len(ws.txs) != 1 {
		t.Fatalf("expected 1 transaction record, got %d", len(ws.txs))
	}
	if ws.wallet.Balance != 200_000 {
		t.Errorf("balance = %d, want 200000", ws.wallet.Balance)
	}
}

func TestGetTransactions_Pagination(t *testing.T) {
	svc, ws, _ := newTestSvc(0, models.KYCTierUnverified)

	for i := 0; i < 25; i++ {
		ws.txs = append(ws.txs, &models.Transaction{
			ID: uuid.New(), Reference: uuid.NewString(),
			Type: models.TxTypeCredit, Status: models.TxSuccess,
		})
	}

	resp, err := svc.GetTransactions(context.Background(), uuid.NewString(), 1, 10)
	if err != nil {
		t.Fatalf("GetTransactions error: %v", err)
	}
	if resp.Total != 25 {
		t.Errorf("Total = %d, want 25", resp.Total)
	}
	if len(resp.Transactions) != 10 {
		t.Errorf("len(Transactions) = %d, want 10", len(resp.Transactions))
	}
	if resp.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", resp.TotalPages)
	}
}

func TestGetTransactions_DefaultsInvalidPagination(t *testing.T) {
	svc, _, _ := newTestSvc(0, models.KYCTierUnverified)

	resp, err := svc.GetTransactions(context.Background(), uuid.NewString(), 0, 999)
	if err != nil {
		t.Fatalf("GetTransactions error: %v", err)
	}
	if resp.Page != 1 {
		t.Errorf("Page = %d, want 1 (clamped)", resp.Page)
	}
	if resp.Limit != 20 {
		t.Errorf("Limit = %d, want 20 (clamped)", resp.Limit)
	}
}
