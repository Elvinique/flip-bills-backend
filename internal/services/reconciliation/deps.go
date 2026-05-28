package reconciliation

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type walletRepo interface {
	DebitWithLock(ctx context.Context, userID string, amount int64) (*models.Wallet, error)
	CreditBalance(ctx context.Context, walletID uuid.UUID, amount int64) error
	InsertTransaction(ctx context.Context, tx *models.Transaction) error
	UpdateTransactionStatus(ctx context.Context, ref string, status models.TransactionStatus, extRef string) error
	ReverseDebitIfNeeded(ctx context.Context, original *models.Transaction, reversal *models.Transaction) (bool, error)
}

// Compile-time check.
var _ walletRepo = (*postgres.WalletRepository)(nil)
