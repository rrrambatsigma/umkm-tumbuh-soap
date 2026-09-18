package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the SOAP service.
type Config struct {
	DatabaseURL string
	Port        string
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
		DatabaseURL: dbURL,
		Port:        getEnv("SOAP_PORT", "9090"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
