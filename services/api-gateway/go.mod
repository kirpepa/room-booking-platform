module github.com/room-booking/services/api-gateway

go 1.22.0

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/room-booking/pkg v0.0.0
)

replace github.com/room-booking/pkg => ../../pkg
