package postgres

import (
	"context"
	"fmt"

	"github.com/flip-bills/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *models.User) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users
			(id,phone,email,password_hash,first_name,last_name,kyc_tier,
			 bvn,nin,is_active,pin_hash,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		u.ID, u.Phone, nullableString(u.Email), u.PasswordHash,
		u.FirstName, u.LastName, u.KYCTier,
		nullableString(u.BVN), nullableString(u.NIN),
		u.IsActive, nullableString(u.PinHash),
		u.CreatedAt, u.UpdatedAt,
	)
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id,phone,email,password_hash,first_name,last_name,kyc_tier,
		        bvn,nin,is_active,pin_hash,created_at,updated_at
		 FROM users WHERE id=$1 AND deleted_at IS NULL`, id)
	return scanUser(row)
}

func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*models.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id,phone,email,password_hash,first_name,last_name,kyc_tier,
		        bvn,nin,is_active,pin_hash,created_at,updated_at
		 FROM users WHERE phone=$1 AND deleted_at IS NULL`, phone)
	return scanUser(row)
}

func (r *UserRepository) UpdateKYCTier(ctx context.Context, userID string, tier models.KYCTier) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET kyc_tier=$1, updated_at=NOW() WHERE id=$2`, tier, userID)
	return err
}

func (r *UserRepository) UpdatePIN(ctx context.Context, userID, pinHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET pin_hash=$1, updated_at=NOW() WHERE id=$2`, pinHash, userID)
	return err
}

func (r *UserRepository) UpdateBVN(ctx context.Context, userID, bvn string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET bvn=$1, updated_at=NOW() WHERE id=$2`, bvn, userID)
	return err
}

func (r *UserRepository) UpdateNIN(ctx context.Context, userID, nin string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET nin=$1, updated_at=NOW() WHERE id=$2`, nin, userID)
	return err
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanUser(row scannable) (*models.User, error) {
	u := &models.User{}
	var email, bvn, nin, pinHash *string
	err := row.Scan(
		&u.ID, &u.Phone, &email, &u.PasswordHash,
		&u.FirstName, &u.LastName, &u.KYCTier,
		&bvn, &nin, &u.IsActive, &pinHash,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if email != nil {
		u.Email = *email
	}
	if bvn != nil {
		u.BVN = *bvn
	}
	if nin != nil {
		u.NIN = *nin
	}
	if pinHash != nil {
		u.PinHash = *pinHash
	}
	return u, nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
