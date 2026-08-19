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
FROM golang:1.26-alpine AS go-builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=1.0.0
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/bin/actond \
    ./cmd/actond

# ==============================================================================
# Stage 3: Agent Batteries-Included Production Image
# ==============================================================================
FROM alpine:3.21

# Install system dependencies, Agent CLI tools, Python, Node.js, and Headless Chromium
RUN apk add --no-cache \
    # Core & Isolation
    ca-certificates \
    tzdata \
    bubblewrap \
    bash \
    tini \
    coreutils \
    findutils \
    procps \
    # Search & Inspection Utilities
    ripgrep \
    jq \
    sqlite \
    tree \
    # Networking & Source Control
    curl \
    wget \
    git \
    openssh-client \
    # Archive Tools
    tar \
    gzip \
    unzip \
    # Runtimes
    python3 \
    py3-pip \
    py3-virtualenv \
    nodejs \
    npm \
    # Headless Browser & Fonts (for Browser Tool & Screenshots)
    chromium \
    font-noto \
    font-noto-cjk \
    && rm -rf /var/cache/apk/*

# Create pre-configured Python virtual environment for Agent runtime
RUN python3 -m venv /opt/venv \
    && /opt/venv/bin/pip install --no-cache-dir --upgrade pip wheel setuptools \
    && /opt/venv/bin/pip install --no-cache-dir \
        requests \
        httpx \
        beautifulsoup4 \
        pyyaml \
        pydantic

# Create non-root 'acton' user and setup directory hierarchy
RUN addgroup -g 1000 acton \
    && adduser -u 1000 -G acton -h /data -D acton \
    && chown -R acton:acton /opt/venv \
    && mkdir -p /data/config /data/workspace /data/logs /data/skills /data/releases /data/bin /data/.npm-global \
    && chown -R acton:acton /data

# Copy compiled static binary
COPY --from=go-builder /app/bin/actond /usr/local/bin/actond

# Runtime Environment Variables
ENV RUNTIME_MODE=docker \
    PORT=8080 \
    DATA_DIR=/data \
    PATH="/opt/venv/bin:/data/bin:/data/.npm-global/bin:/usr/local/bin:/usr/bin:/bin" \
    NPM_CONFIG_PREFIX="/data/.npm-global" \
    CHROME_BIN="/usr/bin/chromium-browser" \
    CHROME_PATH="/usr/lib/chromium/" \
    PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true \
    PYTHONUNBUFFERED=1

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/health || exit 1

USER acton
WORKDIR /data

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/actond"]
