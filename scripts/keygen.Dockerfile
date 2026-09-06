FROM alpine:3.24

RUN apk add --no-cache openssl
COPY scripts/generate-dev-keys.sh /usr/local/bin/generate-dev-keys

ENTRYPOINT ["/bin/sh", "/usr/local/bin/generate-dev-keys"]
