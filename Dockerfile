## ---------- Build Stage ----------
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Install build deps
RUN apk add --no-cache --update build-base git ca-certificates tzdata && update-ca-certificates

# Leverage go mod cache
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Set build flags for smaller binary
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -o server .

## ---------- Runtime Stage ----------
FROM alpine:3.20 AS runtime
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata aws-cli wget curl && update-ca-certificates \
    && adduser -D -g '' appuser \
    && mkdir -p /app/logs && chown -R appuser:appuser /app/logs

# Copy binary and required assets (static files, migrations if any)
COPY --from=builder /app/server /app/server
COPY --from=builder /app/storage /app/storage

ENV APP_ENV=production PORT=3000
EXPOSE 3000

USER appuser

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://localhost:3000/health || exit 1

ENTRYPOINT ["/app/server"]