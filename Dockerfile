# syntax=docker/dockerfile:1

FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /bbg-telegram-media-server

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /bbg-telegram-media-server /bbg-telegram-media-server

WORKDIR /app

CMD ["/bbg-telegram-media-server"]
