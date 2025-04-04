#!/bin/bash

set -e

ARCHS=("amd64" "arm64")
OUTPUT_DIR="packages/debian"

mkdir -p "$OUTPUT_DIR"

for ARCH in "${ARCHS[@]}"; do
    echo "Packaging for Debian/Ubuntu ($ARCH)..."
    fpm -s dir -t deb -n telegram-media-server -v 1.1.6 \
        --description "Telegram Media Server" \
        --license "MIT" \
        --url "https://github.com/NikitaDmitryuk/telegram-media-server" \
        --maintainer "Your Name <your.email@example.com>" \
        --architecture "$ARCH" \
        --depends "yt-dlp" \
        --depends "aria2" \
        --config-files /etc/telegram-media-server/.env.example \
        --deb-systemd build/debian/telegram-media-server.service \
        -C dist/telegram-media-server-"$ARCH" .
    mv *.deb "$OUTPUT_DIR/"
done

echo "Debian/Ubuntu packages are in the $OUTPUT_DIR directory."
