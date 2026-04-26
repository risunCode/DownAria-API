# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/downaria-api ./cmd/downaria-api

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache \
  ca-certificates \
  tzdata \
  ffmpeg \
  python3 \
  py3-pip \
  curl \
  && pip3 install --no-cache-dir --break-system-packages yt-dlp \
  && rm -rf /var/cache/apk/*

COPY --from=builder /out/downaria-api /app/downaria-api

ENV ADDR=:8080
ENV HTTP_WRITE_TIMEOUT=5m
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json
ENV EXTRACT_TIMEOUT=30s
ENV EXTRACT_CACHE_TTL=10m
ENV YTDLP_BINARY=yt-dlp

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/health || exit 1

CMD ["/app/downaria-api"]
