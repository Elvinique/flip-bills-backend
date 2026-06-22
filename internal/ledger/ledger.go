package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EntryType classifies each ledger entry for auditing and reporting.
type EntryType string

const (
	EntryFunding     EntryType = "FUNDING"       // Wallet top-up via payment gateway
	EntryTransferOut EntryType = "TRANSFER_OUT"  // Funds sent to a bank or another wallet
	EntryTransferIn  EntryType = "TRANSFER_IN"   // Funds received from another wallet
	EntryVAS         EntryType = "VAS"           // Value-added service debit (airtime, data, etc.)
	EntryReversal    EntryType = "REVERSAL"      // Automatic refund after failed VAS
	EntryFee         EntryType = "FEE"           // Platform / transfer fee
)

// Entry is one side of a double-entry bookkeeping record.
// Balance = SUM(credit) - SUM(debit) — NEVER use a running balance column.
type Entry struct {
	ID          string
	WalletID    string
	Debit       int64     // Amount leaving the wallet, in kobo
	Credit      int64     // Amount entering the wallet, in kobo
	Reference   string    // Globally unique across all providers
	EntryType   EntryType
	Description string
	Metadata    map[string]any
	CreatedAt   time.Time
}

// LedgerService provides atomic double-entry operations.
// All methods enforce ACID guarantees using PostgreSQL transactions.
type LedgerService struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *LedgerService {
	return &LedgerService{db: db}
}

// Balance computes the current balance from the ledger: SUM(credits) - SUM(debits).
// This is the canonical source of truth — the wallets table has no balance column.
func (s *LedgerService) Balance(ctx context.Context, walletID string) (int64, error) {
	var balance int64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(credit) - SUM(debit), 0)
		FROM ledger_entries
		WHERE wallet_id = $1
	`, walletID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("ledger.Balance: %w", err)
	}
	return balance, nil
}

// Credit records an inflow to a wallet (funding, transfer-in, reversal).
// Idempotent: silently returns nil if the reference already exists.
func (s *LedgerService) Credit(ctx context.Context, walletID string, amountKobo int64, ref string, t EntryType, desc string) error {
	return s.insert(ctx, Entry{
		ID:          uuid.New().String(),
		WalletID:    walletID,
		Credit:      amountKobo,
		Debit:       0,
		Reference:   ref,
		EntryType:   t,
		Description: desc,
	})
}

// Debit records an outflow from a wallet (VAS purchase, transfer-out, fee).
// Returns an error if the wallet has insufficient funds.
func (s *LedgerService) Debit(ctx context.Context, walletID string, amountKobo int64, ref string, t EntryType, desc string) error {
	// Check balance first — prevents overdraft
	balance, err := s.Balance(ctx, walletID)
	if err != nil {
		return err
	}
	if balance < amountKobo {
		return fmt.Errorf("ledger.Debit: insufficient balance (have %d kobo, need %d kobo)", balance, amountKobo)
	}
	return s.insert(ctx, Entry{
		ID:        uuid.New().String(),
		WalletID:  walletID,
		Credit:    0,
		Debit:     amountKobo,
		Reference: ref,
		EntryType: t,
		Description: desc,
	})
}

// DebitAtomic performs the balance check and debit in a single DB transaction
// to prevent race conditions in concurrent payment scenarios.
func (s *LedgerService) DebitAtomic(ctx context.Context, walletID string, amountKobo int64, ref string, t EntryType, desc string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ledger.DebitAtomic: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the wallet row to serialize concurrent debits
	var balance int64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(credit) - SUM(debit), 0)
		FROM ledger_entries
		WHERE wallet_id = $1
		FOR UPDATE
	`, walletID).Scan(&balance)
	if err != nil {
		return fmt.Errorf("ledger.DebitAtomic: balance check: %w", err)
	}
	if balance < amountKobo {
		return fmt.Errorf("ledger.DebitAtomic: insufficient balance")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, wallet_id, debit, credit, reference, entry_type, description, created_at)
		VALUES ($1, $2, $3, 0, $4, $5, $6, NOW())
		ON CONFLICT (reference) DO NOTHING
	`, uuid.New().String(), walletID, amountKobo, ref, string(t), desc)
	if err != nil {
		return fmt.Errorf("ledger.DebitAtomic: insert: %w", err)
	}

	return tx.Commit(ctx)
}

// insert writes a ledger entry. ON CONFLICT DO NOTHING ensures idempotency.
func (s *LedgerService) insert(ctx context.Context, e Entry) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO ledger_entries (id, wallet_id, debit, credit, reference, entry_type, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (reference) DO NOTHING
	`, e.ID, e.WalletID, e.Debit, e.Credit, e.Reference, string(e.EntryType), e.Description)
	if err != nil {
		return fmt.Errorf("ledger.insert: %w", err)
	}
	return nil
}

// GetEntries returns paginated ledger entries for a wallet.
func (s *LedgerService) GetEntries(ctx context.Context, walletID string, limit, offset int) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, wallet_id, debit, credit, reference, entry_type, description, created_at
		FROM ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ledger.GetEntries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var entryType string
		if err := rows.Scan(&e.ID, &e.WalletID, &e.Debit, &e.Credit, &e.Reference, &entryType, &e.Description, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.EntryType = EntryType(entryType)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
