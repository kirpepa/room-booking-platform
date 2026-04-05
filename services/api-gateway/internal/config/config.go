package config

import "os"

type Config struct {
	Port                   string
	PublicKeyPath          string
	AuthServiceURL         string
	RoomServiceURL         string
	AvailabilityServiceURL string
	BookingServiceURL      string
}

func Load() Config {
	return Config{
		Port:                   getEnv("PORT", "8080"),
		PublicKeyPath:          getEnv("JWT_PUBLIC_KEY_PATH", "/app/keys/public.pem"),
		AuthServiceURL:         getEnv("AUTH_SERVICE_URL", "http://auth-service:8081"),
		RoomServiceURL:         getEnv("ROOM_SERVICE_URL", "http://room-service:8082"),
		AvailabilityServiceURL: getEnv("AVAILABILITY_SERVICE_URL", "http://availability-service:8083"),
		BookingServiceURL:      getEnv("BOOKING_SERVICE_URL", "http://booking-service:8084"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
