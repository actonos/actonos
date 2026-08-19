#!/usr/bin/env bash
# ==============================================================================
# ActonOS 1-Click Branded Bare-metal Appliance ISO Generator
# Supports: amd64 (x86_64) & arm64 (aarch64)
# Usage:
#   bash scripts/build-iso.sh              # builds for amd64 (default)
#   ARCH=arm64 bash scripts/build-iso.sh   # builds for arm64
# ==============================================================================
set -e

VERSION="${VERSION:-0.1.0}"
ARCH="${ARCH:-amd64}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUTPUT_ISO="${ROOT_DIR}/build/ActonOS-v${VERSION}-${ARCH}.iso"
LIVE_BUILD_SRC="${ROOT_DIR}/deploy/live-build"

echo "======================================================================"
echo "   ⚡ Building ActonOS Branded Bare-metal ISO (Architecture: ${ARCH})   "
echo "======================================================================"

mkdir -p "${ROOT_DIR}/build"

# 1. Check if running inside Docker or native host with live-build
if ! command -v lb &> /dev/null; then
    if command -v docker &> /dev/null; then
        echo "[i] live-build ('lb') not found on host. Building via Dockerized ISO Builder (${ARCH})..."
        docker build -t actonos-iso-builder -f "${LIVE_BUILD_SRC}/Dockerfile.isobuilder" "${ROOT_DIR}"
        
        # Convert path for Docker on Windows / Linux / macOS
        WIN_DIR="$(pwd -W 2>/dev/null || pwd)"
        export MSYS_NO_PATHCONV=1
        docker run --rm --privileged \
            -e ARCH="${ARCH}" \
            -v "${WIN_DIR}:/workspace" \
            -w /workspace \
            actonos-iso-builder \
            bash scripts/build-iso.sh --in-container
        exit 0
    else
        echo "[!] Error: Neither live-build ('lb') nor Docker found on host."
        exit 1
    fi
fi

# 2. Use a native container filesystem directory for debootstrap/chroot (avoids Windows NTFS mount issues)
WORK_DIR="/tmp/live-iso-${ARCH}"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

# 3. Compile Go static binary for target architecture
echo "[1/4] Compiling actond static binary for Linux (${ARCH})..."
echo "[i] Building React web frontend (React 19 + Tailwind v4)..."
cd "${ROOT_DIR}/web"
npm ci 2>/dev/null || npm install
npm run build
cd "$WORK_DIR"

cd "${ROOT_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH="${ARCH}" go build -trimpath -ldflags="-s -w" -o "${WORK_DIR}/actond" ./cmd/actond
cd "$WORK_DIR"

# 4. Setup live-build workspace & assets
echo "[2/4] Setting up live-build configuration and branding assets..."

# Generate fresh splash screens
if [ -f "${LIVE_BUILD_SRC}/assets/generate_splash.py" ]; then
    python3 "${LIVE_BUILD_SRC}/assets/generate_splash.py" 2>/dev/null || true
fi

# Determine bootloader based on architecture (AMD64 supports Dual Hybrid BIOS + UEFI)
if [ "${ARCH}" == "amd64" ]; then
    BOOTLOADER="syslinux,grub-efi"
else
    BOOTLOADER="grub-efi"
fi

# Clean any residual configuration
lb clean --purge 2>/dev/null || true

# Initialize Debian Live configuration
lb config \
    --distribution bookworm \
    --architectures "${ARCH}" \
    --archive-areas "main contrib non-free non-free-firmware" \
    --bootloader "${BOOTLOADER}" \
    --system live \
    --binary-images iso-hybrid \
    --memtest none \
    --bootappend-live "boot=live components username=acton user-default-groups=sudo,adm,systemd-journal,dialout,plugdev,netdev hostname=acton console=tty0" \
    --iso-application "ActonOS AI Agent Operating System (${ARCH})" \
    --iso-preparer "ActonOS Release Team" \
    --iso-publisher "ActonOS Foundation" \
    --iso-volume "ACTONOS_${ARCH^^}"

# Copy branding and custom chroot files
mkdir -p config/includes.chroot/usr/local/bin
mkdir -p config/includes.chroot/data
cp -r "${LIVE_BUILD_SRC}/config/"* config/
cp "${WORK_DIR}/actond" config/includes.chroot/usr/local/bin/actond
chmod +x config/includes.chroot/usr/local/bin/* 2>/dev/null || true
chmod +x config/hooks/*/*.hook.chroot 2>/dev/null || true

# Dynamic kernel and grub packages per architecture
sed -i "s/linux-image-amd64/linux-image-${ARCH}/g" config/package-lists/actonos.list.chroot 2>/dev/null || true
sed -i "s/grub-efi-amd64-bin/grub-efi-${ARCH}-bin/g" config/package-lists/actonos.list.chroot 2>/dev/null || true

# Preseed configuration is already included via config/includes.binary/preseed/auto-install.cfg

# 5. Build the Hybrid ISO
echo "[3/4] Running live-build packaging for ${ARCH}..."
lb build

# Move generated ISO to output location on mounted workspace
if [ -f "live-image-${ARCH}.hybrid.iso" ]; then
    mv -f "live-image-${ARCH}.hybrid.iso" "$OUTPUT_ISO"
    echo "[4/4] Output ISO created: $OUTPUT_ISO"
    echo "======================================================================"
    echo "   ✨ SUCCESS: ActonOS Branded ISO (${ARCH}) is Ready!                "
    echo "======================================================================"
elif [ -f "live-image-amd64.hybrid.iso" ]; then
    mv -f "live-image-amd64.hybrid.iso" "$OUTPUT_ISO"
    echo "[4/4] Output ISO created: $OUTPUT_ISO"
fi
