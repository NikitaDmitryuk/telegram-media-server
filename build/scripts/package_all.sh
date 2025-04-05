#!/bin/bash

set -e

# Разделяем архитектуры по пробелам, если они указаны через запятую
DEBIAN_ARCHS=${DEBIAN_ARCHS:-"amd64 arm64"}
ARCHLINUX_ARCHS=${ARCHLINUX_ARCHS:-"x86_64 aarch64"}

# Устанавливаем разделитель для обработки списков
IFS=', ' read -r -a DEBIAN_ARCH_ARRAY <<< "$DEBIAN_ARCHS"
IFS=', ' read -r -a ARCHLINUX_ARCH_ARRAY <<< "$ARCHLINUX_ARCHS"

echo "Building Debian packages for architectures: ${DEBIAN_ARCH_ARRAY[*]}"
for ARCH in "${DEBIAN_ARCH_ARRAY[@]}"; do
    echo "Building .deb for $ARCH..."
    (cd /app/source && GOOS=linux GOARCH=$ARCH go build -o /app/dist/telegram-media-server-$ARCH ./cmd/telegram-media-server)
    fpm -s dir -t deb -n telegram-media-server -v 1.1.6 \
        --architecture $ARCH \
        --description "Telegram Media Server" \
        --license "MIT" \
        --url "https://github.com/NikitaDmitryuk/telegram-media-server" \
        --maintainer "Your Name <your.email@example.com>" \
        --depends "yt-dlp" \
        --depends "aria2" \
        --config-files /app/build/.env.example \
        --deb-systemd /app/build/telegram-media-server.service \
        -C /app/dist telegram-media-server-$ARCH=/usr/bin/telegram-media-server
done

echo "Building Arch Linux packages for architectures: ${ARCHLINUX_ARCH_ARRAY[*]}"
for ARCH in "${ARCHLINUX_ARCH_ARRAY[@]}"; do
    echo "Building Arch package for $ARCH..."
    cd /app/build/archlinux
    CARCH=$ARCH makepkg --syncdeps --clean --force --noconfirm
    mv *.pkg.tar.zst /app/dist/
    cd -
done

echo "All packages are built and available in /app/dist"
