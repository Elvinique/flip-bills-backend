package operators

// AmadeusOperator adapts the Amadeus Self-Service APIs to the FlightOperator
// interface: OAuth token, flight search, price confirmation, and booking.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type AmadeusOperator struct {
	clientID     string
	clientSecret string
	baseURL      string
	client       *http.Client
	log          *zap.Logger

	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time
	offers      map[string]amadeusFlightOffer
	normalized  map[string]models.FlightSearchResult
}

func NewAmadeusOperator(clientID, clientSecret, baseURL string, log *zap.Logger) *AmadeusOperator {
	return &AmadeusOperator{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      strings.TrimRight(baseURL, "/"),
		client:       &http.Client{Timeout: 25 * time.Second},
		log:          log,
		offers:       make(map[string]amadeusFlightOffer),
		normalized:   make(map[string]models.FlightSearchResult),
	}
}

func (a *AmadeusOperator) Code() string { return "AMADEUS" }
func (a *AmadeusOperator) Name() string { return "Amadeus GDS" }

func (a *AmadeusOperator) Search(ctx context.Context, req FlightSearchRequest) ([]models.FlightSearchResult, error) {
	if err := a.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("Amadeus auth failed: %w", err)
	}

	endpoint, err := url.Parse(a.baseURL + "/v2/shopping/flight-offers")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("originLocationCode", req.Origin)
	query.Set("destinationLocationCode", req.Destination)
	query.Set("departureDate", req.DepartureDate)
	query.Set("adults", strconv.Itoa(defaultAdults(req.Adults)))
	query.Set("travelClass", amadeusClass(req.CabinClass))
	query.Set("currencyCode", "NGN")
	query.Set("max", "20")
	endpoint.RawQuery = query.Encode()

	var result amadeusSearchResponse
	if err := a.doJSON(ctx, http.MethodGet, endpoint.String(), nil, &result); err != nil {
		return nil, err
	}

	out := make([]models.FlightSearchResult, 0, len(result.Data))
	for _, offer := range result.Data {
		mapped, err := mapAmadeusOffer(offer, result.Dictionaries.Carriers, req.CabinClass)
		if err != nil {
			a.log.Warn("could not map Amadeus offer", zap.String("offer_id", offer.ID), zap.Error(err))
			continue
		}
		out = append(out, mapped)

		a.mu.Lock()
		a.offers[offer.ID] = offer
		a.normalized[offer.ID] = mapped
		a.mu.Unlock()
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Amadeus returned no mappable flight offers")
	}
	return out, nil
}

func (a *AmadeusOperator) PriceOffer(ctx context.Context, gdsRef string) (*models.FlightSearchResult, error) {
	if err := a.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("Amadeus auth failed: %w", err)
	}

	a.mu.RLock()
	rawOffer, ok := a.offers[gdsRef]
	fallback := a.normalized[gdsRef]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("flight offer %q not found; search again before booking", gdsRef)
	}

	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"type":         "flight-offers-pricing",
			"flightOffers": []amadeusFlightOffer{rawOffer},
		},
	}

	var result amadeusPricingResponse
	if err := a.doJSON(ctx, http.MethodPost, a.baseURL+"/v1/shopping/flight-offers/pricing", payload, &result); err != nil {
		return nil, err
	}
	if len(result.Data.FlightOffers) == 0 {
		return nil, fmt.Errorf("Amadeus pricing response did not include flight offer")
	}

	priced := result.Data.FlightOffers[0]
	mapped, err := mapAmadeusOffer(priced, result.Dictionaries.Carriers, fallback.CabinClass)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.offers[gdsRef] = priced
	a.normalized[gdsRef] = mapped
	a.mu.Unlock()

	return &mapped, nil
}

func (a *AmadeusOperator) Book(ctx context.Context, gdsRef string, passenger PassengerInfo) (string, error) {
	if err := a.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("Amadeus auth failed: %w", err)
	}

	a.mu.RLock()
	offer, ok := a.offers[gdsRef]
	a.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("flight offer %q not found; search again before booking", gdsRef)
	}

	firstName, lastName := splitPassengerName(passenger.FullName)
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"type":         "flight-order",
			"flightOffers": []amadeusFlightOffer{offer},
			"travelers": []map[string]interface{}{
				{
					"id":          "1",
					"dateOfBirth": "1990-01-01",
					"name": map[string]string{
						"firstName": firstName,
						"lastName":  lastName,
					},
					"contact": map[string]interface{}{
						"emailAddress": passenger.Email,
						"phones": []map[string]string{
							{
								"deviceType":         "MOBILE",
								"countryCallingCode": "234",
								"number":             normalizeNigeriaPhone(passenger.Phone),
							},
						},
					},
				},
			},
		},
	}

	var result amadeusBookingResponse
	if err := a.doJSON(ctx, http.MethodPost, a.baseURL+"/v1/booking/flight-orders", payload, &result); err != nil {
		return "", err
	}
	if result.Data.ID != "" {
		return result.Data.ID, nil
	}
	for _, record := range result.Data.AssociatedRecords {
		if strings.TrimSpace(record.Reference) != "" {
			return record.Reference, nil
		}
	}
	return "", fmt.Errorf("Amadeus booking response missing order reference")
}

func (a *AmadeusOperator) Cancel(ctx context.Context, ticketCode string) error {
	if err := a.ensureToken(ctx); err != nil {
		return fmt.Errorf("Amadeus auth failed: %w", err)
	}
	return a.doJSON(ctx, http.MethodDelete, a.baseURL+"/v1/booking/flight-orders/"+url.PathEscape(ticketCode), nil, nil)
}

func (a *AmadeusOperator) ensureToken(ctx context.Context) error {
	a.mu.RLock()
	if a.accessToken != "" && time.Now().Before(a.tokenExpiry) {
		a.mu.RUnlock()
		return nil
	}
	a.mu.RUnlock()

	if a.clientID == "" || a.clientSecret == "" {
		return fmt.Errorf("Amadeus credentials are not configured")
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", a.clientID)
	form.Set("client_secret", a.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/v1/security/oauth2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Amadeus auth returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return err
	}
	if result.AccessToken == "" {
		return fmt.Errorf("Amadeus auth response missing access token")
	}
	if result.ExpiresIn <= 30 {
		result.ExpiresIn = 1800
	}

	a.mu.Lock()
	a.accessToken = result.AccessToken
	a.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-30) * time.Second)
	a.mu.Unlock()
	return nil
}

func (a *AmadeusOperator) doJSON(ctx context.Context, method, endpoint string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.accessToken)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("Amadeus request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Amadeus returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if out == nil || len(rawBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(rawBody, out); err != nil {
		return fmt.Errorf("Amadeus response decode failed: %w", err)
	}
	return nil
}

type amadeusSearchResponse struct {
	Data         []amadeusFlightOffer `json:"data"`
	Dictionaries struct {
		Carriers map[string]string `json:"carriers"`
	} `json:"dictionaries"`
}

type amadeusPricingResponse struct {
	Data struct {
		FlightOffers []amadeusFlightOffer `json:"flightOffers"`
	} `json:"data"`
	Dictionaries struct {
		Carriers map[string]string `json:"carriers"`
	} `json:"dictionaries"`
}

type amadeusBookingResponse struct {
	Data struct {
		ID                string `json:"id"`
		AssociatedRecords []struct {
			Reference string `json:"reference"`
		} `json:"associatedRecords"`
	} `json:"data"`
}

type amadeusFlightOffer struct {
	Type                     string `json:"type,omitempty"`
	ID                       string `json:"id"`
	Source                   string `json:"source,omitempty"`
	InstantTicketingRequired bool   `json:"instantTicketingRequired,omitempty"`
	NonHomogeneous           bool   `json:"nonHomogeneous,omitempty"`
	OneWay                   bool   `json:"oneWay,omitempty"`
	LastTicketingDate        string `json:"lastTicketingDate,omitempty"`
	LastTicketingDateTime    string `json:"lastTicketingDateTime,omitempty"`
	NumberOfBookableSeats    int    `json:"numberOfBookableSeats,omitempty"`
	Itineraries              []struct {
		Duration string `json:"duration"`
		Segments []struct {
			Departure struct {
				IATACode string `json:"iataCode"`
				At       string `json:"at"`
			} `json:"departure"`
			Arrival struct {
				IATACode string `json:"iataCode"`
				At       string `json:"at"`
			} `json:"arrival"`
			CarrierCode string `json:"carrierCode"`
			Number      string `json:"number"`
		} `json:"segments"`
	} `json:"itineraries"`
	Price struct {
		Currency   string `json:"currency"`
		Total      string `json:"total"`
		GrandTotal string `json:"grandTotal"`
	} `json:"price"`
	TravelerPricings []struct {
		FareDetailsBySegment []struct {
			Cabin string `json:"cabin"`
		} `json:"fareDetailsBySegment"`
	} `json:"travelerPricings,omitempty"`
}

func mapAmadeusOffer(offer amadeusFlightOffer, carriers map[string]string, fallbackCabin string) (models.FlightSearchResult, error) {
	if offer.ID == "" {
		return models.FlightSearchResult{}, fmt.Errorf("missing offer ID")
	}
	if len(offer.Itineraries) == 0 || len(offer.Itineraries[0].Segments) == 0 {
		return models.FlightSearchResult{}, fmt.Errorf("offer %s has no itinerary segments", offer.ID)
	}

	segments := offer.Itineraries[0].Segments
	first := segments[0]
	last := segments[len(segments)-1]
	departure, _ := time.Parse("2006-01-02T15:04:05", first.Departure.At)
	arrival, _ := time.Parse("2006-01-02T15:04:05", last.Arrival.At)

	price := offer.Price.GrandTotal
	if price == "" {
		price = offer.Price.Total
	}
	priceNGN, _ := strconv.ParseFloat(strings.ReplaceAll(price, ",", ""), 64)
	priceKobo := int64(priceNGN * 100)
	if priceKobo <= 0 {
		return models.FlightSearchResult{}, fmt.Errorf("offer %s has no price", offer.ID)
	}

	airline := carriers[first.CarrierCode]
	if airline == "" {
		airline = first.CarrierCode
	}

	cabin := strings.ToLower(fallbackCabin)
	if len(offer.TravelerPricings) > 0 && len(offer.TravelerPricings[0].FareDetailsBySegment) > 0 {
		cabin = strings.ToLower(offer.TravelerPricings[0].FareDetailsBySegment[0].Cabin)
	}
	if cabin == "" {
		cabin = "economy"
	}

	return models.FlightSearchResult{
		FlightNumber:   first.CarrierCode + first.Number,
		Airline:        airline,
		Origin:         first.Departure.IATACode,
		Destination:    last.Arrival.IATACode,
		DepartureTime:  departure,
		ArrivalTime:    arrival,
		Duration:       offer.Itineraries[0].Duration,
		CabinClass:     cabin,
		PriceKobo:      priceKobo,
		PriceNGN:       float64(priceKobo) / 100,
		SeatsAvailable: offer.NumberOfBookableSeats,
		GDSRef:         offer.ID,
		Stops:          len(segments) - 1,
	}, nil
}

func amadeusClass(c string) string {
	if strings.EqualFold(c, "business") {
		return "BUSINESS"
	}
	return "ECONOMY"
}

func defaultAdults(adults int) int {
	if adults < 1 {
		return 1
	}
	return adults
}

func splitPassengerName(fullName string) (string, string) {
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return "Passenger", "FlipBills"
	}
	if len(parts) == 1 {
		return parts[0], "Passenger"
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

func normalizeNigeriaPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	phone = strings.TrimPrefix(phone, "234")
	phone = strings.TrimPrefix(phone, "0")
	return phone
}
