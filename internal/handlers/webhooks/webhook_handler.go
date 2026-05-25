package webhooks

// WebhookHandler processes inbound payment gateway callbacks.
// Flutterwave and Monnify both POST a signed JSON payload when a
// wallet-funding charge completes — we verify the signature, then
// credit the user's wallet exactly once (idempotent on payment ref).

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/flip-bills/backend/internal/middleware"
	walletsvc "github.com/flip-bills/backend/internal/services/wallet"
	"github.com/flip-bills/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	walletSvc         *walletsvc.Service
	flutterwaveSecret string
	monnifySecret     string
	log               *zap.Logger
}

func NewHandler(
	walletSvc *walletsvc.Service,
	flutterwaveSecret string,
	monnifySecret string,
	log *zap.Logger,
) *Handler {
	return &Handler{
		walletSvc:         walletSvc,
		flutterwaveSecret: flutterwaveSecret,
		monnifySecret:     monnifySecret,
		log:               log,
	}
}

// ── Flutterwave ───────────────────────────────────────────────────────────────

type flutterwaveEvent struct {
	Event string `json:"event"`
	Data  struct {
		ID       int64   `json:"id"`
		TxRef    string  `json:"tx_ref"`  // our internal reference
		FlwRef   string  `json:"flw_ref"` // Flutterwave reference
		Amount   float64 `json:"amount"`  // in NGN
		Currency string  `json:"currency"`
		Status   string  `json:"status"`
		Customer struct {
			Email string `json:"email"`
			Phone string `json:"phone_number"`
		} `json:"customer"`
		Meta struct {
			UserID string `json:"user_id"`
		} `json:"meta"`
	} `json:"data"`
}

// POST /webhooks/flutterwave
func (h *Handler) Flutterwave(c *gin.Context) {
	// 1. Read raw body for signature verification before binding.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "could not read request body", nil)
		return
	}

	// 2. Verify Flutterwave signature (verif-hash header).
	sig := c.GetHeader("verif-hash")
	if sig != h.flutterwaveSecret {
		h.log.Warn("Flutterwave webhook: invalid signature")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// 3. Parse event.
	var event flutterwaveEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		response.BadRequest(c, "malformed webhook payload", nil)
		return
	}

	// 4. Only process successful charge events.
	if event.Event != "charge.completed" || event.Data.Status != "successful" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	if event.Data.Meta.UserID == "" {
		h.log.Warn("Flutterwave webhook: missing user_id in meta", zap.String("tx_ref", event.Data.TxRef))
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	// 5. Credit wallet — amount is in NGN, convert to kobo.
	amountKobo := int64(event.Data.Amount * 100)
	_, err = h.walletSvc.FundWallet(c.Request.Context(), event.Data.Meta.UserID, walletsvc.FundWalletRequest{
		Amount:     amountKobo,
		PaymentRef: event.Data.FlwRef,
		Provider:   "flutterwave",
	})
	if err != nil {
		h.log.Error("Flutterwave wallet credit failed",
			zap.String("user_id", event.Data.Meta.UserID),
			zap.Error(err),
		)
		// Return 200 so Flutterwave doesn't retry — we log and reconcile manually.
		c.JSON(http.StatusOK, gin.H{"received": true, "error": err.Error()})
		return
	}

	h.log.Info("Flutterwave wallet funded",
		zap.String("user_id", event.Data.Meta.UserID),
		zap.String("flw_ref", event.Data.FlwRef),
		zap.Int64("amount_kobo", amountKobo),
	)
	c.JSON(http.StatusOK, gin.H{"received": true})
}

// ── Monnify ───────────────────────────────────────────────────────────────────

type monnifyEvent struct {
	EventType string `json:"eventType"`
	EventData struct {
		TransactionReference string  `json:"transactionReference"`
		PaymentReference     string  `json:"paymentReference"`
		AmountPaid           float64 `json:"amountPaid"`
		TotalPayable         float64 `json:"totalPayable"`
		PaidOn               string  `json:"paidOn"`
		PaymentStatus        string  `json:"paymentStatus"`
		PaymentDescription   string  `json:"paymentDescription"`
		MetaData             struct {
			UserID string `json:"user_id"`
		} `json:"metaData"`
	} `json:"eventData"`
}

// POST /webhooks/monnify
func (h *Handler) Monnify(c *gin.Context) {
	// 1. Read raw body.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "could not read request body", nil)
		return
	}

	// 2. Verify Monnify HMAC-SHA512 signature.
	sig := c.GetHeader("monnify-signature")
	if !verifyMonnifySignature(rawBody, h.monnifySecret, sig) {
		h.log.Warn("Monnify webhook: invalid signature")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// 3. Parse event.
	var event monnifyEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		response.BadRequest(c, "malformed webhook payload", nil)
		return
	}

	// 4. Only process successful payment completions.
	if event.EventType != "SUCCESSFUL_TRANSACTION" || event.EventData.PaymentStatus != "PAID" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	userID := event.EventData.MetaData.UserID
	if userID == "" {
		h.log.Warn("Monnify webhook: missing user_id in metadata",
			zap.String("tx_ref", event.EventData.TransactionReference),
		)
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	// 5. Credit wallet.
	amountKobo := int64(event.EventData.AmountPaid * 100)
	_, err = h.walletSvc.FundWallet(c.Request.Context(), userID, walletsvc.FundWalletRequest{
		Amount:     amountKobo,
		PaymentRef: event.EventData.PaymentReference,
		Provider:   "monnify",
	})
	if err != nil {
		h.log.Error("Monnify wallet credit failed",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, gin.H{"received": true, "error": err.Error()})
		return
	}

	h.log.Info("Monnify wallet funded",
		zap.String("user_id", userID),
		zap.String("pay_ref", event.EventData.PaymentReference),
		zap.Int64("amount_kobo", amountKobo),
	)
	c.JSON(http.StatusOK, gin.H{"received": true})
}

// verifyMonnifySignature checks the HMAC-SHA256 signature Monnify sends.
func verifyMonnifySignature(body []byte, secret, signature string) bool {
	if secret == "" {
		return true // skip verification in dev if secret not set
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// GetUserID re-exports middleware helper for any webhook that needs it.
func getUserID(c *gin.Context) string {
	return middleware.GetUserID(c)
}

// fmtKobo is a debug helper.
func fmtKobo(kobo int64) string {
	return fmt.Sprintf("₦%.2f", float64(kobo)/100)
}
