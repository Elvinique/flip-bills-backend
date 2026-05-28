package utilities

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"go.uber.org/zap"
)

type MonnifyFallbackClient struct {
	apiKey    string
	secretKey string
	baseURL   string
	client    *http.Client
	log       *zap.Logger
}

func NewMonnifyFallbackClient(apiKey, secretKey, baseURL string, log *zap.Logger) *MonnifyFallbackClient {
	return &MonnifyFallbackClient{
		apiKey:    apiKey,
		secretKey: secretKey,
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 20 * time.Second},
		log:       log,
	}
}

func (m *MonnifyFallbackClient) PurchaseBill(ctx context.Context, params BillPurchaseParams) (*UnifiedBillResponse, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("monnify fallback: api key not configured")
	}

	serviceType := monnifyServiceType(params.Category)
	if serviceType == "" {
		return nil, fmt.Errorf("monnify fallback: unsupported category %q", params.Category)
	}

	payload := map[string]interface{}{
		"amount":          float64(params.Amount) / 100,
		"serviceType":     serviceType,
		"uniqueReference": params.Reference,
		"device":          params.CustomerID,
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.baseURL+"/api/v1/bills-payment/bills/pay", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+monnifyBasicAuth(m.apiKey, m.secretKey))

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("monnify PurchaseBill: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		RequestSuccessful bool   `json:"requestSuccessful"`
		ResponseMessage   string `json:"responseMessage"`
		ResponseBody      struct {
			UniqueReference string `json:"uniqueReference"`
			RechargeToken   string `json:"rechargeToken"`
		} `json:"responseBody"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("monnify PurchaseBill decode: %w", err)
	}
	if !result.RequestSuccessful {
		return nil, fmt.Errorf("monnify PurchaseBill failed: %s", result.ResponseMessage)
	}

	return &UnifiedBillResponse{
		ExternalReference: result.ResponseBody.UniqueReference,
		RechargeToken:     result.ResponseBody.RechargeToken,
		Status:            "success",
		RawMessage:        respBody,
	}, nil
}

func (m *MonnifyFallbackClient) CheckBillStatus(ctx context.Context, reference string) (*UnifiedBillResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		m.baseURL+"/api/v1/bills-payment/bills/"+reference, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Basic "+monnifyBasicAuth(m.apiKey, m.secretKey))

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("monnify CheckBillStatus: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		RequestSuccessful bool `json:"requestSuccessful"`
		ResponseBody      struct {
			Status            string `json:"status"`
			CustomerReference string `json:"customerReference"`
			ProductName       string `json:"productName"`
		} `json:"responseBody"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("monnify CheckBillStatus decode: %w", err)
	}

	monnifyStatus := "pending"
	if result.RequestSuccessful && result.ResponseBody.Status == "PAID" {
		monnifyStatus = "success"
	}

	return &UnifiedBillResponse{
		ExternalReference: result.ResponseBody.CustomerReference,
		Status:            monnifyStatus,
		RawMessage:        respBody,
	}, nil
}

func monnifyServiceType(category models.ServiceCategory) string {
	switch category {
	case models.CategoryAirtime:
		return "AIRTIME"
	case models.CategoryData:
		return "DATA"
	case models.CategoryElectricity:
		return "ELECTRICITY"
	default:
		return ""
	}
}

func monnifyBasicAuth(apiKey, secretKey string) string {
	raw := apiKey + ":" + secretKey
	return base64.StdEncoding.EncodeToString([]byte(raw))
}
