package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LoyaltyRepository struct {
	db *pgxpool.Pool
}

func NewLoyaltyRepository(db *pgxpool.Pool) *LoyaltyRepository {
	return &LoyaltyRepository{db: db}
}

// GetOrCreate returns the loyalty account for a user, creating one if it
// doesn't exist yet. Safe to call on every transaction.
func (r *LoyaltyRepository) GetOrCreate(ctx context.Context, userID uuid.UUID) (*models.LoyaltyAccount, error) {
	// Try to find existing account first.
	row := r.db.QueryRow(ctx,
		`SELECT id, user_id, points_balance, lifetime_points, tier, created_at, updated_at
		 FROM loyalty_accounts WHERE user_id = $1`, userID,
	)
	acc := &models.LoyaltyAccount{}
	err := row.Scan(&acc.ID, &acc.UserID, &acc.PointsBalance,
		&acc.LifetimePoints, &acc.Tier, &acc.CreatedAt, &acc.UpdatedAt)
	if err == nil {
		return acc, nil
	}

	// Doesn't exist — create it.
	acc = &models.LoyaltyAccount{
		ID:             uuid.New(),
		UserID:         userID,
		PointsBalance:  0,
		LifetimePoints: 0,
		Tier:           models.TierBronze,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO loyalty_accounts
		 (id, user_id, points_balance, lifetime_points, tier, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (user_id) DO NOTHING`,
		acc.ID, acc.UserID, acc.PointsBalance, acc.LifetimePoints,
		acc.Tier, acc.CreatedAt, acc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create loyalty account: %w", err)
	}
	return acc, nil
}

// EarnPoints credits points atomically and appends a ledger entry.
func (r *LoyaltyRepository) EarnPoints(
	ctx context.Context,
	userID uuid.UUID,
	points int64,
	sourceTxID *uuid.UUID,
	category, narration string,
) (*models.LoyaltyTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock and fetch account.
	var acc models.LoyaltyAccount
	err = tx.QueryRow(ctx,
		`SELECT id, points_balance, lifetime_points FROM loyalty_accounts
		 WHERE user_id=$1 FOR UPDATE`, userID,
	).Scan(&acc.ID, &acc.PointsBalance, &acc.LifetimePoints)
	if err != nil {
		return nil, fmt.Errorf("lock loyalty account: %w", err)
	}

	newBalance := acc.PointsBalance + points
	newLifetime := acc.LifetimePoints + points
	newTier := models.CalculateTier(newLifetime)
	expiresAt := time.Now().Add(365 * 24 * time.Hour)

	// Update account.
	_, err = tx.Exec(ctx,
		`UPDATE loyalty_accounts
		 SET points_balance=$1, lifetime_points=$2, tier=$3, updated_at=NOW()
		 WHERE id=$4`,
		newBalance, newLifetime, newTier, acc.ID,
	)
	if err != nil {
		return nil, err
	}

	// Append ledger entry.
	ltx := &models.LoyaltyTransaction{
		ID:            uuid.New(),
		UserID:        userID,
		AccountID:     acc.ID,
		Type:          models.LoyaltyEarn,
		Points:        points,
		BalanceBefore: acc.PointsBalance,
		BalanceAfter:  newBalance,
		SourceTxID:    sourceTxID,
		Category:      category,
		Narration:     narration,
		ExpiresAt:     &expiresAt,
		CreatedAt:     time.Now(),
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO loyalty_transactions
		 (id, user_id, account_id, type, points, balance_before, balance_after,
		  source_tx_id, category, narration, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ltx.ID, ltx.UserID, ltx.AccountID, ltx.Type, ltx.Points,
		ltx.BalanceBefore, ltx.BalanceAfter, ltx.SourceTxID,
		ltx.Category, ltx.Narration, ltx.ExpiresAt, ltx.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ltx, nil
}

// RedeemPoints debits points atomically and appends a ledger entry.
func (r *LoyaltyRepository) RedeemPoints(
	ctx context.Context,
	userID uuid.UUID,
	points int64,
	narration string,
) (*models.LoyaltyTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var acc models.LoyaltyAccount
	err = tx.QueryRow(ctx,
		`SELECT id, points_balance, lifetime_points FROM loyalty_accounts
		 WHERE user_id=$1 FOR UPDATE`, userID,
	).Scan(&acc.ID, &acc.PointsBalance, &acc.LifetimePoints)
	if err != nil {
		return nil, fmt.Errorf("loyalty account not found: %w", err)
	}
	if acc.PointsBalance < points {
		return nil, fmt.Errorf("insufficient points: have %d, need %d", acc.PointsBalance, points)
	}

	newBalance := acc.PointsBalance - points

	_, err = tx.Exec(ctx,
		`UPDATE loyalty_accounts SET points_balance=$1, updated_at=NOW() WHERE id=$2`,
		newBalance, acc.ID,
	)
	if err != nil {
		return nil, err
	}

	ltx := &models.LoyaltyTransaction{
		ID:            uuid.New(),
		UserID:        userID,
		AccountID:     acc.ID,
		Type:          models.LoyaltyRedeem,
		Points:        points,
		BalanceBefore: acc.PointsBalance,
		BalanceAfter:  newBalance,
		Narration:     narration,
		CreatedAt:     time.Now(),
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO loyalty_transactions
		 (id, user_id, account_id, type, points, balance_before, balance_after,
		  source_tx_id, category, narration, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ltx.ID, ltx.UserID, ltx.AccountID, ltx.Type, ltx.Points,
		ltx.BalanceBefore, ltx.BalanceAfter, nil, nil, ltx.Narration, nil, ltx.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ltx, nil
}

// ListTransactions returns paginated points history for a user.
func (r *LoyaltyRepository) ListTransactions(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int,
) ([]*models.LoyaltyTransaction, int64, error) {
	var total int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM loyalty_transactions WHERE user_id=$1`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, account_id, type, points, balance_before, balance_after,
		        source_tx_id, category, narration, expires_at, created_at
		 FROM loyalty_transactions
		 WHERE user_id=$1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var txs []*models.LoyaltyTransaction
	for rows.Next() {
		t := &models.LoyaltyTransaction{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.AccountID, &t.Type, &t.Points,
			&t.BalanceBefore, &t.BalanceAfter, &t.SourceTxID,
			&t.Category, &t.Narration, &t.ExpiresAt, &t.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		txs = append(txs, t)
	}
	return txs, total, rows.Err()
}
