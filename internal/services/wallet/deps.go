package wallet

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type walletStore interface {
	FindByUserID(ctx context.Context, userID string) (*models.Wallet, error)
	ListTransactions(ctx context.Context, userID string, limit, offset int) ([]*models.Transaction, int64, error)
	CreditWithTransaction(ctx context.Context, walletID uuid.UUID, tx *models.Transaction) error
}

type userStore interface {
	FindByID(ctx context.Context, id string) (*models.User, error)
}

// Compile-time interface satisfaction checks.
var _ walletStore = (*postgres.WalletRepository)(nil)
var _ userStore = (*postgres.UserRepository)(nil)

// CategorySpendStats is re-exported here so tests don't need to import postgres.
type CategorySpendStats = postgres.CategorySpendStats
