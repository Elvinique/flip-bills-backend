package auth

// deps.go defines the repository and notification interfaces that auth.Service
// depends on. The production code passes concrete *postgres.XRepository and
// *jwtpkg.Manager types which satisfy these interfaces automatically —
// no changes to main.go needed. Tests inject in-memory fakes.

import (
	"context"

	"github.com/flip-bills/backend/internal/models"
	"github.com/flip-bills/backend/pkg/jwt"
	"github.com/google/uuid"
)

type userRepo interface {
	Create(ctx context.Context, u *models.User) error
	FindByID(ctx context.Context, id string) (*models.User, error)
	FindByPhone(ctx context.Context, phone string) (*models.User, error)
	UpdateKYCTier(ctx context.Context, userID string, tier models.KYCTier) error
	UpdatePIN(ctx context.Context, userID, pinHash string) error
	UpdateBVN(ctx context.Context, userID, bvn string) error
	UpdateNIN(ctx context.Context, userID, nin string) error
}

type walletRepo interface {
	Create(ctx context.Context, w *models.Wallet) error
	UpdateDailyLimit(ctx context.Context, userID string, newLimit int64) error
}

type otpRepo interface {
	Create(ctx context.Context, otp *models.OTPToken) error
	FindValid(ctx context.Context, phone string, purpose models.OTPPurpose) (*models.OTPToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

type smsService interface {
	SendOTP(ctx context.Context, phone, otp, purpose string) error
}

type jwtManager interface {
	GenerateAccessToken(userID string, kycTier int) (string, error)
	GenerateRefreshToken(userID string, kycTier int) (string, error)
	Validate(tokenString string) (*jwt.Claims, error)
}
