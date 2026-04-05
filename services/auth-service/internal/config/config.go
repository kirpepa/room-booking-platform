package config

import "os"

type Config struct {
	Port          string
	DatabaseURL   string
	PrivateKeyPath string
}

func Load() Config {
	return Config{
		Port:          getEnv("PORT", "8081"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/auth_db?sslmode=disable"),
		PrivateKeyPath: getEnv("JWT_PRIVATE_KEY_PATH", "/app/keys/private.pem"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
