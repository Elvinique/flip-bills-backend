package loyalty

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type loyaltyRepo interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (*models.LoyaltyAccount, error)
	EarnPoints(ctx context.Context, userID uuid.UUID, points int64, sourceTxID *uuid.UUID, category, narration string) (*models.LoyaltyTransaction, error)
	RedeemPoints(ctx context.Context, userID uuid.UUID, points int64, narration string) (*models.LoyaltyTransaction, error)
	ListTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.LoyaltyTransaction, int64, error)
}

type walletRepo interface {
	FindByUserID(ctx context.Context, userID string) (*models.Wallet, error)
	CreditBalance(ctx context.Context, walletID uuid.UUID, amount int64) error
	InsertTransaction(ctx context.Context, tx *models.Transaction) error
}

var _ loyaltyRepo = (*postgres.LoyaltyRepository)(nil)
var _ walletRepo  = (*postgres.WalletRepository)(nil)
