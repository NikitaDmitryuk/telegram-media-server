#!/bin/bash

set -e

ARCHS=("amd64" "arm64")
OUTPUT_DIR="dist"

mkdir -p "$OUTPUT_DIR"

for ARCH in "${ARCHS[@]}"; do
    echo "Building for $ARCH..."
    case "$ARCH" in
        amd64)
            CC=gcc
            ;;
        arm64)
            CC=aarch64-linux-gnu-gcc
            ;;
        *)
            echo "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    GOOS=linux GOARCH=$ARCH CC=$CC CGO_ENABLED=1 go build -trimpath -o "$OUTPUT_DIR/telegram-media-server-$ARCH" ./cmd/telegram-media-server
done

echo "Build completed. Binaries are in the $OUTPUT_DIR directory."
