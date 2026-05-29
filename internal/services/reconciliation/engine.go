package reconciliation

// AsyncReconciliationEngine implements the PRD's "VAS Blackhole" solution.
// When a third-party billing API fails to confirm within 45 seconds,
// this engine reverses the wallet debit and switches to a fallback aggregator.

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BillerCallFn is the function signature for any third-party billing call.
// It returns an external reference string on success.
type BillerCallFn func(ctx context.Context) (externalRef string, err error)

// Engine holds shared dependencies for reconciliation operations.
type Engine struct {
	walletRepo walletRepo
	log        *zap.Logger
	timeout    time.Duration
}

func NewEngine(repo *postgres.WalletRepository, log *zap.Logger, timeoutSeconds int) *Engine {
	return &Engine{
		walletRepo: repo,
		log:        log,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

// ExecuteWithFallback runs primaryCall first. If it times out or errors, it
// automatically triggers a wallet reversal and runs fallbackCall.
// This directly implements PRD Section 3A.
func (e *Engine) ExecuteWithFallback(
	ctx context.Context,
	tx *models.Transaction,
	primaryCall BillerCallFn,
	fallbackCall BillerCallFn,
) (string, error) {
	// ── Attempt primary aggregator ────────────────────────────────────────────
	primaryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	extRef, err := primaryCall(primaryCtx)
	if err == nil {
		// Primary succeeded — update transaction to success.
		e.log.Info("primary aggregator succeeded",
			zap.String("ref", tx.Reference),
			zap.String("ext_ref", extRef),
		)
		_ = e.walletRepo.UpdateTransactionStatus(ctx, tx.Reference, models.TxSuccess, extRef)
		return extRef, nil
	}

	e.log.Warn("primary aggregator failed — attempting reversal and fallback",
		zap.String("ref", tx.Reference),
		zap.Error(err),
	)

	// ── Primary failed — reverse the original debit ───────────────────────────
	if reverseErr := e.reverseDebit(ctx, tx); reverseErr != nil {
		e.log.Error("CRITICAL: reversal failed after primary aggregator failure",
			zap.String("ref", tx.Reference),
			zap.Error(reverseErr),
		)
		return "", fmt.Errorf("payment failed and reversal also failed — support notified")
	}

	if fallbackCall == nil {
		return "", fmt.Errorf("payment failed — your wallet has been refunded")
	}

	// ── Re-debit and attempt fallback aggregator ──────────────────────────────
	fallbackRef := fmt.Sprintf("%s_FB", tx.Reference)
	fallbackTx := &models.Transaction{
		ID:        uuid.New(),
		UserID:    tx.UserID,
		WalletID:  tx.WalletID,
		Reference: fallbackRef,
		Type:      models.TxTypeDebit,
		Category:  tx.Category,
		Amount:    tx.Amount,
		Fee:       tx.Fee,
		Status:    models.TxProcessing,
		Provider:  "fallback",
		Narration: tx.Narration + " (fallback attempt)",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, debitErr := e.walletRepo.DebitWithLock(ctx, tx.UserID, tx.Amount)
	if debitErr != nil {
		return "", fmt.Errorf("fallback re-debit failed: %w", debitErr)
	}
	_ = e.walletRepo.InsertTransaction(ctx, fallbackTx)

	fallbackCtx, fCancel := context.WithTimeout(ctx, e.timeout)
	defer fCancel()

	extRef, err = fallbackCall(fallbackCtx)
	if err != nil {
		// Both failed — reverse the fallback debit too.
		_ = e.reverseDebit(ctx, fallbackTx)
		return "", fmt.Errorf("payment failed on all aggregators — wallet refunded")
	}

	_ = e.walletRepo.UpdateTransactionStatus(ctx, fallbackRef, models.TxSuccess, extRef)
	e.log.Info("fallback aggregator succeeded", zap.String("ref", fallbackRef))
	return extRef, nil
}

// reverseDebit credits the wallet back and writes a reversal transaction.
func (e *Engine) reverseDebit(ctx context.Context, original *models.Transaction) error {
	reversalRef := fmt.Sprintf("REV_%s", original.Reference)
	reversedTxID := original.ID
	reversal := &models.Transaction{
		ID:           uuid.New(),
		UserID:       original.UserID,
		WalletID:     original.WalletID,
		Reference:    reversalRef,
		Type:         models.TxTypeReversal,
		Category:     original.Category,
		Amount:       original.Amount,
		Status:       models.TxSuccess,
		Narration:    fmt.Sprintf("Auto-reversal for failed tx: %s", original.Reference),
		ReversedTxID: &reversedTxID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	created, err := e.walletRepo.ReverseDebitIfNeeded(ctx, original, reversal)
	if err != nil {
		return err
	}
	if !created {
		e.log.Warn("reversal already exists; skipping duplicate wallet credit",
			zap.String("ref", original.Reference),
			zap.String("reversal_ref", reversalRef),
		)
	}
	return nil
}
