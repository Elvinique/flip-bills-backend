package dispatcher

import (
	"github.com/google/uuid"
)

type DispatcherAction string

const (
	ActionCancelRoute    DispatcherAction = "CANCEL_ROUTE"
	ActionDelayRoute     DispatcherAction = "DELAY_ROUTE"
	ActionVehicleFailure DispatcherAction = "VEHICLE_FAILURE"
)

// DispatcherBroadcastRequest comes from the partner web portal panel.
type DispatcherBroadcastRequest struct {
	OperatorCode  string           `json:"operator_code"   binding:"required"`
	VehicleRef    string           `json:"vehicle_ref"     binding:"required"`
	DepartureDate string           `json:"departure_date"  binding:"required"` // YYYY-MM-DD
	Action        DispatcherAction `json:"action"          binding:"required,oneof=CANCEL_ROUTE DELAY_ROUTE VEHICLE_FAILURE"`
	Reason        string           `json:"reason"          binding:"required,max=250"`
	NewDeparture  string           `json:"new_departure"   binding:"omitempty"` // YYYY-MM-DD HH:MM if delayed
}

// UserResolutionRequest handles the user's single-tap selection response.
type UserResolutionRequest struct {
	BookingID      uuid.UUID `json:"booking_id"      binding:"required"`
	ResolutionType string    `json:"resolution_type" binding:"required,oneof=REFUND RESCHEDULE"`
	NewVehicleRef  string    `json:"new_vehicle_ref" binding:"omitempty"` // Required if RESCHEDULE
	NewSeatNumber  string    `json:"new_seat_number" binding:"omitempty"` // Required if RESCHEDULE
}
