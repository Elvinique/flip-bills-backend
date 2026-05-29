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

type InitializeFundingRequest struct {
	Amount   int64  `json:"amount"   binding:"required,min=10000"` // min ₦100 (in kobo)
	Provider string `json:"provider" binding:"required"`           // "flutterwave" or "monnify"
}

type InitializeFundingResponse struct {
	Reference   string `json:"reference"`
	CheckoutURL string `json:"checkout_url,omitempty"`
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
func (s *Service) FundWallet(ctx context.Context, userID string, req FundWalletRequest) (*models.Transaction, error) {
	var tx *models.Transaction

	err := s.walletRepo.WithinTx(ctx, func(txCtx context.Context) error {
		wallet, err := s.walletRepo.FindByUserID(txCtx, userID)
		if err != nil {
			return fmt.Errorf("wallet not found")
		}

		ref := fmt.Sprintf("FUND-%s-%d", uuid.NewString()[:8], time.Now().UnixMilli())

		tx = &models.Transaction{
			ID:             uuid.New(),
			UserID:         wallet.UserID.String(), // Fixed type mapping: UUID array to clean primitive string
			WalletID:       wallet.ID,
			Reference:      ref,
			ExternalRef:    req.PaymentRef,
			Type:           models.TxTypeCredit,
			Category:       models.CategoryWalletFund,
			Amount:         req.Amount,
			CommissionKobo: 0, // No utility vendor fee applied to straight wallet funding
			Fee:            0,
			BalanceBefore:  wallet.Balance,
			BalanceAfter:   wallet.Balance + req.Amount,
			Status:         models.TxSuccess,
			Provider:       req.Provider,
			Narration:      fmt.Sprintf("Wallet funded via %s", req.Provider),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if err := s.walletRepo.CreditBalance(txCtx, wallet.ID, req.Amount); err != nil {
			return fmt.Errorf("credit balance: %w", err)
		}

		if err := s.walletRepo.InsertTransaction(txCtx, tx); err != nil {
			return fmt.Errorf("insert ledger transaction: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.log.Info("wallet funded",
		zap.String("user_id", userID),
		zap.String("amount_ngn", strconv.FormatFloat(koboToNGN(req.Amount), 'f', 2, 64)),
	)

	if s.loyaltySvc != nil {
		go s.loyaltySvc.AwardPoints(context.Background(), userID, tx.ID, models.CategoryWalletFund, req.Amount)
	}

	return tx, nil
}

// InitializeFunding prepares a pending transaction in the ledger and generates checkout metadata.
func (s *Service) InitializeFunding(ctx context.Context, userID string, req InitializeFundingRequest) (*InitializeFundingResponse, error) {
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}

	ref := fmt.Sprintf("FUND-INIT-%s-%d", uuid.NewString()[:8], time.Now().UnixMilli())

	txPlaceholder := &models.Transaction{
		ID:             uuid.New(),
		UserID:         wallet.UserID.String(), // Fixed type mapping: UUID array to clean primitive string
		WalletID:       wallet.ID,
		Reference:      ref,
		ExternalRef:    "",
		Type:           models.TxTypeCredit,
		Category:       models.CategoryWalletFund,
		Amount:         req.Amount,
		CommissionKobo: 0,
		Fee:            0,
		BalanceBefore:  wallet.Balance,
		BalanceAfter:   wallet.Balance,
		Status:         models.TxPending,
		Provider:       req.Provider,
		Narration:      fmt.Sprintf("Wallet funding initialized via %s", req.Provider),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.walletRepo.InsertTransaction(ctx, txPlaceholder); err != nil {
		return nil, fmt.Errorf("failed to log pending funding record: %w", err)
	}

	var checkoutURL string
	switch req.Provider {
	case "flutterwave":
		checkoutURL = fmt.Sprintf("https://checkout.flutterwave.com/v3/hosted/pay/%s", ref)
	case "monnify":
		checkoutURL = fmt.Sprintf("https://sandbox.monnify.com/v1/checkout/pay/%s", ref)
	default:
		checkoutURL = fmt.Sprintf("https://checkout.flipbills.com/fallback/%s", ref)
	}

	s.log.Info("payment funding initialized securely",
		zap.String("user_id", userID),
		zap.String("reference", ref),
		zap.Int64("amount_kobo", req.Amount),
	)

	return &InitializeFundingResponse{
		Reference:   ref,
		CheckoutURL: checkoutURL,
	}, nil
}

// ProcessFundingWebhook settles an initialized transaction from PENDING to SUCCESS.
func (s *Service) ProcessFundingWebhook(ctx context.Context, reference string, externalRef string, incomingAmountKobo int64) error {
	return s.walletRepo.WithinTx(ctx, func(txCtx context.Context) error {
		tx, err := s.walletRepo.FindTransactionByReference(txCtx, "", reference)
		if err != nil {
			return fmt.Errorf("transaction not found for ref %s: %w", reference, err)
		}

		if tx.Status == models.TxSuccess {
			s.log.Warn("transaction already processed successfully", zap.String("ref", reference))
			return nil
		}
		if tx.Status != models.TxPending {
			return fmt.Errorf("transaction cannot be settled; unexpected status: %s", tx.Status)
		}

		if tx.Amount != incomingAmountKobo {
			return fmt.Errorf("amount mismatch: initialized %d kobo, gateway paid %d kobo", tx.Amount, incomingAmountKobo)
		}

		wallet, err := s.walletRepo.FindByUserID(txCtx, tx.UserID) // tx.UserID is a plain string primitive
		if err != nil {
			return fmt.Errorf("wallet not found for user %s: %w", tx.UserID, err)
		}

		tx.BalanceBefore = wallet.Balance
		tx.BalanceAfter = wallet.Balance + incomingAmountKobo

		if err := s.walletRepo.CreditBalance(txCtx, wallet.ID, incomingAmountKobo); err != nil {
			return fmt.Errorf("failed to credit wallet balance: %w", err)
		}

		if err := s.walletRepo.UpdateTransactionStatus(txCtx, reference, models.TxSuccess, externalRef); err != nil {
			return fmt.Errorf("failed to update transaction status to success: %w", err)
		}

		if s.loyaltySvc != nil {
			go s.loyaltySvc.AwardPoints(context.Background(), tx.UserID, tx.ID, models.CategoryWalletFund, incomingAmountKobo)
		}

		return nil
	})
}

// koboToNGN converts integer kobo to a human-readable NGN float.
func koboToNGN(kobo int64) float64 {
	return float64(kobo) / 100
}
