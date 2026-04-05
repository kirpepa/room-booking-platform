package config

import "os"

type Config struct {
	Port           string
	DatabaseURL    string
	RoomServiceURL string
}

func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8083"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/availability_db?sslmode=disable"),
		RoomServiceURL: getEnv("ROOM_SERVICE_URL", "http://room-service:8082"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
