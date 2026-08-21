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
LIVE_BUILD_SRC="${ROOT_DIR}/deploy/live-build"
ISO_NAME="ActonOS-v${VERSION}-${ARCH}.iso"
EMBEDDING_REVISION="614241f622f53c4eeff9890bdc4f31cfecc418b3"
ONNXRUNTIME_VERSION="1.28.0"

case "${ARCH}" in
    amd64)
        ORT_ARCH="x64"
        EMBEDDING_CC="gcc"
        ;;
    arm64)
        ORT_ARCH="aarch64"
        if [ "$(go env GOARCH)" = "arm64" ]; then
            EMBEDDING_CC="gcc"
        else
            EMBEDDING_CC="aarch64-linux-gnu-gcc"
        fi
        ;;
    *)
        echo "[!] Unsupported architecture: ${ARCH}"
        exit 1
        ;;
esac

echo "======================================================================"
echo "   ⚡ Building ActonOS Branded Bare-metal ISO (Architecture: ${ARCH})   "
echo "======================================================================"

mkdir -p "${ROOT_DIR}/build"

# 1. Check if running inside Docker or native host with live-build
if ! command -v lb &> /dev/null; then
    if command -v docker &> /dev/null; then
        echo "[i] live-build ('lb') not found on host. Building Docker image with embedded source..."
        docker build -t actonos-iso-builder -f "${LIVE_BUILD_SRC}/Dockerfile.isobuilder" "${ROOT_DIR}"
        
        # Convert path for Docker on Windows / Linux / macOS for the output directory only
        WIN_BUILD_DIR="$(cd "${ROOT_DIR}/build" && (pwd -W 2>/dev/null || pwd))"
        export MSYS_NO_PATHCONV=1
        
        echo "[i] Running ISO builder inside isolated privileged container..."
        docker run --rm --privileged \
            -e ARCH="${ARCH}" \
            -e VERSION="${VERSION}" \
            -v "${WIN_BUILD_DIR}:/output" \
            actonos-iso-builder \
            bash scripts/build-iso.sh --in-container
            
        echo "======================================================================"
        echo "   ✨ SUCCESS: ISO generated at: ${ROOT_DIR}/build/${ISO_NAME}"
        echo "======================================================================"
        exit 0
    else
        echo "[!] Error: Neither live-build ('lb') nor Docker found on host."
        exit 1
    fi
fi

# 2. Use a native container filesystem directory for debootstrap/chroot
WORK_DIR="/tmp/live-iso-${ARCH}"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

# 3. Compile Go static binary for target architecture
echo "[1/4] Compiling actond static binary for Linux (${ARCH})..."
echo "[i] Installing dependencies and building React web frontend in container..."
cd "${ROOT_DIR}/web"
npm install
npm run build
cd "$WORK_DIR"

cd "${ROOT_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH="${ARCH}" go build -trimpath -ldflags="-s -w" -o "${WORK_DIR}/actond" ./cmd/actond

echo "[i] Compiling embeddingd with ONNX Runtime support..."
CGO_ENABLED=1 GOOS=linux GOARCH="${ARCH}" CC="${EMBEDDING_CC}" \
    go build -trimpath -tags ORT -ldflags="-s -w" -o "${WORK_DIR}/embeddingd" ./cmd/embeddingd

echo "[i] Downloading pinned multilingual-e5-small model and ONNX Runtime..."
bash scripts/download-embedding-model.sh "${WORK_DIR}/embedding-model"
curl -fsSL --retry 3 \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-${ORT_ARCH}-${ONNXRUNTIME_VERSION}.tgz" \
    -o "${WORK_DIR}/onnxruntime.tgz"
mkdir -p "${WORK_DIR}/onnxruntime"
tar -xzf "${WORK_DIR}/onnxruntime.tgz" --strip-components=1 -C "${WORK_DIR}/onnxruntime"
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
mkdir -p config/includes.chroot/usr/lib
mkdir -p "config/includes.chroot/opt/actonos/models/multilingual-e5-small/${EMBEDDING_REVISION}"
cp -r "${LIVE_BUILD_SRC}/config/"* config/
cp "${WORK_DIR}/actond" config/includes.chroot/usr/local/bin/actond
cp "${WORK_DIR}/embeddingd" config/includes.chroot/usr/local/bin/embeddingd
cp "${WORK_DIR}/onnxruntime/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION}" \
    "config/includes.chroot/usr/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION}"
ln -sf "libonnxruntime.so.${ONNXRUNTIME_VERSION}" config/includes.chroot/usr/lib/libonnxruntime.so
cp -r "${WORK_DIR}/embedding-model/." \
    "config/includes.chroot/opt/actonos/models/multilingual-e5-small/${EMBEDDING_REVISION}/"
chmod +x config/includes.chroot/usr/local/bin/* 2>/dev/null || true
chmod +x config/hooks/*/*.hook.chroot 2>/dev/null || true

# Dynamic kernel and grub packages per architecture
sed -i "s/linux-image-amd64/linux-image-${ARCH}/g" config/package-lists/actonos.list.chroot 2>/dev/null || true
sed -i "s/grub-efi-amd64-bin/grub-efi-${ARCH}-bin/g" config/package-lists/actonos.list.chroot 2>/dev/null || true

# 5. Build the Hybrid ISO
echo "[3/4] Running live-build packaging for ${ARCH}..."
lb build

# Output ISO handling
mkdir -p "${ROOT_DIR}/build"
OUTPUT_ISO="${ROOT_DIR}/build/${ISO_NAME}"

GEN_ISO=""
if [ -f "live-image-${ARCH}.hybrid.iso" ]; then
    GEN_ISO="live-image-${ARCH}.hybrid.iso"
elif [ -f "live-image-amd64.hybrid.iso" ]; then
    GEN_ISO="live-image-amd64.hybrid.iso"
fi

if [ -n "$GEN_ISO" ]; then
    cp -f "$GEN_ISO" "$OUTPUT_ISO"
    # If /output volume is mounted, also export directly to host output
    if [ -d "/output" ]; then
        cp -f "$GEN_ISO" "/output/${ISO_NAME}"
    fi
    echo "[4/4] Output ISO created: ${ISO_NAME}"
    echo "======================================================================"
    echo "   ✨ SUCCESS: ActonOS Branded ISO (${ARCH}) is Ready!                "
    echo "======================================================================"
else
    echo "[!] Error: No ISO file was generated by live-build."
    exit 1
fi
