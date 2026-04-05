FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build

COPY pkg/ /build/pkg/
COPY services/auth-service/ /build/services/auth-service/

WORKDIR /build/services/auth-service
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/server ./cmd/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /app/server
COPY keys/ /app/keys/
COPY services/auth-service/migrations/ /app/migrations/
WORKDIR /app
CMD ["/app/server"]
