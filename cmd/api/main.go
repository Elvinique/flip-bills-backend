package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flip-bills/backend/internal/config"
	authhandler "github.com/flip-bills/backend/internal/handlers/auth"
	utilhandler "github.com/flip-bills/backend/internal/handlers/utilities"
	"github.com/flip-bills/backend/internal/middleware"
	"github.com/flip-bills/backend/internal/repository/postgres"
	"github.com/flip-bills/backend/internal/services/auth"
	"github.com/flip-bills/backend/internal/services/reconciliation"
	"github.com/flip-bills/backend/internal/services/utilities"
	jwtpkg "github.com/flip-bills/backend/pkg/jwt"
	"github.com/flip-bills/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	// ── 1. Load config ────────────────────────────────────────────────────────
	cfg := config.Load()
	log := logger.New(cfg.AppEnv)
	defer log.Sync()

	log.Info("starting Flip Bills API", zap.String("env", cfg.AppEnv))

	// ── 2. Connect PostgreSQL ─────────────────────────────────────────────────
	pgConnStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s pool_max_conns=20",
		cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode,
	)
	pgPool, err := pgxpool.New(context.Background(), pgConnStr)
	if err != nil {
		log.Fatal("failed to connect to PostgreSQL", zap.Error(err))
	}
	defer pgPool.Close()
	log.Info("PostgreSQL connected")

	// ── 3. Connect Redis ──────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatal("failed to connect to Redis", zap.Error(err))
	}
	log.Info("Redis connected")

	// ── 4. Bootstrap dependencies ─────────────────────────────────────────────
	jwtManager  := jwtpkg.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	userRepo    := postgres.NewUserRepository(pgPool)
	walletRepo  := postgres.NewWalletRepository(pgPool)
	reconEngine := reconciliation.NewEngine(walletRepo, log, cfg.Recon.TimeoutSeconds)

	authSvc    := auth.NewService(userRepo, walletRepo, jwtManager, log)
	utilitySvc := utilities.NewService(walletRepo, reconEngine, log)

	authH    := authhandler.NewHandler(authSvc, log)
	utilityH := utilhandler.NewHandler(utilitySvc, log)

	// ── 5. Build Gin router ───────────────────────────────────────────────────
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())

	// Global rate limit: 100 req/min per IP
	r.Use(middleware.RateLimit(rdb, 100, time.Minute))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "flip-bills-api"})
	})

	// ── Auth routes (public) ──────────────────────────────────────────────────
	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.POST("/register", authH.Register)
		authGroup.POST("/login",    authH.Login)
	}

	// ── Protected routes ──────────────────────────────────────────────────────
	protected := r.Group("/api/v1")
	protected.Use(middleware.Auth(jwtManager))
	{
		// Utilities — Phase 1
		vas := protected.Group("/vas")
		{
			vas.POST("/airtime",     utilityH.BuyAirtime)
			vas.POST("/data",        utilityH.BuyData)
			vas.POST("/electricity", utilityH.PayElectricity)
			vas.POST("/betting",     utilityH.FundBetting)
		}

		// Travel — Phase 2 (stubs, to be wired in Phase 2)
		travel := protected.Group("/travel")
		{
			travel.GET("/bus/search",    placeholderHandler("bus search — Phase 2"))
			travel.POST("/bus/book",     placeholderHandler("bus booking — Phase 2"))
			travel.GET("/flight/search", placeholderHandler("flight search — Phase 2"))
			travel.POST("/flight/book",  placeholderHandler("flight booking — Phase 2"))
		}

		// Wallet
		wallet := protected.Group("/wallet")
		{
			wallet.GET("/balance",      placeholderHandler("wallet balance — Phase 1"))
			wallet.GET("/transactions", placeholderHandler("transaction history — Phase 1"))
		}
	}

	// ── 6. Start HTTP server with graceful shutdown ────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		log.Info("API server listening", zap.String("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal (CTRL+C or container SIGTERM).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down gracefully — draining in-flight requests...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("forced shutdown", zap.Error(err))
	}
	log.Info("server exited")
}

// placeholderHandler returns a 501 response for routes planned in future phases.
func placeholderHandler(feature string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"success": false,
			"message": fmt.Sprintf("coming soon: %s", feature),
		})
	}
}
