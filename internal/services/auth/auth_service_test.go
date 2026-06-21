package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/pkg/crypto"
	jwtpkg "github.com/flip-bills/backend/pkg/jwt"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── In-memory fakes ───────────────────────────────────────────────────────────

type fakeUserRepo struct {
	mu      sync.RWMutex
	byPhone map[string]*models.User
	byID    map[string]*models.User
	byEmail map[string]*models.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byPhone: map[string]*models.User{}, byID: map[string]*models.User{}, byEmail: map[string]*models.User{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *models.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byPhone[u.Phone] = u
	r.byID[u.ID.String()] = u
	if u.Email != "" {
		r.byEmail[u.Email] = u
	}
	return nil
}
func (r *fakeUserRepo) FindByPhone(_ context.Context, phone string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byPhone[phone]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byEmail[email]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (r *fakeUserRepo) UpdateProfile(_ context.Context, userID, firstName, lastName, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return errors.New("not found")
	}
	u.FirstName = firstName
	u.LastName = lastName
	u.Email = email
	return nil
}
func (r *fakeUserRepo) FindByID(_ context.Context, id string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}
func (r *fakeUserRepo) UpdateKYCTier(_ context.Context, userID string, tier models.KYCTier) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[userID]; ok {
		u.KYCTier = tier
	}
	return nil
}
func (r *fakeUserRepo) UpdatePIN(_ context.Context, userID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[userID]; ok {
		u.PinHash = hash
	}
	return nil
}
func (r *fakeUserRepo) UpdateBVN(_ context.Context, userID, bvn string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[userID]; ok {
		u.BVN = bvn
	}
	return nil
}
func (r *fakeUserRepo) UpdateNIN(_ context.Context, userID, nin string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[userID]; ok {
		u.NIN = nin
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeWalletRepo struct{}

func (r *fakeWalletRepo) Create(_ context.Context, _ *models.Wallet) error            { return nil }
func (r *fakeWalletRepo) UpdateDailyLimit(_ context.Context, _ string, _ int64) error { return nil }

// ─────────────────────────────────────────────────────────────────────────────

type fakeOTPRepo struct {
	mu     sync.Mutex
	tokens []*models.OTPToken
}

func (r *fakeOTPRepo) Create(_ context.Context, o *models.OTPToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if t.Phone == o.Phone && t.Purpose == o.Purpose {
			t.Used = true
		}
	}
	r.tokens = append(r.tokens, o)
	return nil
}
func (r *fakeOTPRepo) FindValid(_ context.Context, phone string, purpose models.OTPPurpose) (*models.OTPToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.tokens) - 1; i >= 0; i-- {
		t := r.tokens[i]
		if t.Phone == phone && t.Purpose == purpose && !t.Used && t.ExpiresAt.After(time.Now()) {
			return t, nil
		}
	}
	return nil, errors.New("no valid OTP")
}
func (r *fakeOTPRepo) MarkUsed(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if t.ID == id {
			t.Used = true
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────

type fakeSMS struct {
	mu   sync.Mutex
	sent []string
}

func (s *fakeSMS) SendOTP(_ context.Context, phone, _, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, phone)
	return nil
}

// ── Test helper ───────────────────────────────────────────────────────────────

type fakeEmail struct {
	mu   sync.Mutex
	Sent []string
}

func (f *fakeEmail) SendOTP(_ context.Context, email, otp, purpose string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = append(f.Sent, email)
	return nil
}

func newTestService(t *testing.T) (*Service, *fakeUserRepo, *fakeOTPRepo, *fakeSMS) {
	t.Helper()
	users := newFakeUserRepo()
	otp := &fakeOTPRepo{}
	sms := &fakeSMS{}
	svc := &Service{
		userRepo:   users,
		walletRepo: &fakeWalletRepo{},
		otpRepo:    otp,
		sms:        sms,
		email:      &fakeEmail{},
		jwt:        jwtpkg.NewManager("test-secret-min-32-chars-long!!!!!", 15*time.Minute, 720*time.Hour),
		log:        zap.NewNop(),
	}
	return svc, users, otp, sms
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	svc, users, _, sms := newTestService(t)
	ctx := context.Background()

	msg, err := svc.Register(ctx, RegisterRequest{
		Phone: "+2348012345678", Password: "Password123!", FirstName: "Ada", LastName: "Okonkwo",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if msg == "" {
		t.Fatal("expected success message")
	}

	u, err := users.FindByPhone(ctx, "+2348012345678")
	if err != nil {
		t.Fatalf("user not persisted: %v", err)
	}
	if u.FirstName != "Ada" {
		t.Errorf("FirstName = %q, want Ada", u.FirstName)
	}
	if u.PasswordHash == "Password123!" {
		t.Fatal("password stored as plain text")
	}
	if u.KYCTier != models.KYCTierUnverified {
		t.Errorf("KYCTier = %d, want 0", u.KYCTier)
	}

	sms.mu.Lock()
	sent := len(sms.sent)
	sms.mu.Unlock()
	if sent == 0 {
		t.Fatal("OTP SMS not sent after registration")
	}
}

func TestRegister_DuplicatePhone(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	ctx := context.Background()
	req := RegisterRequest{Phone: "+2348012345678", Password: "Pass1234!", FirstName: "A", LastName: "B"}

	if _, err := svc.Register(ctx, req); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := svc.Register(ctx, req); err == nil {
		t.Fatal("duplicate phone should fail")
	}
}

func TestLogin_Success(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterRequest{Phone: "+2348099887766", Password: "Secure99!", FirstName: "E", LastName: "E"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	tokens, err := svc.Login(ctx, LoginRequest{Phone: "+2348099887766", Password: "Secure99!"})
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("access token empty")
	}
	if tokens.RefreshToken == "" {
		t.Fatal("refresh token empty")
	}
	if tokens.AccessToken == tokens.RefreshToken {
		t.Fatal("access and refresh tokens must differ")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348011223344", Password: "RealPass1!", FirstName: "X", LastName: "Y"})

	if _, err := svc.Login(ctx, LoginRequest{Phone: "+2348011223344", Password: "WrongPass!"}); err == nil {
		t.Fatal("wrong password should fail")
	}
}

func TestLogin_UnknownPhone(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	if _, err := svc.Login(context.Background(), LoginRequest{Phone: "+2340000000000", Password: "any"}); err == nil {
		t.Fatal("unknown phone should fail")
	}
}

func TestLogin_InactiveAccount(t *testing.T) {
	svc, users, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348055443322", Password: "Pass1234!", FirstName: "X", LastName: "Y"})

	u, _ := users.FindByPhone(ctx, "+2348055443322")
	users.mu.Lock()
	u.IsActive = false
	users.mu.Unlock()

	if _, err := svc.Login(ctx, LoginRequest{Phone: "+2348055443322", Password: "Pass1234!"}); err == nil {
		t.Fatal("inactive account should fail")
	}
}

func TestVerifyPhone_Success(t *testing.T) {
	svc, _, otpStore, _ := newTestService(t)
	ctx := context.Background()
	phone := "+2348022334455"

	otp := "482910"
	hash, _ := crypto.HashPassword(otp)
	_ = otpStore.Create(ctx, &models.OTPToken{
		ID: uuid.New(), Phone: phone, OTPHash: hash,
		Purpose: models.OTPPhoneVerify, ExpiresAt: time.Now().Add(10 * time.Minute),
	})

	if err := svc.VerifyPhone(ctx, VerifyPhoneRequest{Phone: phone, OTP: otp}); err != nil {
		t.Fatalf("VerifyPhone error: %v", err)
	}
	// Second use of same OTP must fail.
	if err := svc.VerifyPhone(ctx, VerifyPhoneRequest{Phone: phone, OTP: otp}); err == nil {
		t.Fatal("second VerifyPhone with same OTP should fail")
	}
}

func TestVerifyPhone_WrongOTP(t *testing.T) {
	svc, _, otpStore, _ := newTestService(t)
	ctx := context.Background()
	phone := "+2348011112222"
	hash, _ := crypto.HashPassword("123456")
	_ = otpStore.Create(ctx, &models.OTPToken{
		ID: uuid.New(), Phone: phone, OTPHash: hash,
		Purpose: models.OTPPhoneVerify, ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err := svc.VerifyPhone(ctx, VerifyPhoneRequest{Phone: phone, OTP: "999999"}); err == nil {
		t.Fatal("wrong OTP should fail")
	}
}

func TestVerifyPhone_ExpiredOTP(t *testing.T) {
	svc, _, otpStore, _ := newTestService(t)
	ctx := context.Background()
	phone := "+2348033221100"
	hash, _ := crypto.HashPassword("111111")
	_ = otpStore.Create(ctx, &models.OTPToken{
		ID: uuid.New(), Phone: phone, OTPHash: hash,
		Purpose: models.OTPPhoneVerify, ExpiresAt: time.Now().Add(-1 * time.Minute), // already expired
	})
	if err := svc.VerifyPhone(ctx, VerifyPhoneRequest{Phone: phone, OTP: "111111"}); err == nil {
		t.Fatal("expired OTP should fail")
	}
}

func TestSetPIN_Success(t *testing.T) {
	svc, users, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348031415926", Password: "Password1!", FirstName: "F", LastName: "L"})
	u, _ := users.FindByPhone(ctx, "+2348031415926")

	if err := svc.SetPIN(ctx, u.ID.String(), SetPINRequest{PIN: "123456", ConfirmPIN: "123456"}); err != nil {
		t.Fatalf("SetPIN error: %v", err)
	}
	updated, _ := users.FindByID(ctx, u.ID.String())
	if updated.PinHash == "" {
		t.Fatal("PinHash not stored")
	}
	if err := crypto.CheckPassword(updated.PinHash, "123456"); err != nil {
		t.Fatal("stored PIN hash does not match")
	}
}

func TestSetPIN_MismatchedPINs(t *testing.T) {
	svc, users, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348041592653", Password: "Password1!", FirstName: "F", LastName: "L"})
	u, _ := users.FindByPhone(ctx, "+2348041592653")

	if err := svc.SetPIN(ctx, u.ID.String(), SetPINRequest{PIN: "123456", ConfirmPIN: "654321"}); err == nil {
		t.Fatal("mismatched PINs should fail")
	}
}

func TestRefreshToken_Success(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348051623578", Password: "Pass1234!", FirstName: "R", LastName: "T"})
	tokens, _ := svc.Login(ctx, LoginRequest{Phone: "+2348051623578", Password: "Pass1234!"})

	newTokens, err := svc.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken error: %v", err)
	}
	if newTokens.AccessToken == "" {
		t.Fatal("new access token empty")
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	if _, err := svc.RefreshToken(context.Background(), "not.a.real.token"); err == nil {
		t.Fatal("invalid refresh token should fail")
	}
}

func TestUpgradeKYC_BVN(t *testing.T) {
	svc, users, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348061803398", Password: "Pass1234!", FirstName: "K", LastName: "Y"})
	u, _ := users.FindByPhone(ctx, "+2348061803398")

	tier, err := svc.UpgradeKYC(ctx, u.ID.String(), KYCUpgradeRequest{BVN: "12345678901"})
	if err != nil {
		t.Fatalf("UpgradeKYC BVN error: %v", err)
	}
	if tier != models.KYCTierOne {
		t.Errorf("tier = %d, want KYCTierOne", tier)
	}

	updated, _ := users.FindByID(ctx, u.ID.String())
	if updated.KYCTier != models.KYCTierOne {
		t.Errorf("stored tier = %d, want KYCTierOne", updated.KYCTier)
	}
	if updated.BVN != "12345678901" {
		t.Errorf("BVN = %q, want 12345678901", updated.BVN)
	}
}

func TestUpgradeKYC_NIN_GoesToTierTwo(t *testing.T) {
	svc, users, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348071067811", Password: "Pass1234!", FirstName: "N", LastName: "I"})
	u, _ := users.FindByPhone(ctx, "+2348071067811")

	tier, err := svc.UpgradeKYC(ctx, u.ID.String(), KYCUpgradeRequest{NIN: "98765432101"})
	if err != nil {
		t.Fatalf("UpgradeKYC NIN error: %v", err)
	}
	if tier != models.KYCTierTwo {
		t.Errorf("tier = %d, want KYCTierTwo", tier)
	}
}

func TestUpgradeKYC_NoCredentials(t *testing.T) {
	svc, users, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterRequest{Phone: "+2348081828384", Password: "Pass1234!", FirstName: "X", LastName: "Y"})
	u, _ := users.FindByPhone(ctx, "+2348081828384")

	if _, err := svc.UpgradeKYC(ctx, u.ID.String(), KYCUpgradeRequest{}); err == nil {
		t.Fatal("empty KYC upgrade should fail")
	}
}
