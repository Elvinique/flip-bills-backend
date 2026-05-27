package models

import (
	"testing"
)

func TestCalculateTier(t *testing.T) {
	tests := []struct {
		name           string
		lifetimePoints int64
		want           LoyaltyTier
	}{
		{"zero points is bronze", 0, TierBronze},
		{"9999 points is bronze", 9_999, TierBronze},
		{"10000 points is silver", 10_000, TierSilver},
		{"49999 points is silver", 49_999, TierSilver},
		{"50000 points is gold", 50_000, TierGold},
		{"199999 points is gold", 199_999, TierGold},
		{"200000 points is platinum", 200_000, TierPlatinum},
		{"1000000 points is platinum", 1_000_000, TierPlatinum},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTier(tt.lifetimePoints)
			if got != tt.want {
				t.Fatalf("CalculateTier(%d) = %q, want %q", tt.lifetimePoints, got, tt.want)
			}
		})
	}
}

func TestCalculatePointsEarned(t *testing.T) {
	tests := []struct {
		name     string
		category ServiceCategory
		amount   int64 // kobo
		want     int64
	}{
		{"airtime 1000 kobo = 10 points", CategoryAirtime, 1000, 10},
		{"data 5000 kobo = 50 points", CategoryData, 5000, 50},
		{"electricity 2x rate 10000 kobo = 200 points", CategoryElectricity, 10000, 200},
		{"cable tv 2x rate 5000 kobo = 100 points", CategoryCableTV, 5000, 100},
		{"betting 1000 kobo = 10 points", CategoryBetting, 1000, 10},
		{"bus travel 5x rate 75000 kobo = 3750 points", CategoryBusTravel, 75000, 3750},
		{"flight 10x rate 550000 kobo = 55000 points", CategoryFlight, 550000, 55000},
		{"wallet funding earns nothing", CategoryWalletFund, 1000000, 0},
		{"transfer earns nothing", CategoryTransfer, 500000, 0},
		{"unknown category earns nothing", ServiceCategory("unknown"), 1000, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePointsEarned(tt.category, tt.amount)
			if got != tt.want {
				t.Fatalf("CalculatePointsEarned(%q, %d) = %d, want %d",
					tt.category, tt.amount, got, tt.want)
			}
		})
	}
}

func TestPointsToKobo(t *testing.T) {
	tests := []struct {
		points int64
		want   int64
	}{
		{100, 100},   // 100 points = ₦1 = 100 kobo
		{500, 500},   // 500 points = ₦5 = 500 kobo
		{1000, 1000}, // 1000 points = ₦10 = 1000 kobo
		{0, 0},
	}

	for _, tt := range tests {
		got := PointsToKobo(tt.points)
		if got != tt.want {
			t.Fatalf("PointsToKobo(%d) = %d, want %d", tt.points, got, tt.want)
		}
	}
}
