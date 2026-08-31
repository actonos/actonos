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
FROM golang:1.26-bookworm AS go-builder
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates curl build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /app/internal/server/dist ./internal/server/dist

ARG VERSION=1.0.0
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/bin/actond \
    ./cmd/actond

RUN CGO_ENABLED=1 GOOS=linux go build \
    -trimpath \
    -tags ORT \
    -o /app/bin/embeddingd \
    ./cmd/embeddingd

RUN bash scripts/download-embedding-model.sh /app/model

ARG TARGETARCH
RUN case "${TARGETARCH:-amd64}" in \
      amd64) ORT_ARCH=x64 ;; \
      arm64) ORT_ARCH=aarch64 ;; \
      *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/v1.28.0/onnxruntime-linux-${ORT_ARCH}-1.28.0.tgz" -o /tmp/ort.tgz \
    && mkdir -p /app/ort \
    && tar -xzf /tmp/ort.tgz --strip-components=1 -C /app/ort

# ==============================================================================
# Stage 3: Agent Batteries-Included Production Image
# ==============================================================================
FROM debian:bookworm-slim

# Install system dependencies, Agent CLI tools, Python, Node.js, and Headless Chromium
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata bubblewrap bash tini coreutils findutils procps \
    ripgrep jq sqlite3 tree curl wget git openssh-client tar gzip unzip \
    python3 python3-pip python3-venv python-is-python3 nodejs npm chromium fonts-noto fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*

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
RUN groupadd --gid 1000 acton \
    && useradd --uid 1000 --gid acton --home-dir /data --create-home --shell /bin/bash acton \
    && chown -R acton:acton /opt/venv \
    && mkdir -p /data/config /data/workspace /data/logs /data/skills /data/releases /data/bin /data/.npm-global \
    && chown -R acton:acton /data

# Copy compiled static binary
COPY --from=go-builder /app/bin/actond /usr/local/bin/actond
COPY --from=go-builder /app/bin/embeddingd /usr/local/bin/embeddingd
COPY --from=go-builder /app/ort/lib/libonnxruntime.so.1.28.0 /usr/lib/libonnxruntime.so.1.28.0
COPY --from=go-builder /app/model /opt/actonos/models/multilingual-e5-small/614241f622f53c4eeff9890bdc4f31cfecc418b3
COPY deploy/docker/entrypoint.sh /usr/local/bin/actonos-entrypoint
RUN ln -s /usr/lib/libonnxruntime.so.1.28.0 /usr/lib/libonnxruntime.so \
	&& chown -R acton:acton /opt/actonos \
	&& chmod +x /usr/local/bin/embeddingd /usr/local/bin/actonos-entrypoint

# Runtime Environment Variables
ENV RUNTIME_MODE=docker \
    PORT=8080 \
    DATA_DIR=/data \
    PATH="/opt/venv/bin:/data/bin:/data/.npm-global/bin:/usr/local/bin:/usr/bin:/bin" \
    NPM_CONFIG_PREFIX="/data/.npm-global" \
    CHROME_BIN="/usr/bin/chromium" \
    CHROME_PATH="/usr/lib/chromium/" \
    PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true \
    PYTHONUNBUFFERED=1
ENV ACTON_EMBEDDING_URL=http://127.0.0.1:8091 \
    ACTON_EMBEDDING_LISTEN_ADDR=127.0.0.1:8091 \
    ACTON_EMBEDDING_MODEL_DIR=/opt/actonos/models/multilingual-e5-small/614241f622f53c4eeff9890bdc4f31cfecc418b3 \
    ONNXRUNTIME_SHARED_LIBRARY_PATH=/usr/lib/libonnxruntime.so.1.28.0

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/health || exit 1

USER acton
WORKDIR /data

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/actonos-entrypoint"]
CMD ["--data-dir=/data", "--listen-addr=:8080"]
