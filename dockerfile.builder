# syntax=docker/dockerfile:1

FROM debian:bullseye-slim AS base

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    git \
    make \
    fakeroot \
    devscripts \
    gcc \
    g++ \
    python3 \
    python3-pip \
    ruby \
    ruby-dev \
    build-essential \
    pacman && \
    gem install --no-document fpm && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY ./build/scripts /app/scripts
COPY ./build/archlinux /app/build/archlinux
COPY ./build/debian /app/build/debian
COPY ./build/telegram-media-server.service /app/build/telegram-media-server.service
COPY ./dist /app/dist
COPY ./cmd /app/source/cmd
COPY ./internal /app/source/internal
COPY ./go.mod /app/source/go.mod
COPY ./go.sum /app/source/go.sum

RUN chmod +x /app/scripts/*.sh

CMD ["/bin/bash", "/app/scripts/package_all.sh"]
