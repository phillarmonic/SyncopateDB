# Build stage
FROM golang:1.24-alpine AS builder
ARG TARGETARCH
ARG UPX_VERSION=5.0.0

# Install build dependencies
RUN apk add --no-cache git curl tar xz

# Download and install UPX for the correct architecture
RUN if [ "$TARGETARCH" = "amd64" ] || [ -z "$TARGETARCH" ]; then \
      ARCH="amd64_linux"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
      ARCH="arm64_linux"; \
    else \
      echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -L https://github.com/upx/upx/releases/download/v${UPX_VERSION}/upx-${UPX_VERSION}-${ARCH}.tar.xz -o upx.tar.xz && \
    tar -xf upx.tar.xz && \
    mv upx-${UPX_VERSION}-${ARCH}/upx /usr/local/bin/ && \
    rm -rf upx.tar.xz upx-${UPX_VERSION}-${ARCH} && \
    upx --version

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

# Compress the binary with UPX
RUN upx --best --lzma syncopatedb

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