package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DispatcherRepository struct {
	db *pgxpool.Pool
}

func NewDispatcherRepository(db *pgxpool.Pool) *DispatcherRepository {
	return &DispatcherRepository{db: db}
}

// CreateEvent inserts a new dispatcher disruption event.
func (r *DispatcherRepository) CreateEvent(ctx context.Context, e *models.DispatcherEvent) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO dispatcher_events
		 (id, operator_code, event_type, vehicle_ref, origin, destination,
		  departure_time, message, status, api_key_hash, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.ID, e.OperatorCode, e.EventType, e.VehicleRef, e.Origin, e.Destination,
		e.DepartureTime, e.Message, e.Status, e.APIKeyHash, e.CreatedAt, e.UpdatedAt,
	)
	return err
}

// FindAffectedBookings returns all confirmed bookings matching the disrupted
// vehicle, route, and departure time window (±2 hours to catch schedule drifts).
func (r *DispatcherRepository) FindAffectedBookings(
	ctx context.Context,
	operatorCode, vehicleRef, origin, destination string,
	departureTime time.Time,
) ([]*models.TravelBooking, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, transaction_id, mode, operator_code, operator_name,
		        origin, destination, departure_time, seat_number, vehicle_ref,
		        passenger_name, passenger_phone, ticket_code, status,
		        offline_cache_hash, price_paid, created_at, updated_at
		 FROM travel_bookings
		 WHERE operator_code = $1
		   AND vehicle_ref   = $2
		   AND origin        = $3
		   AND destination   = $4
		   AND departure_time BETWEEN $5 AND $6
		   AND status IN ('confirmed', 'pending')`,
		operatorCode, vehicleRef, origin, destination,
		departureTime.Add(-2*time.Hour),
		departureTime.Add(2*time.Hour),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []*models.TravelBooking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}

// CreateDisruption inserts a per-passenger disruption record.
func (r *DispatcherRepository) CreateDisruption(ctx context.Context, d *models.PassengerDisruption) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO passenger_disruptions
		 (id, event_id, booking_id, user_id, resolution, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		d.ID, d.EventID, d.BookingID, d.UserID, d.Resolution, d.CreatedAt, d.UpdatedAt,
	)
	return err
}

// FindDisruptionByBooking fetches the pending disruption for a booking.
func (r *DispatcherRepository) FindDisruptionByBooking(ctx context.Context, bookingID uuid.UUID) (*models.PassengerDisruption, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, event_id, booking_id, user_id, resolution,
		        new_booking_id, refund_tx_id, notified_at, resolved_at,
		        created_at, updated_at
		 FROM passenger_disruptions
		 WHERE booking_id = $1 AND resolution = 'pending'
		 ORDER BY created_at DESC LIMIT 1`, bookingID,
	)
	d := &models.PassengerDisruption{}
	err := row.Scan(
		&d.ID, &d.EventID, &d.BookingID, &d.UserID, &d.Resolution,
		&d.NewBookingID, &d.RefundTxID, &d.NotifiedAt, &d.ResolvedAt,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("disruption not found: %w", err)
	}
	return d, nil
}

// MarkNotified updates notified_at and event status after SMS is sent.
func (r *DispatcherRepository) MarkNotified(ctx context.Context, eventID uuid.UUID) error {
	now := time.Now()
	_, err := r.db.Exec(ctx,
		`UPDATE passenger_disruptions SET notified_at=$1, updated_at=$1
		 WHERE event_id=$2 AND resolution='pending'`,
		now, eventID,
	)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx,
		`UPDATE dispatcher_events SET status='notified', updated_at=$1 WHERE id=$2`,
		now, eventID,
	)
	return err
}

// ResolveDisruption marks a disruption as rescheduled or refunded.
func (r *DispatcherRepository) ResolveDisruption(
	ctx context.Context,
	disruptionID uuid.UUID,
	resolution models.DisruptionResolution,
	newBookingID *uuid.UUID,
	refundTxID *uuid.UUID,
) error {
	now := time.Now()
	_, err := r.db.Exec(ctx,
		`UPDATE passenger_disruptions
		 SET resolution=$1, new_booking_id=$2, refund_tx_id=$3,
		     resolved_at=$4, updated_at=$4
		 WHERE id=$5`,
		resolution, newBookingID, refundTxID, now, disruptionID,
	)
	return err
}
