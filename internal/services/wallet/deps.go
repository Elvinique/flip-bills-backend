package wallet

import (
	"context"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres" // <-- Add this import!
	"github.com/google/uuid"
)

type walletStore interface {
	FindCreditByExternalRef(ctx context.Context, externalRef string) (*models.Transaction, error)
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
	Create(ctx context.Context, w *models.Wallet) error
	FindByUserID(ctx context.Context, userID string) (*models.Wallet, error)
	DebitWithLock(ctx context.Context, userID string, amount int64) (*models.Wallet, error)
	CreditBalance(ctx context.Context, walletID uuid.UUID, amount int64) error
	UpdateDailyLimit(ctx context.Context, userID string, newLimit int64) error
	InsertTransaction(ctx context.Context, tx *models.Transaction) error
	UpdateTransactionStatus(ctx context.Context, ref string, status models.TransactionStatus, extRef string) error
	UpdateTransactionBalances(ctx context.Context, ref string, balanceBefore, balanceAfter int64) error
	UpdateTransactionMeta(ctx context.Context, ref string, meta []byte) error
	ReverseDebitIfNeeded(ctx context.Context, original *models.Transaction, reversal *models.Transaction) (bool, error)
	CategorySpendStats(ctx context.Context, userID string, category models.ServiceCategory, since time.Time) (*postgres.CategorySpendStats, error)
	FindTransactionByReference(ctx context.Context, userID string, reference string) (*models.Transaction, error)
	ListTransactions(ctx context.Context, userID string, limit, offset int) ([]*models.Transaction, int64, error)
	CreditWithTransaction(ctx context.Context, walletID uuid.UUID, tx *models.Transaction) error
}

type userStore interface {
	FindByID(ctx context.Context, id string) (*models.User, error)
}
