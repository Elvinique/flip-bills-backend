package models

import (
	"time"

	"github.com/google/uuid"
)

// LoyaltyTier defines the user's reward tier based on lifetime points.
type LoyaltyTier string

const (
	TierBronze   LoyaltyTier = "bronze"   // 0 – 9,999 lifetime points
	TierSilver   LoyaltyTier = "silver"   // 10,000 – 49,999
	TierGold     LoyaltyTier = "gold"     // 50,000 – 199,999
	TierPlatinum LoyaltyTier = "platinum" // 200,000+
)

// LoyaltyAccount is the per-user points wallet.
type LoyaltyAccount struct {
	ID             uuid.UUID   `db:"id"              json:"id"`
	UserID         uuid.UUID   `db:"user_id"         json:"user_id"`
	PointsBalance  int64       `db:"points_balance"  json:"points_balance"`
	LifetimePoints int64       `db:"lifetime_points" json:"lifetime_points"`
	Tier           LoyaltyTier `db:"tier"            json:"tier"`
	CreatedAt      time.Time   `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"      json:"updated_at"`
}

// LoyaltyTxType classifies a points movement.
type LoyaltyTxType string

const (
	LoyaltyEarn   LoyaltyTxType = "earn"
	LoyaltyRedeem LoyaltyTxType = "redeem"
	LoyaltyExpire LoyaltyTxType = "expire"
)

// LoyaltyTransaction is the immutable points ledger entry.
type LoyaltyTransaction struct {
	ID            uuid.UUID     `db:"id"             json:"id"`
	UserID        uuid.UUID     `db:"user_id"        json:"user_id"`
	AccountID     uuid.UUID     `db:"account_id"     json:"account_id"`
	Type          LoyaltyTxType `db:"type"           json:"type"`
	Points        int64         `db:"points"         json:"points"`
	BalanceBefore int64         `db:"balance_before" json:"balance_before"`
	BalanceAfter  int64         `db:"balance_after"  json:"balance_after"`
	SourceTxID    *uuid.UUID    `db:"source_tx_id"   json:"source_tx_id,omitempty"`
	Category      string        `db:"category"       json:"category,omitempty"`
	Narration     string        `db:"narration"      json:"narration"`
	ExpiresAt     *time.Time    `db:"expires_at"     json:"expires_at,omitempty"`
	CreatedAt     time.Time     `db:"created_at"     json:"created_at"`
}

// ── Points earning rates ──────────────────────────────────────────────────────
// Points earned per 100 kobo (₦1) spent. Zero means no points for that category.

var PointsRatePerNaira = map[ServiceCategory]int64{
	CategoryAirtime:     1,  // 1 pt per ₦1
	CategoryData:        1,  // 1 pt per ₦1
	CategoryElectricity: 2,  // 2 pts per ₦1
	CategoryCableTV:     2,  // 2 pts per ₦1
	CategoryBetting:     1,  // 1 pt per ₦1
	CategoryBusTravel:   5,  // 5 pts per ₦1
	CategoryFlight:      10, // 10 pts per ₦1
	CategoryWalletFund:  0,  // no points for funding
	CategoryTransfer:    0,  // no points for transfers
}

// PointsRedemptionRate — 100 points = ₦1 (100 kobo).
const PointsRedemptionRate int64 = 100

// TierThresholds maps lifetime points to tier.
var TierThresholds = []struct {
	Min  int64
	Tier LoyaltyTier
}{
	{200_000, TierPlatinum},
	{50_000, TierGold},
	{10_000, TierSilver},
	{0, TierBronze},
}

// CalculateTier returns the correct tier for a given lifetime points total.
func CalculateTier(lifetimePoints int64) LoyaltyTier {
	for _, t := range TierThresholds {
		if lifetimePoints >= t.Min {
			return t.Tier
		}
	}
	return TierBronze
}

// CalculatePointsEarned computes points for a transaction amount in kobo.
func CalculatePointsEarned(category ServiceCategory, amountKobo int64) int64 {
	rate, ok := PointsRatePerNaira[category]
	if !ok || rate == 0 {
		return 0
	}
	// amountKobo / 100 = NGN amount; multiply by rate
	return (amountKobo / 100) * rate
}

// PointsToKobo converts points to kobo for redemption.
func PointsToKobo(points int64) int64 {
	return points * 100 / PointsRedemptionRate
}
