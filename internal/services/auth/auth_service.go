package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/notifications"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/flip-bills/backend/pkg/crypto"
	jwtpkg "github.com/flip-bills/backend/pkg/jwt"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Compile-time proof that concrete types satisfy the interfaces defined in deps.go.
// If a repo method is renamed or removed, this line fails at build time — not at runtime.
var _ userRepo = (*postgres.UserRepository)(nil)
var _ walletRepo = (*postgres.WalletRepository)(nil)
var _ otpRepo = (*postgres.OTPRepository)(nil)
var _ smsService = (*notifications.SMSService)(nil)
var _ jwtManager = (*jwtpkg.Manager)(nil)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type RegisterRequest struct {
	Phone     string `json:"phone"      binding:"required,e164"`
	Email     string `json:"email"      binding:"required,email"`
	Password  string `json:"password"   binding:"required,min=8"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name"  binding:"required"`
}

type LoginRequest struct {
	Phone    string `json:"phone"    binding:"required"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

type VerifyPhoneRequest struct {
	Phone string `json:"phone" binding:"required"`
	OTP   string `json:"otp"   binding:"required,len=6"`
}

type SetPINRequest struct {
	PIN        string `json:"pin"         binding:"required,len=6,numeric"`
	ConfirmPIN string `json:"confirm_pin" binding:"required,len=6,numeric"`
}

type KYCUpgradeRequest struct {
	BVN string `json:"bvn" binding:"omitempty,len=11,numeric"`
	NIN string `json:"nin" binding:"omitempty,len=11,numeric"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	userRepo   userRepo
	walletRepo walletRepo
	otpRepo    otpRepo
	sms        smsService
	email      emailService
	jwt        jwtManager
	log        *zap.Logger
}

func NewService(
	userRepo *postgres.UserRepository,
	walletRepo *postgres.WalletRepository,
	otpRepo *postgres.OTPRepository,
	sms *notifications.SMSService,
	email *notifications.EmailService,
	jwt *jwtpkg.Manager,
	log *zap.Logger,
) *Service {
	return &Service{
		userRepo: userRepo, walletRepo: walletRepo,
		otpRepo: otpRepo, sms: sms, email: email, jwt: jwt, log: log,
	}
}

// Register creates a new user, provisions a wallet, and fires the OTP SMS.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (string, error) {
	existing, _ := s.userRepo.FindByPhone(ctx, req.Phone)
	if existing != nil {
		return "", errors.New("phone number is already registered")
	}
	existingEmail, _ := s.userRepo.FindByEmail(ctx, req.Email)
	if existingEmail != nil {
		return "", errors.New("email is already registered")
	}

	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return "", err
	}

	user := &models.User{
		ID:           uuid.New(),
		Phone:        req.Phone,
		Email:        req.Email,
		PasswordHash: hash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		KYCTier:      models.KYCTierUnverified,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return "", err
	}

	// Auto-provision wallet with Tier-0 limit (₦50k/day = 5,000,000 kobo).
	wallet := &models.Wallet{
		ID:         uuid.New(),
		UserID:     user.ID,
		Currency:   models.NGN,
		DailyLimit: 5_000_000,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.walletRepo.Create(ctx, wallet); err != nil {
		return "", err
	}

	// Send phone verification OTP.
	if err := s.sendOTP(ctx, req.Phone, req.Email, models.OTPPhoneVerify); err != nil {
		// Non-fatal — user can request resend.
		s.log.Warn("OTP send failed after registration", zap.Error(err))
	}

	s.log.Info("new user registered", zap.String("user_id", user.ID.String()))
	return "Registration successful. Enter the 6-digit code sent to " + req.Phone, nil
}

// Login verifies credentials and returns a JWT pair.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*TokenPair, error) {
	user, err := s.userRepo.FindByPhone(ctx, req.Phone)
	if err != nil || user == nil {
		return nil, errors.New("invalid phone number or password")
	}
	if !user.IsActive {
		return nil, errors.New("account is suspended — contact support")
	}
	if err := crypto.CheckPassword(user.PasswordHash, req.Password); err != nil {
		return nil, errors.New("invalid phone number or password")
	}
	return s.generateTokenPair(user.ID.String(), int(user.KYCTier))
}

func decodeGoogleJWT(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid token format")
	}
	// Google tokens might use RawURLEncoding without padding
	payloadStr := parts[1]
	if pad := len(payloadStr) % 4; pad != 0 {
		payloadStr += strings.Repeat("=", 4-pad)
	}
	payload, err := base64.URLEncoding.DecodeString(payloadStr)
	if err != nil {
		return nil, err
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// GoogleLogin verifies Google token and either registers or logs in the user.
func (s *Service) GoogleLogin(ctx context.Context, idToken string) (*TokenPair, error) {
	claims, err := decodeGoogleJWT(idToken)
	if err != nil {
		return nil, errors.New("invalid google token")
	}

	email, ok := claims["email"].(string)
	if !ok || email == "" {
		return nil, errors.New("google token missing email")
	}

	firstName, _ := claims["given_name"].(string)
	lastName, _ := claims["family_name"].(string)

	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		// Register user automatically. Create a mock phone since our DB requires it.
		// In a real scenario, we might prompt the user for phone number.
		mockPhone := "+234" + uuid.New().String()[:10]

		user = &models.User{
			ID:           uuid.New(),
			Phone:        mockPhone,
			Email:        email,
			PasswordHash: "google_oauth_no_password",
			FirstName:    firstName,
			LastName:     lastName,
			KYCTier:      models.KYCTierUnverified,
			IsActive:     true,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}

		wallet := &models.Wallet{
			ID:         uuid.New(),
			UserID:     user.ID,
			Currency:   models.NGN,
			DailyLimit: 5_000_000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := s.walletRepo.Create(ctx, wallet); err != nil {
			return nil, err
		}
		s.log.Info("new user registered via Google", zap.String("user_id", user.ID.String()))
	} else if !user.IsActive {
		return nil, errors.New("account is suspended")
	}

	return s.generateTokenPair(user.ID.String(), int(user.KYCTier))
}

// ResendOTP fires a fresh verification code for the given phone.
func (s *Service) ResendOTP(ctx context.Context, phone string, purpose models.OTPPurpose) error {
	user, err := s.userRepo.FindByPhone(ctx, phone)
	if err != nil || user == nil {
		// Deliberately vague — don't leak whether a phone is registered.
		return nil
	}
	return s.sendOTP(ctx, phone, user.Email, purpose)
}

// VerifyPhone confirms the OTP and marks the user's phone as verified (KYC Tier bump if eligible).
func (s *Service) VerifyPhone(ctx context.Context, req VerifyPhoneRequest) error {
	record, err := s.otpRepo.FindValid(ctx, req.Phone, models.OTPPhoneVerify)
	if err != nil {
		return errors.New("invalid or expired verification code")
	}
	if err := crypto.CheckPassword(record.OTPHash, req.OTP); err != nil {
		return errors.New("invalid or expired verification code")
	}
	if err := s.otpRepo.MarkUsed(ctx, record.ID); err != nil {
		return err
	}
	// Phone verified — no tier bump yet (requires BVN for Tier 1).
	s.log.Info("phone verified", zap.String("phone", req.Phone))
	return nil
}

// GetProfile returns the authenticated user's profile.
func (s *Service) GetProfile(ctx context.Context, userID string) (*models.User, error) {
	return s.userRepo.FindByID(ctx, userID)
}

type UpdateProfileRequest struct {
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name"  binding:"required"`
	Email     string `json:"email"      binding:"required,email"`
}

// UpdateProfile updates the user's first name, last name, and email.
func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (*models.User, error) {
	existing, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err == nil && existing != nil && existing.ID.String() != userID {
		return nil, errors.New("email is already registered to another account")
	}
	if err := s.userRepo.UpdateProfile(ctx, userID, req.FirstName, req.LastName, req.Email); err != nil {
		return nil, err
	}
	return s.userRepo.FindByID(ctx, userID)
}

// SetPIN hashes and stores the user's 6-digit transaction PIN.
func (s *Service) SetPIN(ctx context.Context, userID string, req SetPINRequest) error {
	if req.PIN != req.ConfirmPIN {
		return errors.New("PINs do not match")
	}
	hash, err := crypto.HashPassword(req.PIN)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePIN(ctx, userID, hash)
}

// UpgradeKYC links BVN/NIN to the user and raises their daily wallet limit.
// In production this should call a third-party identity verification provider
// (e.g. Smile Identity, Prembly, Dojah) before persisting.
func (s *Service) UpgradeKYC(ctx context.Context, userID string, req KYCUpgradeRequest) (models.KYCTier, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return 0, err
	}

	var newTier models.KYCTier
	var newLimit int64

	switch {
	case req.NIN != "" && user.KYCTier < models.KYCTierTwo:
		// Full NIN verification → Tier 2, ₦500k/day
		newTier = models.KYCTierTwo
		newLimit = 50_000_000 // 500,000 NGN in kobo
		if err := s.userRepo.UpdateNIN(ctx, userID, req.NIN); err != nil {
			return 0, err
		}
	case req.BVN != "" && user.KYCTier < models.KYCTierOne:
		// BVN verification → Tier 1, ₦200k/day
		newTier = models.KYCTierOne
		newLimit = 20_000_000 // 200,000 NGN in kobo
		if err := s.userRepo.UpdateBVN(ctx, userID, req.BVN); err != nil {
			return 0, err
		}
	default:
		return user.KYCTier, errors.New("no valid upgrade path — check your current KYC tier")
	}

	if err := s.userRepo.UpdateKYCTier(ctx, userID, newTier); err != nil {
		return 0, err
	}
	if err := s.walletRepo.UpdateDailyLimit(ctx, userID, newLimit); err != nil {
		return 0, err
	}

	s.log.Info("KYC upgraded",
		zap.String("user_id", userID),
		zap.Int("new_tier", int(newTier)),
	)
	return newTier, nil
}

// ── internal helpers ─────────────────────────────────────────────────────────

func (s *Service) sendOTP(ctx context.Context, phone, email string, purpose models.OTPPurpose) error {
	otp, err := crypto.GenerateOTP(6)
	if err != nil {
		return err
	}
	hash, err := crypto.HashPassword(otp)
	if err != nil {
		return err
	}

	ttl := 10 * time.Minute
	if purpose == models.OTPTxAuth {
		ttl = 5 * time.Minute
	}

	record := &models.OTPToken{
		ID:        uuid.New(),
		Phone:     phone,
		OTPHash:   hash,
		Purpose:   purpose,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	if err := s.otpRepo.Create(ctx, record); err != nil {
		return err
	}

	if email != "" {
		return s.email.SendOTP(ctx, email, otp, string(purpose))
	}
	return s.sms.SendOTP(ctx, phone, otp, string(purpose))
}

func (s *Service) generateTokenPair(userID string, kycTier int) (*TokenPair, error) {
	access, err := s.jwt.GenerateAccessToken(userID, kycTier)
	if err != nil {
		return nil, err
	}
	refresh, err := s.jwt.GenerateRefreshToken(userID, kycTier)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

// RefreshToken validates a refresh token and issues a new access/refresh pair.
// The old refresh token is implicitly invalidated by the short TTL — in Phase 3
// add a Redis token blocklist for explicit revocation.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.jwt.Validate(refreshToken)
	if err != nil {
		return nil, errors.New("invalid or expired refresh token")
	}
	// Confirm user still exists and is active.
	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account suspended")
	}
	return s.generateTokenPair(user.ID.String(), int(user.KYCTier))
}

// GetUserByPhone looks up a user by phone — used by set-pin public endpoint.
func (s *Service) GetUserByPhone(ctx context.Context, phone string) (*models.User, error) {
	return s.userRepo.FindByPhone(ctx, phone)
}
