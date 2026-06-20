package notifications

// EmailService wraps the Brevo (formerly Sendinblue) transactional email API
// for OTP delivery. Used as the primary OTP channel while SMS (Termii) is
// not yet configured with production credentials.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type EmailService struct {
	apiKey      string
	senderEmail string
	senderName  string
	client      *http.Client
	log         *zap.Logger
}

func NewEmailService(apiKey, senderEmail, senderName string, log *zap.Logger) *EmailService {
	return &EmailService{
		apiKey:      apiKey,
		senderEmail: senderEmail,
		senderName:  senderName,
		client:      &http.Client{Timeout: 10 * time.Second},
		log:         log,
	}
}

type brevoSender struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type brevoRecipient struct {
	Email string `json:"email"`
}

type brevoSendRequest struct {
	Sender      brevoSender       `json:"sender"`
	To          []brevoRecipient  `json:"to"`
	Subject     string            `json:"subject"`
	HTMLContent string            `json:"htmlContent"`
}

// SendOTP dispatches an OTP code to a user's email address.
func (e *EmailService) SendOTP(ctx context.Context, email, otp, purpose string) error {
	var subject, intro string
	switch purpose {
	case "phone_verify":
		subject = "Verify your Flip Bills account"
		intro = "Your verification code is"
	case "pin_reset":
		subject = "Reset your Flip Bills PIN"
		intro = "Your PIN reset code is"
	case "tx_auth":
		subject = "Authorise your Flip Bills transaction"
		intro = "Your transaction authorisation code is"
	default:
		subject = "Your Flip Bills code"
		intro = "Your code is"
	}

	html := fmt.Sprintf(`
		<div style="font-family: sans-serif; max-width: 480px; margin: 0 auto;">
			<h2 style="color: #0B6E4F;">Flip Bills</h2>
			<p>%s:</p>
			<p style="font-size: 32px; font-weight: bold; letter-spacing: 4px; color: #0B6E4F;">%s</p>
			<p>This code expires in 10 minutes. Do not share it with anyone.</p>
			<p style="color: #888; font-size: 12px;">If you did not request this, you can safely ignore this email.</p>
		</div>`, intro, otp)

	return e.send(ctx, email, subject, html)
}

func (e *EmailService) send(ctx context.Context, toEmail, subject, htmlContent string) error {
	payload := brevoSendRequest{
		Sender: brevoSender{
			Name:  e.senderName,
			Email: e.senderEmail,
		},
		To:          []brevoRecipient{{Email: toEmail}},
		Subject:     subject,
		HTMLContent: htmlContent,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("api-key", e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		e.log.Error("email delivery failed", zap.String("email", toEmail), zap.Error(err))
		return fmt.Errorf("email delivery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("email gateway returned status %d", resp.StatusCode)
	}

	e.log.Info("email sent", zap.String("email", toEmail))
	return nil
}
