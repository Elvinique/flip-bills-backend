package dispatcher

// DispatcherService implements PRD Section 3B — Inter-State Bus Operations
// & Off-Grid Terminals. When an operator reports a disruption, this service:
//  1. Finds all affected confirmed bookings
//  2. Creates a PassengerDisruption record for each
//  3. Fires SMS notifications to all affected passengers
//  4. Exposes reschedule and refund actions for passengers to resolve

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/notifications"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

// DispatchEventRequest is what the operator portal POSTs to the webhook.
type DispatchEventRequest struct {
	OperatorCode  string `json:"operator_code"  binding:"required"`
	EventType     string `json:"event_type"     binding:"required,oneof=vehicle_breakdown route_change double_booking schedule_change cancellation"`
	VehicleRef    string `json:"vehicle_ref"    binding:"required"`
	Origin        string `json:"origin"         binding:"required"`
	Destination   string `json:"destination"    binding:"required"`
	DepartureTime string `json:"departure_time" binding:"required"` // RFC3339
	Message       string `json:"message"        binding:"required"`
	APIKey        string `json:"api_key"        binding:"required"`
}

type DispatchEventResponse struct {
	EventID          string `json:"event_id"`
	AffectedBookings int    `json:"affected_bookings"`
	SMSDispatched    int    `json:"sms_dispatched"`
}

type RescheduleRequest struct {
	NewVehicleRef    string `json:"new_vehicle_ref"    binding:"required"`
	NewDepartureTime string `json:"new_departure_time" binding:"required"` // RFC3339
	NewSeatNumber    string `json:"new_seat_number"    binding:"required"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	dispatcherRepo *postgres.DispatcherRepository
	travelRepo     *postgres.TravelRepository
	walletRepo     *postgres.WalletRepository
	sms            *notifications.SMSService
	log            *zap.Logger
	// operatorKeys maps operator_code → SHA-256 of their API key
	// In production this comes from a database; here we use config injection.
	operatorKeys map[string]string
}

func NewService(
	dispatcherRepo *postgres.DispatcherRepository,
	travelRepo *postgres.TravelRepository,
	walletRepo *postgres.WalletRepository,
	sms *notifications.SMSService,
	operatorKeys map[string]string,
	log *zap.Logger,
) *Service {
	return &Service{
		dispatcherRepo: dispatcherRepo,
		travelRepo:     travelRepo,
		walletRepo:     walletRepo,
		sms:            sms,
		operatorKeys:   operatorKeys,
		log:            log,
	}
}

// HandleDispatchEvent is the core PRD 3B flow:
// operator reports disruption → find bookings → notify passengers.
func (s *Service) HandleDispatchEvent(ctx context.Context, req DispatchEventRequest) (*DispatchEventResponse, error) {
	// 1. Verify operator API key.
	if err := s.verifyOperatorKey(req.OperatorCode, req.APIKey); err != nil {
		return nil, err
	}

	// 2. Parse departure time.
	departure, err := time.Parse(time.RFC3339, req.DepartureTime)
	if err != nil {
		return nil, fmt.Errorf("invalid departure_time format — use RFC3339 (e.g. 2026-06-01T07:00:00+01:00)")
	}

	// 3. Create the dispatcher event record.
	event := &models.DispatcherEvent{
		ID:            uuid.New(),
		OperatorCode:  req.OperatorCode,
		EventType:     models.DispatcherEventType(req.EventType),
		VehicleRef:    req.VehicleRef,
		Origin:        req.Origin,
		Destination:   req.Destination,
		DepartureTime: departure,
		Message:       req.Message,
		Status:        models.EventStatusOpen,
		APIKeyHash:    hashAPIKey(req.APIKey),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.dispatcherRepo.CreateEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to record event: %w", err)
	}

	// 4. Find all affected bookings.
	bookings, err := s.dispatcherRepo.FindAffectedBookings(
		ctx, req.OperatorCode, req.VehicleRef,
		req.Origin, req.Destination, departure,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find affected bookings: %w", err)
	}

	s.log.Info("dispatcher event created",
		zap.String("event_id", event.ID.String()),
		zap.String("operator", req.OperatorCode),
		zap.Int("affected_bookings", len(bookings)),
	)

	if len(bookings) == 0 {
		return &DispatchEventResponse{
			EventID:          event.ID.String(),
			AffectedBookings: 0,
			SMSDispatched:    0,
		}, nil
	}

	// 5. Create per-passenger disruption records and fire SMS.
	smsSent := 0
	for _, booking := range bookings {
		disruption := &models.PassengerDisruption{
			ID:         uuid.New(),
			EventID:    event.ID,
			BookingID:  booking.ID,
			UserID:     booking.UserID,
			Resolution: models.ResolutionPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := s.dispatcherRepo.CreateDisruption(ctx, disruption); err != nil {
			s.log.Error("failed to create disruption record",
				zap.String("booking_id", booking.ID.String()),
				zap.Error(err),
			)
			continue
		}

		// Mark booking as disrupted.
		_ = s.travelRepo.UpdateStatus(ctx, booking.ID.String(), models.BookingRescheduled)

		// Fire SMS — PRD "real-time fleet adjustments broadcast instantly".
		if booking.PassengerPhone != "" {
			err := s.sms.SendDisruptionAlert(
				ctx,
				booking.PassengerPhone,
				booking.PassengerName,
				booking.TicketCode,
				fmt.Sprintf("%s → %s", booking.Origin, booking.Destination),
				booking.DepartureTime.Format("Mon 02 Jan 2006, 03:04 PM"),
				req.Message,
				booking.ID.String(),
			)
			if err == nil {
				smsSent++
			} else {
				s.log.Warn("disruption SMS failed",
					zap.String("phone", booking.PassengerPhone),
					zap.Error(err),
				)
			}
		}
	}

	// 6. Mark event as notified.
	_ = s.dispatcherRepo.MarkNotified(ctx, event.ID)

	return &DispatchEventResponse{
		EventID:          event.ID.String(),
		AffectedBookings: len(bookings),
		SMSDispatched:    smsSent,
	}, nil
}

// RescheduleBooking moves a passenger onto a new trip — PRD "single-tap reschedule".
func (s *Service) RescheduleBooking(ctx context.Context, userID, bookingID string, req RescheduleRequest) (*models.TravelBooking, error) {
	// 1. Fetch original booking and verify ownership.
	original, err := s.travelRepo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("booking not found")
	}
	if original.UserID.String() != userID {
		return nil, fmt.Errorf("unauthorized")
	}
	if original.Status != models.BookingRescheduled {
		return nil, fmt.Errorf("this booking is not eligible for rescheduling")
	}

	// 2. Find the pending disruption.
	disruption, err := s.dispatcherRepo.FindDisruptionByBooking(ctx, original.ID)
	if err != nil {
		return nil, fmt.Errorf("no active disruption found for this booking")
	}

	// 3. Parse new departure time.
	newDeparture, err := time.Parse(time.RFC3339, req.NewDepartureTime)
	if err != nil {
		return nil, fmt.Errorf("invalid new_departure_time format")
	}

	// 4. Create new booking on the replacement vehicle — no wallet debit
	//    since the passenger already paid.
	newBooking := &models.TravelBooking{
		ID:             uuid.New(),
		UserID:         original.UserID,
		TransactionID:  original.TransactionID,
		Mode:           original.Mode,
		OperatorCode:   original.OperatorCode,
		OperatorName:   original.OperatorName,
		Origin:         original.Origin,
		Destination:    original.Destination,
		DepartureTime:  newDeparture,
		SeatNumber:     req.NewSeatNumber,
		VehicleRef:     req.NewVehicleRef,
		PassengerName:  original.PassengerName,
		PassengerPhone: original.PassengerPhone,
		TicketCode:     fmt.Sprintf("RSCH-%s", original.TicketCode),
		Status:         models.BookingConfirmed,
		PricePaid:      original.PricePaid,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := s.travelRepo.Create(ctx, newBooking); err != nil {
		return nil, fmt.Errorf("failed to create rescheduled booking: %w", err)
	}

	// 5. Cancel original booking.
	_ = s.travelRepo.UpdateStatus(ctx, bookingID, models.BookingCancelled)

	// 6. Resolve the disruption.
	newID := newBooking.ID
	_ = s.dispatcherRepo.ResolveDisruption(
		ctx, disruption.ID,
		models.ResolutionRescheduled,
		&newID, nil,
	)

	// 7. Notify passenger of new booking.
	if original.PassengerPhone != "" {
		_ = s.sms.SendBookingConfirmation(
			ctx,
			original.PassengerPhone,
			newBooking.TicketCode,
			fmt.Sprintf("%s → %s", newBooking.Origin, newBooking.Destination),
			newDeparture.Format("Mon 02 Jan 2006, 03:04 PM"),
		)
	}

	s.log.Info("booking rescheduled",
		zap.String("original_id", bookingID),
		zap.String("new_id", newBooking.ID.String()),
	)
	return newBooking, nil
}

// RefundBooking issues an instant wallet credit — PRD "instant credit back".
func (s *Service) RefundBooking(ctx context.Context, userID, bookingID string) (*models.Transaction, error) {
	// 1. Fetch booking and verify ownership.
	booking, err := s.travelRepo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("booking not found")
	}
	if booking.UserID.String() != userID {
		return nil, fmt.Errorf("unauthorized")
	}
	if booking.Status != models.BookingRescheduled {
		return nil, fmt.Errorf("this booking is not eligible for a refund")
	}

	// 2. Find disruption.
	disruption, err := s.dispatcherRepo.FindDisruptionByBooking(ctx, booking.ID)
	if err != nil {
		return nil, fmt.Errorf("no active disruption found for this booking")
	}

	// 3. Load wallet.
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}

	// 4. Credit wallet — full refund of price paid.
	if err := s.walletRepo.CreditBalance(ctx, wallet.ID, booking.PricePaid); err != nil {
		return nil, fmt.Errorf("refund credit failed: %w", err)
	}

	// 5. Write refund transaction.
	refundTx := &models.Transaction{
		ID:            uuid.New(),
		UserID:        wallet.UserID,
		WalletID:      wallet.ID,
		Reference:     fmt.Sprintf("DISP-REF-%s", uuid.NewString()[:8]),
		Type:          models.TxTypeCredit,
		Category:      models.CategoryBusTravel,
		Amount:        booking.PricePaid,
		BalanceBefore: wallet.Balance,
		BalanceAfter:  wallet.Balance + booking.PricePaid,
		Status:        models.TxSuccess,
		Provider:      "dispatcher_refund",
		Narration:     fmt.Sprintf("Disruption refund: %s → %s (%s)", booking.Origin, booking.Destination, booking.TicketCode),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.walletRepo.InsertTransaction(ctx, refundTx); err != nil {
		return nil, fmt.Errorf("failed to record refund transaction: %w", err)
	}

	// 6. Cancel booking and resolve disruption.
	_ = s.travelRepo.UpdateStatus(ctx, bookingID, models.BookingCancelled)
	refundID := refundTx.ID
	_ = s.dispatcherRepo.ResolveDisruption(
		ctx, disruption.ID,
		models.ResolutionRefunded,
		nil, &refundID,
	)

	s.log.Info("disruption refund issued",
		zap.String("booking_id", bookingID),
		zap.String("refund_tx", refundTx.Reference),
		zap.Int64("amount_kobo", booking.PricePaid),
	)
	return refundTx, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (s *Service) verifyOperatorKey(operatorCode, apiKey string) error {
	expectedHash, exists := s.operatorKeys[operatorCode]
	if !exists {
		return fmt.Errorf("unknown operator code: %s", operatorCode)
	}
	if hashAPIKey(apiKey) != expectedHash {
		return fmt.Errorf("invalid API key for operator %s", operatorCode)
	}
	return nil
}

func hashAPIKey(key string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}
