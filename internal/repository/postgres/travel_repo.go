package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TravelRepository struct {
	db *pgxpool.Pool
}

func NewTravelRepository(db *pgxpool.Pool) *TravelRepository {
	return &TravelRepository{db: db}
}

// Create inserts a new booking record.
func (r *TravelRepository) Create(ctx context.Context, b *models.TravelBooking) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO travel_bookings
		 (id,user_id,transaction_id,mode,operator_code,operator_name,
		  origin,destination,departure_time,seat_number,vehicle_ref,
		  passenger_name,passenger_phone,ticket_code,status,
		  offline_cache_hash,price_paid,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		b.ID, b.UserID, b.TransactionID, b.Mode, b.OperatorCode, b.OperatorName,
		b.Origin, b.Destination, b.DepartureTime, b.SeatNumber, b.VehicleRef,
		b.PassengerName, b.PassengerPhone, b.TicketCode, b.Status,
		b.OfflineCacheHash, b.PricePaid, b.CreatedAt, b.UpdatedAt,
	)
	return err
}

// FindByID fetches a booking by its primary key.
func (r *TravelRepository) FindByID(ctx context.Context, id string) (*models.TravelBooking, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id,user_id,transaction_id,mode,operator_code,operator_name,
		        origin,destination,departure_time,seat_number,vehicle_ref,
		        passenger_name,passenger_phone,ticket_code,status,
		        offline_cache_hash,price_paid,created_at,updated_at
		 FROM travel_bookings WHERE id=$1`, id)
	return scanBooking(row)
}

// ListByUser returns upcoming bookings for a user in departure order.
func (r *TravelRepository) ListByUser(ctx context.Context, userID string) ([]*models.TravelBooking, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id,user_id,transaction_id,mode,operator_code,operator_name,
		        origin,destination,departure_time,seat_number,vehicle_ref,
		        passenger_name,passenger_phone,ticket_code,status,
		        offline_cache_hash,price_paid,created_at,updated_at
		 FROM travel_bookings
		 WHERE user_id=$1
		 ORDER BY departure_time ASC`, userID)
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

// UpdateStatus is used by the Terminal Dispatcher webhook (Phase 3)
// and by the reschedule flow (PRD Section 3B).
func (r *TravelRepository) UpdateStatus(ctx context.Context, id string, status models.TravelBookingStatus) error {
	_, err := r.db.Exec(ctx,
		`UPDATE travel_bookings SET status=$1, updated_at=$2 WHERE id=$3`,
		status, time.Now(), id)
	return err
}

// ── scanner ───────────────────────────────────────────────────────────────────

type bookingScanner interface {
	Scan(dest ...interface{}) error
}

func scanBooking(row bookingScanner) (*models.TravelBooking, error) {
	b := &models.TravelBooking{}
	err := row.Scan(
		&b.ID, &b.UserID, &b.TransactionID, &b.Mode, &b.OperatorCode, &b.OperatorName,
		&b.Origin, &b.Destination, &b.DepartureTime, &b.SeatNumber, &b.VehicleRef,
		&b.PassengerName, &b.PassengerPhone, &b.TicketCode, &b.Status,
		&b.OfflineCacheHash, &b.PricePaid, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan booking: %w", err)
	}
	return b, nil
}
