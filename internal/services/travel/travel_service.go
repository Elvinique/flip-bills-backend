package travel

// TravelService orchestrates bus and flight booking end-to-end.
// It fans out search requests to all registered operators concurrently,
// aggregates results, handles seat holds, wallet debits, QR generation,
// and SMS fallback dispatch — all as described in PRD Phase 2.

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/notifications"
	"github.com/flip-bills/backend/internal/repository/mongo"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/flip-bills/backend/internal/services/travel/operators"
	"github.com/flip-bills/backend/pkg/crypto"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type BusSearchRequest struct {
	Origin        string `json:"origin"         binding:"required"`
	Destination   string `json:"destination"    binding:"required"`
	DepartureDate string `json:"departure_date" binding:"required"` // "YYYY-MM-DD"
}

type FlightSearchRequest struct {
	Origin        string `json:"origin"         binding:"required"`
	Destination   string `json:"destination"    binding:"required"`
	DepartureDate string `json:"departure_date" binding:"required"`
	CabinClass    string `json:"cabin_class"    binding:"oneof=economy business"`
	Adults        int    `json:"adults"         binding:"min=1,max=9"`
}

type BusBookRequest struct {
	Origin        string                  `json:"origin"         binding:"required"`
	Destination   string                  `json:"destination"    binding:"required"`
	OperatorCode  string                  `json:"operator_code"  binding:"required"`
	VehicleRef    string                  `json:"vehicle_ref"    binding:"required"`
	SeatNumber    string                  `json:"seat_number"    binding:"required"`
	DepartureDate string                  `json:"departure_date" binding:"required"`
	Passenger     operators.PassengerInfo `json:"passenger"      binding:"required"`
}

type FlightBookRequest struct {
	GDSRef    string                  `json:"gds_ref"   binding:"required"`
	Passenger operators.PassengerInfo `json:"passenger" binding:"required"`
}

type BookingResponse struct {
	Booking   *models.TravelBooking `json:"booking"`
	OfflineQR string                `json:"offline_qr_payload"` // store in SQLite/Room on device
	SMSSent   bool                  `json:"sms_confirmation_sent"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	busOperators    []operators.BusOperator
	flightOperators []operators.FlightOperator
	travelRepo      *postgres.TravelRepository
	walletRepo      *postgres.WalletRepository
	cacheRepo       *mongo.TravelCacheRepository
	sms             *notifications.SMSService
	loyaltySvc      loyaltyIface
	qrSecret        string
	log             *zap.Logger
}

// loyaltyIface avoids an import cycle — travel → loyalty → models is fine,
// but we define the minimal surface we need here.
type loyaltyIface interface {
	AwardPoints(ctx context.Context, userID string, sourceTxID uuid.UUID, category models.ServiceCategory, amountKobo int64)
}

func NewService(
	busOps []operators.BusOperator,
	flightOps []operators.FlightOperator,
	travelRepo *postgres.TravelRepository,
	walletRepo *postgres.WalletRepository,
	cacheRepo *mongo.TravelCacheRepository,
	sms *notifications.SMSService,
	loyaltySvc loyaltyIface,
	qrSecret string,
	log *zap.Logger,
) *Service {
	return &Service{
		busOperators:    busOps,
		flightOperators: flightOps,
		travelRepo:      travelRepo,
		walletRepo:      walletRepo,
		cacheRepo:       cacheRepo,
		sms:             sms,
		loyaltySvc:      loyaltySvc,
		qrSecret:        qrSecret,
		log:             log,
	}
}

// SearchBus fans out to all registered bus operators concurrently (PRD Click 1).
// Results are sorted by price ascending so the cheapest option leads.
func (s *Service) SearchBus(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	cacheKey := fmt.Sprintf("bus:%s:%s:%s", req.Origin, req.Destination, req.DepartureDate)

	// Check MongoDB search cache first (10-minute TTL).
	if cached, err := s.cacheRepo.GetSearchCache(ctx, cacheKey); err == nil {
		s.log.Info("bus search cache hit", zap.String("key", cacheKey))
		return decodeBusResults(cached.Results), nil
	}

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		all  []models.BusSearchResult
		errs []error
	)

	// Fan out to all operators in parallel — PRD "parallel worker APIs" (Click 1).
	for _, op := range s.busOperators {
		wg.Add(1)
		go func(o operators.BusOperator) {
			defer wg.Done()
			results, err := o.Search(ctx, operators.BusSearchRequest{
				Origin:        req.Origin,
				Destination:   req.Destination,
				DepartureDate: req.DepartureDate,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				s.log.Warn("operator search failed", zap.String("op", o.Code()), zap.Error(err))
				errs = append(errs, err)
				return
			}
			all = append(all, results...)
		}(op)
	}
	wg.Wait()

	if len(all) == 0 {
		return nil, fmt.Errorf("no trips found for this route and date")
	}

	// Sort by price ascending.
	sort.Slice(all, func(i, j int) bool { return all[i].PriceKobo < all[j].PriceKobo })

	// Cache the aggregated results in MongoDB.
	_ = s.cacheRepo.SetSearchCache(ctx, cacheKey, encodeBusResults(all))

	return all, nil
}

// SearchFlights fans out to all GDS operators concurrently.
func (s *Service) SearchFlights(ctx context.Context, req FlightSearchRequest) ([]models.FlightSearchResult, error) {
	if req.Adults == 0 {
		req.Adults = 1
	}
	if req.CabinClass == "" {
		req.CabinClass = "economy"
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		all []models.FlightSearchResult
	)

	for _, op := range s.flightOperators {
		wg.Add(1)
		go func(o operators.FlightOperator) {
			defer wg.Done()
			results, err := o.Search(ctx, operators.FlightSearchRequest{
				Origin:        req.Origin,
				Destination:   req.Destination,
				DepartureDate: req.DepartureDate,
				CabinClass:    req.CabinClass,
				Adults:        req.Adults,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				s.log.Warn("flight operator search failed", zap.String("op", o.Code()), zap.Error(err))
				return
			}
			all = append(all, results...)
		}(op)
	}
	wg.Wait()

	if len(all) == 0 {
		return nil, fmt.Errorf("no flights found for this route and date")
	}

	sort.Slice(all, func(i, j int) bool { return all[i].PriceKobo < all[j].PriceKobo })
	return all, nil
}

// BookBus is the PRD's Click 2 → Click 3 flow for bus travel.
// Steps: seat hold → wallet debit → operator confirm → QR generation → SMS fallback.
func (s *Service) BookBus(ctx context.Context, userID string, req BusBookRequest) (*BookingResponse, error) {
	op := s.findBusOperator(req.OperatorCode)
	if op == nil {
		return nil, fmt.Errorf("operator %q not supported", req.OperatorCode)
	}

	// 1. Hold the seat while we process payment.
	holdRef, err := op.Hold(ctx, req.VehicleRef, req.SeatNumber)
	if err != nil {
		return nil, fmt.Errorf("seat hold failed: %w", err)
	}

	// 2. Re-query the selected operator for live price to prevent client tampering.
	offer, err := s.findLiveBusOffer(ctx, op, req)
	if err != nil {
		_ = op.Cancel(ctx, holdRef)
		return nil, err
	}
	priceKobo := offer.PriceKobo

	// 3. Debit wallet — atomic, checked against daily limit.
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}
	if _, err := s.walletRepo.DebitWithLock(ctx, userID, priceKobo); err != nil {
		// Release the hold so seat becomes available again.
		_ = op.Cancel(ctx, holdRef)
		return nil, err
	}

	// 4. Write the pending transaction to the ledger.
	txID := uuid.New()
	ref := fmt.Sprintf("FB-BUS-%s", uuid.NewString()[:8])
	tx := &models.Transaction{
		ID:            txID,
		UserID:        wallet.UserID.String(),
		WalletID:      wallet.ID,
		Reference:     ref,
		Type:          models.TxTypeDebit,
		Category:      models.CategoryBusTravel,
		Amount:        priceKobo,
		BalanceBefore: wallet.Balance,
		BalanceAfter:  wallet.Balance - priceKobo,
		Status:        models.TxProcessing,
		Provider:      req.OperatorCode,
		Narration:     fmt.Sprintf("Bus: %s → %s, %s (%s)", req.Passenger.FullName, req.Origin, req.Destination, req.OperatorCode),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.walletRepo.InsertTransaction(ctx, tx); err != nil {
		_ = s.walletRepo.CreditBalance(ctx, wallet.ID, priceKobo)
		_ = op.Cancel(ctx, holdRef)
		return nil, fmt.Errorf("could not record bus transaction: %w", err)
	}

	// 5. Confirm booking with operator.
	ticketCode, err := op.Confirm(ctx, holdRef, req.Passenger)
	if err != nil {
		// Confirm failed — reverse the debit.
		_ = s.walletRepo.CreditBalance(ctx, wallet.ID, priceKobo)
		_ = s.walletRepo.UpdateTransactionStatus(ctx, ref, models.TxReversed, "")
		return nil, fmt.Errorf("operator booking failed — wallet refunded")
	}
	_ = s.walletRepo.UpdateTransactionStatus(ctx, ref, models.TxSuccess, ticketCode)

	// 6. Generate offline QR payload (PRD Section 3C).
	departure, _ := time.Parse("2006-01-02", req.DepartureDate)
	qrPayload, qrHash, err := crypto.GenerateOfflineQRPayload(crypto.OfflineTicketPayload{
		BookingID:     uuid.NewString(),
		TicketCode:    ticketCode,
		PassengerName: req.Passenger.FullName,
		Route:         fmt.Sprintf("%s → %s", req.Origin, req.Destination),
		DepartureTime: departure,
		SeatNumber:    req.SeatNumber,
		OperatorName:  op.Name(),
	}, s.qrSecret)
	if err != nil {
		s.log.Error("QR generation failed", zap.Error(err))
		qrPayload = ticketCode // fallback to plain ticket code
		qrHash = ""
	}

	// 7. Persist the booking.
	booking := &models.TravelBooking{
		ID:               uuid.New(),
		UserID:           wallet.UserID,
		TransactionID:    txID,
		Mode:             models.TravelBus,
		OperatorCode:     req.OperatorCode,
		OperatorName:     op.Name(),
		Origin:           req.Origin,
		Destination:      req.Destination,
		DepartureTime:    firstNonZeroTime(offer.DepartureTime, departure),
		SeatNumber:       req.SeatNumber,
		VehicleRef:       req.VehicleRef,
		PassengerName:    req.Passenger.FullName,
		PassengerPhone:   req.Passenger.Phone,
		TicketCode:       ticketCode,
		Status:           models.BookingConfirmed,
		OfflineCacheHash: qrHash,
		PricePaid:        priceKobo,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.travelRepo.Create(ctx, booking); err != nil {
		s.log.Error("failed to persist booking", zap.Error(err))
	}

	// 8. SMS confirmation — PRD Section 3C hard fallback.
	smsSent := false
	if req.Passenger.Phone != "" {
		err := s.sms.SendBookingConfirmation(
			ctx,
			req.Passenger.Phone,
			ticketCode,
			fmt.Sprintf("%s → %s", req.Origin, req.Destination),
			departure.Format("Mon 02 Jan 2006, 03:04 PM"),
		)
		smsSent = err == nil
	}

	// 9. Award loyalty points — non-blocking, never fails the parent booking.
	if s.loyaltySvc != nil {
		go s.loyaltySvc.AwardPoints(context.Background(), userID, txID, models.CategoryBusTravel, priceKobo)
	}

	return &BookingResponse{
		Booking:   booking,
		OfflineQR: qrPayload,
		SMSSent:   smsSent,
	}, nil
}

// BookFlight follows the same pattern as BookBus but uses the GDS flow.
func (s *Service) BookFlight(ctx context.Context, userID string, req FlightBookRequest) (*BookingResponse, error) {
	op := s.findFlightOperator("AMADEUS")
	if op == nil {
		return nil, fmt.Errorf("no flight operator available")
	}

	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found")
	}

	// Price confirmation step (prevents price-drift between search and book).
	offer, err := op.PriceOffer(ctx, req.GDSRef)
	if err != nil || offer == nil {
		return nil, fmt.Errorf("could not confirm flight price — please search again")
	}

	if _, err := s.walletRepo.DebitWithLock(ctx, userID, offer.PriceKobo); err != nil {
		return nil, err
	}

	txID := uuid.New()
	ref := fmt.Sprintf("FB-FLIGHT-%s", uuid.NewString()[:8])
	tx := &models.Transaction{
		ID:            txID,
		UserID:        wallet.UserID.String(),
		WalletID:      wallet.ID,
		Reference:     ref,
		Type:          models.TxTypeDebit,
		Category:      models.CategoryFlight,
		Amount:        offer.PriceKobo,
		BalanceBefore: wallet.Balance,
		BalanceAfter:  wallet.Balance - offer.PriceKobo,
		Status:        models.TxProcessing,
		Provider:      op.Code(),
		Narration:     fmt.Sprintf("Flight: %s → %s (%s)", offer.Origin, offer.Destination, offer.Airline),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.walletRepo.InsertTransaction(ctx, tx); err != nil {
		_ = s.walletRepo.CreditBalance(ctx, wallet.ID, offer.PriceKobo)
		return nil, fmt.Errorf("could not record flight transaction: %w", err)
	}

	ticketCode, err := op.Book(ctx, req.GDSRef, req.Passenger)
	if err != nil {
		_ = s.walletRepo.CreditBalance(ctx, wallet.ID, offer.PriceKobo)
		_ = s.walletRepo.UpdateTransactionStatus(ctx, ref, models.TxReversed, "")
		return nil, fmt.Errorf("GDS booking failed — wallet refunded")
	}
	_ = s.walletRepo.UpdateTransactionStatus(ctx, ref, models.TxSuccess, ticketCode)

	qrPayload, qrHash, _ := crypto.GenerateOfflineQRPayload(crypto.OfflineTicketPayload{
		BookingID:     uuid.NewString(),
		TicketCode:    ticketCode,
		PassengerName: req.Passenger.FullName,
		Route:         fmt.Sprintf("%s → %s", offer.Origin, offer.Destination),
		DepartureTime: offer.DepartureTime,
		OperatorName:  offer.Airline,
	}, s.qrSecret)

	booking := &models.TravelBooking{
		ID:               uuid.New(),
		UserID:           wallet.UserID,
		TransactionID:    txID,
		Mode:             models.TravelFlight,
		OperatorCode:     offer.Airline,
		OperatorName:     offer.Airline,
		Origin:           offer.Origin,
		Destination:      offer.Destination,
		DepartureTime:    offer.DepartureTime,
		PassengerName:    req.Passenger.FullName,
		PassengerPhone:   req.Passenger.Phone,
		TicketCode:       ticketCode,
		Status:           models.BookingConfirmed,
		OfflineCacheHash: qrHash,
		PricePaid:        offer.PriceKobo,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.travelRepo.Create(ctx, booking); err != nil {
		s.log.Error("failed to persist flight booking", zap.Error(err))
	}

	smsSent := false
	if req.Passenger.Phone != "" {
		err := s.sms.SendBookingConfirmation(ctx, req.Passenger.Phone, ticketCode,
			fmt.Sprintf("%s → %s", offer.Origin, offer.Destination),
			offer.DepartureTime.Format("Mon 02 Jan 2006, 03:04 PM"))
		smsSent = err == nil
	}

	// Award loyalty points — non-blocking.
	if s.loyaltySvc != nil {
		go s.loyaltySvc.AwardPoints(context.Background(), userID, booking.TransactionID, models.CategoryFlight, offer.PriceKobo)
	}

	return &BookingResponse{Booking: booking, OfflineQR: qrPayload, SMSSent: smsSent}, nil
}

// GetMyBookings returns all travel bookings for a user.
func (s *Service) GetMyBookings(ctx context.Context, userID string) ([]*models.TravelBooking, error) {
	return s.travelRepo.ListByUser(ctx, userID)
}

// GetBooking fetches a single booking — used for QR re-render on the app.
func (s *Service) GetBooking(ctx context.Context, bookingID string) (*models.TravelBooking, error) {
	return s.travelRepo.FindByID(ctx, bookingID)
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (s *Service) findBusOperator(code string) operators.BusOperator {
	for _, op := range s.busOperators {
		if op.Code() == code {
			return op
		}
	}
	return nil
}

func (s *Service) findFlightOperator(code string) operators.FlightOperator {
	for _, op := range s.flightOperators {
		if op.Code() == code {
			return op
		}
	}
	return nil
}

func (s *Service) findLiveBusOffer(ctx context.Context, op operators.BusOperator, req BusBookRequest) (*models.BusSearchResult, error) {
	results, err := op.Search(ctx, operators.BusSearchRequest{
		Origin:        req.Origin,
		Destination:   req.Destination,
		DepartureDate: req.DepartureDate,
	})
	if err != nil {
		return nil, fmt.Errorf("could not validate live bus inventory: %w", err)
	}
	for _, result := range results {
		if result.VehicleRef != req.VehicleRef {
			continue
		}
		if !seatIsAvailable(result.SeatLayout, req.SeatNumber) {
			return nil, fmt.Errorf("selected seat is no longer available")
		}
		if result.PriceKobo <= 0 {
			return nil, fmt.Errorf("operator returned invalid fare for selected trip")
		}
		return &result, nil
	}
	return nil, fmt.Errorf("selected trip is no longer available")
}

func seatIsAvailable(layout []models.SeatRow, seatNumber string) bool {
	if len(layout) == 0 {
		return true
	}
	for _, row := range layout {
		for _, seat := range row.Seats {
			if seat.Number == seatNumber {
				return seat.Available
			}
		}
	}
	return false
}

func firstNonZeroTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

// encodeBusResults / decodeBusResults convert between typed and generic maps for MongoDB.
func encodeBusResults(results []models.BusSearchResult) []map[string]interface{} {
	out := make([]map[string]interface{}, len(results))
	for i, r := range results {
		out[i] = map[string]interface{}{
			"operator_code":   r.OperatorCode,
			"operator_name":   r.OperatorName,
			"origin":          r.Origin,
			"destination":     r.Destination,
			"departure_time":  r.DepartureTime,
			"arrival_time":    r.ArrivalTime,
			"price_kobo":      r.PriceKobo,
			"price_ngn":       r.PriceNGN,
			"seats_available": r.SeatsAvailable,
			"vehicle_ref":     r.VehicleRef,
			"vehicle_class":   r.VehicleClass,
			"rating":          r.Rating,
			"seat_layout":     r.SeatLayout,
		}
	}
	return out
}

func decodeBusResults(raw []map[string]interface{}) []models.BusSearchResult {
	out := make([]models.BusSearchResult, len(raw))
	for i, m := range raw {
		out[i] = models.BusSearchResult{
			OperatorCode: fmt.Sprintf("%v", m["operator_code"]),
			OperatorName: fmt.Sprintf("%v", m["operator_name"]),
			Origin:       fmt.Sprintf("%v", m["origin"]),
			Destination:  fmt.Sprintf("%v", m["destination"]),
			VehicleRef:   fmt.Sprintf("%v", m["vehicle_ref"]),
			VehicleClass: fmt.Sprintf("%v", m["vehicle_class"]),
		}
		if p, ok := m["price_kobo"].(int64); ok {
			out[i].PriceKobo = p
			out[i].PriceNGN = float64(p) / 100
		}
		if p, ok := m["price_kobo"].(int32); ok {
			out[i].PriceKobo = int64(p)
			out[i].PriceNGN = float64(p) / 100
		}
		if p, ok := m["price_kobo"].(float64); ok {
			out[i].PriceKobo = int64(p)
			out[i].PriceNGN = p / 100
		}
		if seats, ok := m["seats_available"].(int32); ok {
			out[i].SeatsAvailable = int(seats)
		}
		if seats, ok := m["seats_available"].(int64); ok {
			out[i].SeatsAvailable = int(seats)
		}
		if seats, ok := m["seats_available"].(float64); ok {
			out[i].SeatsAvailable = int(seats)
		}
		if rating, ok := m["rating"].(float64); ok {
			out[i].Rating = rating
		}
		if departure, ok := m["departure_time"].(time.Time); ok {
			out[i].DepartureTime = departure
		}
		if arrival, ok := m["arrival_time"].(time.Time); ok {
			out[i].ArrivalTime = arrival
		}
	}
	return out
}
