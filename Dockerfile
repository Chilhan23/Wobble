# Build Stage
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/tenantctl ./cmd/tenantctl/main.go

# Production Runner Stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup \
    && mkdir -p /app/uploads && chown -R appuser:appgroup /app

WORKDIR /app

COPY --from=builder /bin/server /app/server
COPY --from=builder /bin/tenantctl /app/tenantctl
COPY --from=builder /src/migrations /app/migrations
COPY --from=builder /src/web /app/web

USER appuser

EXPOSE 8080

CMD ["/app/server"]
