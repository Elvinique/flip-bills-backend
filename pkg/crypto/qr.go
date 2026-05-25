package crypto

// OfflineQR implements PRD Section 3C — Cryptographic Offline Caching.
// Upon booking, we generate a deterministic signed payload that renders
// a valid QR code even when the device has zero network connectivity.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// OfflineTicketPayload is the minimal verifiable struct encoded into the QR.
type OfflineTicketPayload struct {
	BookingID     string    `json:"bid"`
	TicketCode    string    `json:"tc"`
	PassengerName string    `json:"pn"`
	Route         string    `json:"rt"`
	DepartureTime time.Time `json:"dt"`
	SeatNumber    string    `json:"sn"`
	OperatorName  string    `json:"op"`
	IssuedAt      time.Time `json:"ia"`
}

// GenerateOfflineQRPayload produces a signed, base64-encoded string that
// encodes the ticket data plus an HMAC signature.
// The mobile app stores this in SQLite/Room and renders it as a QR code.
func GenerateOfflineQRPayload(ticket OfflineTicketPayload, secret string) (string, string, error) {
	ticket.IssuedAt = time.Now()

	data, err := json.Marshal(ticket)
	if err != nil {
		return "", "", fmt.Errorf("marshal ticket: %w", err)
	}

	// HMAC-SHA256 signature — terminal scanners verify this before boarding.
	sig := generateHMAC(data, secret)

	envelope := map[string]string{
		"d": base64.StdEncoding.EncodeToString(data),
		"s": sig,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", "", err
	}

	qrPayload := base64.URLEncoding.EncodeToString(raw)

	// SHA-256 hash stored in PostgreSQL for fast server-side verification.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(qrPayload)))

	return qrPayload, hash, nil
}

// VerifyOfflineQRPayload checks the HMAC signature on an incoming QR scan.
// Called by the Terminal Dispatcher portal (Phase 3) when a passenger presents
// their QR at a checkpoint with no connectivity.
func VerifyOfflineQRPayload(qrPayload, secret string) (*OfflineTicketPayload, error) {
	raw, err := base64.URLEncoding.DecodeString(qrPayload)
	if err != nil {
		return nil, fmt.Errorf("invalid QR payload encoding")
	}

	var envelope map[string]string
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("malformed QR envelope")
	}

	data, err := base64.StdEncoding.DecodeString(envelope["d"])
	if err != nil {
		return nil, fmt.Errorf("malformed ticket data")
	}

	// Constant-time HMAC comparison — prevents timing attacks.
	expected := generateHMAC(data, secret)
	if !hmac.Equal([]byte(expected), []byte(envelope["s"])) {
		return nil, fmt.Errorf("QR signature invalid — possible forgery")
	}

	var ticket OfflineTicketPayload
	if err := json.Unmarshal(data, &ticket); err != nil {
		return nil, fmt.Errorf("corrupt ticket data")
	}

	return &ticket, nil
}

func generateHMAC(data []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return fmt.Sprintf("%x", mac.Sum(nil))
}
