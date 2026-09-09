# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# builder — 1 module, 3 binary
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api       ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker    ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scheduler ./cmd/scheduler

# ---------------------------------------------------------------------------
# api — chỉ CRUD + enqueue, không cần yt-dlp/ffmpeg
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS api

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app

WORKDIR /app
USER app

COPY --from=builder /out/api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]

# ---------------------------------------------------------------------------
# scheduler — chỉ phát task theo lịch, cũng không cần binary media
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS scheduler

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app

WORKDIR /app
USER app

COPY --from=builder /out/scheduler /app/scheduler
ENTRYPOINT ["/app/scheduler"]

# ---------------------------------------------------------------------------
# worker — cần yt-dlp (tải/tách audio) + ffmpeg/ffprobe (convert + đo metadata)
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS worker

RUN apk add --no-cache ca-certificates tzdata ffmpeg python3 py3-pip && \
    pip install --break-system-packages --no-cache-dir yt-dlp && \
    adduser -D -u 10001 app

WORKDIR /app
USER app

COPY --from=builder /out/worker /app/worker
ENTRYPOINT ["/app/worker"]
