package postgres

import (
	"context"
	"fmt"

	"github.com/flip-bills/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository handles all PostgreSQL operations for the users table.
type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *models.User) error {
	query := `
		INSERT INTO users
			(id, phone, email, password_hash, first_name, last_name, kyc_tier,
			 bvn, nin, is_active, pin_hash, created_at, updated_at)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	_, err := r.db.Exec(ctx, query,
		u.ID, u.Phone, u.Email, u.PasswordHash,
		u.FirstName, u.LastName, u.KYCTier,
		u.BVN, u.NIN, u.IsActive, u.PinHash,
		u.CreatedAt, u.UpdatedAt,
	)
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	query := `SELECT id,phone,email,password_hash,first_name,last_name,kyc_tier,
	                 bvn,nin,is_active,pin_hash,created_at,updated_at
	          FROM users WHERE id=$1 AND deleted_at IS NULL`
	row := r.db.QueryRow(ctx, query, id)
	return scanUser(row)
}

func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*models.User, error) {
	query := `SELECT id,phone,email,password_hash,first_name,last_name,kyc_tier,
	                 bvn,nin,is_active,pin_hash,created_at,updated_at
	          FROM users WHERE phone=$1 AND deleted_at IS NULL`
	row := r.db.QueryRow(ctx, query, phone)
	return scanUser(row)
}

func (r *UserRepository) UpdateKYCTier(ctx context.Context, userID string, tier models.KYCTier) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET kyc_tier=$1, updated_at=NOW() WHERE id=$2`,
		tier, userID)
	return err
}

// scanUser is a DRY row scanner.
type scannable interface {
	Scan(dest ...interface{}) error
}

func scanUser(row scannable) (*models.User, error) {
	u := &models.User{}
	err := row.Scan(
		&u.ID, &u.Phone, &u.Email, &u.PasswordHash,
		&u.FirstName, &u.LastName, &u.KYCTier,
		&u.BVN, &u.NIN, &u.IsActive, &u.PinHash,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return u, nil
}
