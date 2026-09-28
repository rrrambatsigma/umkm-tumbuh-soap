package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the SOAP service.
type Config struct {
	DatabaseURL       string
	Port              string
	JWTSecret         string
	MaxRequestBytes   int64
	RequestTimeout    time.Duration
	DatabaseTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// Load reads configuration from .env files and environment variables.
// It tries the project root .env first, then a local .env.
func Load() *Config {
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load(".env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		user := getEnv("POSTGRES_USER", "umkm_user")
		pass := getEnv("POSTGRES_PASSWORD", "umkm_password")
		host := getEnv("POSTGRES_HOST", "localhost")
		port := getEnv("POSTGRES_PORT", "5432")
		db := getEnv("POSTGRES_DB", "umkm_tumbuh")
		dbURL = "postgres://" + user + ":" + pass + "@" + host + ":" + port + "/" + db + "?sslmode=disable"
	}

	if dbURL == "" {
		log.Fatal("DATABASE_URL or individual POSTGRES_* variables are required")
	}

	return &Config{
		DatabaseURL:       dbURL,
		Port:              getEnv("SOAP_PORT", "9090"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		MaxRequestBytes:   positiveInt64Env("SOAP_MAX_REQUEST_BYTES", 1<<20),
		RequestTimeout:    positiveDurationEnv("SOAP_REQUEST_TIMEOUT", 10*time.Second),
		DatabaseTimeout:   positiveDurationEnv("SOAP_DATABASE_TIMEOUT", 10*time.Second),
		ReadHeaderTimeout: positiveDurationEnv("SOAP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       positiveDurationEnv("SOAP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      positiveDurationEnv("SOAP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:       positiveDurationEnv("SOAP_IDLE_TIMEOUT", 60*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func positiveInt64Env(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(key)), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func positiveDurationEnv(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
