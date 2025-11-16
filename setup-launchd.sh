#!/bin/bash

# Setup script for calendar-sync launchd service
# This script automates the installation of the hourly sync service on macOS

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_SOURCE="${SCRIPT_DIR}/config.json"
CONFIG_DIR="${HOME}/.config/calsync"
CONFIG_DEST="${CONFIG_DIR}/config.json"
PLIST_SOURCE="${SCRIPT_DIR}/com.beekhof.calsync.plist"
PLIST_DEST="${HOME}/Library/LaunchAgents/com.beekhof.calsync.plist"
LOG_DIR="${HOME}/Library/Logs/calsync"
SERVICE_NAME="com.beekhof.calsync"

echo "📅 Calendar Sync - launchd Setup"
echo "================================="
echo ""

# Check if calsync is installed
if ! command -v calsync &> /dev/null; then
    echo -e "${RED}❌ Error: calsync not found in PATH${NC}"
    echo ""
    echo "Please install calsync first:"
    echo "  go install github.com/beekhof/calendar-sync/cmd/calsync@latest"
    echo ""
    echo "Or build from source:"
    echo "  cd ${SCRIPT_DIR}"
    echo "  make build"
    echo "  # Then add the binary to your PATH or use full path in plist"
    exit 1
fi

CALSYNC_PATH=$(which calsync)
echo -e "${GREEN}✓${NC} Found calsync at: ${CALSYNC_PATH}"
echo ""

# Check if config.json exists
if [ ! -f "${CONFIG_SOURCE}" ]; then
    echo -e "${RED}❌ Error: config.json not found at ${CONFIG_SOURCE}${NC}"
    echo ""
    echo "Please create config.json first. You can use config.example.json as a template:"
    echo "  cp ${SCRIPT_DIR}/config.example.json ${CONFIG_SOURCE}"
    echo "  # Then edit ${CONFIG_SOURCE} with your settings"
    exit 1
fi

echo -e "${GREEN}✓${NC} Found config.json at: ${CONFIG_SOURCE}"
echo ""

# Create config directory if it doesn't exist
if [ ! -d "${CONFIG_DIR}" ]; then
    echo "Creating config directory: ${CONFIG_DIR}"
    mkdir -p "${CONFIG_DIR}"
fi

# Copy config.json to standard location
if [ -f "${CONFIG_DEST}" ]; then
    echo -e "${YELLOW}⚠${NC}  Config already exists at ${CONFIG_DEST}"
    read -p "Overwrite? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Keeping existing config. Using: ${CONFIG_DEST}"
    else
        cp "${CONFIG_SOURCE}" "${CONFIG_DEST}"
        echo -e "${GREEN}✓${NC} Copied config to: ${CONFIG_DEST}"
    fi
else
    cp "${CONFIG_SOURCE}" "${CONFIG_DEST}"
    echo -e "${GREEN}✓${NC} Copied config to: ${CONFIG_DEST}"
fi
echo ""

# Create log directory
if [ ! -d "${LOG_DIR}" ]; then
    echo "Creating log directory: ${LOG_DIR}"
    mkdir -p "${LOG_DIR}"
fi
echo -e "${GREEN}✓${NC} Log directory: ${LOG_DIR}"
echo ""

# Check if service is already loaded
if launchctl list | grep -q "${SERVICE_NAME}"; then
    echo -e "${YELLOW}⚠${NC}  Service is already loaded"
    read -p "Unload existing service? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Unloading existing service..."
        launchctl unload "${PLIST_DEST}" 2>/dev/null || true
        echo -e "${GREEN}✓${NC} Service unloaded"
    else
        echo "Keeping existing service. Exiting."
        exit 0
    fi
    echo ""
fi

# Generate plist with correct paths
echo "Generating launchd plist..."
cat > "${PLIST_DEST}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${SERVICE_NAME}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${CALSYNC_PATH}</string>
        <string>--config</string>
        <string>${CONFIG_DEST}</string>
    </array>
    <key>StartInterval</key>
    <integer>3600</integer>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${LOG_DIR}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>${LOG_DIR}/stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:${HOME}/go/bin</string>
    </dict>
</dict>
</plist>
EOF

echo -e "${GREEN}✓${NC} Created plist at: ${PLIST_DEST}"
echo ""

# Load the service
echo "Loading launchd service..."
launchctl load "${PLIST_DEST}"
echo -e "${GREEN}✓${NC} Service loaded successfully"
echo ""

# Verify it's running
echo "Verifying service status..."
if launchctl list | grep -q "${SERVICE_NAME}"; then
    echo -e "${GREEN}✓${NC} Service is active"
    echo ""
    echo "Service details:"
    launchctl list | grep "${SERVICE_NAME}" || true
else
    echo -e "${YELLOW}⚠${NC}  Service loaded but may not be running yet"
fi
echo ""

# Show log locations
echo "Log files:"
echo "  stdout: ${LOG_DIR}/stdout.log"
echo "  stderr: ${LOG_DIR}/stderr.log"
echo ""

# Test run
echo "Running test sync (this may take a moment)..."
if calsync --config "${CONFIG_DEST}" > /tmp/calsync-test.log 2>&1; then
    echo -e "${GREEN}✓${NC} Test sync completed successfully"
    echo ""
    echo "Setup complete! The service will run every hour."
    echo ""
    echo "Useful commands:"
    echo "  View logs:        tail -f ${LOG_DIR}/stderr.log"
    echo "  Check status:      launchctl list | grep ${SERVICE_NAME}"
    echo "  Stop service:      launchctl stop ${SERVICE_NAME}"
    echo "  Start service:    launchctl start ${SERVICE_NAME}"
    echo "  Unload service:   launchctl unload ${PLIST_DEST}"
else
    echo -e "${YELLOW}⚠${NC}  Test sync had errors (check logs above)"
    echo "  This might be normal if it's the first run and OAuth is needed"
    echo "  Check the logs: cat /tmp/calsync-test.log"
    echo ""
    echo "Service is still installed and will run hourly."
fi
echo ""

