module github.com/room-booking/services/api-gateway

go 1.25.0

toolchain go1.25.13

require (
	github.com/go-chi/chi/v5 v5.3.2
	github.com/google/uuid v1.6.0
	github.com/room-booking/pkg v0.0.0
)

require github.com/golang-jwt/jwt/v5 v5.3.1 // indirect

replace github.com/room-booking/pkg => ../../pkg
