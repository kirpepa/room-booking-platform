FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build

COPY services/conference-mock-service/ /build/services/conference-mock-service/

WORKDIR /build/services/conference-mock-service
RUN go mod tidy && CGO_ENABLED=0 go build -o /app/server ./cmd/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /app/server
WORKDIR /app
CMD ["/app/server"]
