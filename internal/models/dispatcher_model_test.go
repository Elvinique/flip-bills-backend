package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDispatcherEventTypes(t *testing.T) {
	validTypes := []DispatcherEventType{
		EventVehicleBreakdown,
		EventRouteChange,
		EventDoubleBooking,
		EventScheduleChange,
		EventCancellation,
	}
	for _, et := range validTypes {
		if string(et) == "" {
			t.Fatalf("event type should not be empty")
		}
	}
}

func TestDispatcherEventStatusTransitions(t *testing.T) {
	event := DispatcherEvent{
		ID:            uuid.New(),
		OperatorCode:  "GIGM",
		EventType:     EventVehicleBreakdown,
		VehicleRef:    "GIGM-BUS-0042",
		Origin:        "Lagos",
		Destination:   "Abuja",
		DepartureTime: time.Now().Add(2 * time.Hour),
		Message:       "Vehicle breakdown on route",
		Status:        EventStatusOpen,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if event.Status != EventStatusOpen {
		t.Fatalf("initial status should be open")
	}

	event.Status = EventStatusNotified
	if event.Status != EventStatusNotified {
		t.Fatal("status should be notified")
	}

	event.Status = EventStatusResolved
	if event.Status != EventStatusResolved {
		t.Fatal("status should be resolved")
	}
}

func TestPassengerDisruptionResolutions(t *testing.T) {
	disruption := PassengerDisruption{
		ID:         uuid.New(),
		EventID:    uuid.New(),
		BookingID:  uuid.New(),
		UserID:     uuid.New(),
		Resolution: ResolutionPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if disruption.Resolution != ResolutionPending {
		t.Fatal("initial resolution should be pending")
	}
	if disruption.NewBookingID != nil {
		t.Fatal("new booking ID should be nil initially")
	}
	if disruption.RefundTxID != nil {
		t.Fatal("refund tx ID should be nil initially")
	}

	// Simulate reschedule
	newID := uuid.New()
	disruption.NewBookingID = &newID
	disruption.Resolution = ResolutionRescheduled
	if disruption.Resolution != ResolutionRescheduled {
		t.Fatal("resolution should be rescheduled")
	}
}
