# syntax=docker/dockerfile:1

FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /telegram-media-server ./cmd/telegram-media-server

FROM ubuntu:24.04 AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    aria2 \
    ffmpeg \
    ca-certificates \
    dnsutils \
    net-tools \
    iputils-ping \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN update-ca-certificates
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

RUN python3 -m pip install --no-cache-dir --break-system-packages yt-dlp
RUN yt-dlp --update-to stable

COPY --from=builder /telegram-media-server /telegram-media-server
COPY locales /app/locales

RUN mkdir -p /app/media

WORKDIR /app

CMD ["/telegram-media-server"]
