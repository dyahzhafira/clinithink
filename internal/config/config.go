package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	AppEnv      string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	JWTExpiry   string
	GCPTTSKey   string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:      getEnv("PORT", "8080"),
		AppEnv:    getEnv("APP_ENV", "development"),
		JWTExpiry: getEnv("JWT_EXPIRY", "24h"),
	}

	var missing []string
	if cfg.DatabaseURL = os.Getenv("DATABASE_URL"); cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.RedisURL = os.Getenv("REDIS_URL"); cfg.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if cfg.JWTSecret = os.Getenv("JWT_SECRET"); cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}

	cfg.GCPTTSKey = os.Getenv("GCP_TTS_API_KEY")

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
