package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                   string
	PublicKeyPath          string
	AuthServiceURL         string
	RoomServiceURL         string
	AvailabilityServiceURL string
	BookingServiceURL      string
	TestTaskMode           bool
}

func Load() Config {
	return Config{
		Port:                   getEnv("PORT", "8080"),
		PublicKeyPath:          getEnv("JWT_PUBLIC_KEY_PATH", "/run/secrets/jwt/public.pem"),
		AuthServiceURL:         getEnv("AUTH_SERVICE_URL", "http://auth-service:8081"),
		RoomServiceURL:         getEnv("ROOM_SERVICE_URL", "http://room-service:8082"),
		AvailabilityServiceURL: getEnv("AVAILABILITY_SERVICE_URL", "http://availability-service:8083"),
		BookingServiceURL:      getEnv("BOOKING_SERVICE_URL", "http://booking-service:8084"),
		TestTaskMode:           getEnvBool("TEST_TASK_MODE", false),
	}
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
