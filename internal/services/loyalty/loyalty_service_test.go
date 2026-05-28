package loyalty

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeLoyaltyRepo struct {
	mu        sync.Mutex
	accounts  map[uuid.UUID]*models.LoyaltyAccount
	txs       []*models.LoyaltyTransaction
	earnErr   error
	redeemErr error
}

func newFakeLoyaltyRepo() *fakeLoyaltyRepo {
	return &fakeLoyaltyRepo{accounts: map[uuid.UUID]*models.LoyaltyAccount{}}
}

func (r *fakeLoyaltyRepo) GetOrCreate(_ context.Context, userID uuid.UUID) (*models.LoyaltyAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if acc, ok := r.accounts[userID]; ok {
		return acc, nil
	}
	acc := &models.LoyaltyAccount{
		ID: uuid.New(), UserID: userID,
		Tier: models.TierBronze, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	r.accounts[userID] = acc
	return acc, nil
}

func (r *fakeLoyaltyRepo) EarnPoints(_ context.Context, userID uuid.UUID, points int64, sourceTxID *uuid.UUID, category, narration string) (*models.LoyaltyTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.earnErr != nil {
		return nil, r.earnErr
	}
	acc := r.accounts[userID]
	before := acc.PointsBalance
	acc.PointsBalance += points
	acc.LifetimePoints += points
	acc.Tier = models.CalculateTier(acc.LifetimePoints)
	ltx := &models.LoyaltyTransaction{
		ID: uuid.New(), UserID: userID, AccountID: acc.ID,
		Type: models.LoyaltyEarn, Points: points,
		BalanceBefore: before, BalanceAfter: acc.PointsBalance,
		SourceTxID: sourceTxID, Category: category, Narration: narration,
		CreatedAt: time.Now(),
	}
	r.txs = append(r.txs, ltx)
	return ltx, nil
}

func (r *fakeLoyaltyRepo) RedeemPoints(_ context.Context, userID uuid.UUID, points int64, narration string) (*models.LoyaltyTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.redeemErr != nil {
		return nil, r.redeemErr
	}
	acc := r.accounts[userID]
	if acc == nil || acc.PointsBalance < points {
		return nil, errors.New("insufficient points balance")
	}
	before := acc.PointsBalance
	acc.PointsBalance -= points
	ltx := &models.LoyaltyTransaction{
		ID: uuid.New(), UserID: userID, AccountID: acc.ID,
		Type: models.LoyaltyRedeem, Points: points,
		BalanceBefore: before, BalanceAfter: acc.PointsBalance,
		Narration: narration, CreatedAt: time.Now(),
	}
	r.txs = append(r.txs, ltx)
	return ltx, nil
}

func (r *fakeLoyaltyRepo) ListTransactions(_ context.Context, userID uuid.UUID, limit, offset int) ([]*models.LoyaltyTransaction, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*models.LoyaltyTransaction
	for _, t := range r.txs {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	total := int64(len(out))
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	if offset > len(out) {
		return nil, total, nil
	}
	return out[offset:end], total, nil
}

type fakeWalletRepo struct {
	mu       sync.Mutex
	balance  int64
	walletID uuid.UUID
	userID   uuid.UUID
	txs      []*models.Transaction
}

func newFakeWalletRepo(balance int64) *fakeWalletRepo {
	return &fakeWalletRepo{balance: balance, walletID: uuid.New(), userID: uuid.New()}
}

func (r *fakeWalletRepo) FindByUserID(_ context.Context, _ string) (*models.Wallet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &models.Wallet{ID: r.walletID, UserID: r.userID, Balance: r.balance}, nil
}

func (r *fakeWalletRepo) CreditBalance(_ context.Context, _ uuid.UUID, amount int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.balance += amount
	return nil
}

func (r *fakeWalletRepo) InsertTransaction(_ context.Context, tx *models.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.txs = append(r.txs, tx)
	return nil
}

func newTestSvc(walletBalance int64) (*Service, *fakeLoyaltyRepo, *fakeWalletRepo) {
	lr := newFakeLoyaltyRepo()
	wr := newFakeWalletRepo(walletBalance)
	return &Service{loyaltyRepo: lr, walletRepo: wr, log: zap.NewNop()}, lr, wr
}

func seedAccount(lr *fakeLoyaltyRepo, userID uuid.UUID, points, lifetime int64) {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	lr.accounts[userID] = &models.LoyaltyAccount{
		ID: uuid.New(), UserID: userID,
		PointsBalance: points, LifetimePoints: lifetime,
		Tier: models.CalculateTier(lifetime), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestGetBalance_NewUser_StartsBronzeZeroPoints(t *testing.T) {
	svc, _, _ := newTestSvc(0)
	resp, err := svc.GetBalance(context.Background(), uuid.NewString())
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if resp.PointsBalance != 0 {
		t.Errorf("PointsBalance = %d, want 0", resp.PointsBalance)
	}
	if resp.Tier != models.TierBronze {
		t.Errorf("Tier = %q, want bronze", resp.Tier)
	}
	if resp.NextTier == nil {
		t.Error("NextTier should not be nil for bronze")
	}
}

func TestGetBalance_CorrectRedeemableNGN(t *testing.T) {
	svc, lr, _ := newTestSvc(0)
	uid := uuid.New()
	seedAccount(lr, uid, 1000, 1000)
	resp, err := svc.GetBalance(context.Background(), uid.String())
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if resp.RedeemableNGN != 10.0 {
		t.Errorf("RedeemableNGN = %.2f, want 10.00", resp.RedeemableNGN)
	}
}

func TestGetBalance_TierProgression(t *testing.T) {
	tests := []struct {
		lifetime int64
		wantTier models.LoyaltyTier
		hasNext  bool
	}{
		{0, models.TierBronze, true},
		{9_999, models.TierBronze, true},
		{10_000, models.TierSilver, true},
		{49_999, models.TierSilver, true},
		{50_000, models.TierGold, true},
		{199_999, models.TierGold, true},
		{200_000, models.TierPlatinum, false},
	}
	for _, tt := range tests {
		svc, lr, _ := newTestSvc(0)
		uid := uuid.New()
		seedAccount(lr, uid, tt.lifetime, tt.lifetime)
		resp, err := svc.GetBalance(context.Background(), uid.String())
		if err != nil {
			t.Fatalf("lifetime=%d: %v", tt.lifetime, err)
		}
		if resp.Tier != tt.wantTier {
			t.Errorf("lifetime=%d: Tier=%q, want %q", tt.lifetime, resp.Tier, tt.wantTier)
		}
		if tt.hasNext && resp.NextTier == nil {
			t.Errorf("lifetime=%d: expected NextTier, got nil", tt.lifetime)
		}
		if !tt.hasNext && resp.NextTier != nil {
			t.Errorf("lifetime=%d: expected no NextTier for platinum", tt.lifetime)
		}
	}
}

func TestGetBalance_InvalidUserID(t *testing.T) {
	svc, _, _ := newTestSvc(0)
	if _, err := svc.GetBalance(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid user ID")
	}
}

func TestAwardPoints_CorrectPointsCalculated(t *testing.T) {
	tests := []struct {
		category   models.ServiceCategory
		amountKobo int64
		wantPoints int64
	}{
		{models.CategoryAirtime, 100_000, 1000},
		{models.CategoryElectricity, 500_000, 10000},
		{models.CategoryBusTravel, 750_000, 37500},
		{models.CategoryFlight, 5_500_000, 550000},
		{models.CategoryWalletFund, 1_000_000, 0},
	}
	for _, tt := range tests {
		svc, lr, _ := newTestSvc(0)
		uid := uuid.New()
		seedAccount(lr, uid, 0, 0)
		svc.AwardPoints(context.Background(), uid.String(), uuid.New(), tt.category, tt.amountKobo)
		if tt.wantPoints == 0 {
			lr.mu.Lock()
			n := len(lr.txs)
			lr.mu.Unlock()
			if n != 0 {
				t.Errorf("category=%s: expected no tx for zero-earning category, got %d", tt.category, n)
			}
			continue
		}
		lr.mu.Lock()
		acc := lr.accounts[uid]
		lr.mu.Unlock()
		if acc.PointsBalance != tt.wantPoints {
			t.Errorf("category=%s: PointsBalance=%d, want %d", tt.category, acc.PointsBalance, tt.wantPoints)
		}
	}
}

func TestAwardPoints_TierUpgradesOnEarn(t *testing.T) {
	svc, lr, _ := newTestSvc(0)
	uid := uuid.New()
	seedAccount(lr, uid, 9_990, 9_990)
	svc.AwardPoints(context.Background(), uid.String(), uuid.New(), models.CategoryAirtime, 1_000)
	lr.mu.Lock()
	acc := lr.accounts[uid]
	lr.mu.Unlock()
	if acc.Tier != models.TierSilver {
		t.Errorf("Tier = %q, want silver after crossing 10,000 lifetime points", acc.Tier)
	}
}

func TestAwardPoints_InvalidUserIDIsSilent(t *testing.T) {
	svc, _, _ := newTestSvc(0)
	svc.AwardPoints(context.Background(), "bad-uuid", uuid.New(), models.CategoryAirtime, 1_000)
}

func TestRedeemPoints_Success(t *testing.T) {
	svc, lr, wr := newTestSvc(50_000)
	uid := uuid.New()
	seedAccount(lr, uid, 500, 500)
	resp, err := svc.RedeemPoints(context.Background(), uid.String(), RedeemRequest{Points: 500})
	if err != nil {
		t.Fatalf("RedeemPoints error: %v", err)
	}
	if resp.PointsRedeemed != 500 {
		t.Errorf("PointsRedeemed = %d, want 500", resp.PointsRedeemed)
	}
	if resp.KoboCredit != 500 {
		t.Errorf("KoboCredit = %d, want 500", resp.KoboCredit)
	}
	if resp.NGNCredit != 5.0 {
		t.Errorf("NGNCredit = %.2f, want 5.00", resp.NGNCredit)
	}
	if resp.NewBalance != 0 {
		t.Errorf("NewBalance = %d, want 0", resp.NewBalance)
	}
	wr.mu.Lock()
	bal := wr.balance
	txCount := len(wr.txs)
	wr.mu.Unlock()
	if bal != 50_500 {
		t.Errorf("wallet balance = %d, want 50500", bal)
	}
	if txCount != 1 {
		t.Errorf("wallet tx count = %d, want 1", txCount)
	}
}

func TestRedeemPoints_InsufficientBalance(t *testing.T) {
	svc, lr, _ := newTestSvc(0)
	uid := uuid.New()
	seedAccount(lr, uid, 100, 100)
	if _, err := svc.RedeemPoints(context.Background(), uid.String(), RedeemRequest{Points: 200}); err == nil {
		t.Fatal("expected error for insufficient points")
	}
}

func TestRedeemPoints_MustBeMultipleOf100(t *testing.T) {
	svc, lr, _ := newTestSvc(0)
	uid := uuid.New()
	seedAccount(lr, uid, 1000, 1000)
	if _, err := svc.RedeemPoints(context.Background(), uid.String(), RedeemRequest{Points: 150}); err == nil {
		t.Fatal("expected error for non-multiple-of-100 points")
	}
}

func TestGetHistory_Pagination(t *testing.T) {
	svc, lr, _ := newTestSvc(0)
	uid := uuid.New()
	seedAccount(lr, uid, 5000, 5000)
	for i := 0; i < 15; i++ {
		lr.mu.Lock()
		lr.txs = append(lr.txs, &models.LoyaltyTransaction{
			ID: uuid.New(), UserID: uid, Type: models.LoyaltyEarn, Points: 100, CreatedAt: time.Now(),
		})
		lr.mu.Unlock()
	}
	resp, err := svc.GetHistory(context.Background(), uid.String(), 1, 10)
	if err != nil {
		t.Fatalf("GetHistory error: %v", err)
	}
	if resp.Total != 15 {
		t.Errorf("Total = %d, want 15", resp.Total)
	}
	if len(resp.Transactions) != 10 {
		t.Errorf("len(Transactions) = %d, want 10", len(resp.Transactions))
	}
	if resp.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", resp.TotalPages)
	}
}

func TestGetHistory_InvalidUserID(t *testing.T) {
	svc, _, _ := newTestSvc(0)
	if _, err := svc.GetHistory(context.Background(), "not-a-uuid", 1, 20); err == nil {
		t.Fatal("expected error for invalid user ID")
	}
}
