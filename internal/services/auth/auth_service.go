package auth

import (
	"context"
	"errors"
	"time"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/flip-bills/backend/pkg/crypto"
	jwtpkg "github.com/flip-bills/backend/pkg/jwt"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ── Request / Response DTOs ───────────────────────────────────────────────────

type RegisterRequest struct {
	Phone     string `json:"phone"      binding:"required,e164"`
	Password  string `json:"password"   binding:"required,min=8"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name"  binding:"required"`
}

type LoginRequest struct {
	Phone    string `json:"phone"    binding:"required"`
	Password string `json:"password" binding:"required"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	userRepo   *postgres.UserRepository
	walletRepo *postgres.WalletRepository
	jwt        *jwtpkg.Manager
	log        *zap.Logger
}

func NewService(
	userRepo *postgres.UserRepository,
	walletRepo *postgres.WalletRepository,
	jwt *jwtpkg.Manager,
	log *zap.Logger,
) *Service {
	return &Service{userRepo: userRepo, walletRepo: walletRepo, jwt: jwt, log: log}
}

// Register creates a new user record and auto-provisions their wallet.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*TokenPair, error) {
	// 1. Check phone uniqueness.
	existing, _ := s.userRepo.FindByPhone(ctx, req.Phone)
	if existing != nil {
		return nil, errors.New("phone number is already registered")
	}

	// 2. Hash password.
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// 3. Persist user.
	user := &models.User{
		ID:           uuid.New(),
		Phone:        req.Phone,
		PasswordHash: hash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		KYCTier:      models.KYCTierUnverified,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	// 4. Auto-provision wallet with KYC Tier-0 daily limit (₦50k = 5_000_000 kobo).
	wallet := &models.Wallet{
		ID:         uuid.New(),
		UserID:     user.ID,
		Balance:    0,
		Currency:   models.NGN,
		DailyLimit: 5_000_000,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.walletRepo.Create(ctx, wallet); err != nil {
		return nil, err
	}

	s.log.Info("new user registered", zap.String("user_id", user.ID.String()))
	return s.generateTokenPair(user.ID.String(), int(user.KYCTier))
}

// Login verifies credentials and returns a token pair.
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

// generateTokenPair is a DRY helper for creating the JWT pair.
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
