#!/usr/bin/env bash
#
# build-release-macos.sh — Build a signed & notarized macOS DMG for VoiceSnap
#
# Prerequisites (one-time):
#   1. Install Developer ID certificate in Keychain
#   2. Store notarization credentials:
#      xcrun notarytool store-credentials "voicesnap" \
#        --apple-id "your@apple.id" --team-id "3HY4G7M2AL" \
#        --password "app-specific-password"
#
# Usage:
#   cd VoiceSnapGo && ./scripts/build-release-macos.sh
#
set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
IDENTITY="Developer ID Application: Ningbo Zhuozhi Innovation Network Technology Co., Ltd. (3HY4G7M2AL)"
KEYCHAIN_PROFILE="voicesnap"

APP_NAME="VoiceSnap"
BUNDLE_ID="com.voicesnap.app"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=$(defaults read "$PROJECT_DIR/build/darwin/Info.plist" CFBundleShortVersionString 2>/dev/null || echo "2.1.0")
DMG_NAME="${APP_NAME}-v${VERSION}-arm64.dmg"

BUILD_DIR="$PROJECT_DIR/build/release"
APP_BUNDLE="$BUILD_DIR/${APP_NAME}.app"
ENTITLEMENTS="$PROJECT_DIR/build/darwin/entitlements.plist"
INFO_PLIST="$PROJECT_DIR/build/darwin/Info.plist"

GOMODCACHE=$(go env GOMODCACHE)
SHERPA_LIB_DIR="$GOMODCACHE/github.com/k2-fsa/sherpa-onnx-go-macos@v1.12.24/lib/aarch64-apple-darwin"

# ── Helpers ───────────────────────────────────────────────────────────────────
step() { echo ""; echo "==> $1"; }
fail() { echo "ERROR: $1" >&2; exit 1; }

# ── Preflight checks ─────────────────────────────────────────────────────────
step "Preflight checks"
[[ -d "$SHERPA_LIB_DIR" ]] || fail "sherpa-onnx dylibs not found at $SHERPA_LIB_DIR"
[[ -f "$ENTITLEMENTS" ]]   || fail "entitlements.plist not found at $ENTITLEMENTS"
[[ -f "$INFO_PLIST" ]]     || fail "Info.plist not found at $INFO_PLIST"
security find-identity -v -p codesigning | grep -q "$IDENTITY" || fail "Signing identity not found in keychain"

# ── Step 1: Clean ─────────────────────────────────────────────────────────────
step "Cleaning old build artifacts"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# ── Step 2: Build frontend ────────────────────────────────────────────────────
step "Building frontend"
cd "$PROJECT_DIR/frontend"
npm install --prefer-offline
npm run build
cd "$PROJECT_DIR"

# ── Step 3: Build Go binary ──────────────────────────────────────────────────
step "Building Go binary (arm64)"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  go build -buildvcs=false -gcflags=all="-l" -ldflags="-s -w" \
  -o "$BUILD_DIR/${APP_NAME}"

# ── Step 4: Assemble .app bundle ─────────────────────────────────────────────
step "Assembling ${APP_NAME}.app bundle"
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Frameworks"
mkdir -p "$APP_BUNDLE/Contents/Resources"

cp "$INFO_PLIST"                         "$APP_BUNDLE/Contents/Info.plist"
cp "$BUILD_DIR/${APP_NAME}"              "$APP_BUNDLE/Contents/MacOS/${APP_NAME}"
cp "$PROJECT_DIR/bin/VoiceSnap.app/Contents/Resources/icon.icns" \
                                         "$APP_BUNDLE/Contents/Resources/icon.icns"

# Copy dylibs
cp "$SHERPA_LIB_DIR/libsherpa-onnx-c-api.dylib"   "$APP_BUNDLE/Contents/Frameworks/"
cp "$SHERPA_LIB_DIR/libonnxruntime.1.23.2.dylib"   "$APP_BUNDLE/Contents/Frameworks/"

# ── Step 5: Fix rpaths ───────────────────────────────────────────────────────
step "Fixing rpaths with install_name_tool"

# Add @executable_path/../Frameworks rpath to binary
install_name_tool -add_rpath @executable_path/../Frameworks \
  "$APP_BUNDLE/Contents/MacOS/${APP_NAME}" 2>/dev/null || true

# Fix dylib install names
install_name_tool -id @rpath/libsherpa-onnx-c-api.dylib \
  "$APP_BUNDLE/Contents/Frameworks/libsherpa-onnx-c-api.dylib"

install_name_tool -id @rpath/libonnxruntime.1.23.2.dylib \
  "$APP_BUNDLE/Contents/Frameworks/libonnxruntime.1.23.2.dylib"

# Fix libsherpa's dependency on libonnxruntime
install_name_tool -change \
  @rpath/libonnxruntime.1.23.2.dylib \
  @rpath/libonnxruntime.1.23.2.dylib \
  "$APP_BUNDLE/Contents/Frameworks/libsherpa-onnx-c-api.dylib" 2>/dev/null || true

# ── Step 6: Code sign ────────────────────────────────────────────────────────
step "Code signing"

# Sign dylibs first (inside-out signing)
codesign --force --options runtime \
  --entitlements "$ENTITLEMENTS" \
  --sign "$IDENTITY" \
  "$APP_BUNDLE/Contents/Frameworks/libonnxruntime.1.23.2.dylib"

codesign --force --options runtime \
  --entitlements "$ENTITLEMENTS" \
  --sign "$IDENTITY" \
  "$APP_BUNDLE/Contents/Frameworks/libsherpa-onnx-c-api.dylib"

# Sign the main binary
codesign --force --options runtime \
  --entitlements "$ENTITLEMENTS" \
  --sign "$IDENTITY" \
  "$APP_BUNDLE/Contents/MacOS/${APP_NAME}"

# Sign the whole .app bundle
codesign --force --options runtime \
  --entitlements "$ENTITLEMENTS" \
  --sign "$IDENTITY" \
  "$APP_BUNDLE"

# Verify signature
step "Verifying code signature"
codesign -vvv --deep --strict "$APP_BUNDLE"

# ── Step 7: Create DMG ───────────────────────────────────────────────────────
step "Creating DMG"

DMG_PATH="$BUILD_DIR/$DMG_NAME"
DMG_TEMP="$BUILD_DIR/dmg-staging"

mkdir -p "$DMG_TEMP"
cp -R "$APP_BUNDLE" "$DMG_TEMP/"
ln -s /Applications "$DMG_TEMP/Applications"

hdiutil create -volname "$APP_NAME" \
  -srcfolder "$DMG_TEMP" \
  -ov -format UDZO \
  "$DMG_PATH"

rm -rf "$DMG_TEMP"

# ── Step 8: Sign DMG ─────────────────────────────────────────────────────────
step "Signing DMG"
codesign --force --sign "$IDENTITY" "$DMG_PATH"

# ── Step 9: Notarize ─────────────────────────────────────────────────────────
step "Submitting for notarization (this may take a few minutes)..."
xcrun notarytool submit "$DMG_PATH" \
  --keychain-profile "$KEYCHAIN_PROFILE" \
  --wait

# ── Step 10: Staple ──────────────────────────────────────────────────────────
step "Stapling notarization ticket"
xcrun stapler staple "$DMG_PATH"

# ── Done ──────────────────────────────────────────────────────────────────────
step "Build complete!"
echo ""
echo "  DMG: $DMG_PATH"
echo "  Size: $(du -h "$DMG_PATH" | cut -f1)"
echo ""
echo "Verification commands:"
echo "  codesign -vvv --deep --strict \"$APP_BUNDLE\""
echo "  spctl -a -vvv \"$APP_BUNDLE\""
echo "  xcrun stapler validate \"$DMG_PATH\""
