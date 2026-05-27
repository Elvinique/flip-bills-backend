package jwt

import (
	"testing"
	"time"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	mgr := NewManager("super-secret-key-for-testing-only", 15*time.Minute, 720*time.Hour)

	token, err := mgr.GenerateAccessToken("user-123", 1)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	claims, err := mgr.Validate(token)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Fatalf("user_id = %q, want user-123", claims.UserID)
	}
	if claims.KYCTier != 1 {
		t.Fatalf("kyc_tier = %d, want 1", claims.KYCTier)
	}
}

func TestGenerateAndValidateRefreshToken(t *testing.T) {
	mgr := NewManager("super-secret-key-for-testing-only", 15*time.Minute, 720*time.Hour)

	token, err := mgr.GenerateRefreshToken("user-456", 2)
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}

	claims, err := mgr.Validate(token)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if claims.UserID != "user-456" {
		t.Fatalf("user_id = %q, want user-456", claims.UserID)
	}
	if claims.KYCTier != 2 {
		t.Fatalf("kyc_tier = %d, want 2", claims.KYCTier)
	}
}

func TestValidateWrongSecret(t *testing.T) {
	mgr1 := NewManager("secret-one", 15*time.Minute, 720*time.Hour)
	mgr2 := NewManager("secret-two", 15*time.Minute, 720*time.Hour)

	token, _ := mgr1.GenerateAccessToken("user-789", 0)

	_, err := mgr2.Validate(token)
	if err == nil {
		t.Fatal("Validate should fail with wrong secret")
	}
}

func TestValidateExpiredToken(t *testing.T) {
	mgr := NewManager("super-secret-key-for-testing-only", -1*time.Second, 720*time.Hour)

	token, err := mgr.GenerateAccessToken("user-expired", 0)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	_, err = mgr.Validate(token)
	if err == nil {
		t.Fatal("Validate should fail for expired token")
	}
}

func TestValidateMalformedToken(t *testing.T) {
	mgr := NewManager("super-secret-key-for-testing-only", 15*time.Minute, 720*time.Hour)

	_, err := mgr.Validate("not.a.valid.jwt.token")
	if err == nil {
		t.Fatal("Validate should fail for malformed token")
	}
}

func TestTokensAreDifferentEachCall(t *testing.T) {
	mgr := NewManager("super-secret-key-for-testing-only", 15*time.Minute, 720*time.Hour)

	t1, _ := mgr.GenerateAccessToken("user-123", 0)
	t2, _ := mgr.GenerateAccessToken("user-123", 0)

	if t1 == t2 {
		t.Fatal("tokens should be different due to unique JTI")
	}
}

func TestKYCTierZeroIsValid(t *testing.T) {
	mgr := NewManager("super-secret-key-for-testing-only", 15*time.Minute, 720*time.Hour)

	token, _ := mgr.GenerateAccessToken("user-tier0", 0)
	claims, err := mgr.Validate(token)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if claims.KYCTier != 0 {
		t.Fatalf("kyc_tier = %d, want 0", claims.KYCTier)
	}
}
