package models

import (
	"time"

	"github.com/google/uuid"
)

type TravelBookingStatus string

const (
	BookingConfirmed   TravelBookingStatus = "confirmed"
	BookingPending     TravelBookingStatus = "pending"
	BookingRescheduled TravelBookingStatus = "rescheduled"
	BookingCancelled TravelBookingStatus = "canceled"
	BookingBoarded     TravelBookingStatus = "boarded"
)

type TravelMode string

const (
	TravelBus    TravelMode = "bus"
	TravelFlight TravelMode = "flight"
)

// TravelBooking is the core booking record stored in PostgreSQL.
type TravelBooking struct {
	ID               uuid.UUID           `db:"id"                json:"id"`
	UserID           uuid.UUID           `db:"user_id"           json:"user_id"`
	TransactionID    uuid.UUID           `db:"transaction_id"    json:"transaction_id"`
	Mode             TravelMode          `db:"mode"              json:"mode"`
	OperatorCode     string              `db:"operator_code"     json:"operator_code"`
	OperatorName     string              `db:"operator_name"     json:"operator_name"`
	Origin           string              `db:"origin"            json:"origin"`
	Destination      string              `db:"destination"       json:"destination"`
	DepartureTime    time.Time           `db:"departure_time"    json:"departure_time"`
	SeatNumber       string              `db:"seat_number"       json:"seat_number"`
	VehicleRef       string              `db:"vehicle_ref"       json:"vehicle_ref"`
	PassengerName    string              `db:"passenger_name"    json:"passenger_name"`
	PassengerPhone   string              `db:"passenger_phone"   json:"passenger_phone"`
	TicketCode       string              `db:"ticket_code"       json:"ticket_code"`
	Status           TravelBookingStatus `db:"status"            json:"status"`
	OfflineCacheHash string              `db:"offline_cache_hash" json:"offline_cache_hash"`
	PricePaid        int64               `db:"price_paid"        json:"price_paid"` // kobo
	CreatedAt        time.Time           `db:"created_at"        json:"created_at"`
	UpdatedAt        time.Time           `db:"updated_at"        json:"updated_at"`
}

// OfflineTicketPayload is the struct that gets JSON-encoded, hashed,
// and cached on the device for offline QR verification (PRD Section 3C).
type OfflineTicketPayload struct {
	BookingID     string    `json:"booking_id"`
	TicketCode    string    `json:"ticket_code"`
	PassengerName string    `json:"passenger_name"`
	Route         string    `json:"route"` // "Lagos → Abuja"
	DepartureTime time.Time `json:"departure_time"`
	SeatNumber    string    `json:"seat_number"`
	OperatorName  string    `json:"operator_name"`
	IssuedAt      time.Time `json:"issued_at"`
}

// ── Bus-specific types ────────────────────────────────────────────────────────

// BusSearchResult is a normalised fare returned from any operator API.
// We normalise all partner formats into this single struct so the handler
// layer never needs to know which operator provided the data.
type BusSearchResult struct {
	OperatorCode   string    `json:"operator_code"`
	OperatorName   string    `json:"operator_name"`
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	PriceKobo      int64     `json:"price_kobo"`
	PriceNGN       float64   `json:"price_ngn"`
	SeatsAvailable int       `json:"seats_available"`
	VehicleRef     string    `json:"vehicle_ref"`
	VehicleClass   string    `json:"vehicle_class"` // "standard" | "executive" | "vip"
	SeatLayout     []SeatRow `json:"seat_layout"`
	Rating         float64   `json:"rating"`
}

// SeatRow describes one row of the vehicle seat map (PRD: interactive layout map).
type SeatRow struct {
	Row   int    `json:"row"`
	Seats []Seat `json:"seats"`
}

type Seat struct {
	Number    string `json:"number"`
	Available bool   `json:"available"`
	Class     string `json:"class"`
}

// ── Flight-specific types ────────────────────────────────────────────────────

// FlightSearchResult is a normalised itinerary from the GDS (Amadeus/Travelport).
type FlightSearchResult struct {
	FlightNumber   string    `json:"flight_number"`
	Airline        string    `json:"airline"`
	Origin         string    `json:"origin"`      // IATA code e.g. "LOS"
	Destination    string    `json:"destination"` // IATA code e.g. "ABV"
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	Duration       string    `json:"duration"`
	CabinClass     string    `json:"cabin_class"` // "economy" | "business"
	PriceKobo      int64     `json:"price_kobo"`
	PriceNGN       float64   `json:"price_ngn"`
	SeatsAvailable int       `json:"seats_available"`
	GDSRef         string    `json:"gds_ref"` // raw GDS offer ID — needed for booking
	Stops          int       `json:"stops"`
}
