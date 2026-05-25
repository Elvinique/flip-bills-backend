package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OTPRepository struct {
	db *pgxpool.Pool
}

func NewOTPRepository(db *pgxpool.Pool) *OTPRepository {
	return &OTPRepository{db: db}
}

// Create inserts a new OTP record. Any previous unused OTPs for the same
// phone+purpose are invalidated first to prevent brute-force stacking.
func (r *OTPRepository) Create(ctx context.Context, otp *models.OTPToken) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Invalidate previous unused tokens for this phone + purpose.
	_, err = tx.Exec(ctx,
		`UPDATE otp_tokens SET used=TRUE WHERE phone=$1 AND purpose=$2 AND used=FALSE`,
		otp.Phone, otp.Purpose,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO otp_tokens (id, phone, otp_hash, purpose, expires_at, used, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		otp.ID, otp.Phone, otp.OTPHash, otp.Purpose, otp.ExpiresAt, false, otp.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// FindValid fetches the most recent unused, unexpired OTP for phone+purpose.
func (r *OTPRepository) FindValid(ctx context.Context, phone string, purpose models.OTPPurpose) (*models.OTPToken, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, phone, otp_hash, purpose, expires_at, used, created_at
		 FROM otp_tokens
		 WHERE phone=$1 AND purpose=$2 AND used=FALSE AND expires_at > NOW()
		 ORDER BY created_at DESC LIMIT 1`,
		phone, purpose,
	)
	o := &models.OTPToken{}
	err := row.Scan(&o.ID, &o.Phone, &o.OTPHash, &o.Purpose, &o.ExpiresAt, &o.Used, &o.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("no valid OTP found: %w", err)
	}
	return o, nil
}

// MarkUsed burns an OTP after successful verification — single use only.
func (r *OTPRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE otp_tokens SET used=TRUE WHERE id=$1`, id)
	return err
}

// PruneExpired is a maintenance helper (run via cron or background goroutine).
func (r *OTPRepository) PruneExpired(ctx context.Context) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM otp_tokens WHERE expires_at < $1`, time.Now().Add(-24*time.Hour))
	return err
}
