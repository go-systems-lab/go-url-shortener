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

# Build the redirect service
WORKDIR /app/services/redirect-svc
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o redirect-svc ./cmd/

# Stage 2: Create the final image
FROM alpine:3.19

# Install runtime dependencies with updated repositories
RUN apk update && apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/services/redirect-svc/redirect-svc .

# Copy any config files if needed
# COPY --from=builder /app/services/redirect-svc/config/ ./config/

# Change ownership of the app directory
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port (HTTP port)
EXPOSE 8081

# Health check for HTTP service
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

# Run the binary
ENTRYPOINT ["./redirect-svc"] 