# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o gyroskop main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create app user
RUN addgroup -g 1001 appgroup && \
    adduser -u 1001 -G appgroup -s /bin/sh -D appuser

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/gyroskop .

# Change ownership to app user
RUN chown appuser:appgroup /app/gyroskop && \
    chmod +x /app/gyroskop

# Switch to app user
USER appuser

# Expose port (if needed for health checks)
EXPOSE 8080

CMD ["./gyroskop"]