package notifications

// SMSService wraps the Termii API for OTP delivery and transactional SMS.
// Termii is the recommended Nigerian SMS gateway with strong local delivery rates.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type SMSService struct {
	apiKey  string
	baseURL string
	client  *http.Client
	log     *zap.Logger
}

func NewSMSService(apiKey, baseURL string, log *zap.Logger) *SMSService {
	return &SMSService{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log,
	}
}

type termiiSendRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	SMS     string `json:"sms"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
	APIKey  string `json:"api_key"`
}

// SendOTP dispatches an OTP code to a Nigerian phone number.
func (s *SMSService) SendOTP(ctx context.Context, phone, otp, purpose string) error {
	var msg string
	switch purpose {
	case "phone_verify":
		msg = fmt.Sprintf("Your Flip Bills verification code is %s. Valid for 10 minutes. Do not share this code.", otp)
	case "pin_reset":
		msg = fmt.Sprintf("Your Flip Bills PIN reset code is %s. Valid for 10 minutes. If you did not request this, contact support.", otp)
	case "tx_auth":
		msg = fmt.Sprintf("Your Flip Bills transaction authorisation code is %s. Valid for 5 minutes.", otp)
	default:
		msg = fmt.Sprintf("Your Flip Bills code is %s. Valid for 10 minutes.", otp)
	}
	return s.send(ctx, phone, msg)
}

// SendTransactionalAlert sends a post-payment confirmation SMS.
// This doubles as the hard fallback receipt described in PRD Section 3C.
func (s *SMSService) SendTransactionalAlert(ctx context.Context, phone, narration string, amountKobo int64) error {
	msg := fmt.Sprintf(
		"Flip Bills: ₦%.2f debited for %s. If this was not you, call 0800-FLIP-BILLS immediately.",
		float64(amountKobo)/100, narration,
	)
	return s.send(ctx, phone, msg)
}

func (s *SMSService) SendVASSuccessAlert(ctx context.Context, phone, narration string, amountKobo int64, reference string, token string) error {
	msg := fmt.Sprintf(
		"Flip Bills: ₦%.2f payment successful for %s. Ref: %s.",
		float64(amountKobo)/100, narration, reference,
	)
	if token != "" {
		msg = fmt.Sprintf("%s Token: %s.", msg, token)
	}
	return s.send(ctx, phone, msg)
}

func (s *SMSService) SendVASRefundAlert(ctx context.Context, phone, narration string, amountKobo int64, reference string) error {
	msg := fmt.Sprintf(
		"Flip Bills: Your ₦%.2f payment for %s could not be completed and has been refunded. Ref: %s.",
		float64(amountKobo)/100, narration, reference,
	)
	return s.send(ctx, phone, msg)
}

// SendBookingConfirmation is the PRD Section 3C SMS fallback for travel tickets.
func (s *SMSService) SendBookingConfirmation(ctx context.Context, phone, ticketCode, route, departure string) error {
	msg := fmt.Sprintf(
		"Flip Bills Booking Confirmed!\nRoute: %s\nDeparture: %s\nTicket: %s\nShow this SMS if app is offline.",
		route, departure, ticketCode,
	)
	return s.send(ctx, phone, msg)
}

// SendDisruptionAlert is the PRD Section 3B SMS broadcast to affected passengers.
func (s *SMSService) SendDisruptionAlert(
	ctx context.Context,
	phone, passengerName, ticketCode, route, departure, reason, bookingID string,
) error {
	msg := fmt.Sprintf(
		"Flip Bills Alert: Hi %s, your trip %s (%s) on %s has been disrupted. Reason: %s. "+
			"Open the app to reschedule or get an instant refund. Booking ref: %s",
		passengerName, route, ticketCode, departure, reason, bookingID,
	)
	return s.send(ctx, phone, msg)
}

func (s *SMSService) send(ctx context.Context, phone, message string) error {
	payload := termiiSendRequest{
		To:      phone,
		From:    "FlipBills",
		SMS:     message,
		Type:    "plain",
		Channel: "dnd", // DND-compliant channel for Nigerian numbers
		APIKey:  s.apiKey,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/sms/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("SMS delivery failed", zap.String("phone", phone), zap.Error(err))
		return fmt.Errorf("SMS delivery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SMS gateway returned status %d", resp.StatusCode)
	}

	s.log.Info("SMS sent", zap.String("phone", phone))
	return nil
}
