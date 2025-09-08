FROM golang:1.21-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build all packages; adjust path (./cmd/...) if your main package lives elsewhere
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o /app ./...

FROM alpine:3.18 AS runtime
RUN apk add --no-cache ca-certificates
COPY --from=build /app /app
EXPOSE 8080
# run as non-root user
RUN adduser -D -u 1000 appuser || true
USER 1000
ENTRYPOINT ["/app"]