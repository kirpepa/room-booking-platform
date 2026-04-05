package config

import "os"

type Config struct {
	Port                   string
	DatabaseURL            string
	AvailabilityServiceURL string
	ConferenceServiceURL   string
}

func Load() Config {
	return Config{
		Port:                   getEnv("PORT", "8084"),
		DatabaseURL:            getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/booking_db?sslmode=disable"),
		AvailabilityServiceURL: getEnv("AVAILABILITY_SERVICE_URL", "http://availability-service:8083"),
		ConferenceServiceURL:   getEnv("CONFERENCE_SERVICE_URL", "http://conference-mock-service:8085"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
