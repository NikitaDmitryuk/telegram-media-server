# syntax=docker/dockerfile:1

FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /telegram-media-server ./cmd/telegram-media-server

FROM archlinux:latest

RUN pacman -Syu --noconfirm && \
pacman -S --noconfirm yt-dlp ca-certificates && \
pacman -Scc --noconfirm

COPY --from=builder /telegram-media-server /telegram-media-server

WORKDIR /app

CMD ["/telegram-media-server"]
