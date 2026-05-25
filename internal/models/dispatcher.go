package models

import (
	"time"

	"github.com/google/uuid"
)

type DispatcherEventType string

const (
	EventVehicleBreakdown DispatcherEventType = "vehicle_breakdown"
	EventRouteChange      DispatcherEventType = "route_change"
	EventDoubleBooking    DispatcherEventType = "double_booking"
	EventScheduleChange   DispatcherEventType = "schedule_change"
	EventCancellation     DispatcherEventType = "cancellation"
)

type DispatcherEventStatus string

const (
	EventStatusOpen     DispatcherEventStatus = "open"
	EventStatusNotified DispatcherEventStatus = "notified"
	EventStatusResolved DispatcherEventStatus = "resolved"
)

// DispatcherEvent is created when a bus operator reports a disruption
// via the Terminal Dispatcher webhook (PRD Section 3B).
type DispatcherEvent struct {
	ID            uuid.UUID             `db:"id"             json:"id"`
	OperatorCode  string                `db:"operator_code"  json:"operator_code"`
	EventType     DispatcherEventType   `db:"event_type"     json:"event_type"`
	VehicleRef    string                `db:"vehicle_ref"    json:"vehicle_ref"`
	Origin        string                `db:"origin"         json:"origin"`
	Destination   string                `db:"destination"    json:"destination"`
	DepartureTime time.Time             `db:"departure_time" json:"departure_time"`
	Message       string                `db:"message"        json:"message"`
	Status        DispatcherEventStatus `db:"status"         json:"status"`
	APIKeyHash    string                `db:"api_key_hash"   json:"-"`
	CreatedAt     time.Time             `db:"created_at"     json:"created_at"`
	UpdatedAt     time.Time             `db:"updated_at"     json:"updated_at"`
}

type DisruptionResolution string

const (
	ResolutionPending     DisruptionResolution = "pending"
	ResolutionRescheduled DisruptionResolution = "rescheduled"
	ResolutionRefunded    DisruptionResolution = "refunded"
)

// PassengerDisruption tracks the per-passenger state for a dispatcher event.
// One DispatcherEvent → many PassengerDisruptions (one per affected booking).
type PassengerDisruption struct {
	ID           uuid.UUID            `db:"id"              json:"id"`
	EventID      uuid.UUID            `db:"event_id"        json:"event_id"`
	BookingID    uuid.UUID            `db:"booking_id"      json:"booking_id"`
	UserID       uuid.UUID            `db:"user_id"         json:"user_id"`
	Resolution   DisruptionResolution `db:"resolution"      json:"resolution"`
	NewBookingID *uuid.UUID           `db:"new_booking_id"  json:"new_booking_id,omitempty"`
	RefundTxID   *uuid.UUID           `db:"refund_tx_id"    json:"refund_tx_id,omitempty"`
	NotifiedAt   *time.Time           `db:"notified_at"     json:"notified_at,omitempty"`
	ResolvedAt   *time.Time           `db:"resolved_at"     json:"resolved_at,omitempty"`
	CreatedAt    time.Time            `db:"created_at"      json:"created_at"`
	UpdatedAt    time.Time            `db:"updated_at"      json:"updated_at"`
}
