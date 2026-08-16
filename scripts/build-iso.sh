#!/usr/bin/env bash
# ==============================================================================
# ActonOS 1-Click Bare-metal ISO Generator
# Produces: ActonOS-v1.0.iso (< 800MB)
# ==============================================================================
set -e

BUILD_DIR="build/live-iso"
OUTPUT_ISO="ActonOS-v1.0.iso"

echo "=== Building ActonOS Bare-metal Appliance ISO ==="

mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

# 1. Compile Go static binary
echo "[1/4] Compiling actond static binary for Linux amd64..."
cd ../../
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BUILD_DIR/actond" ./cmd/actond
cd "$BUILD_DIR"

# 2. Configure Debian Live-build
echo "[2/4] Initializing live-build configuration..."
if command -v lb &> /dev/null; then
    lb config \
        --distribution bookworm \
        --architectures amd64 \
        --archive-areas "main contrib non-free non-free-firmware" \
        --bootloader syslinux \
        --system live \
        --binary-images iso-hybrid

    echo "[3/4] Running live-build packaging..."
    sudo lb build
    mv live-image-amd64.hybrid.iso "../../$OUTPUT_ISO"
    echo "=== SUCCESS: Output ISO created at $OUTPUT_ISO ==="
else
    echo "[!] live-build ('lb') not found on host. Generated binary and config artifacts ready in $BUILD_DIR."
fi
