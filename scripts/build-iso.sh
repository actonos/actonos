#!/usr/bin/env bash
# ==============================================================================
# ActonOS — ISO Build Script
# ==============================================================================
#
# Builds a bootable Debian-based ISO for bare-metal MiniPC installation.
#
# Prerequisites (Debian/Ubuntu host):
#   sudo apt-get install -y live-build debootstrap
#
# Usage:
#   bash scripts/build-iso.sh
#
# Output:
#   build/ActonOS-v<VERSION>.iso
#
# ==============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION=$(cat "${ROOT_DIR}/VERSION" | tr -d '[:space:]')
BUILD_DIR="${ROOT_DIR}/build"
LIVE_BUILD_DIR="${ROOT_DIR}/deploy/live-build"
ISO_NAME="ActonOS-v${VERSION}.iso"
BINARY="${BUILD_DIR}/actond"

# ==============================================================================
# Preflight Checks
# ==============================================================================

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║     ActonOS ISO Builder v${VERSION}           ║"
echo "╚══════════════════════════════════════════╝"
echo ""

# Check for required tools
for cmd in lb debootstrap; do
  if ! command -v "${cmd}" &> /dev/null; then
    echo -e "${RED}[ERROR]${NC} '${cmd}' is not installed."
    echo "Install with: sudo apt-get install -y live-build debootstrap"
    exit 1
  fi
done

# Check for actond binary
if [[ ! -f "${BINARY}" ]]; then
  echo -e "${YELLOW}[WARN]${NC} actond binary not found at ${BINARY}"
  echo -e "${BLUE}[INFO]${NC} Building actond first..."

  cd "${ROOT_DIR}"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o "${BINARY}" ./cmd/actond/

  echo -e "${GREEN}[OK]${NC} Binary built: ${BINARY}"
fi

# ==============================================================================
# Build ISO
# ==============================================================================

echo -e "${BLUE}[ISO]${NC} Preparing live-build environment..."

cd "${LIVE_BUILD_DIR}"

# Clean previous build
sudo lb clean 2>/dev/null || true

# Configure live-build
lb config \
  --distribution bookworm \
  --architectures amd64 \
  --binary-images iso-hybrid \
  --debian-installer live \
  --archive-areas "main contrib non-free non-free-firmware" \
  --bootloaders "grub-efi" \
  --iso-application "ActonOS" \
  --iso-volume "ActonOS v${VERSION}" \
  --memtest none

# Create directories for custom files
mkdir -p config/includes.chroot/usr/local/bin
mkdir -p config/includes.chroot/etc/systemd/system
mkdir -p config/includes.chroot/data/{bin,config,agents,tokens,storage,logs,overrides,plugins,skills,mcp-servers,workspace,releases}

# Copy actond binary
cp "${BINARY}" config/includes.chroot/data/releases/v${VERSION}/actond
chmod +x config/includes.chroot/data/releases/v${VERSION}/actond

# Create symlink for actond
cd config/includes.chroot/data/bin
ln -sf ../releases/v${VERSION}/actond actond
cd "${LIVE_BUILD_DIR}"

# Create systemd service unit
cat > config/includes.chroot/etc/systemd/system/actond.service <<EOF
[Unit]
Description=ActonOS AI Agent Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/data/bin/actond
Environment=DATA_DIR=/data
Environment=LOG_LEVEL=info
Environment=LISTEN_ADDR=:8080
Environment=RUNTIME_MODE=baremetal
Restart=on-failure
RestartSec=5
WatchdogSec=60
StandardOutput=journal
StandardError=journal
SyslogIdentifier=actond

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/data

[Install]
WantedBy=multi-user.target
EOF

# Create package list
mkdir -p config/package-lists
cat > config/package-lists/actonos.list.chroot <<EOF
bubblewrap
network-manager
wpasupplicant
avahi-daemon
curl
jq
EOF

# Create hook to enable the service
mkdir -p config/hooks/normal
cat > config/hooks/normal/0100-enable-actond.hook.chroot <<'EOF'
#!/bin/bash
systemctl enable actond.service
systemctl enable NetworkManager.service
systemctl enable avahi-daemon.service
EOF
chmod +x config/hooks/normal/0100-enable-actond.hook.chroot

# Build the ISO
echo -e "${BLUE}[ISO]${NC} Building ISO (this may take several minutes)..."
sudo lb build 2>&1 | tail -n 20

# Move output
if [[ -f live-image-amd64.hybrid.iso ]]; then
  mkdir -p "${BUILD_DIR}"
  mv live-image-amd64.hybrid.iso "${BUILD_DIR}/${ISO_NAME}"
  echo ""
  echo -e "${GREEN}[OK]${NC} ISO built successfully!"
  echo -e "${GREEN}[OK]${NC} Output: ${BUILD_DIR}/${ISO_NAME}"
  echo -e "${GREEN}[OK]${NC} Size: $(du -h "${BUILD_DIR}/${ISO_NAME}" | cut -f1)"
  echo ""
  echo -e "${BLUE}[INFO]${NC} Flash to USB with:"
  echo "  sudo dd if=${BUILD_DIR}/${ISO_NAME} of=/dev/sdX bs=4M status=progress"
else
  echo -e "${RED}[ERROR]${NC} ISO build failed. Check the log output above."
  exit 1
fi

# Cleanup
sudo lb clean
echo -e "${GREEN}[OK]${NC} Build environment cleaned."
