#!/usr/bin/env bash
set -euo pipefail

# Usage: ./scripts/release-cli.sh <version> [--force]
# Example: ./scripts/release-cli.sh 0.1.0
#          ./scripts/release-cli.sh 0.1.0 --force

VERSION="${1:-}"
FORCE="${2:-}"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version> [--force]"
  echo "  e.g. $0 0.1.0"
  echo "  e.g. $0 0.1.0 --force"
  exit 1
fi

TAG="wkr-cli-v${VERSION}"

echo "→ Tagging ${TAG}"

if [ "$FORCE" = "--force" ]; then
  git tag -fa "$TAG" -m "Release ${TAG}"
  git push origin "$TAG" --force
else
  git tag -a "$TAG" -m "Release ${TAG}"
  git push origin "$TAG"
fi

echo "✓ Tag ${TAG} pushed — GitHub Actions will build and release."
