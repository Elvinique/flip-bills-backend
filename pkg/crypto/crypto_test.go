package crypto

import (
	"strings"
	"testing"
	"time"
)

func TestHashPasswordAndCheck(t *testing.T) {
	hash, err := HashPassword("securepassword123")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" {
		t.Fatal("hash is empty")
	}
	if err := CheckPassword(hash, "securepassword123"); err != nil {
		t.Fatalf("CheckPassword failed for correct password: %v", err)
	}
	if err := CheckPassword(hash, "wrongpassword"); err == nil {
		t.Fatal("CheckPassword should fail for wrong password")
	}
}

func TestHashPasswordDifferentEachTime(t *testing.T) {
	h1, _ := HashPassword("samepassword")
	h2, _ := HashPassword("samepassword")
	if h1 == h2 {
		t.Fatal("bcrypt should produce different hashes each time due to random salt")
	}
}

func TestGenerateOTP(t *testing.T) {
	otp, err := GenerateOTP(6)
	if err != nil {
		t.Fatalf("GenerateOTP error: %v", err)
	}
	if len(otp) != 6 {
		t.Fatalf("OTP length = %d, want 6", len(otp))
	}
	for _, c := range otp {
		if c < '0' || c > '9' {
			t.Fatalf("OTP contains non-digit character: %q", c)
		}
	}
}

func TestGenerateOTPDifferentEachTime(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		otp, _ := GenerateOTP(6)
		seen[otp] = true
	}
	// With 10 random 6-digit OTPs, highly unlikely all are the same
	if len(seen) == 1 {
		t.Fatal("OTPs appear to not be random")
	}
}

func TestRandomHex(t *testing.T) {
	hex, err := RandomHex(16)
	if err != nil {
		t.Fatalf("RandomHex error: %v", err)
	}
	if len(hex) != 32 { // 16 bytes = 32 hex chars
		t.Fatalf("hex length = %d, want 32", len(hex))
	}
}

func TestGenerateOfflineQRPayload(t *testing.T) {
	secret := "test-secret-key-32-bytes-long!!!"
	ticket := OfflineTicketPayload{
		BookingID:     "booking-123",
		TicketCode:    "TKT-001",
		PassengerName: "Ada Okonkwo",
		Route:         "Lagos → Abuja",
		DepartureTime: time.Date(2026, 6, 1, 7, 0, 0, 0, time.UTC),
		SeatNumber:    "3A",
		OperatorName:  "GIGM Transport",
	}

	payload, hash, err := GenerateOfflineQRPayload(ticket, secret)
	if err != nil {
		t.Fatalf("GenerateOfflineQRPayload error: %v", err)
	}
	if payload == "" {
		t.Fatal("QR payload is empty")
	}
	if len(hash) != 64 { // SHA-256 hex = 64 chars
		t.Fatalf("hash length = %d, want 64", len(hash))
	}
}

func TestVerifyOfflineQRPayload(t *testing.T) {
	secret := "test-secret-key-32-bytes-long!!!"
	ticket := OfflineTicketPayload{
		BookingID:     "booking-456",
		TicketCode:    "TKT-002",
		PassengerName: "John Doe",
		Route:         "Abuja → Lagos",
		DepartureTime: time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC),
		SeatNumber:    "5B",
		OperatorName:  "ABC Transport",
	}

	payload, _, err := GenerateOfflineQRPayload(ticket, secret)
	if err != nil {
		t.Fatalf("generate error: %v", err)
	}

	// Verify with correct secret
	verified, err := VerifyOfflineQRPayload(payload, secret)
	if err != nil {
		t.Fatalf("VerifyOfflineQRPayload error: %v", err)
	}
	if verified.TicketCode != ticket.TicketCode {
		t.Fatalf("ticket code = %q, want %q", verified.TicketCode, ticket.TicketCode)
	}
	if verified.PassengerName != ticket.PassengerName {
		t.Fatalf("passenger name = %q, want %q", verified.PassengerName, ticket.PassengerName)
	}

	// Verify with wrong secret should fail
	_, err = VerifyOfflineQRPayload(payload, "wrong-secret")
	if err == nil {
		t.Fatal("VerifyOfflineQRPayload should fail with wrong secret")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid signature error, got: %v", err)
	}
}

func TestVerifyOfflineQRPayloadTamperedData(t *testing.T) {
	secret := "test-secret-key-32-bytes-long!!!"
	ticket := OfflineTicketPayload{
		BookingID:  "booking-789",
		TicketCode: "TKT-003",
		Route:      "Lagos → Enugu",
	}
	payload, _, _ := GenerateOfflineQRPayload(ticket, secret)

	// Tamper with the payload
	tampered := payload[:len(payload)-4] + "XXXX"
	_, err := VerifyOfflineQRPayload(tampered, secret)
	if err == nil {
		t.Fatal("tampered payload should fail verification")
	}
}
