FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build

COPY pkg/ /build/pkg/
COPY services/api-gateway/ /build/services/api-gateway/

WORKDIR /build/services/api-gateway
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/server ./cmd/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /app/server
COPY keys/ /app/keys/
WORKDIR /app
CMD ["/app/server"]
