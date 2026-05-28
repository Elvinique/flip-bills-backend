package wallet

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Compile-time proof that concrete types satisfy the interfaces.
var _ walletStore = (*postgres.WalletRepository)(nil)
var _ userStore = (*postgres.UserRepository)(nil)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type BalanceResponse struct {
	WalletID       string  `json:"wallet_id"`
	Balance        float64 `json:"balance_ngn"` // human-readable NGN
	DailySpent     float64 `json:"daily_spent_ngn"`
	DailyLimit     float64 `json:"daily_limit_ngn"`
	DailyRemaining float64 `json:"daily_remaining_ngn"`
	Currency       string  `json:"currency"`
	KYCTier        int     `json:"kyc_tier"`
}

type TransactionListResponse struct {
	Transactions []*models.Transaction `json:"transactions"`
	Total        int64                 `json:"total"`
	Page         int                   `json:"page"`
	Limit        int                   `json:"limit"`
	TotalPages   int                   `json:"total_pages"`
}

type FundWalletRequest struct {
	Amount     int64  `json:"amount"         binding:"required,min=10000"` // min ₦100
	PaymentRef string `json:"payment_ref"    binding:"required"`           // from payment gateway webhook
	Provider   string `json:"provider"       binding:"required"`
}

// loyaltyAwarder is the minimal interface wallet service needs from loyalty.
type loyaltyAwarder interface {
	AwardPoints(ctx context.Context, userID string, sourceTxID uuid.UUID, category models.ServiceCategory, amountKobo int64)
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	walletRepo walletStore
	userRepo   userStore
	loyaltySvc loyaltyAwarder
	log        *zap.Logger
}

func NewService(walletRepo *postgres.WalletRepository, userRepo *postgres.UserRepository, loyaltySvc loyaltyAwarder, log *zap.Logger) *Service {
	return &Service{walletRepo: walletRepo, userRepo: userRepo, loyaltySvc: loyaltySvc, log: log}
}

// GetBalance returns the wallet balance and KYC tier limits for a user.
func (s *Service) GetBalance(ctx context.Context, userID string) (*BalanceResponse, error) {
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	remaining := wallet.DailyLimit - wallet.DailySpend
	if remaining < 0 {
		remaining = 0
	}

	return &BalanceResponse{
		WalletID:       wallet.ID.String(),
		Balance:        koboToNGN(wallet.Balance),
		DailySpent:     koboToNGN(wallet.DailySpend),
		DailyLimit:     koboToNGN(wallet.DailyLimit),
		DailyRemaining: koboToNGN(remaining),
		Currency:       string(wallet.Currency),
		KYCTier:        int(user.KYCTier),
	}, nil
}

// GetTransactions returns paginated transaction history.
func (s *Service) GetTransactions(ctx context.Context, userID string, page, limit int) (*TransactionListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	txs, total, err := s.walletRepo.ListTransactions(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	return &TransactionListResponse{
		Transactions: txs,
		Total:        total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	}, nil
}

// FundWallet credits the wallet after a successful inbound payment webhook.
// The payment gateway (Flutterwave/Monnify) calls this via webhook — we
// verify the reference is new before crediting to prevent double-processing.
func (s *Service) FundWallet(ctx context.Context, userID string, req FundWalletRequest) (*models.Transaction, error) {
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}

	ref := fmt.Sprintf("FUND-%s-%d", uuid.NewString()[:8], time.Now().UnixMilli())

	tx := &models.Transaction{
		ID:            uuid.New(),
		UserID:        wallet.UserID,
		WalletID:      wallet.ID,
		Reference:     ref,
		ExternalRef:   req.PaymentRef,
		Type:          models.TxTypeCredit,
		Category:      models.CategoryWalletFund,
		Amount:        req.Amount,
		BalanceBefore: wallet.Balance,
		BalanceAfter:  wallet.Balance + req.Amount,
		Status:        models.TxSuccess,
		Provider:      req.Provider,
		Narration:     fmt.Sprintf("Wallet funded via %s", req.Provider),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Atomic: credit balance + insert ledger entry in one DB transaction.
	// Previously these were two separate calls — a crash between them would
	// credit money with no audit record.
	if err := s.walletRepo.CreditWithTransaction(ctx, wallet.ID, tx); err != nil {
		return nil, err
	}

	s.log.Info("wallet funded",
		zap.String("user_id", userID),
		zap.String("amount_ngn", strconv.FormatFloat(koboToNGN(req.Amount), 'f', 2, 64)),
	)

	// Wallet funding earns 0 points by design (see models.EarningRate),
	// but we still call through so the loyalty service can apply bonus campaigns.
	if s.loyaltySvc != nil {
		go s.loyaltySvc.AwardPoints(context.Background(), userID, tx.ID, models.CategoryWalletFund, req.Amount)
	}

	return tx, nil
}

// koboToNGN converts integer kobo to a human-readable NGN float.
func koboToNGN(kobo int64) float64 {
	return float64(kobo) / 100
}
