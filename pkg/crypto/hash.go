package crypto

import (
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// HashPassword returns a bcrypt hash of the plain-text password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(b), err
}

// CheckPassword compares a bcrypt hash against a plain-text candidate.
func CheckPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

// GenerateOTP produces a cryptographically random N-digit numeric OTP.
func GenerateOTP(digits int) (string, error) {
	b := make([]byte, digits)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Map each byte to a digit 0-9.
	otp := make([]byte, digits)
	for i, v := range b {
		otp[i] = '0' + (v % 10)
	}
	return string(otp), nil
}

// RandomHex returns a hex-encoded random token of n bytes.
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
