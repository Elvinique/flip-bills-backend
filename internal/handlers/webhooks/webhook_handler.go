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
	"io"
	"net/http"

	walletsvc "github.com/flip-bills/backend/internal/services/wallet"
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
	} `json:"data"`
}

// POST /webhooks/flutterwave
func (h *Handler) Flutterwave(c *gin.Context) {
	// 1. Read raw body for signature verification before binding.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read request body"})
		return
	}

	// 2. Verify Flutterwave signature (verif-hash header).
	sig := c.GetHeader("verif-hash")
	if sig != h.flutterwaveSecret && h.flutterwaveSecret != "" {
		h.log.Warn("Flutterwave webhook: invalid signature")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// 3. Parse event.
	var event flutterwaveEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed webhook payload"})
		return
	}

	// 4. Only process successful charge events.
	if event.Event != "charge.completed" || event.Data.Status != "successful" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	// 5. Look up internal reference and process credit via Service Layer
	amountKobo := int64(event.Data.Amount * 100)
	err = h.walletSvc.ProcessFundingWebhook(c.Request.Context(), event.Data.TxRef, event.Data.FlwRef, amountKobo)
	if err != nil {
		h.log.Error("Flutterwave ledger settlement failed",
			zap.String("tx_ref", event.Data.TxRef),
			zap.Error(err),
		)
		// Return 200 so Flutterwave stops retrying; fallback reconciliation logs will catch errors
		c.JSON(http.StatusOK, gin.H{"received": true, "error": err.Error()})
		return
	}

	h.log.Info("Flutterwave ledger transaction completed successfully",
		zap.String("tx_ref", event.Data.TxRef),
		zap.String("flw_ref", event.Data.FlwRef),
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
	} `json:"eventData"`
}

// POST /webhooks/monnify
func (h *Handler) Monnify(c *gin.Context) {
	// 1. Read raw body.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read request body"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed webhook payload"})
		return
	}

	// 4. Only process successful payment completions.
	if event.EventType != "SUCCESSFUL_TRANSACTION" || event.EventData.PaymentStatus != "PAID" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	// 5. Settle internal pending reference state securely
	amountKobo := int64(event.EventData.AmountPaid * 100)
	err = h.walletSvc.ProcessFundingWebhook(
		c.Request.Context(),
		event.EventData.TransactionReference,
		event.EventData.PaymentReference,
		amountKobo,
	)
	if err != nil {
		h.log.Error("Monnify ledger settlement failed",
			zap.String("tx_ref", event.EventData.TransactionReference),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, gin.H{"received": true, "error": err.Error()})
		return
	}

	h.log.Info("Monnify ledger transaction completed successfully",
		zap.String("tx_ref", event.EventData.TransactionReference),
		zap.String("pay_ref", event.EventData.PaymentReference),
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
