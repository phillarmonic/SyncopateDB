FROM alpine:latest

# Build arg to specify architecture
ARG ARCH=amd64

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

# Copy the binary for the specific architecture
COPY --chown=syncopate:syncopate bin/${ARCH}/syncopatedb /app/syncopatedb
RUN chmod +x /app/syncopatedb

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