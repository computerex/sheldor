#!/bin/bash
# ==========================================
# EmuBuddy Distribution - macOS
# ==========================================
# Run this script ON A MAC to create macOS distribution
# Usage: ./dist-macos.sh [version]
# ==========================================

set -e

VERSION="${1:-1.0.0}"

echo ""
echo "=========================================="
echo "  EmuBuddy Distribution (macOS)"
echo "  Version: $VERSION"
echo "=========================================="
echo ""

# Create dist folder
mkdir -p dist

# ==========================================
# Build Installer
# ==========================================
echo "Building Installer..."
cd installer
CGO_ENABLED=0 go build -ldflags="-s -w" -o ../EmuBuddySetup-macos main.go
echo "  [OK] EmuBuddySetup-macos"
cd ..

# ==========================================
# Build Launcher
# ==========================================
echo "Building Launcher..."
cd launcher/gui
go build -ldflags="-s -w" -o ../../EmuBuddyLauncher-macos .
echo "  [OK] EmuBuddyLauncher-macos"
cd ../..

# ==========================================
# Create macOS ZIP
# ==========================================
echo ""
echo "Creating macOS distribution..."
MAC_DIR="dist/EmuBuddy-macOS-v${VERSION}"
rm -rf "$MAC_DIR"
mkdir -p "$MAC_DIR/1g1rsets"

cp EmuBuddyLauncher-macos "$MAC_DIR/"
cp EmuBuddySetup-macos "$MAC_DIR/"
cp systems.json "$MAC_DIR/"
cp README.md "$MAC_DIR/"
cp 1g1rsets/*.json "$MAC_DIR/1g1rsets/"

# Create .command files for double-click
cat > "$MAC_DIR/Start EmuBuddy.command" << 'EOF'
#!/bin/bash
cd "$(dirname "$0")"
./EmuBuddyLauncher-macos
EOF
chmod +x "$MAC_DIR/Start EmuBuddy.command"

cat > "$MAC_DIR/Run Setup.command" << 'EOF'
#!/bin/bash
cd "$(dirname "$0")"
./EmuBuddySetup-macos
EOF
chmod +x "$MAC_DIR/Run Setup.command"

# Make binaries executable
chmod +x "$MAC_DIR/EmuBuddyLauncher-macos"
chmod +x "$MAC_DIR/EmuBuddySetup-macos"

# Create ZIP
cd dist
zip -rq "EmuBuddy-macOS-v${VERSION}.zip" "EmuBuddy-macOS-v${VERSION}"
cd ..
echo "  [OK] dist/EmuBuddy-macOS-v${VERSION}.zip"

# ==========================================
# Summary
# ==========================================
echo ""
echo "=========================================="
echo "  Distribution Complete!"
echo "=========================================="
echo ""
ls -la dist/*.zip 2>/dev/null
echo ""
