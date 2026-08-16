# ==============================================================================
# Stage 1: Build Web Frontend (React 19 + Tailwind v4 + Vite)
# ==============================================================================
FROM node:22-alpine AS web-builder
WORKDIR /app/web

COPY web/package.json web/package-lock.json* ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ==============================================================================
# Stage 2: Compile Go Static Binary (CGO_ENABLED=0)
# ==============================================================================
FROM golang:1.24-alpine AS go-builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy compiled web assets into embed directory
COPY --from=web-builder /app/web/dist ./internal/server/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=1.0.0" \
    -o /app/bin/actond \
    ./cmd/actond

# ==============================================================================
# Stage 3: Minimal Production Image (< 35MB)
# ==============================================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata bubblewrap bash \
    && addgroup -g 1000 acton \
    && adduser -u 1000 -G acton -h /data -D acton \
    && mkdir -p /data/config /data/workspace /data/logs /data/skills /data/releases /data/bin \
    && chown -R acton:acton /data

COPY --from=go-builder /app/bin/actond /usr/local/bin/actond

ENV RUNTIME_MODE=docker \
    PORT=8080 \
    DATA_DIR=/data

EXPOSE 8080
VOLUME ["/data"]

USER acton
WORKDIR /data

ENTRYPOINT ["/usr/local/bin/actond"]
