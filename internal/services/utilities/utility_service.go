package utilities

// UtilityService handles airtime, data, electricity, cable TV, and betting top-ups.
// All calls pass through the AsyncReconciliationEngine for guaranteed delivery.
// Points are awarded via LoyaltyService after every successful transaction.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/notifications"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/flip-bills/backend/internal/services/loyalty"
	"github.com/flip-bills/backend/internal/services/reconciliation"
	"github.com/flip-bills/backend/pkg/crypto"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type AirtimeRequest struct {
	Phone           string `json:"phone"           binding:"required"`
	Amount          int64  `json:"amount"          binding:"required,min=50"`
	Network         string `json:"network"         binding:"required,oneof=MTN GLO AIRTEL 9MOBILE"`
	TransactionPIN  string `json:"transaction_pin" binding:"required,len=6,numeric"`
	ClientReference string `json:"client_reference" binding:"omitempty,max=60"`
}

type DataRequest struct {
	Phone           string `json:"phone"            binding:"required"`
	PlanCode        string `json:"plan_code"        binding:"required"`
	BillerCode      string `json:"biller_code"      binding:"required"`
	Network         string `json:"network"          binding:"required,oneof=MTN GLO AIRTEL 9MOBILE"`
	Amount          int64  `json:"amount"           binding:"required,min=1000"`
	TransactionPIN  string `json:"transaction_pin"  binding:"required,len=6,numeric"`
	ClientReference string `json:"client_reference" binding:"omitempty,max=60"`
}

type ElectricityRequest struct {
	MeterNumber     string `json:"meter_number"    binding:"required"`
	DisCo           string `json:"disco"           binding:"required"`
	Amount          int64  `json:"amount"          binding:"required,min=10000"`
	MeterType       string `json:"meter_type"      binding:"required,oneof=prepaid postpaid"`
	TransactionPIN  string `json:"transaction_pin" binding:"required,len=6,numeric"`
	ClientReference string `json:"client_reference" binding:"omitempty,max=60"`
}

type BettingFundRequest struct {
	CustomerID        string `json:"customer_id"         binding:"required"`
	Platform          string `json:"platform"            binding:"required"`
	Amount            int64  `json:"amount"              binding:"required"`
	RiskConfirmed     bool   `json:"risk_confirmed"`
	BiometricVerified bool   `json:"biometric_verified"`
	TransactionPIN    string `json:"transaction_pin"      binding:"required,len=6,numeric"`
	ClientReference   string `json:"client_reference"     binding:"omitempty,max=60"`
}

type RiskChallengeError struct {
	Category       models.ServiceCategory `json:"category"`
	Amount         int64                  `json:"amount"`
	WeeklyAverage  int64                  `json:"weekly_average"`
	WeeklyMax      int64                  `json:"weekly_max"`
	WeeklyCount    int64                  `json:"weekly_count"`
	RequiredFields []string               `json:"required_fields"`
}

func (e *RiskChallengeError) Error() string {
	return "extra confirmation required for unusual betting top-up"
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	walletRepo    *postgres.WalletRepository
	userRepo      *postgres.UserRepository
	recon         *reconciliation.Engine
	loyaltySvc    *loyalty.Service
	sms           *notifications.SMSService
	bills         BillProvider
	fallbackBills BillProvider // Monnify — used when primary (Flutterwave) fails
	log           *zap.Logger
}

func NewService(
	walletRepo *postgres.WalletRepository,
	userRepo *postgres.UserRepository,
	recon *reconciliation.Engine,
	loyaltySvc *loyalty.Service,
	sms *notifications.SMSService,
	bills BillProvider,
	fallbackBills BillProvider,
	log *zap.Logger,
) *Service {
	return &Service{walletRepo: walletRepo, userRepo: userRepo, recon: recon, loyaltySvc: loyaltySvc, sms: sms, bills: bills, fallbackBills: fallbackBills, log: log}
}

func (s *Service) PurchaseAirtime(ctx context.Context, userID string, req AirtimeRequest) (*models.Transaction, error) {
	if err := s.validateTransactionPIN(ctx, userID, req.TransactionPIN); err != nil {
		return nil, err
	}
	return s.executeVAS(ctx, userID, models.CategoryAirtime, req.Amount,
		fmt.Sprintf("Airtime %s — %s", req.Network, req.Phone),
		map[string]interface{}{"phone": req.Phone, "network": req.Network},
		req.ClientReference,
	)
}

func (s *Service) PurchaseData(ctx context.Context, userID string, req DataRequest) (*models.Transaction, error) {
	if err := s.validateTransactionPIN(ctx, userID, req.TransactionPIN); err != nil {
		return nil, err
	}

	return s.executeVAS(ctx, userID, models.CategoryData, req.Amount,
		fmt.Sprintf("Data %s — %s", req.PlanCode, req.Phone),
		map[string]interface{}{
			"phone":       req.Phone,
			"network":     req.Network,
			"plan_code":   req.PlanCode,
			"item_code":   req.PlanCode,
			"biller_code": req.BillerCode,
		},
		req.ClientReference,
	)
}

func (s *Service) PayElectricity(ctx context.Context, userID string, req ElectricityRequest) (*models.Transaction, error) {
	if err := s.validateTransactionPIN(ctx, userID, req.TransactionPIN); err != nil {
		return nil, err
	}

	return s.executeVAS(ctx, userID, models.CategoryElectricity, req.Amount,
		fmt.Sprintf("Electricity %s — %s", req.DisCo, req.MeterNumber),
		map[string]interface{}{
			"meter_number": req.MeterNumber,
			"disco":        req.DisCo,
			"meter_type":   req.MeterType,
		},
		req.ClientReference,
	)
}

func (s *Service) FundBettingWallet(ctx context.Context, userID string, req BettingFundRequest) (*models.Transaction, error) {
	if err := s.enforceBettingRiskGuard(ctx, userID, req); err != nil {
		return nil, err
	}
	if err := s.validateTransactionPIN(ctx, userID, req.TransactionPIN); err != nil {
		return nil, err
	}

	return s.executeVAS(ctx, userID, models.CategoryBetting, req.Amount,
		fmt.Sprintf("Betting top-up %s — %s", req.Platform, req.CustomerID),
		map[string]interface{}{"customer_id": req.CustomerID, "platform": req.Platform},
		req.ClientReference,
	)
}

func (s *Service) executeVAS(
	ctx context.Context,
	userID string,
	category models.ServiceCategory,
	amount int64,
	narration string,
	meta map[string]interface{},
	clientReference string,
) (*models.Transaction, error) {
	if s.bills == nil {
		return nil, fmt.Errorf("bill payment provider is not configured")
	}

	ref, idempotent := vasReference(userID, clientReference)
	if idempotent {
		existing, err := s.walletRepo.FindTransactionByReference(ctx, userID, ref)
		if err == nil {
			return existing, nil
		}
		if err != nil && !postgres.IsNotFound(err) {
			return nil, err
		}
		meta["client_reference"] = strings.TrimSpace(clientReference)
	}

	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	_, err = s.walletRepo.DebitWithLock(ctx, userID, amount)
	if err != nil {
		return nil, err
	}

	metaBytes, _ := json.Marshal(meta)
	tx := &models.Transaction{
		ID:            uuid.New(),
		UserID:        wallet.UserID.String(),
		WalletID:      wallet.ID,
		Reference:     ref,
		Type:          models.TxTypeDebit,
		Category:      category,
		Amount:        amount,
		BalanceBefore: wallet.Balance,
		BalanceAfter:  wallet.Balance - amount,
		Status:        models.TxProcessing,
		Provider:      "flutterwave",
		Narration:     narration,
		Meta:          metaBytes,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.walletRepo.InsertTransaction(ctx, tx); err != nil {
		_ = s.walletRepo.CreditBalance(ctx, wallet.ID, amount)
		if idempotent {
			if existing, findErr := s.walletRepo.FindTransactionByReference(ctx, userID, ref); findErr == nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("create transaction: %w", err)
	}

	var billReceiptMeta []byte
	var billToken string
	extRef, reconErr := s.recon.ExecuteWithFallback(
		ctx, tx,
		func(c context.Context) (string, error) {
			s.log.Info("calling primary bill provider api", zap.String("ref", ref))
			resp, err := s.bills.PurchaseBill(c, BillPurchaseParams{
				Category:   category,
				Reference:  ref,
				CustomerID: customerIDFromMeta(category, meta),
				Amount:     amount,
				Meta:       meta,
			})
			if err != nil {
				return "", err
			}

			status, err := s.confirmProviderDelivery(c, s.bills, ref, resp.ExternalReference)
			if err != nil {
				return "", err
			}

			billReceiptMeta = buildReceiptEnvelope(meta, resp.RawMessage, status.RawMessage)
			billToken = status.RechargeToken
			return resp.ExternalReference, nil
		},
		s.buildFallbackCall(ctx, ref, category, amount, meta, &billReceiptMeta, &billToken),
	)

	if reconErr != nil {
		s.sendVASRefundAlert(userID, tx)
		return nil, reconErr
	}

	tx.ExternalRef = extRef
	tx.Status = models.TxSuccess
	tx.UpdatedAt = time.Now()

	if len(billReceiptMeta) > 0 {
		if err := s.walletRepo.UpdateTransactionMeta(ctx, tx.Reference, billReceiptMeta); err != nil {
			s.log.Warn("could not persist receipt metadata", zap.String("ref", tx.Reference), zap.Error(err))
		} else {
			tx.Meta = billReceiptMeta
		}
	}

	go s.loyaltySvc.AwardPoints(context.Background(), userID, tx.ID, category, amount)
	s.sendVASSuccessAlert(userID, tx, billToken)

	return tx, nil
}

func (s *Service) GetTransaction(ctx context.Context, userID string, reference string) (*models.Transaction, error) {
	return s.walletRepo.FindTransactionByReference(ctx, userID, reference)
}

func (s *Service) enforceBettingRiskGuard(ctx context.Context, userID string, req BettingFundRequest) error {
	if s.userRepo == nil {
		return fmt.Errorf("user repository is not configured")
	}

	stats, err := s.walletRepo.CategorySpendStats(ctx, userID, models.CategoryBetting, time.Now().AddDate(0, 0, -7))
	if err != nil {
		return err
	}
	if !requiresBettingRiskChallenge(req.Amount, stats) {
		return nil
	}

	challenge := &RiskChallengeError{
		Category:       models.CategoryBetting,
		Amount:         req.Amount,
		WeeklyAverage:  stats.Avg,
		WeeklyMax:      stats.Max,
		WeeklyCount:    stats.Count,
		RequiredFields: []string{"risk_confirmed", "biometric_verified", "transaction_pin"},
	}

	if !req.RiskConfirmed || !req.BiometricVerified || req.TransactionPIN == "" {
		return challenge
	}

	return nil
}

func (s *Service) validateTransactionPIN(ctx context.Context, userID string, pin string) error {
	if s.userRepo == nil {
		return fmt.Errorf("user repository is not configured")
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.PinHash == "" {
		return errors.New("transaction PIN is not set")
	}
	if err := crypto.CheckPassword(user.PinHash, pin); err != nil {
		return errors.New("invalid transaction PIN")
	}
	return nil
}

func requiresBettingRiskChallenge(amount int64, stats *postgres.CategorySpendStats) bool {
	const highValueBettingTopUp = int64(5_000_000) // ₦50,000 in kobo.
	if stats == nil || stats.Count == 0 {
		return amount >= highValueBettingTopUp
	}
	if stats.Count < 3 {
		return amount >= highValueBettingTopUp && amount >= stats.Max*2
	}
	if stats.Avg > 0 && amount >= stats.Avg*3 {
		return true
	}
	if stats.Max > 0 && amount >= stats.Max*2 {
		return true
	}
	return false
}

func vasReference(userID string, clientReference string) (string, bool) {
	clientReference = strings.TrimSpace(clientReference)
	if clientReference == "" {
		return fmt.Sprintf("FB-%s-%d", uuid.NewString()[:8], time.Now().UnixMilli()), false
	}

	prefix := userID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return fmt.Sprintf("FB-%s-%s", prefix, normalizeReference(clientReference)), true
}

func normalizeReference(ref string) string {
	ref = strings.TrimSpace(ref)
	var b strings.Builder
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
		if b.Len() >= 60 {
			break
		}
	}
	if b.Len() == 0 {
		return uuid.NewString()[:8]
	}
	return b.String()
}

func (s *Service) confirmProviderDelivery(ctx context.Context, provider BillProvider, reference string, externalRef string) (*UnifiedBillResponse, error) {
	if status, err := provider.CheckBillStatus(ctx, reference); err == nil && status.Status == "success" {
		return status, nil
	}

	if externalRef != "" && externalRef != reference {
		if status, err := provider.CheckBillStatus(ctx, externalRef); err == nil && status.Status == "success" {
			return status, nil
		}
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("bill delivery not confirmed through provider: %w", ctx.Err())
		case <-ticker.C:
			if status, err := provider.CheckBillStatus(ctx, reference); err == nil && status.Status == "success" {
				return status, nil
			}
			if externalRef != "" && externalRef != reference {
				if status, err := provider.CheckBillStatus(ctx, externalRef); err == nil && status.Status == "success" {
					return status, nil
				}
			}
		}
	}
}

func buildReceiptEnvelope(base map[string]interface{}, purchaseRaw []byte, statusRaw []byte) []byte {
	receipt := make(map[string]interface{}, len(base)+1)
	for key, value := range base {
		receipt[key] = value
	}

	var purchaseJSON, statusJSON interface{}
	_ = json.Unmarshal(purchaseRaw, &purchaseJSON)
	_ = json.Unmarshal(statusRaw, &statusJSON)

	receipt["provider_receipt"] = map[string]interface{}{
		"purchase_payload": purchaseJSON,
		"status_payload":   statusJSON,
		"confirmed_at":     time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(receipt)
	return body
}

func (s *Service) sendVASSuccessAlert(userID string, tx *models.Transaction, token string) {
	if s.sms == nil || tx == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		user, err := s.userRepo.FindByID(ctx, userID)
		if err != nil {
			s.log.Warn("could not load user for VAS success SMS", zap.String("user_id", userID), zap.Error(err))
			return
		}
		if err := s.sms.SendVASSuccessAlert(ctx, user.Phone, tx.Narration, tx.Amount, tx.Reference, token); err != nil {
			s.log.Warn("VAS success SMS failed", zap.String("ref", tx.Reference), zap.Error(err))
		}
	}()
}

func (s *Service) buildFallbackCall(
	_ context.Context,
	ref string,
	category models.ServiceCategory,
	amount int64,
	meta map[string]interface{},
	receiptMeta *[]byte,
	billToken *string,
) reconciliation.BillerCallFn {
	if s.fallbackBills == nil {
		return nil
	}
	return func(c context.Context) (string, error) {
		s.log.Info("switching execution to Monnify fallback provider", zap.String("ref", ref))
		fallbackRef := ref + "_FB"
		resp, err := s.fallbackBills.PurchaseBill(c, BillPurchaseParams{
			Category:   category,
			Reference:  fallbackRef,
			CustomerID: customerIDFromMeta(category, meta),
			Amount:     amount,
			Meta:       meta,
		})
		if err != nil {
			return "", err
		}

		status, err := s.fallbackBills.CheckBillStatus(c, fallbackRef)
		if err != nil {
			// Succeeded even if the status verification times out
			return resp.ExternalReference, nil
		}

		*receiptMeta = buildReceiptEnvelope(meta, resp.RawMessage, status.RawMessage)
		*billToken = status.RechargeToken
		return resp.ExternalReference, nil
	}
}

func (s *Service) sendVASRefundAlert(userID string, tx *models.Transaction) {
	if s.sms == nil || tx == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		user, err := s.userRepo.FindByID(ctx, userID)
		if err != nil {
			s.log.Warn("could not load user for VAS refund SMS", zap.String("user_id", userID), zap.Error(err))
			return
		}
		if err := s.sms.SendVASRefundAlert(ctx, user.Phone, tx.Narration, tx.Amount, tx.Reference); err != nil {
			s.log.Warn("VAS refund SMS failed", zap.String("ref", tx.Reference), zap.Error(err))
		}
	}()
}

func customerIDFromMeta(category models.ServiceCategory, meta map[string]interface{}) string {
	switch category {
	case models.CategoryAirtime, models.CategoryData:
		return stringFromMeta(meta, "phone")
	case models.CategoryElectricity:
		return stringFromMeta(meta, "meter_number")
	case models.CategoryBetting:
		return stringFromMeta(meta, "customer_id")
	default:
		return ""
	}
}

func stringFromMeta(meta map[string]interface{}, key string) string {
	if value, ok := meta[key].(string); ok {
		return value
	}
	return ""
}
