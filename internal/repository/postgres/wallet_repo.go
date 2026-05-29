package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// TxRunner defines the common database behaviors shared by *pgxpool.Pool and pgx.Tx
type TxRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

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

// WithTx embeds an active pgx.Tx inside the context for repository operations.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// getRunner extracts the active transaction from context, or returns the db pool fallback.
func (r *WalletRepository) getRunner(ctx context.Context) TxRunner {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return r.db
}

// WithinTx executes a set of repository actions inside a single standalone transaction block.
func (r *WalletRepository) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin context transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(WithTx(ctx, tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit context transaction: %w", err)
	}
	return nil
}

func (r *WalletRepository) Create(ctx context.Context, w *models.Wallet) error {
	_, err := r.getRunner(ctx).Exec(ctx,
		`INSERT INTO wallets (id,user_id,balance,ledger_balance,currency,daily_spend,daily_limit,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		w.ID, w.UserID, w.Balance, w.LedgerBalance, w.Currency,
		w.DailySpend, w.DailyLimit, w.CreatedAt, w.UpdatedAt,
	)
	return err
}

func (r *WalletRepository) FindByUserID(ctx context.Context, userID string) (*models.Wallet, error) {
	row := r.getRunner(ctx).QueryRow(ctx,
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
	runner := r.getRunner(ctx)
	tx, isLocalTx := runner.(pgx.Tx)

	if !isLocalTx {
		var err error
		tx, err = r.db.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		runner = tx
	}

	var w models.Wallet
	err := runner.QueryRow(ctx,
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

	_, err = runner.Exec(ctx,
		`UPDATE wallets SET balance=balance-$1, daily_spend=daily_spend+$1, updated_at=NOW() WHERE id=$2`,
		amount, w.ID,
	)
	if err != nil {
		return nil, err
	}

	if !isLocalTx {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
	}

	w.Balance -= amount
	w.DailySpend += amount
	return &w, nil
}

// CreditBalance adds funds — used for wallet funding and reversals.
func (r *WalletRepository) CreditBalance(ctx context.Context, walletID uuid.UUID, amount int64) error {
	_, err := r.getRunner(ctx).Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		amount, walletID,
	)
	return err
}

// UpdateDailyLimit bumps the limit when a user's KYC tier is upgraded.
func (r *WalletRepository) UpdateDailyLimit(ctx context.Context, userID string, newLimit int64) error {
	_, err := r.getRunner(ctx).Exec(ctx,
		`UPDATE wallets SET daily_limit=$1, updated_at=NOW() WHERE user_id=$2`,
		newLimit, userID,
	)
	return err
}

// InsertTransaction appends an immutable ledger entry.
func (r *WalletRepository) InsertTransaction(ctx context.Context, tx *models.Transaction) error {
	_, err := r.getRunner(ctx).Exec(ctx,
		`INSERT INTO transactions
         (id,user_id,wallet_id,reference,external_ref,type,category,amount,commission_kobo,fee,
          balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		tx.ID, tx.UserID, tx.WalletID, tx.Reference, tx.ExternalRef,
		tx.Type, tx.Category, tx.Amount, tx.CommissionKobo, tx.Fee,
		tx.BalanceBefore, tx.BalanceAfter, tx.Status, tx.Provider,
		tx.Narration, tx.Meta, tx.ReversedTxID, tx.CreatedAt, tx.UpdatedAt,
	)
	return err
}

// UpdateTransactionStatus is the only mutation allowed on a transaction row.
func (r *WalletRepository) UpdateTransactionStatus(ctx context.Context, ref string, status models.TransactionStatus, extRef string) error {
	_, err := r.getRunner(ctx).Exec(ctx,
		`UPDATE transactions SET status=$1, external_ref=$2, updated_at=NOW() WHERE reference=$3`,
		status, extRef, ref,
	)
	return err
}

func (r *WalletRepository) UpdateTransactionMeta(ctx context.Context, ref string, meta []byte) error {
	_, err := r.getRunner(ctx).Exec(ctx,
		`UPDATE transactions SET meta=$1, updated_at=NOW() WHERE reference=$2`,
		meta, ref,
	)
	return err
}

func (r *WalletRepository) ReverseDebitIfNeeded(ctx context.Context, original *models.Transaction, reversal *models.Transaction) (bool, error) {
	runner := r.getRunner(ctx)
	tx, isLocalTx := runner.(pgx.Tx)

	if !isLocalTx {
		var err error
		tx, err = r.db.Begin(ctx)
		if err != nil {
			return false, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		runner = tx
	}

	var existingID uuid.UUID
	err := runner.QueryRow(ctx,
		`SELECT id FROM transactions WHERE user_id=$1 AND reference=$2 FOR UPDATE`,
		original.UserID, reversal.Reference,
	).Scan(&existingID)
	if err == nil {
		_, err = runner.Exec(ctx,
			`UPDATE transactions SET status=$1, updated_at=NOW() WHERE reference=$2`,
			models.TxReversed, original.Reference,
		)
		if err != nil {
			return false, err
		}
		if !isLocalTx {
			if err := tx.Commit(ctx); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	_, err = runner.Exec(ctx,
		`INSERT INTO transactions
         (id,user_id,wallet_id,reference,external_ref,type,category,amount,commission_kobo,fee,
          balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		reversal.ID, reversal.UserID, reversal.WalletID, reversal.Reference, reversal.ExternalRef,
		reversal.Type, reversal.Category, reversal.Amount, reversal.CommissionKobo, reversal.Fee,
		reversal.BalanceBefore, reversal.BalanceAfter, reversal.Status, reversal.Provider,
		reversal.Narration, reversal.Meta, reversal.ReversedTxID, reversal.CreatedAt, reversal.UpdatedAt,
	)
	if err != nil {
		return false, err
	}

	_, err = runner.Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		original.Amount, original.WalletID,
	)
	if err != nil {
		return false, err
	}

	_, err = runner.Exec(ctx,
		`UPDATE transactions SET status=$1, updated_at=NOW() WHERE reference=$2`,
		models.TxReversed, original.Reference,
	)
	if err != nil {
		return false, err
	}

	if !isLocalTx {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *WalletRepository) CategorySpendStats(ctx context.Context, userID string, category models.ServiceCategory, since time.Time) (*CategorySpendStats, error) {
	stats := &CategorySpendStats{
		Since:  since,
		UserID: userID,
	}
	err := r.getRunner(ctx).QueryRow(ctx,
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
	var query string
	var args []any

	if userID == "" {
		query = `SELECT id,user_id,wallet_id,reference,external_ref,type,category,amount,commission_kobo,fee,
                    balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at
             FROM transactions WHERE reference=$1`
		args = []any{reference}
	} else {
		query = `SELECT id,user_id,wallet_id,reference,external_ref,type,category,amount,commission_kobo,fee,
                    balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at
             FROM transactions WHERE user_id=$1 AND reference=$2`
		args = []any{userID, reference}
	}

	row := r.getRunner(ctx).QueryRow(ctx, query, args...)
	t := &models.Transaction{}
	if err := row.Scan(
		&t.ID, &t.UserID, &t.WalletID, &t.Reference, &t.ExternalRef,
		&t.Type, &t.Category, &t.Amount, &t.CommissionKobo, &t.Fee,
		&t.BalanceBefore, &t.BalanceAfter, &t.Status, &t.Provider,
		&t.Narration, &t.Meta, &t.ReversedTxID, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("find transaction by reference: %w", err)
	}
	return t, nil
}

func (r *WalletRepository) ListTransactions(ctx context.Context, userID string, limit, offset int) ([]*models.Transaction, int64, error) {
	var total int64
	err := r.getRunner(ctx).QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions WHERE user_id=$1`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.getRunner(ctx).Query(ctx,
		`SELECT id,user_id,wallet_id,reference,external_ref,type,category,amount,commission_kobo,fee,
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
			&t.Type, &t.Category, &t.Amount, &t.CommissionKobo, &t.Fee,
			&t.BalanceBefore, &t.BalanceAfter, &t.Status, &t.Provider,
			&t.Narration, &t.Meta, &t.ReversedTxID, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		txs = append(txs, t)
	}
	return txs, total, rows.Err()
}

// CreditWithTransaction atomically credits the wallet balance AND inserts the transaction ledger entry.
func (r *WalletRepository) CreditWithTransaction(ctx context.Context, walletID uuid.UUID, tx *models.Transaction) error {
	runner := r.getRunner(ctx)
	dbTx, isLocalTx := runner.(pgx.Tx)

	if !isLocalTx {
		var err error
		dbTx, err = r.db.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = dbTx.Rollback(ctx) }()
		runner = dbTx
	}

	_, err := runner.Exec(ctx,
		`UPDATE wallets SET balance=balance+$1, updated_at=NOW() WHERE id=$2`,
		tx.Amount, walletID,
	)
	if err != nil {
		return fmt.Errorf("credit balance: %w", err)
	}

	_, err = runner.Exec(ctx,
		`INSERT INTO transactions
         (id,user_id,wallet_id,reference,external_ref,type,category,amount,commission_kobo,fee,
          balance_before,balance_after,status,provider,narration,meta,reversed_tx_id,created_at,updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		tx.ID, tx.UserID, tx.WalletID, tx.Reference, tx.ExternalRef,
		tx.Type, tx.Category, tx.Amount, tx.CommissionKobo, tx.Fee,
		tx.BalanceBefore, tx.BalanceAfter, tx.Status, tx.Provider,
		tx.Narration, tx.Meta, tx.ReversedTxID, tx.CreatedAt, tx.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	if !isLocalTx {
		return dbTx.Commit(ctx)
	}
	return nil
}
