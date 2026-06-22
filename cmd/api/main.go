package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flip-bills/backend/internal/config"
	authhandler "github.com/flip-bills/backend/internal/handlers/auth"
	dispatcherhandler "github.com/flip-bills/backend/internal/handlers/dispatcher"
	loyaltyhandler "github.com/flip-bills/backend/internal/handlers/loyalty"
	travelhandler "github.com/flip-bills/backend/internal/handlers/travel"
	utilhandler "github.com/flip-bills/backend/internal/handlers/utilities"
	wallethandler "github.com/flip-bills/backend/internal/handlers/wallet"
	webhookhandler "github.com/flip-bills/backend/internal/handlers/webhooks"
	"github.com/flip-bills/backend/internal/middleware"
	"github.com/flip-bills/backend/internal/notifications"
	mongorepo "github.com/flip-bills/backend/internal/repository/mongo"
	"github.com/flip-bills/backend/internal/repository/postgres"
	authsvc "github.com/flip-bills/backend/internal/services/auth"
	dispatchersvc "github.com/flip-bills/backend/internal/services/dispatcher"
	loyaltysvc "github.com/flip-bills/backend/internal/services/loyalty"
	reconcile "github.com/flip-bills/backend/internal/services/reconciliation"
	travelsvc "github.com/flip-bills/backend/internal/services/travel"
	"github.com/flip-bills/backend/internal/services/travel/operators"
	utilitysvc "github.com/flip-bills/backend/internal/services/utilities"
	walletsvc "github.com/flip-bills/backend/internal/services/wallet"
	
	"github.com/flip-bills/backend/internal/ledger"
	"github.com/flip-bills/backend/internal/providers"
	"github.com/flip-bills/backend/internal/providers/opay"
	"github.com/flip-bills/backend/internal/providers/paystack"
	"github.com/flip-bills/backend/internal/queue"
	transferhandler "github.com/flip-bills/backend/internal/handlers/transfer"
	transfersvc "github.com/flip-bills/backend/internal/services/transfer"
	vahandler "github.com/flip-bills/backend/internal/handlers/virtualaccount"
	vasvc "github.com/flip-bills/backend/internal/services/virtualaccount"

	jwtpkg "github.com/flip-bills/backend/pkg/jwt"
	"github.com/flip-bills/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.AppEnv)
	defer func() { _ = log.Sync() }()
	log.Info("starting Flip Bills API", zap.String("env", cfg.AppEnv))

	// ── PostgreSQL ────────────────────────────────────────────────────────────
	pgPool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s pool_max_conns=20",
		cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode,
	))
	if err != nil {
		log.Fatal("PostgreSQL connection failed", zap.Error(err))
	}
	defer pgPool.Close()
	log.Info("PostgreSQL connected")

	// ── MongoDB ───────────────────────────────────────────────────────────────
	mongoClient, err := mongo.Connect(context.Background(), mongooptions.Client().ApplyURI(cfg.Mongo.URI))
	if err != nil {
		log.Fatal("MongoDB connection failed", zap.Error(err))
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	mongoDB := mongoClient.Database(cfg.Mongo.DB)
	log.Info("MongoDB connected")

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatal("Redis connection failed", zap.Error(err))
	}
	log.Info("Redis connected")

	// ── Repositories ──────────────────────────────────────────────────────────
	userRepo := postgres.NewUserRepository(pgPool)
	walletRepo := postgres.NewWalletRepository(pgPool)
	otpRepo := postgres.NewOTPRepository(pgPool)
	travelRepo := postgres.NewTravelRepository(pgPool)
	dispatcherRepo := postgres.NewDispatcherRepository(pgPool)
	loyaltyRepo := postgres.NewLoyaltyRepository(pgPool)
	travelCacheRepo := mongorepo.NewTravelCacheRepository(mongoDB)

	// ── Shared services ───────────────────────────────────────────────────────
	jwtManager := jwtpkg.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	smsSvc := notifications.NewSMSService(cfg.SMS.TermiiAPIKey, cfg.SMS.TermiiBaseURL, log)
	emailSvc := notifications.NewEmailService(cfg.Brevo.APIKey, cfg.Brevo.SenderEmail, cfg.Brevo.SenderName, log)
	reconEngine := reconcile.NewEngine(walletRepo, log, cfg.Recon.TimeoutSeconds)
	flutterwaveBills := utilitysvc.NewFlutterwaveClient(cfg.Pay.FlutterwaveKey, cfg.Pay.FlutterwaveBaseURL)

	var monnifyFallback utilitysvc.BillProvider
	if cfg.Pay.MonnifyAPIKey != "" && cfg.Pay.MonnifySecret != "" {
		monnifyFallback = utilitysvc.NewMonnifyFallbackClient(
			cfg.Pay.MonnifyAPIKey, cfg.Pay.MonnifySecret, cfg.Pay.MonnifyBaseURL, log)
	}

	// ── Travel operator adapters ──────────────────────────────────────────────
	busOperators := buildBusOperators(cfg, log)
	flightOperators := []operators.FlightOperator{
		operators.NewAmadeusOperator(
			cfg.Travel.AmadeusClientID,
			cfg.Travel.AmadeusSecret,
			cfg.Travel.AmadeusBaseURL,
			log,
		),
	}

	// ── Domain services ───────────────────────────────────────────────────────
	loyaltyService := loyaltysvc.NewService(loyaltyRepo, walletRepo, log)
	authService := authsvc.NewService(userRepo, walletRepo, otpRepo, smsSvc, emailSvc, jwtManager, log)
	walletService := walletsvc.NewService(walletRepo, userRepo, loyaltyService, log)
	utilityService := utilitysvc.NewService(walletRepo, userRepo, reconEngine, loyaltyService, smsSvc, flutterwaveBills, monnifyFallback, log)
	travelService := travelsvc.NewService(
		busOperators, flightOperators,
		travelRepo, walletRepo, travelCacheRepo,
		smsSvc, loyaltyService, cfg.Travel.OfflineQRSecret, log,
	)
	operatorKeys := map[string]string{
		"GIGM": cfg.Travel.GIGMDispatcherKey,
		"ABC":  cfg.Travel.ABCDispatcherKey,
	}
	dispatcherService := dispatchersvc.NewService(
		dispatcherRepo, travelRepo, walletRepo,
		smsSvc, operatorKeys, log,
	)

	// ── Phase 2 Infrastructure ────────────────────────────────────────────────
	ledgerSvc := ledger.NewService(pgPool)

	eventQueue, err := queue.NewProducer(cfg.Pay.RabbitMQURL)
	if err != nil {
		log.Warn("RabbitMQ connection failed, async events disabled", zap.Error(err))
	} else {
		defer eventQueue.Close()
	}

	paystackClient := paystack.New(cfg.Pay.PaystackSecret, "")
	opayClient := opay.New("", "", "", "")
	
	provRouter := providers.NewProviderRouter(
		[]providers.PaymentProvider{paystackClient, opayClient},
		3, 60*time.Second, log,
	)
	_ = provRouter // To be fully wired into utilitysvc in future phases

	transferService := transfersvc.NewService(pgPool, ledgerSvc, paystackClient) // or provRouter if configured
	vaService := vasvc.NewService(pgPool, paystackClient)

	// ── Handlers ──────────────────────────────────────────────────────────────
	authH := authhandler.NewHandler(authService, log)
	walletH := wallethandler.NewHandler(walletService, log)
	utilityH := utilhandler.NewHandler(utilityService, log)
	travelH := travelhandler.NewHandler(travelService, log)
	dispatcherH := dispatcherhandler.NewHandler(dispatcherService, log)
	loyaltyH := loyaltyhandler.NewHandler(loyaltyService, log)
	webhookH := webhookhandler.NewHandler(
		walletService,
		cfg.Pay.FlutterwaveWebhookSecret,
		cfg.Pay.MonnifySecret,
		log,
	)
	transferH := transferhandler.NewHandler(transferService)
	vaH := vahandler.NewHandler(vaService)

	// ── Router ────────────────────────────────────────────────────────────────
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RateLimit(rdb, 100, time.Minute))

	// CORS middleware — reflects request origin so credentials + CORS work together
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "flip-bills-api", "version": "2.0.0"})
	})

	// Webhook routes — no auth middleware, verified by signature instead
	webhooks := r.Group("/webhooks")
	{
		webhooks.POST("/flutterwave", webhookH.Flutterwave)
		webhooks.POST("/monnify", webhookH.Monnify)
		webhooks.POST("/dispatcher", dispatcherH.HandleEvent)
	}

	v1 := r.Group("/api/v1")

	// Public Auth Group
	auth := v1.Group("/auth")
	{
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
		auth.POST("/google", authH.GoogleLogin)
		auth.POST("/verify-phone", authH.VerifyPhone)
		auth.POST("/resend-otp", authH.ResendOTP)
		auth.POST("/refresh", authH.RefreshToken)
	}

	// OPTIMIZED: Exposed Public Travel Group (Bypasses JWT auth validation so anonymous lookups work)
	v1.POST("/auth/set-pin", authH.SetPIN)
	publicTravel := v1.Group("/travel")
	{
		publicTravel.GET("/bus/search", travelH.SearchBus)
		publicTravel.GET("/flight/search", travelH.SearchFlights)
	}

	// Protected Group - Remaining transactional engines stay guarded by JWT hardware auth middleware
	p := v1.Group("")
	p.Use(middleware.Auth(jwtManager))
	{
		p.POST("/auth/kyc/upgrade", authH.UpgradeKYC)
		user := p.Group("/user")
		{
			user.GET("/profile", authH.GetProfile)
			user.PATCH("/profile", authH.UpdateProfile)
		}

		wallet := p.Group("/wallet")
		{
			wallet.GET("/balance", walletH.GetBalance)
			wallet.GET("/transactions", walletH.GetTransactions)
			wallet.POST("/fund", walletH.FundWallet)
			wallet.POST("/initialize-funding", walletH.InitializeFunding)
			vaH.RegisterRoutes(p) // mounts inside /wallet automatically per handler
		}

		transfer := p.Group("")
		{
			transferH.RegisterRoutes(transfer)
		}

		vas := p.Group("/vas")
		{
			vas.GET("/catalog", utilityH.GetCatalog)
			vas.GET("/transactions/:reference", utilityH.GetTransaction)
			vas.POST("/airtime", utilityH.BuyAirtime)
			vas.POST("/data", utilityH.BuyData)
			vas.POST("/electricity", utilityH.PayElectricity)
			vas.POST("/betting", utilityH.FundBetting)
		}

		travel := p.Group("/travel")
		{
			// Protected checkout booking execution layers requiring active balances
			travel.POST("/bus/book", travelH.BookBus)
			travel.POST("/flight/book", travelH.BookFlight)
			travel.GET("/bookings", travelH.GetMyBookings)
			travel.GET("/bookings/:id", travelH.GetBooking)
			travel.POST("/bookings/:id/reschedule", dispatcherH.Reschedule)
			travel.POST("/bookings/:id/refund", dispatcherH.Refund)
		}

		loyalty := p.Group("/loyalty")
		{
			loyalty.GET("/balance", loyaltyH.GetBalance)
			loyalty.GET("/history", loyaltyH.GetHistory)
			loyalty.POST("/redeem", loyaltyH.RedeemPoints)
		}
	}

	// ── Server ────────────────────────────────────────────────────────────────
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("graceful shutdown — draining requests...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("forced shutdown", zap.Error(err))
	}
	log.Info("server exited cleanly")
}

func buildBusOperators(cfg *config.Config, log *zap.Logger) []operators.BusOperator {
	log.Warn("initializing transit module with unified sandbox bus operator profiles")
	return []operators.BusOperator{
		operators.NewSandboxBusOperator("GIGM", "God is Good Motors"),
		operators.NewSandboxBusOperator("ABC", "ABC Transport"),
	}
}
