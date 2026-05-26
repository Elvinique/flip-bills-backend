package operators

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type busPartnerClient struct {
	code    string
	name    string
	apiKey  string
	baseURL string
	client  *http.Client
	log     *zap.Logger
}

func newBusPartnerClient(code, name, apiKey, baseURL string, log *zap.Logger) busPartnerClient {
	return busPartnerClient{
		code:    code,
		name:    name,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 20 * time.Second},
		log:     log,
	}
}

func (c *busPartnerClient) search(ctx context.Context, req BusSearchRequest) ([]models.BusSearchResult, error) {
	payload := map[string]string{
		"origin":         req.Origin,
		"destination":    req.Destination,
		"departure_date": req.DepartureDate,
		"from":           req.Origin,
		"to":             req.Destination,
		"date":           req.DepartureDate,
	}

	var raw map[string]interface{}
	if err := c.postJSON(ctx, "/trips/search", payload, &raw); err != nil {
		return nil, err
	}

	trips := findObjectSlice(raw, "data", "trips", "routes", "results", "inventory")
	if len(trips) == 0 {
		return nil, fmt.Errorf("%s returned no trips", c.code)
	}

	results := make([]models.BusSearchResult, 0, len(trips))
	for _, trip := range trips {
		mapped, err := c.mapTrip(req, trip)
		if err != nil {
			c.log.Warn("could not map bus partner trip",
				zap.String("operator", c.code),
				zap.Error(err),
			)
			continue
		}
		results = append(results, mapped)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%s returned trips but none could be mapped", c.code)
	}
	return results, nil
}

func (c *busPartnerClient) hold(ctx context.Context, vehicleRef, seatNumber string) (string, error) {
	payload := map[string]string{
		"vehicle_ref": vehicleRef,
		"trip_id":     vehicleRef,
		"seat_number": seatNumber,
	}

	var raw map[string]interface{}
	if err := c.postJSON(ctx, "/seats/hold", payload, &raw); err != nil {
		return "", err
	}

	if ref := firstString(raw, "hold_ref", "hold_reference", "reservation_ref", "reservation_id", "id"); ref != "" {
		return ref, nil
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if ref := firstString(data, "hold_ref", "hold_reference", "reservation_ref", "reservation_id", "id"); ref != "" {
			return ref, nil
		}
	}
	return "", fmt.Errorf("%s hold response missing hold reference", c.code)
}

func (c *busPartnerClient) confirm(ctx context.Context, holdRef string, passenger PassengerInfo) (string, error) {
	payload := map[string]interface{}{
		"hold_ref":       holdRef,
		"reservation_id": holdRef,
		"passenger": map[string]string{
			"full_name": passenger.FullName,
			"phone":     passenger.Phone,
			"email":     passenger.Email,
			"nin":       passenger.NIN,
		},
	}

	var raw map[string]interface{}
	if err := c.postJSON(ctx, "/bookings/confirm", payload, &raw); err != nil {
		return "", err
	}

	if ticket := firstString(raw, "ticket_code", "ticket_number", "pnr", "booking_reference", "reference", "id"); ticket != "" {
		return ticket, nil
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if ticket := firstString(data, "ticket_code", "ticket_number", "pnr", "booking_reference", "reference", "id"); ticket != "" {
			return ticket, nil
		}
	}
	return "", fmt.Errorf("%s confirm response missing ticket code", c.code)
}

func (c *busPartnerClient) cancel(ctx context.Context, ticketCode string) error {
	payload := map[string]string{
		"ticket_code":       ticketCode,
		"booking_reference": ticketCode,
	}
	var raw map[string]interface{}
	return c.postJSON(ctx, "/bookings/cancel", payload, &raw)
}

func (c *busPartnerClient) postJSON(ctx context.Context, path string, payload interface{}, out interface{}) error {
	if c.baseURL == "" {
		return fmt.Errorf("%s base URL is not configured", c.code)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", c.code, err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s returned status %d: %s", c.code, resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if len(rawBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(rawBody, out); err != nil {
		return fmt.Errorf("%s response decode failed: %w", c.code, err)
	}
	return nil
}

func (c *busPartnerClient) mapTrip(req BusSearchRequest, trip map[string]interface{}) (models.BusSearchResult, error) {
	vehicleRef := firstString(trip, "vehicle_ref", "vehicleRef", "trip_id", "tripId", "schedule_id", "scheduleId", "id", "bus_id")
	if vehicleRef == "" {
		return models.BusSearchResult{}, fmt.Errorf("missing vehicle reference")
	}

	departure := parsePartnerTime(
		firstString(trip, "departure_time", "departureTime", "departure_datetime", "departureDateTime", "departure"),
		req.DepartureDate,
		firstString(trip, "time", "departure_hour", "departureHour"),
	)
	arrival := parsePartnerTime(
		firstString(trip, "arrival_time", "arrivalTime", "arrival_datetime", "arrivalDateTime", "arrival"),
		req.DepartureDate,
		firstString(trip, "arrival_hour", "arrivalHour"),
	)
	if arrival.IsZero() && !departure.IsZero() {
		arrival = departure.Add(7 * time.Hour)
	}

	priceKobo := amountKobo(trip, "price_kobo", "priceKobo", "fare_kobo", "fareKobo")
	if priceKobo == 0 {
		priceKobo = amountNairaToKobo(trip, "price", "fare", "amount", "ticket_price", "ticketPrice")
	}
	if priceKobo <= 0 {
		return models.BusSearchResult{}, fmt.Errorf("missing price")
	}

	seats := firstInt(trip, "seats_available", "seatsAvailable", "available_seats", "availableSeats", "available", "capacity_left")
	if seats == 0 {
		seats = countAvailableSeats(trip)
	}

	layout := mapSeatLayout(trip)
	if len(layout) == 0 {
		layout = buildDefaultSeatLayout(14, 4)
	}

	return models.BusSearchResult{
		OperatorCode:   c.code,
		OperatorName:   c.name,
		Origin:         stringOrDefault(firstString(trip, "origin", "from", "departure_city"), req.Origin),
		Destination:    stringOrDefault(firstString(trip, "destination", "to", "arrival_city"), req.Destination),
		DepartureTime:  departure,
		ArrivalTime:    arrival,
		PriceKobo:      priceKobo,
		PriceNGN:       float64(priceKobo) / 100,
		SeatsAvailable: seats,
		VehicleRef:     vehicleRef,
		VehicleClass:   stringOrDefault(firstString(trip, "vehicle_class", "vehicleClass", "class", "bus_type"), "standard"),
		SeatLayout:     layout,
		Rating:         firstFloat(trip, "rating", "operator_rating"),
	}, nil
}

func findObjectSlice(root map[string]interface{}, keys ...string) []map[string]interface{} {
	for _, key := range keys {
		if items := objectSlice(root[key]); len(items) > 0 {
			return items
		}
	}
	for _, value := range root {
		if nested, ok := value.(map[string]interface{}); ok {
			if items := findObjectSlice(nested, keys...); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func objectSlice(value interface{}) []map[string]interface{} {
	raw, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case float64:
				return strconv.FormatInt(int64(v), 10)
			case int:
				return strconv.Itoa(v)
			case json.Number:
				return v.String()
			}
		}
	}
	return ""
}

func firstInt(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if n, ok := numberValue(m[key]); ok {
			return int(n)
		}
	}
	return 0
}

func firstFloat(m map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if n, ok := numberValue(m[key]); ok {
			return n
		}
	}
	return 0
}

func numberValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	case string:
		cleaned := strings.ReplaceAll(strings.TrimSpace(v), ",", "")
		cleaned = strings.TrimPrefix(cleaned, "NGN")
		cleaned = strings.TrimPrefix(cleaned, "₦")
		n, err := strconv.ParseFloat(strings.TrimSpace(cleaned), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func amountKobo(m map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		if n, ok := numberValue(m[key]); ok {
			return int64(math.Round(n))
		}
	}
	return 0
}

func amountNairaToKobo(m map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		if n, ok := numberValue(m[key]); ok {
			return int64(math.Round(n * 100))
		}
	}
	return 0
}

func parsePartnerTime(value, fallbackDate, fallbackClock string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	if fallbackDate != "" {
		clock := fallbackClock
		if clock == "" {
			clock = "07:00"
		}
		if len(clock) == 5 {
			if t, err := time.Parse("2006-01-02 15:04", fallbackDate+" "+clock); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func mapSeatLayout(trip map[string]interface{}) []models.SeatRow {
	rawSeats := objectSlice(trip["seats"])
	if len(rawSeats) == 0 {
		rawSeats = objectSlice(trip["seat_layout"])
	}
	if len(rawSeats) == 0 {
		return nil
	}

	rows := map[int][]models.Seat{}
	for _, seat := range rawSeats {
		number := firstString(seat, "number", "seat_number", "seatNumber", "label")
		if number == "" {
			continue
		}
		row := firstInt(seat, "row")
		if row == 0 {
			row = inferSeatRow(number)
		}
		available := true
		if status := strings.ToLower(firstString(seat, "status", "state")); status != "" {
			available = status == "available" || status == "free"
		}
		if v, ok := seat["available"].(bool); ok {
			available = v
		}
		rows[row] = append(rows[row], models.Seat{
			Number:    number,
			Available: available,
			Class:     stringOrDefault(firstString(seat, "class", "seat_class"), "standard"),
		})
	}

	out := make([]models.SeatRow, 0, len(rows))
	for row := 1; row <= len(rows)+1; row++ {
		seats, ok := rows[row]
		if !ok {
			continue
		}
		out = append(out, models.SeatRow{Row: row, Seats: seats})
	}
	return out
}

func countAvailableSeats(trip map[string]interface{}) int {
	count := 0
	for _, row := range mapSeatLayout(trip) {
		for _, seat := range row.Seats {
			if seat.Available {
				count++
			}
		}
	}
	return count
}

func inferSeatRow(number string) int {
	digits := strings.Builder{}
	for _, r := range number {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			continue
		}
		break
	}
	row, _ := strconv.Atoi(digits.String())
	if row == 0 {
		return 1
	}
	return row
}

func stringOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
