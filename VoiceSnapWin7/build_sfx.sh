#!/bin/bash
# Build VoiceSnap SFX self-extracting archive
# Usage: bash build_sfx.sh [portable|appdata|standard]
#   portable  - Modified SFX, extracts to VoiceSnap/ folder next to the .exe (default)
#   appdata   - Modified SFX, extracts to %LOCALAPPDATA%\VoiceSnap
#   standard  - Standard 7zSD.sfx, extracts to temp + batch installer copies to AppData

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

SEVENZIP="/c/Program Files/7-Zip/7z.exe"
MODE="${1:-portable}"

echo "=== Building VoiceSnap SFX ($MODE mode) ==="

# Step 1: Create the .7z archive with only distributable files
echo "[1/3] Creating 7z archive..."
rm -f voicesnap_payload.7z

if [ "$MODE" = "standard" ]; then
    # Standard mode: include install.bat in the archive
    "$SEVENZIP" a -t7z -mx=5 -ms=on -m0=LZMA voicesnap_payload.7z \
        voicesnap_win7.exe \
        config.json \
        install.bat \
        *.dll
else
    # Modified SFX mode: no install.bat needed
    # Model files NOT included — app auto-downloads on first launch
    "$SEVENZIP" a -t7z -mx=5 -ms=on -m0=LZMA voicesnap_payload.7z \
        voicesnap_win7.exe \
        config.json \
        *.dll
fi

echo ""
echo "[2/3] Selecting SFX module and config..."

case "$MODE" in
    portable)
        SFX_MODULE="7zsd.sfx"
        SFX_CONFIG="sfx_config_portable.txt"
        OUTPUT="VoiceSnap_Setup.exe"
        echo "  Module: Modified 7zSD (extracts next to exe)"
        echo "  Config: $SFX_CONFIG"
        ;;
    appdata)
        SFX_MODULE="7zsd.sfx"
        SFX_CONFIG="sfx_config.txt"
        OUTPUT="VoiceSnap_Setup.exe"
        echo "  Module: Modified 7zSD (extracts to AppData)"
        echo "  Config: $SFX_CONFIG"
        ;;
    standard)
        SFX_MODULE="7zSD_standard.sfx"
        SFX_CONFIG="sfx_config_standard.txt"
        OUTPUT="VoiceSnap_Setup.exe"
        echo "  Module: Standard 7zSD (temp + batch installer)"
        echo "  Config: $SFX_CONFIG"
        ;;
    *)
        echo "Unknown mode: $MODE"
        echo "Usage: bash build_sfx.sh [portable|appdata|standard]"
        exit 1
        ;;
esac

# Verify files exist
for f in "$SFX_MODULE" "$SFX_CONFIG" voicesnap_payload.7z; do
    if [ ! -f "$f" ]; then
        echo "ERROR: $f not found!"
        exit 1
    fi
done

echo ""
echo "[3/3] Joining SFX module + config + archive..."

# The magic: binary concatenation creates the self-extracting exe
# On Windows, use cmd copy /b for reliable binary concat
cmd.exe //C "copy /b $SFX_MODULE + $SFX_CONFIG + voicesnap_payload.7z $OUTPUT" > /dev/null

# Verify
if [ -f "$OUTPUT" ]; then
    SIZE=$(du -h "$OUTPUT" | cut -f1)
    echo ""
    echo "=== SUCCESS ==="
    echo "Output: $SCRIPT_DIR/$OUTPUT ($SIZE)"
    echo ""
    case "$MODE" in
        portable)
            echo "When user double-clicks $OUTPUT:"
            echo "  1. Prompt: 'Extract and run VoiceSnap?'"
            echo "  2. Extracts to VoiceSnap/ folder next to the .exe"
            echo "  3. Runs voicesnap_win7.exe automatically"
            echo "  4. Files persist - no re-extraction on next run"
            echo "  Note: User can change extract path in the dialog"
            ;;
        appdata)
            echo "When user double-clicks $OUTPUT:"
            echo "  1. Prompt: 'Do you want to install VoiceSnap?'"
            echo "  2. Extracts to %LOCALAPPDATA%\\VoiceSnap"
            echo "  3. Runs voicesnap_win7.exe automatically"
            echo "  4. Files persist - no re-extraction on next run"
            ;;
        standard)
            echo "When user double-clicks $OUTPUT:"
            echo "  1. Prompt: 'Do you want to run VoiceSnap?'"
            echo "  2. Extracts to temp folder"
            echo "  3. install.bat copies to %LOCALAPPDATA%\\VoiceSnap"
            echo "  4. Runs voicesnap_win7.exe from AppData"
            ;;
    esac
else
    echo "ERROR: Failed to create $OUTPUT"
    exit 1
fi

# Clean up intermediate archive
rm -f voicesnap_payload.7z
echo ""
echo "Done!"
