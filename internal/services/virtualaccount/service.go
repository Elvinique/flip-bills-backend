package virtualaccount

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/flip-bills/backend/internal/providers"
)

type Service struct {
	db       *pgxpool.Pool
	provider providers.PaymentProvider // e.g. Monnify or Paystack
}

func NewService(db *pgxpool.Pool, p providers.PaymentProvider) *Service {
	return &Service{
		db:       db,
		provider: p,
	}
}

// GetUserVirtualAccount returns the user's virtual account if it exists.
func (s *Service) GetUserVirtualAccount(ctx context.Context, userID string) (*providers.VirtualAccountResponse, error) {
	var va providers.VirtualAccountResponse
	err := s.db.QueryRow(ctx, `
		SELECT account_number, account_name, bank_name, provider_ref
		FROM virtual_accounts
		WHERE user_id = $1 AND is_active = true
	`, userID).Scan(&va.AccountNumber, &va.AccountName, &va.BankName, &va.ProviderRef)
	
	if err != nil {
		// pgx.ErrNoRows or similar will be returned here if none exists
		return nil, fmt.Errorf("virtualaccount: not found or db error: %w", err)
	}

	return &va, nil
}

// CreateVirtualAccount provisions a new virtual account for the user and saves it.
func (s *Service) CreateVirtualAccount(ctx context.Context, userID, fullName, email, phone, bvn string) (*providers.VirtualAccountResponse, error) {
	// First check if they already have one
	if existing, err := s.GetUserVirtualAccount(ctx, userID); err == nil {
		return existing, nil
	}

	req := providers.VirtualAccountRequest{
		UserID:      userID,
		FullName:    fullName,
		Email:       email,
		PhoneNumber: phone,
		BVN:         bvn,
	}

	resp, err := s.provider.CreateVirtualAccount(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("virtualaccount: failed to provision: %w", err)
	}

	// Save to DB
	_, err = s.db.Exec(ctx, `
		INSERT INTO virtual_accounts (user_id, account_number, account_name, bank_name, bank_code, provider, provider_ref)
		VALUES ($1, $2, $3, $4, '', $5, $6)
		ON CONFLICT (user_id) DO NOTHING
	`, userID, resp.AccountNumber, resp.AccountName, resp.BankName, s.provider.Name(), resp.ProviderRef)

	if err != nil {
		return nil, fmt.Errorf("virtualaccount: failed to save record: %w", err)
	}

	return resp, nil
}
