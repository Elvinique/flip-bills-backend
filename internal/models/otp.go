package models

import (
	"time"

	"github.com/google/uuid"
)

type OTPPurpose string

const (
	OTPPhoneVerify OTPPurpose = "phone_verify"
	OTPPINReset    OTPPurpose = "pin_reset"
	OTPTxAuth      OTPPurpose = "tx_auth"
)

type OTPToken struct {
	ID        uuid.UUID  `db:"id"`
	Phone     string     `db:"phone"`
	OTPHash   string     `db:"otp_hash"`
	Purpose   OTPPurpose `db:"purpose"`
	ExpiresAt time.Time  `db:"expires_at"`
	Used      bool       `db:"used"`
	CreatedAt time.Time  `db:"created_at"`
}
