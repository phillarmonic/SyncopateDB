#!/bin/bash
# Fast multiarch Docker image publishing script (no cache)
# Usage: ./publish-multiarch-no-cache.sh [version]

set -eo pipefail

# Configuration
IMAGE_NAME="phillarmonic/syncopatedb"
DOCKERFILE_PATH="./docker/Dockerfile"
PLATFORMS="linux/amd64,linux/arm64"
BUILDX_INSTANCE="multiarch"

# Default to current date-based version if none provided
VERSION=${1:-$(date +%Y%m%d%H%M)}

echo "🚀 Publishing multiarch Docker image ${IMAGE_NAME}:${VERSION}"

# Check for Docker login status
if ! docker info 2>/dev/null | grep -q "Username"; then
  echo "❌ Not logged in to Docker Hub. Please run 'docker login' first."
  exit 1
fi

# Create or use dedicated builder instance
if ! docker buildx inspect "${BUILDX_INSTANCE}" &>/dev/null; then
  echo "🔧 Creating buildx instance..."
  docker buildx create --name "${BUILDX_INSTANCE}" --driver docker-container --driver-opt network=host --bootstrap --use
else
  echo "✅ Using existing buildx instance: ${BUILDX_INSTANCE}"
  docker buildx use "${BUILDX_INSTANCE}"
fi

# Build and push with NO cache
echo "🏗️ Building and pushing multiarch image with NO cache..."
docker buildx build \
  --platform "${PLATFORMS}" \
  --file "${DOCKERFILE_PATH}" \
  --push \
  --no-cache \
  --tag "${IMAGE_NAME}:${VERSION}" \
  --tag "${IMAGE_NAME}:latest" \
  --progress=plain \
  .

# Verify the pushed images
echo "✅ Verifying multiarch manifest..."
docker buildx imagetools inspect "${IMAGE_NAME}:${VERSION}"

echo "🎉 Successfully published multiarch Docker image:"
echo "  ${IMAGE_NAME}:${VERSION}"
echo "  ${IMAGE_NAME}:latest"