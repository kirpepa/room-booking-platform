#!/bin/sh
set -e

echo "Running auth-service migrations..."
migrate -path /migrations/auth -database "postgres://postgres:postgres@postgres:5432/auth_db?sslmode=disable" up

echo "Running room-service migrations..."
migrate -path /migrations/room -database "postgres://postgres:postgres@postgres:5432/room_db?sslmode=disable" up

echo "Running availability-service migrations..."
migrate -path /migrations/availability -database "postgres://postgres:postgres@postgres:5432/availability_db?sslmode=disable" up

echo "Running booking-service migrations..."
migrate -path /migrations/booking -database "postgres://postgres:postgres@postgres:5432/booking_db?sslmode=disable" up

echo "All migrations completed."
