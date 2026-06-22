package transfer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/flip-bills/backend/internal/ledger"
	"github.com/flip-bills/backend/internal/providers"
)

type Service struct {
	db       *pgxpool.Pool
	ledger   *ledger.LedgerService
	provider providers.PaymentProvider // Paystack or ProviderRouter
}

func NewService(db *pgxpool.Pool, l *ledger.LedgerService, p providers.PaymentProvider) *Service {
	return &Service{
		db:       db,
		ledger:   l,
		provider: p,
	}
}

// InitiateBankTransfer deducts funds from the user's wallet and instructs the provider
// to send funds to the destination bank account.
func (s *Service) InitiateBankTransfer(ctx context.Context, userID, walletID, bankCode, accountNumber, accountName, narration string, amountKobo, feeKobo int64) (*providers.TransferResponse, error) {
	totalDebit := amountKobo + feeKobo
	ref := fmt.Sprintf("TRF-%s", uuid.New().String())

	// 1. Check balance and debit wallet atomically
	// This ensures we have the funds before telling the provider to send money
	err := s.ledger.DebitAtomic(ctx, walletID, totalDebit, ref, ledger.EntryTransferOut, "Bank transfer to "+accountName)
	if err != nil {
		return nil, fmt.Errorf("transfer: insufficient funds or ledger error: %w", err)
	}

	// 2. Record the transfer attempt in the DB
	_, err = s.db.Exec(ctx, `
		INSERT INTO transfers (reference, user_id, wallet_id, amount, fee, status, transfer_type, bank_code, account_number, account_name, narration, provider)
		VALUES ($1, $2, $3, $4, $5, 'pending', 'BANK', $6, $7, $8, $9, $10)
	`, ref, userID, walletID, amountKobo, feeKobo, bankCode, accountNumber, accountName, narration, s.provider.Name())
	if err != nil {
		// Refund if we fail to record the transfer locally
		_ = s.ledger.Credit(ctx, walletID, totalDebit, ref+"-REV", ledger.EntryReversal, "Refund: internal error")
		return nil, fmt.Errorf("transfer: failed to save record: %w", err)
	}

	// 3. Call the provider to execute the transfer
	resp, err := s.provider.Transfer(ctx, providers.TransferRequest{
		Reference:     ref,
		AccountNumber: accountNumber,
		BankCode:      bankCode,
		AccountName:   accountName,
		AmountKobo:    amountKobo,
		Narration:     narration,
	})

	if err != nil {
		// Provider rejected the transfer synchronously.
		// Refund the user and mark transfer as failed.
		_ = s.ledger.Credit(ctx, walletID, totalDebit, ref+"-REV", ledger.EntryReversal, "Refund: transfer failed")
		_, _ = s.db.Exec(ctx, `UPDATE transfers SET status = 'failed', failure_reason = $1, updated_at = NOW() WHERE reference = $2`, err.Error(), ref)
		return nil, fmt.Errorf("transfer: provider error: %w", err)
	}

	// 4. Update the transfer record with provider details
	_, _ = s.db.Exec(ctx, `
		UPDATE transfers 
		SET provider_ref = $1, status = $2, updated_at = NOW() 
		WHERE reference = $3
	`, resp.ProviderRef, resp.Status, ref)

	return resp, nil
}

// HandleWebhook updates the transfer status based on async provider webhooks
func (s *Service) HandleWebhook(ctx context.Context, reference, status, providerRef string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE transfers 
		SET status = $1, provider_ref = $2, updated_at = NOW() 
		WHERE reference = $3 AND status = 'pending'
	`, status, providerRef, reference)
	
	if err != nil {
		return fmt.Errorf("transfer: failed to update status from webhook: %w", err)
	}
	
	// If the transfer ultimately failed asynchronously, we need to refund the user.
	// This would require fetching the transfer details to know the amount and wallet ID.
	if status == "failed" || status == "reversed" {
		var walletID string
		var amount, fee int64
		err = s.db.QueryRow(ctx, `SELECT wallet_id, amount, fee FROM transfers WHERE reference = $1`, reference).Scan(&walletID, &amount, &fee)
		if err == nil {
			total := amount + fee
			_ = s.ledger.Credit(ctx, walletID, total, reference+"-REV-ASYNC", ledger.EntryReversal, "Refund: transfer failed asynchronously")
		}
	}

	return nil
}

// GetSupportedBanks fetches the list of banks from the provider
func (s *Service) GetSupportedBanks(ctx context.Context) ([]providers.BankInfo, error) {
	return s.provider.GetBanks(ctx)
}

// ResolveAccount gets the account name for a given account number and bank code
func (s *Service) ResolveAccount(ctx context.Context, accountNumber, bankCode string) (*providers.AccountInfo, error) {
	return s.provider.ResolveAccount(ctx, accountNumber, bankCode)
}
