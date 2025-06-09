# Stage 1: Build the application
FROM golang:1.23-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies with updated repositories
RUN apk update && apk add --no-cache git ca-certificates tzdata

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project (needed for proto and shared utilities)
COPY . .

# Build the URL shortener service
WORKDIR /app/services/url-shortener-svc
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o url-shortener-svc ./cmd/

# Stage 2: Create the final image
FROM alpine:3.19

# Install runtime dependencies with updated repositories
RUN apk update && apk add --no-cache ca-certificates tzdata netcat-openbsd

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/services/url-shortener-svc/url-shortener-svc .

# Copy any config files if needed
# COPY --from=builder /app/services/url-shortener-svc/config/ ./config/

# Change ownership of the app directory
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port (gRPC port)
EXPOSE 50051

# Health check for gRPC service
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 50051 || exit 1

# Run the binary
ENTRYPOINT ["./url-shortener-svc"] 