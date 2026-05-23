package postgres

import (
	"context"
	"fmt"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WalletRepository handles ACID-safe balance operations.
type WalletRepository struct {
	db *pgxpool.Pool
}

func NewWalletRepository(db *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) Create(ctx context.Context, w *models.Wallet) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO wallets (id,user_id,balance,ledger_balance,currency,daily_spend,daily_limit,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		w.ID, w.UserID, w.Balance, w.LedgerBalance, w.Currency,
		w.DailySpend, w.DailyLimit, w.CreatedAt, w.UpdatedAt,
	)
	return err
}

func (r *WalletRepository) FindByUserID(ctx context.Context, userID string) (*models.Wallet, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id,user_id,balance,ledger_balance,currency,daily_spend,daily_limit,created_at,updated_at
		 FROM wallets WHERE user_id=$1`, userID)
	w := &models.Wallet{}
	err := row.Scan(&w.ID, &w.UserID, &w.Balance, &w.LedgerBalance, &w.Currency,
		&w.DailySpend, &w.DailyLimit, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan wallet: %w", err)
	}
	return w, nil
}

// DebitWithLock performs a SELECT FOR UPDATE then deducts the amount atomically.
// This is the central guard against double-spends and race conditions.
func (r *WalletRepository) DebitWithLock(ctx context.Context, userID string, amount int64) (*models.Wallet, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var w models.Wallet
	err = tx.QueryRow(ctx,
		`SELECT id,balance,daily_spend,daily_limit FROM wallets WHERE user_id=$1 FOR UPDATE`,
		userID,
	).Scan(&w.ID, &w.Balance, &w.DailySpend, &w.DailyLimit)
	if err != nil {
		return nil, fmt.Errorf("lock wallet: %w", err)
	}

	if w.Balance < amount {
		return nil, fmt.Errorf("insufficient funds")
	}
	if w.DailySpend+amount > w.DailyLimit {
		return nil, fmt.Errorf("daily transaction limit exceeded for your KYC tier")
	}

	_, err = tx.Exec(ctx,
		`UPDATE wallets SET balance=balance-$1, daily_spend=daily_spend+$1, updated_at=NOW()
		 WHERE id=$2`,
		amount, w.ID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	w.Balance -= amount
	w.DailySpend += amount
	return &w, nil
}

// CreditBalance adds funds to a user's wallet (for funding and reversals).
func (r *WalletRepository) CreditBalance(ctx context.Context, walletID uuid.UUID, amount int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		amount, walletID,
	)
	return err
}

// InsertTransaction appends an immutable ledger entry.
func (r *WalletRepository) InsertTransaction(ctx context.Context, tx *models.Transaction) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO transactions
		 (id,user_id,wallet_id,reference,external_ref,type,category,amount,fee,
		  balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		tx.ID, tx.UserID, tx.WalletID, tx.Reference, tx.ExternalRef,
		tx.Type, tx.Category, tx.Amount, tx.Fee,
		tx.BalanceBefore, tx.BalanceAfter, tx.Status, tx.Provider,
		tx.Narration, tx.Meta, tx.ReversedTxID, tx.CreatedAt, tx.UpdatedAt,
	)
	return err
}

// UpdateTransactionStatus is the only mutation allowed on a transaction row.
func (r *WalletRepository) UpdateTransactionStatus(ctx context.Context, ref string, status models.TransactionStatus, extRef string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE transactions SET status=$1, external_ref=$2, updated_at=NOW() WHERE reference=$3`,
		status, extRef, ref,
	)
	return err
}
