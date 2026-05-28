package loyalty

// LoyaltyService manages the Flip Bills points reward system (PRD Phase 3).
// Points are earned on every successful VAS and travel transaction,
// and can be redeemed as wallet credit at 100 points = ₦1.

import (
	"context"
	"fmt"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type BalanceResponse struct {
	AccountID      string              `json:"account_id"`
	PointsBalance  int64               `json:"points_balance"`
	LifetimePoints int64               `json:"lifetime_points"`
	Tier           models.LoyaltyTier  `json:"tier"`
	NextTier       *models.LoyaltyTier `json:"next_tier,omitempty"`
	PointsToNext   *int64              `json:"points_to_next_tier,omitempty"`
	RedeemableNGN  float64             `json:"redeemable_ngn"` // how much ₦ the balance is worth
}

type HistoryResponse struct {
	Transactions []*models.LoyaltyTransaction `json:"transactions"`
	Total        int64                        `json:"total"`
	Page         int                          `json:"page"`
	Limit        int                          `json:"limit"`
	TotalPages   int                          `json:"total_pages"`
}

type RedeemRequest struct {
	Points    int64  `json:"points"   binding:"required,min=100"` // minimum 100 points = ₦1
	Narration string `json:"narration"`
}

type RedeemResponse struct {
	PointsRedeemed int64   `json:"points_redeemed"`
	KoboCredit     int64   `json:"kobo_credit"`
	NGNCredit      float64 `json:"ngn_credit"`
	NewBalance     int64   `json:"new_points_balance"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	loyaltyRepo loyaltyRepo
	walletRepo  walletRepo
	log         *zap.Logger
}

func NewService(
	loyaltyRepo loyaltyRepo,
	walletRepo *postgres.WalletRepository,
	log *zap.Logger,
) *Service {
	return &Service{
		loyaltyRepo: loyaltyRepo,
		walletRepo:  walletRepo,
		log:         log,
	}
}

// GetBalance returns the user's points balance, tier, and NGN redeemable value.
func (s *Service) GetBalance(ctx context.Context, userID string) (*BalanceResponse, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID")
	}

	acc, err := s.loyaltyRepo.GetOrCreate(ctx, uid)
	if err != nil {
		return nil, err
	}

	resp := &BalanceResponse{
		AccountID:      acc.ID.String(),
		PointsBalance:  acc.PointsBalance,
		LifetimePoints: acc.LifetimePoints,
		Tier:           acc.Tier,
		RedeemableNGN:  float64(models.PointsToKobo(acc.PointsBalance)) / 100,
	}

	// Calculate next tier and points needed.
	for i, t := range models.TierThresholds {
		if acc.Tier == t.Tier && i > 0 {
			nextTier := models.TierThresholds[i-1].Tier
			pointsNeeded := models.TierThresholds[i-1].Min - acc.LifetimePoints
			resp.NextTier = &nextTier
			resp.PointsToNext = &pointsNeeded
			break
		}
	}

	return resp, nil
}

// GetHistory returns paginated points transaction history.
func (s *Service) GetHistory(ctx context.Context, userID string, page, limit int) (*HistoryResponse, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	txs, total, err := s.loyaltyRepo.ListTransactions(ctx, uid, limit, offset)
	if err != nil {
		return nil, err
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	return &HistoryResponse{
		Transactions: txs,
		Total:        total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	}, nil
}

// RedeemPoints converts points to wallet credit at 100 points = ₦1.
func (s *Service) RedeemPoints(ctx context.Context, userID string, req RedeemRequest) (*RedeemResponse, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID")
	}

	// Points must be in multiples of 100 (1 NGN minimum).
	if req.Points%100 != 0 {
		return nil, fmt.Errorf("points must be in multiples of 100 (100 points = ₦1)")
	}

	koboCredit := models.PointsToKobo(req.Points)
	narration := req.Narration
	if narration == "" {
		narration = fmt.Sprintf("Redeemed %d points for ₦%.2f wallet credit", req.Points, float64(koboCredit)/100)
	}

	// Debit points ledger.
	ltx, err := s.loyaltyRepo.RedeemPoints(ctx, uid, req.Points, narration)
	if err != nil {
		return nil, err
	}

	// Credit wallet.
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}
	if err := s.walletRepo.CreditBalance(ctx, wallet.ID, koboCredit); err != nil {
		return nil, fmt.Errorf("wallet credit failed: %w", err)
	}

	// Write wallet transaction record.
	walletTx := &models.Transaction{
		ID:            uuid.New(),
		UserID:        wallet.UserID,
		WalletID:      wallet.ID,
		Reference:     fmt.Sprintf("LYL-%s", uuid.NewString()[:8]),
		Type:          models.TxTypeCredit,
		Category:      models.CategoryTransfer,
		Amount:        koboCredit,
		BalanceBefore: wallet.Balance,
		BalanceAfter:  wallet.Balance + koboCredit,
		Status:        models.TxSuccess,
		Provider:      "loyalty_redemption",
		Narration:     narration,
	}
	_ = s.walletRepo.InsertTransaction(ctx, walletTx)

	s.log.Info("loyalty points redeemed",
		zap.String("user_id", userID),
		zap.Int64("points", req.Points),
		zap.Int64("kobo_credit", koboCredit),
	)

	return &RedeemResponse{
		PointsRedeemed: req.Points,
		KoboCredit:     koboCredit,
		NGNCredit:      float64(koboCredit) / 100,
		NewBalance:     ltx.BalanceAfter,
	}, nil
}

// AwardPoints is called by other services after a successful transaction.
// It is intentionally non-blocking — a points failure never fails the parent tx.
func (s *Service) AwardPoints(
	ctx context.Context,
	userID string,
	sourceTxID uuid.UUID,
	category models.ServiceCategory,
	amountKobo int64,
) {
	points := models.CalculatePointsEarned(category, amountKobo)
	if points == 0 {
		return
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return
	}

	// Ensure account exists.
	if _, err := s.loyaltyRepo.GetOrCreate(ctx, uid); err != nil {
		s.log.Warn("loyalty GetOrCreate failed", zap.String("user_id", userID), zap.Error(err))
		return
	}

	narration := fmt.Sprintf("Earned %d points on %s transaction", points, category)
	_, err = s.loyaltyRepo.EarnPoints(ctx, uid, points, &sourceTxID, string(category), narration)
	if err != nil {
		s.log.Warn("loyalty earn failed",
			zap.String("user_id", userID),
			zap.Int64("points", points),
			zap.Error(err),
		)
		return
	}

	s.log.Info("loyalty points awarded",
		zap.String("user_id", userID),
		zap.Int64("points", points),
		zap.String("category", string(category)),
	)
}
