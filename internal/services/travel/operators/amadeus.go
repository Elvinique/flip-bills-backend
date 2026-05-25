package operators

// AmadeusOperator adapts the Amadeus GDS REST API to the FlightOperator interface.
// Amadeus is the recommended GDS for Nigerian domestic and international routes.
// Credentials & sandbox: https://developers.amadeus.com
// Key endpoints used:
//   - POST /v2/shopping/flight-offers          (search)
//   - POST /v1/shopping/flight-offers/pricing   (price confirmation)
//   - POST /v1/booking/flight-orders            (booking)

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	accessToken  string
	tokenExpiry  time.Time
}

func NewAmadeusOperator(clientID, clientSecret, baseURL string, log *zap.Logger) *AmadeusOperator {
	return &AmadeusOperator{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		client:       &http.Client{Timeout: 20 * time.Second},
		log:          log,
	}
}

func (a *AmadeusOperator) Code() string { return "AMADEUS" }
func (a *AmadeusOperator) Name() string { return "Amadeus GDS" }

// Search returns normalised flight offers for a given route.
func (a *AmadeusOperator) Search(ctx context.Context, req FlightSearchRequest) ([]models.FlightSearchResult, error) {
	if err := a.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("Amadeus auth failed: %w", err)
	}

	payload := map[string]interface{}{
		"originLocationCode":      req.Origin,
		"destinationLocationCode": req.Destination,
		"departureDate":           req.DepartureDate,
		"adults":                  req.Adults,
		"travelClass":             amadeusClass(req.CabinClass),
		"max":                     10,
		"currencyCode":            "NGN",
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.baseURL+"/v2/shopping/flight-offers", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Amadeus search error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Amadeus returned status %d", resp.StatusCode)
	}

	// TODO: decode actual Amadeus FlightOffer schema and map to models.FlightSearchResult.
	// The stub below is returned for sandbox / local dev.
	a.log.Info("Amadeus search called",
		zap.String("origin", req.Origin),
		zap.String("dest", req.Destination),
	)

	departure, _ := time.Parse("2006-01-02", req.DepartureDate)
	departure = departure.Add(10 * time.Hour)

	return []models.FlightSearchResult{
		{
			FlightNumber:   "P47201",
			Airline:        "Air Peace",
			Origin:         req.Origin,
			Destination:    req.Destination,
			DepartureTime:  departure,
			ArrivalTime:    departure.Add(1*time.Hour + 15*time.Minute),
			Duration:       "1h 15m",
			CabinClass:     req.CabinClass,
			PriceKobo:      5500000, // ₦55,000
			PriceNGN:       55000,
			SeatsAvailable: 4,
			GDSRef:         fmt.Sprintf("AMD-%d", time.Now().UnixMilli()),
			Stops:          0,
		},
		{
			FlightNumber:   "IB1143",
			Airline:        "Ibom Air",
			Origin:         req.Origin,
			Destination:    req.Destination,
			DepartureTime:  departure.Add(2 * time.Hour),
			ArrivalTime:    departure.Add(3*time.Hour + 20*time.Minute),
			Duration:       "1h 20m",
			CabinClass:     req.CabinClass,
			PriceKobo:      4800000, // ₦48,000
			PriceNGN:       48000,
			SeatsAvailable: 9,
			GDSRef:         fmt.Sprintf("AMD-%d", time.Now().UnixMilli()+1),
			Stops:          0,
		},
	}, nil
}

func (a *AmadeusOperator) PriceOffer(ctx context.Context, gdsRef string) (*models.FlightSearchResult, error) {
	// TODO: call /v1/shopping/flight-offers/pricing with the GDS offer ID.
	a.log.Info("Amadeus price offer", zap.String("gds_ref", gdsRef))
	return nil, nil
}

func (a *AmadeusOperator) Book(ctx context.Context, gdsRef string, passenger PassengerInfo) (string, error) {
	// TODO: call /v1/booking/flight-orders with traveller details.
	a.log.Info("Amadeus book", zap.String("gds_ref", gdsRef))
	return fmt.Sprintf("AMD-PNR-%d", time.Now().UnixMilli()), nil
}

func (a *AmadeusOperator) Cancel(ctx context.Context, ticketCode string) error {
	a.log.Info("Amadeus cancel", zap.String("ticket", ticketCode))
	return nil
}

// ensureToken fetches or reuses the Amadeus OAuth2 access token.
func (a *AmadeusOperator) ensureToken(ctx context.Context) error {
	if time.Now().Before(a.tokenExpiry) {
		return nil // token still valid
	}

	body := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s",
		a.clientID, a.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/v1/security/oauth2/token",
		bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	a.accessToken = result.AccessToken
	a.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-30) * time.Second)
	return nil
}

func amadeusClass(c string) string {
	if c == "business" {
		return "BUSINESS"
	}
	return "ECONOMY"
}
