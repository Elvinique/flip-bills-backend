package models

import (
	"time"

	"github.com/google/uuid"
)

// KYCTier represents the user's identity verification level.
// Mirrors Nigerian CBN tiered KYC requirements.
type KYCTier int

const (
	KYCTierUnverified KYCTier = 0 // Phone number only
	KYCTierOne        KYCTier = 1 // BVN linked — ₦50k daily limit
	KYCTierTwo        KYCTier = 2 // NIN + address — ₦500k daily limit
)

// User is the core identity record stored in PostgreSQL.
type User struct {
	ID           uuid.UUID  `db:"id"            json:"id"`
	Phone        string     `db:"phone"         json:"phone"`
	Email        string     `db:"email"         json:"email,omitempty"`
	PasswordHash string     `db:"password_hash" json:"-"`
	FirstName    string     `db:"first_name"    json:"first_name"`
	LastName     string     `db:"last_name"     json:"last_name"`
	KYCTier      KYCTier    `db:"kyc_tier"      json:"kyc_tier"`
	BVN          string     `db:"bvn"           json:"-"` // encrypted at rest
	NIN          string     `db:"nin"           json:"-"` // encrypted at rest
	IsActive     bool       `db:"is_active"     json:"is_active"`
	PinHash      string     `db:"pin_hash"      json:"-"` // 4/6-digit transaction PIN
	CreatedAt    time.Time  `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"    json:"updated_at"`
	DeletedAt    *time.Time `db:"deleted_at"    json:"-"`
}
