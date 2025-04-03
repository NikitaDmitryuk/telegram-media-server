# syntax=docker/dockerfile:1

FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN GOOS=linux go build -o /telegram-media-server ./cmd/telegram-media-server

FROM ubuntu:24.04 AS runtime

RUN apt-get update && \
    apt-get install -y \
    yt-dlp \
    aria2 \
    ca-certificates

COPY --from=builder /telegram-media-server /telegram-media-server

RUN mkdir -p /app/media

WORKDIR /app

CMD ["/telegram-media-server"]
