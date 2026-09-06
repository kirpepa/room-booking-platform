FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build

COPY services/conference-mock-service/ /build/services/conference-mock-service/

WORKDIR /build/services/conference-mock-service
RUN go mod download && CGO_ENABLED=0 go build -trimpath -o /app/server ./cmd/

FROM alpine:3.24
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=builder /app/server /app/server
WORKDIR /app
USER 10001:10001
CMD ["/app/server"]
