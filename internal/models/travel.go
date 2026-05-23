package models

import (
	"time"

	"github.com/google/uuid"
)

// TravelBookingStatus mirrors the PRD's off-grid terminal edge cases.
type TravelBookingStatus string

const (
	BookingConfirmed   TravelBookingStatus = "confirmed"
	BookingPending     TravelBookingStatus = "pending"
	BookingRescheduled TravelBookingStatus = "rescheduled"
	BookingCancelled   TravelBookingStatus = "cancelled"
	BookingBoarded     TravelBookingStatus = "boarded"
)

// TravelMode differentiates bus vs flight.
type TravelMode string

const (
	TravelBus    TravelMode = "bus"
	TravelFlight TravelMode = "flight"
)

// TravelBooking is the core booking record stored in PostgreSQL.
type TravelBooking struct {
	ID              uuid.UUID           `db:"id"               json:"id"`
	UserID          uuid.UUID           `db:"user_id"          json:"user_id"`
	TransactionID   uuid.UUID           `db:"transaction_id"   json:"transaction_id"`
	Mode            TravelMode          `db:"mode"             json:"mode"`
	OperatorCode    string              `db:"operator_code"    json:"operator_code"`  // e.g. "GIGM", "AIR_PEACE"
	OperatorName    string              `db:"operator_name"    json:"operator_name"`
	Origin          string              `db:"origin"           json:"origin"`
	Destination     string              `db:"destination"      json:"destination"`
	DepartureTime   time.Time           `db:"departure_time"   json:"departure_time"`
	SeatNumber      string              `db:"seat_number"      json:"seat_number"`
	VehicleRef      string              `db:"vehicle_ref"      json:"vehicle_ref"`
	PassengerName   string              `db:"passenger_name"   json:"passenger_name"`
	PassengerPhone  string              `db:"passenger_phone"  json:"passenger_phone"`
	TicketCode      string              `db:"ticket_code"      json:"ticket_code"`  // QR code payload
	Status          TravelBookingStatus `db:"status"           json:"status"`
	// OfflineCacheHash is the SHA-256 of the cached QR payload — used for offline verification.
	OfflineCacheHash string             `db:"offline_cache_hash" json:"offline_cache_hash"`
	PricePaid        int64              `db:"price_paid"       json:"price_paid"` // kobo
	CreatedAt        time.Time          `db:"created_at"       json:"created_at"`
	UpdatedAt        time.Time          `db:"updated_at"       json:"updated_at"`
}
