# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/fetchmoona ./cmd/server

FROM alpine:3.19

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

COPY --from=builder /out/fetchmoona /app/fetchmoona

ENV PORT=8081

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

CMD ["/app/fetchmoona"]
