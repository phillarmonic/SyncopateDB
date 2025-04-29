# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o syncopatedb ./cmd/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache tzdata ca-certificates && \
    update-ca-certificates

# Create non-root user for security
RUN addgroup -S syncopate && \
    adduser -S -G syncopate syncopate

# Create directories and set ownership
RUN mkdir -p /data && \
    chown -R syncopate:syncopate /data

# Set the working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/syncopatedb .

# Use the non-root user
USER syncopate

# Define environment variables
ENV PORT=8080 \
    DEBUG=false \
    LOG_LEVEL=info \
    ENABLE_WAL=true \
    ENABLE_ZSTD=true \
    COLORIZED_LOGS=false \
    TZ=UTC

# Create a volume for persistent data
VOLUME ["/data"]

# Expose API port
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/syncopatedb"]

# Default command arguments
CMD ["--data-dir", "/data"]