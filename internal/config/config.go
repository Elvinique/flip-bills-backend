package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv string
	Server ServerConfig
	DB     DBConfig
	Mongo  MongoConfig
	Redis  RedisConfig
	JWT    JWTConfig
	Pay    PaymentConfig
	SMS    SMSConfig
	Brevo  BrevoConfig
	Recon  ReconciliationConfig
	Travel TravelConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type MongoConfig struct {
	URI string
	DB  string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type PaymentConfig struct {
	FlutterwaveKey           string
	FlutterwaveBaseURL       string
	FlutterwaveWebhookSecret string // verif-hash header value set in FLW dashboard
	MonnifyAPIKey            string
	MonnifySecret            string
	MonnifyBaseURL           string
	InterswitchClientID      string
	InterswitchSecret        string
}

type SMSConfig struct {
	TermiiAPIKey  string
	TermiiBaseURL string
}

type BrevoConfig struct {
	APIKey      string
	SenderEmail string
	SenderName  string
}

type ReconciliationConfig struct {
	TimeoutSeconds int
}

// TravelConfig holds credentials for all transport/GDS partners.
type TravelConfig struct {
	UseSandboxBus     bool
	GIGMAPIKey        string
	GIGMPartner       BusPartnerConfig
	GIGMDispatcherKey string // SHA-256 pre-hashed API key for Terminal Dispatcher webhook
	ABCAPIKey         string
	ABCPartner        BusPartnerConfig
	ABCDispatcherKey  string // SHA-256 pre-hashed API key for Terminal Dispatcher webhook
	AmadeusClientID   string
	AmadeusSecret     string
	AmadeusBaseURL    string
	OfflineQRSecret   string // HMAC secret for QR ticket signing
}

type BusPartnerConfig struct {
	BaseURL             string
	SearchPath          string
	HoldPath            string
	ConfirmPath         string
	CancelPath          string
	AuthHeader          string
	AuthScheme          string
	SecondaryAuthHeader string
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		AppEnv: getEnv("APP_ENV", "development"),
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  getDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
		},
		DB: DBConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     getEnv("POSTGRES_USER", "flipbills"),
			Password: getEnv("POSTGRES_PASSWORD", ""),
			Name:     getEnv("POSTGRES_DB", "flipbills_db"),
			SSLMode:  getEnv("POSTGRES_SSL_MODE", "disable"),
		},
		Mongo: MongoConfig{
			URI: getEnv("MONGO_URI", "mongodb://localhost:27017"),
			DB:  getEnv("MONGO_DB", "flipbills_dynamic"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:     mustGetEnv("JWT_SECRET"),
			AccessTTL:  getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: getDuration("JWT_REFRESH_TTL", 720*time.Hour),
		},
		Pay: PaymentConfig{
			FlutterwaveKey:           getEnv("FLUTTERWAVE_SECRET_KEY", ""),
			FlutterwaveBaseURL:       getEnv("FLUTTERWAVE_BASE_URL", "https://api.flutterwave.com/v3"),
			FlutterwaveWebhookSecret: getEnv("FLUTTERWAVE_WEBHOOK_SECRET", ""),
			MonnifyAPIKey:            getEnv("MONNIFY_API_KEY", ""),
			MonnifySecret:            getEnv("MONNIFY_SECRET_KEY", ""),
			MonnifyBaseURL:           getEnv("MONNIFY_BASE_URL", "https://sandbox.monnify.com"),
			InterswitchClientID:      getEnv("INTERSWITCH_CLIENT_ID", ""),
			InterswitchSecret:        getEnv("INTERSWITCH_SECRET", ""),
		},
		SMS: SMSConfig{
			TermiiAPIKey:  getEnv("TERMII_API_KEY", ""),
			TermiiBaseURL: getEnv("TERMII_BASE_URL", "https://api.ng.termii.com"),
		},
		Brevo: BrevoConfig{
			APIKey:      getEnv("BREVO_API_KEY", ""),
			SenderEmail: getEnv("BREVO_SENDER_EMAIL", ""),
			SenderName:  getEnv("BREVO_SENDER_NAME", "Flip Bills"),
		},
		Recon: ReconciliationConfig{
			TimeoutSeconds: getEnvInt("RECONCILIATION_TIMEOUT_SECONDS", 45),
		},
		Travel: TravelConfig{
			UseSandboxBus: getEnvBool("TRAVEL_USE_SANDBOX_BUS", getEnv("APP_ENV", "development") != "production"),
			GIGMAPIKey:    getEnv("GIGM_API_KEY", ""),
			GIGMPartner: BusPartnerConfig{
				BaseURL:             getEnv("GIGM_BASE_URL", "https://api.gigm.com/api"),
				SearchPath:          getEnv("GIGM_SEARCH_PATH", "/trips/search"),
				HoldPath:            getEnv("GIGM_HOLD_PATH", "/seats/hold"),
				ConfirmPath:         getEnv("GIGM_CONFIRM_PATH", "/bookings/confirm"),
				CancelPath:          getEnv("GIGM_CANCEL_PATH", "/bookings/cancel"),
				AuthHeader:          getEnv("GIGM_AUTH_HEADER", "Authorization"),
				AuthScheme:          getEnv("GIGM_AUTH_SCHEME", "Bearer"),
				SecondaryAuthHeader: getEnv("GIGM_SECONDARY_AUTH_HEADER", "X-API-Key"),
			},
			GIGMDispatcherKey: getEnv("GIGM_DISPATCHER_KEY", ""),
			ABCAPIKey:         getEnv("ABC_API_KEY", ""),
			ABCPartner: BusPartnerConfig{
				BaseURL:             getEnv("ABC_BASE_URL", "https://api.abctransport.com.ng"),
				SearchPath:          getEnv("ABC_SEARCH_PATH", "/trips/search"),
				HoldPath:            getEnv("ABC_HOLD_PATH", "/seats/hold"),
				ConfirmPath:         getEnv("ABC_CONFIRM_PATH", "/bookings/confirm"),
				CancelPath:          getEnv("ABC_CANCEL_PATH", "/bookings/cancel"),
				AuthHeader:          getEnv("ABC_AUTH_HEADER", "Authorization"),
				AuthScheme:          getEnv("ABC_AUTH_SCHEME", "Bearer"),
				SecondaryAuthHeader: getEnv("ABC_SECONDARY_AUTH_HEADER", "X-API-Key"),
			},
			ABCDispatcherKey: getEnv("ABC_DISPATCHER_KEY", ""),
			AmadeusClientID:  getEnv("AMADEUS_CLIENT_ID", ""),
			AmadeusSecret:    getEnv("AMADEUS_CLIENT_SECRET", ""),
			AmadeusBaseURL:   getEnv("AMADEUS_BASE_URL", "https://test.api.amadeus.com"),
			OfflineQRSecret:  mustGetEnv("OFFLINE_QR_SECRET"),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("FATAL: required environment variable %q is not set", key)
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
