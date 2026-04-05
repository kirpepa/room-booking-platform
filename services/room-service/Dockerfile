FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build

COPY pkg/ /build/pkg/
COPY services/room-service/ /build/services/room-service/

WORKDIR /build/services/room-service
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/server ./cmd/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /app/server
COPY keys/ /app/keys/
COPY services/room-service/migrations/ /app/migrations/
WORKDIR /app
CMD ["/app/server"]
