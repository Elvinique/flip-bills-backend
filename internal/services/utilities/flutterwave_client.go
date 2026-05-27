package utilities

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
	"time"

	"github.com/flip-bills/backend/internal/models"
)

const (
	defaultFlutterwaveCountry    = "NG"
	defaultFlutterwaveRecurrence = "ONCE"
)

// BillProvider is the VAS provider contract used by the utility service.
type BillProvider interface {
	PurchaseBill(ctx context.Context, req FlutterwaveBillRequest) (*FlutterwaveBillResponse, error)
	CheckBillStatus(ctx context.Context, reference string) (*FlutterwaveBillStatusResponse, error)
}

type FlutterwaveClient struct {
	baseURL    string
	secretKey  string
	httpClient *http.Client
}

type FlutterwavePaymentRequest struct {
	TxRef         string  `json:"tx_ref"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	RedirectURL   string  `json:"redirect_url"`
	CustomerEmail string
	CustomerPhone string
	CustomerName  string
	UserID        string
}

type flutterwavePaymentPayload struct {
	TxRef       string `json:"tx_ref"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	RedirectURL string `json:"redirect_url"`
	Customer    struct {
		Email       string `json:"email"`
		PhoneNumber string `json:"phonenumber"`
		Name        string `json:"name"`
	} `json:"customer"`
	Meta struct {
		UserID string `json:"user_id"`
	} `json:"meta"`
	Customizations struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Logo        string `json:"logo"`
	} `json:"customizations"`
}

type FlutterwavePaymentResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Link string `json:"link"`
	} `json:"data"`
}

type FlutterwaveBillRequest struct {
	Category   models.ServiceCategory
	Reference  string
	CustomerID string
	Amount     int64
	Meta       map[string]interface{}
}

type FlutterwaveBillResponse struct {
	Status  string              `json:"status"`
	Message string              `json:"message"`
	Data    FlutterwaveBillData `json:"data"`
}

type FlutterwaveBillStatusResponse struct {
	Status  string                    `json:"status"`
	Message string                    `json:"message"`
	Data    FlutterwaveBillStatusData `json:"data"`
}

type flutterwaveCategoriesResponse struct {
	Status  string                    `json:"status"`
	Message string                    `json:"message"`
	Data    []FlutterwaveBillCategory `json:"data"`
}

type flutterwaveBillersResponse struct {
	Status  string              `json:"status"`
	Message string              `json:"message"`
	Data    []FlutterwaveBiller `json:"data"`
}

type flutterwaveItemsResponse struct {
	Status  string                `json:"status"`
	Message string                `json:"message"`
	Data    []FlutterwaveBillItem `json:"data"`
}

type FlutterwaveBillData struct {
	Reference      string          `json:"reference"`
	TxRef          string          `json:"tx_ref"`
	BatchReference string          `json:"batch_reference"`
	Code           string          `json:"code"`
	Amount         json.RawMessage `json:"amount"`
	Fee            json.RawMessage `json:"fee"`
	RechargeToken  string          `json:"recharge_token"`
}

type FlutterwaveBillStatusData struct {
	Currency          string          `json:"currency"`
	CustomerID        string          `json:"customer_id"`
	Amount            json.RawMessage `json:"amount"`
	Fee               json.RawMessage `json:"fee"`
	Product           string          `json:"product"`
	ProductName       string          `json:"product_name"`
	CustomerReference string          `json:"customer_reference"`
	Country           string          `json:"country"`
	FlwRef            string          `json:"flw_ref"`
	TxRef             string          `json:"tx_ref"`
	Extra             json.RawMessage `json:"extra"`
	ProductDetails    string          `json:"product_details"`
}

type FlutterwaveBillCategory struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	CodeRaw  string `json:"code"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

type FlutterwaveBiller struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	BillerCode string `json:"biller_code"`
	CodeRaw    string `json:"code"`
	LabelName  string `json:"label_name"`
	Country    string `json:"country"`
	Category   string `json:"category"`
}

type FlutterwaveBillItem struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	ItemCode    string          `json:"item_code"`
	CodeRaw     string          `json:"code"`
	BillerCode  string          `json:"biller_code"`
	Amount      json.RawMessage `json:"amount"`
	Fee         json.RawMessage `json:"fee"`
	Validity    string          `json:"validity"`
	Description string          `json:"description"`
}

type flutterwaveErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func NewFlutterwaveClient(secretKey, baseURL string) *FlutterwaveClient {
	return &FlutterwaveClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		secretKey: secretKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *FlutterwaveClient) PurchaseBill(ctx context.Context, req FlutterwaveBillRequest) (*FlutterwaveBillResponse, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"country":     defaultFlutterwaveCountry,
		"customer":    req.CustomerID,
		"customer_id": req.CustomerID,
		"amount":      koboToNaira(req.Amount),
		"recurrence":  defaultFlutterwaveRecurrence,
		"type":        flutterwaveBillType(req.Category, req.Meta),
		"reference":   req.Reference,
	}

	// For data bundles, Flutterwave requires biller_code and item_code
	// explicitly in the payload — spread meta fields in first, then
	// override with canonical keys to ensure correct field names.
	for key, value := range req.Meta {
		// Skip internal fields that are not Flutterwave payload fields.
		switch key {
		case "plan_code", "network", "meter_type", "platform":
			continue
		}
		payload[key] = value
	}

	// Canonical overrides for data bundle fields.
	if req.Category == models.CategoryData {
		if itemCode := stringFromMeta(req.Meta, "item_code"); itemCode != "" {
			payload["item_code"] = itemCode
		} else if planCode := stringFromMeta(req.Meta, "plan_code"); planCode != "" {
			// Fallback: treat plan_code as item_code for backward compatibility.
			payload["item_code"] = planCode
		}
		if billerCode := stringFromMeta(req.Meta, "biller_code"); billerCode != "" {
			payload["biller_code"] = billerCode
		}
		// Flutterwave data bundles use the phone as customer field.
		if phone := stringFromMeta(req.Meta, "phone"); phone != "" {
			payload["customer"] = phone
			payload["customer_id"] = phone
		}
	}

	// Electricity: meter number is the customer identifier.
	if req.Category == models.CategoryElectricity {
		if meterNumber := stringFromMeta(req.Meta, "meter_number"); meterNumber != "" {
			payload["customer"] = meterNumber
			payload["customer_id"] = meterNumber
		}
		if billerCode := stringFromMeta(req.Meta, "biller_code"); billerCode != "" {
			payload["biller_code"] = billerCode
		}
	}

	// Betting: customer_id is the betting account ID.
	if req.Category == models.CategoryBetting {
		if customerID := stringFromMeta(req.Meta, "customer_id"); customerID != "" {
			payload["customer"] = customerID
			payload["customer_id"] = customerID
		}
		if billerCode := stringFromMeta(req.Meta, "biller_code"); billerCode != "" {
			payload["biller_code"] = billerCode
		}
	}

	var response FlutterwaveBillResponse
	if err := c.do(ctx, http.MethodPost, "/bills", payload, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("flutterwave bill payment failed: %s", response.Message)
	}
	return &response, nil
}

func (c *FlutterwaveClient) CheckBillStatus(ctx context.Context, reference string) (*FlutterwaveBillStatusResponse, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(reference) == "" {
		return nil, fmt.Errorf("flutterwave bill reference is required")
	}

	var response FlutterwaveBillStatusResponse
	if err := c.do(ctx, http.MethodGet, "/bills/"+reference, nil, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("flutterwave bill status check failed: %s", response.Message)
	}
	return &response, nil
}

// InitializePayment creates a standard checkout link for wallet funding.
func (c *FlutterwaveClient) InitializePayment(ctx context.Context, req FlutterwavePaymentRequest) (*FlutterwavePaymentResponse, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if req.TxRef == "" || req.Amount <= 0 {
		return nil, fmt.Errorf("invalid payment request")
	}

	payload := flutterwavePaymentPayload{
		TxRef:       req.TxRef,
		Amount:      fmt.Sprintf("%.2f", req.Amount),
		Currency:    req.Currency,
		RedirectURL: req.RedirectURL,
	}
	payload.Customer.Email = req.CustomerEmail
	payload.Customer.PhoneNumber = req.CustomerPhone
	payload.Customer.Name = req.CustomerName
	payload.Meta.UserID = req.UserID
	payload.Customizations.Title = "Flip Bills Wallet Funding"
	payload.Customizations.Description = "Fund your Flip Bills wallet"

	var resp FlutterwavePaymentResponse
	if err := c.do(ctx, http.MethodPost, "/payments", payload, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("flutterwave payment initialization failed: %s", resp.Message)
	}
	return &resp, nil
}

func (c *FlutterwaveClient) FetchCatalog(ctx context.Context) (*FlutterwaveCatalog, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	var categoriesResponse flutterwaveCategoriesResponse
	if err := c.do(ctx, http.MethodGet, "/top-bill-categories?country="+url.QueryEscape(defaultFlutterwaveCountry), nil, &categoriesResponse); err != nil {
		return nil, err
	}
	if categoriesResponse.Status != "success" {
		return nil, fmt.Errorf("flutterwave bill categories lookup failed: %s", categoriesResponse.Message)
	}

	catalog := &FlutterwaveCatalog{
		Categories: categoriesResponse.Data,
		Billers:    make(map[string][]FlutterwaveBiller),
		Items:      make(map[string][]FlutterwaveBillItem),
	}
	for _, category := range categoriesResponse.Data {
		if !isRelevantFlutterwaveCategory(category) {
			continue
		}
		categoryCode := category.Code()
		var billersResponse flutterwaveBillersResponse
		path := "/bills/" + url.PathEscape(categoryCode) + "/billers?country=" + url.QueryEscape(defaultFlutterwaveCountry)
		if err := c.do(ctx, http.MethodGet, path, nil, &billersResponse); err != nil {
			continue
		}
		if billersResponse.Status != "success" {
			continue
		}
		catalog.Billers[categoryCode] = billersResponse.Data
		if isDataCategory(category) {
			for _, biller := range billersResponse.Data {
				var itemsResponse flutterwaveItemsResponse
				if err := c.do(ctx, http.MethodGet, "/billers/"+url.PathEscape(biller.Code())+"/items", nil, &itemsResponse); err != nil {
					continue
				}
				if itemsResponse.Status == "success" {
					catalog.Items[biller.Code()] = itemsResponse.Data
				}
			}
		}
	}
	return catalog, nil
}

func isRelevantFlutterwaveCategory(category FlutterwaveBillCategory) bool {
	return isAirtimeCategory(category) ||
		isDataCategory(category) ||
		isElectricityCategory(category) ||
		isBettingCategory(category)
}

func (c *FlutterwaveClient) do(ctx context.Context, method, path string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode flutterwave payload: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build flutterwave request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call flutterwave bills api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read flutterwave response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var apiErr flutterwaveErrorResponse
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return fmt.Errorf("flutterwave bills api returned %d: %s", resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("flutterwave bills api returned %d", resp.StatusCode)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode flutterwave response: %w", err)
	}
	return nil
}

func (c *FlutterwaveClient) validate() error {
	if strings.TrimSpace(c.secretKey) == "" {
		return fmt.Errorf("flutterwave secret key is not configured")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("flutterwave base url is not configured")
	}
	return nil
}

func flutterwaveBillType(category models.ServiceCategory, meta map[string]interface{}) string {
	switch category {
	case models.CategoryAirtime:
		return "AIRTIME"
	case models.CategoryData:
		return "MOBILEDATA"
	case models.CategoryElectricity:
		return "UTILITYBILLS"
	case models.CategoryBetting:
		return "BETTING"
	default:
		return strings.ToUpper(string(category))
	}
}

func koboToNaira(amount int64) float64 {
	return float64(amount) / 100
}

func (c FlutterwaveBillCategory) Code() string {
	for _, value := range []string{c.CodeRaw, c.Category, c.Label, c.Name} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return strconv.Itoa(c.ID)
}

func (c FlutterwaveBillCategory) SearchText() string {
	return strings.Join([]string{c.CodeRaw, c.Category, c.Label, c.Name}, " ")
}

func (b FlutterwaveBiller) Code() string {
	for _, value := range []string{b.BillerCode, b.CodeRaw, b.Name} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return strconv.Itoa(b.ID)
}

func (i FlutterwaveBillItem) Code() string {
	for _, value := range []string{i.ItemCode, i.CodeRaw, i.Name} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return strconv.Itoa(i.ID)
}

func (i FlutterwaveBillItem) AmountKobo() int64 {
	if len(i.Amount) == 0 {
		return 0
	}
	var number float64
	if err := json.Unmarshal(i.Amount, &number); err == nil {
		return int64(number * 100)
	}
	var text string
	if err := json.Unmarshal(i.Amount, &text); err == nil {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err == nil {
			return int64(parsed * 100)
		}
	}
	return 0
}
