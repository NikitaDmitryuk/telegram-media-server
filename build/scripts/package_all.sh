#!/bin/bash

set -e

DEBIAN_ARCHS=${DEBIAN_ARCHS:-"amd64 arm64"}
ARCHLINUX_ARCHS=${ARCHLINUX_ARCHS:-"x86_64 aarch64"}

echo "Building Debian packages for architectures: $DEBIAN_ARCHS"
for ARCH in $DEBIAN_ARCHS; do
    echo "Building .deb for $ARCH..."
    GOOS=linux GOARCH=$ARCH go build -o /app/dist/telegram-media-server-$ARCH /app/source/cmd/telegram-media-server
    fpm -s dir -t deb -n telegram-media-server -v 1.1.6 \
        --architecture $ARCH \
        --description "Telegram Media Server" \
        --license "MIT" \
        --url "https://github.com/NikitaDmitryuk/telegram-media-server" \
        --maintainer "Your Name <your.email@example.com>" \
        --depends "yt-dlp" \
        --depends "aria2" \
        --config-files /etc/telegram-media-server/.env.example \
        --deb-systemd /app/build/debian/telegram-media-server.service \
        -C /app/dist telegram-media-server-$ARCH=/usr/bin/telegram-media-server
done

echo "Building Arch Linux packages for architectures: $ARCHLINUX_ARCHS"
for ARCH in $ARCHLINUX_ARCHS; do
    echo "Building Arch package for $ARCH..."
    CARCH=$ARCH makepkg --syncdeps --clean --force --noconfirm -C /app/build/archlinux
    mv /app/build/archlinux/*.pkg.tar.zst /app/dist/
done

echo "All packages are built and available in /app/dist"
