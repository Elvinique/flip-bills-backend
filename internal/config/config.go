package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	AppEnv string
	Server ServerConfig
	DB     DBConfig
	Mongo  MongoConfig
	Redis  RedisConfig
	JWT    JWTConfig
	Pay    PaymentConfig
	SMS    SMSConfig
	Recon  ReconciliationConfig
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
	FlutterwaveKey     string
	FlutterwaveBaseURL string
	MonnifyAPIKey      string
	MonnifySecret      string
	MonnifyBaseURL     string
	InterswitchClientID string
	InterswitchSecret   string
}

type SMSConfig struct {
	TermiiAPIKey  string
	TermiiBaseURL string
}

type ReconciliationConfig struct {
	TimeoutSeconds int
}

// Load reads .env (if present) then pulls values from environment.
func Load() *Config {
	_ = godotenv.Load() // silently OK if .env absent in production

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
			FlutterwaveKey:      getEnv("FLUTTERWAVE_SECRET_KEY", ""),
			FlutterwaveBaseURL:  getEnv("FLUTTERWAVE_BASE_URL", "https://api.flutterwave.com/v3"),
			MonnifyAPIKey:       getEnv("MONNIFY_API_KEY", ""),
			MonnifySecret:       getEnv("MONNIFY_SECRET_KEY", ""),
			MonnifyBaseURL:      getEnv("MONNIFY_BASE_URL", "https://sandbox.monnify.com"),
			InterswitchClientID: getEnv("INTERSWITCH_CLIENT_ID", ""),
			InterswitchSecret:   getEnv("INTERSWITCH_SECRET", ""),
		},
		SMS: SMSConfig{
			TermiiAPIKey:  getEnv("TERMII_API_KEY", ""),
			TermiiBaseURL: getEnv("TERMII_BASE_URL", "https://api.ng.termii.com"),
		},
		Recon: ReconciliationConfig{
			TimeoutSeconds: getEnvInt("RECONCILIATION_TIMEOUT_SECONDS", 45),
		},
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

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

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
