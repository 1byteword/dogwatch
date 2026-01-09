#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}"
echo "  ___  ___   _____      _____  _____ _____  _   _ "
echo " |   \/ _ \ / __\ \    / / _ \|_   _/ __ \| | | |"
echo " | |) | (_) | (_ |\ \/\/ / (_) | | || (__  | |_| |"
echo " |___/ \___/ \___| \_/\_/ \___/  |_| \___| |_| |_|"
echo -e "${NC}"
echo "eBPF-powered observability"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: Please run as root (sudo)${NC}"
    exit 1
fi

# Check OS
if [ "$(uname -s)" != "Linux" ]; then
    echo -e "${RED}Error: dogwatch only runs on Linux${NC}"
    exit 1
fi

# Check architecture
ARCH=$(uname -m)
if [ "$ARCH" != "x86_64" ]; then
    echo -e "${RED}Error: dogwatch currently only supports x86_64 (got $ARCH)${NC}"
    exit 1
fi

# Check kernel version (need 5.8+ for ring buffers)
KERNEL_VERSION=$(uname -r | cut -d. -f1-2)
KERNEL_MAJOR=$(echo $KERNEL_VERSION | cut -d. -f1)
KERNEL_MINOR=$(echo $KERNEL_VERSION | cut -d. -f2)

if [ "$KERNEL_MAJOR" -lt 5 ] || ([ "$KERNEL_MAJOR" -eq 5 ] && [ "$KERNEL_MINOR" -lt 8 ]); then
    echo -e "${RED}Error: Kernel 5.8+ required (got $(uname -r))${NC}"
    echo "BPF ring buffers require kernel 5.8 or later."
    exit 1
fi

echo -e "${GREEN}✓${NC} Linux detected"
echo -e "${GREEN}✓${NC} x86_64 architecture"
echo -e "${GREEN}✓${NC} Kernel $(uname -r)"
echo ""

INSTALL_DIR="/usr/local/bin"
REPO_URL="https://github.com/1byteword/dogwatch"

# Check if Go is available for building
if command -v go &> /dev/null; then
    GO_CMD="go"
elif [ -x "/usr/local/go/bin/go" ]; then
    GO_CMD="/usr/local/go/bin/go"
else
    GO_CMD=""
fi

# Try to download pre-built binary first (when releases exist)
# For now, build from source
echo "Installing dogwatch..."
echo ""

TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR"

echo "Cloning repository..."
git clone --depth 1 "$REPO_URL" dogwatch 2>/dev/null || {
    echo -e "${RED}Error: Failed to clone repository${NC}"
    exit 1
}

cd dogwatch

if [ -z "$GO_CMD" ]; then
    echo -e "${YELLOW}Go not found. Installing Go...${NC}"

    # Download and install Go
    GO_VERSION="1.22.0"
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O go.tar.gz
    tar -C /usr/local -xzf go.tar.gz
    GO_CMD="/usr/local/go/bin/go"
    echo -e "${GREEN}✓${NC} Go installed"
fi

echo "Building dogwatch..."
CGO_ENABLED=0 $GO_CMD build -o dogwatch ./cmd/dogwatch 2>/dev/null || {
    echo -e "${RED}Error: Build failed${NC}"
    exit 1
}

echo "Installing to $INSTALL_DIR..."
mv dogwatch "$INSTALL_DIR/dogwatch"
chmod +x "$INSTALL_DIR/dogwatch"

# Cleanup
cd /
rm -rf "$TEMP_DIR"

echo ""
echo -e "${GREEN}✓ dogwatch installed successfully!${NC}"
echo ""
echo "Usage:"
echo "  sudo dogwatch"
echo ""
echo "Then open http://localhost:9999 in your browser"
echo ""

# Ask about systemd service
read -p "Install systemd service for auto-start? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    cat > /etc/systemd/system/dogwatch.service << 'EOF'
[Unit]
Description=dogwatch eBPF observability
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/dogwatch
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable dogwatch
    systemctl start dogwatch

    echo ""
    echo -e "${GREEN}✓ systemd service installed and started${NC}"
    echo ""
    echo "Commands:"
    echo "  systemctl status dogwatch   # Check status"
    echo "  systemctl stop dogwatch     # Stop"
    echo "  systemctl restart dogwatch  # Restart"
    echo "  journalctl -u dogwatch -f   # View logs"
fi
