package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletRepository struct {
	db *pgxpool.Pool
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

type CategorySpendStats struct {
	Count  int64
	Total  int64
	Avg    int64
	Max    int64
	Since  time.Time
	UserID string
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

// DebitWithLock performs a SELECT FOR UPDATE then deducts atomically.
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
		`UPDATE wallets SET balance=balance-$1, daily_spend=daily_spend+$1, updated_at=NOW() WHERE id=$2`,
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

// CreditBalance adds funds — used for wallet funding and reversals.
func (r *WalletRepository) CreditBalance(ctx context.Context, walletID uuid.UUID, amount int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		amount, walletID,
	)
	return err
}

// UpdateDailyLimit bumps the limit when a user's KYC tier is upgraded.
func (r *WalletRepository) UpdateDailyLimit(ctx context.Context, userID string, newLimit int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE wallets SET daily_limit=$1, updated_at=NOW() WHERE user_id=$2`,
		newLimit, userID,
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

func (r *WalletRepository) UpdateTransactionMeta(ctx context.Context, ref string, meta []byte) error {
	_, err := r.db.Exec(ctx,
		`UPDATE transactions SET meta=$1, updated_at=NOW() WHERE reference=$2`,
		meta, ref,
	)
	return err
}

func (r *WalletRepository) ReverseDebitIfNeeded(ctx context.Context, original *models.Transaction, reversal *models.Transaction) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var existingID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT id FROM transactions WHERE user_id=$1 AND reference=$2 FOR UPDATE`,
		original.UserID, reversal.Reference,
	).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(ctx,
			`UPDATE transactions SET status=$1, updated_at=NOW() WHERE reference=$2`,
			models.TxReversed, original.Reference,
		)
		if err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO transactions
		 (id,user_id,wallet_id,reference,external_ref,type,category,amount,fee,
		  balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		reversal.ID, reversal.UserID, reversal.WalletID, reversal.Reference, reversal.ExternalRef,
		reversal.Type, reversal.Category, reversal.Amount, reversal.Fee,
		reversal.BalanceBefore, reversal.BalanceAfter, reversal.Status, reversal.Provider,
		reversal.Narration, reversal.Meta, reversal.ReversedTxID, reversal.CreatedAt, reversal.UpdatedAt,
	)
	if err != nil {
		return false, err
	}

	_, err = tx.Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		original.Amount, original.WalletID,
	)
	if err != nil {
		return false, err
	}

	_, err = tx.Exec(ctx,
		`UPDATE transactions SET status=$1, updated_at=NOW() WHERE reference=$2`,
		models.TxReversed, original.Reference,
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *WalletRepository) CategorySpendStats(ctx context.Context, userID string, category models.ServiceCategory, since time.Time) (*CategorySpendStats, error) {
	stats := &CategorySpendStats{
		Since:  since,
		UserID: userID,
	}
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(amount), 0), COALESCE(AVG(amount), 0), COALESCE(MAX(amount), 0)
		 FROM transactions
		 WHERE user_id=$1
		   AND category=$2
		   AND type=$3
		   AND status=$4
		   AND created_at >= $5`,
		userID, category, models.TxTypeDebit, models.TxSuccess, since,
	).Scan(&stats.Count, &stats.Total, &stats.Avg, &stats.Max)
	if err != nil {
		return nil, fmt.Errorf("category spend stats: %w", err)
	}
	return stats, nil
}

func (r *WalletRepository) FindTransactionByReference(ctx context.Context, userID string, reference string) (*models.Transaction, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id,user_id,wallet_id,reference,external_ref,type,category,amount,fee,
		        balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at
		 FROM transactions
		 WHERE user_id=$1 AND reference=$2`,
		userID, reference,
	)
	t := &models.Transaction{}
	if err := row.Scan(
		&t.ID, &t.UserID, &t.WalletID, &t.Reference, &t.ExternalRef,
		&t.Type, &t.Category, &t.Amount, &t.Fee,
		&t.BalanceBefore, &t.BalanceAfter, &t.Status, &t.Provider,
		&t.Narration, &t.Meta, &t.ReversedTxID, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("find transaction by reference: %w", err)
	}
	return t, nil
}

// ListTransactions returns paginated transaction history for a user.
func (r *WalletRepository) ListTransactions(ctx context.Context, userID string, limit, offset int) ([]*models.Transaction, int64, error) {
	// Total count
	var total int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions WHERE user_id=$1`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT id,user_id,wallet_id,reference,external_ref,type,category,amount,fee,
		        balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at
		 FROM transactions
		 WHERE user_id=$1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var txs []*models.Transaction
	for rows.Next() {
		t := &models.Transaction{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.WalletID, &t.Reference, &t.ExternalRef,
			&t.Type, &t.Category, &t.Amount, &t.Fee,
			&t.BalanceBefore, &t.BalanceAfter, &t.Status, &t.Provider,
			&t.Narration, &t.Meta, &t.ReversedTxID, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		txs = append(txs, t)
	}
	return txs, total, rows.Err()
}

// CreditWithTransaction atomically credits the wallet balance AND inserts the
// transaction ledger entry in a single database transaction.
// This replaces the two-step CreditBalance + InsertTransaction pattern in
// FundWallet and prevents a crash between the two writes from leaving
// money credited with no audit record.
func (r *WalletRepository) CreditWithTransaction(ctx context.Context, walletID uuid.UUID, tx *models.Transaction) error {
	dbTx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer dbTx.Rollback(ctx)

	// 1. Credit balance.
	_, err = dbTx.Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		tx.Amount, walletID,
	)
	if err != nil {
		return fmt.Errorf("credit balance: %w", err)
	}

	// 2. Insert ledger entry — same transaction, guaranteed to succeed or both roll back.
	_, err = dbTx.Exec(ctx,
		`INSERT INTO transactions
		 (id,user_id,wallet_id,reference,external_ref,type,category,amount,fee,
		  balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		tx.ID, tx.UserID, tx.WalletID, tx.Reference, tx.ExternalRef,
		tx.Type, tx.Category, tx.Amount, tx.Fee,
		tx.BalanceBefore, tx.BalanceAfter, tx.Status, tx.Provider,
		tx.Narration, tx.Meta, tx.ReversedTxID, tx.CreatedAt, tx.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	return dbTx.Commit(ctx)
}
