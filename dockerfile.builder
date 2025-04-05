# syntax=docker/dockerfile:1

# Стадия сборки Debian пакетов
FROM ubuntu:24.04 AS debian-builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    git \
    make \
    fakeroot \
    devscripts \
    gcc \
    g++ \
    golang-go \
    python3 \
    python3-pip \
    ruby \
    ruby-dev \
    build-essential && \
    gem install --no-document fpm && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY ./build/telegram-media-server.service /app/build/telegram-media-server.service
COPY ./.env.example /app/build/.env.example
COPY ./cmd /app/source/cmd
COPY ./internal /app/source/internal
COPY ./go.mod /app/source/go.mod
COPY ./go.sum /app/source/go.sum

RUN bash -c ' \
    DEBIAN_ARCHS="amd64 arm64" && \
    IFS=", " read -r -a DEBIAN_ARCH_ARRAY <<< "$DEBIAN_ARCHS" && \
    mkdir -p /app/dist && \
    for ARCH in "${DEBIAN_ARCH_ARRAY[@]}"; do \
        echo "Building .deb for $ARCH..."; \
        (cd /app/source && GOOS=linux GOARCH=$ARCH go build -o /app/dist/telegram-media-server-$ARCH ./cmd/telegram-media-server); \
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
            -C /app/dist telegram-media-server-$ARCH=/usr/bin/telegram-media-server; \
    done'

# Стадия сборки Arch Linux пакетов
FROM archlinux:base AS archlinux-builder

RUN pacman -Syu --noconfirm && \
    pacman -S --noconfirm base-devel git go sudo && \
    pacman -Scc --noconfirm

# Создаем пользователя для выполнения makepkg
RUN useradd -m builder && echo "builder ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

USER builder
WORKDIR /home/builder/app

COPY --chown=builder:builder ./build/archlinux /home/builder/app/build/archlinux
COPY --chown=builder:builder ./cmd /home/builder/app/source/cmd
COPY --chown=builder:builder ./internal /home/builder/app/source/internal
COPY --chown=builder:builder ./go.mod /home/builder/app/source/go.mod
COPY --chown=builder:builder ./go.sum /home/builder/app/source/go.sum
COPY --chown=builder:builder ./.env.example /home/builder/app/source/.env.example
COPY --chown=builder:builder ./build/telegram-media-server.service /home/builder/app/build/telegram-media-server.service

RUN bash -c ' \
    ARCHLINUX_ARCHS="x86_64 aarch64" && \
    IFS=", " read -r -a ARCHLINUX_ARCH_ARRAY <<< "$ARCHLINUX_ARCHS" && \
    mkdir -p /home/builder/app/dist && \
    for ARCH in "${ARCHLINUX_ARCH_ARRAY[@]}"; do \
        echo "Building Arch package for $ARCH..."; \
        cd /home/builder/app/build/archlinux && \
        CARCH=$ARCH makepkg --syncdeps --clean --force --noconfirm && \
        mv *.pkg.tar.zst /home/builder/app/dist/ && \
        cd -; \
    done'

# Финальная стадия для объединения результатов
FROM ubuntu:24.04 AS final

WORKDIR /app

COPY --from=debian-builder /app/dist /app/dist
COPY --from=archlinux-builder /home/builder/app/dist /app/dist

CMD ["ls", "-l", "/app/dist"]
