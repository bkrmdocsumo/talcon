#!/bin/bash
set -e

export PATH="$HOME/go/bin:$PATH"

APP_NAME="Talon"
APP_BUNDLE="build/bin/talon.app"
DMG_NAME="Talon-Installer"
DMG_DIR="build/dmg"
DMG_OUTPUT="build/${DMG_NAME}.dmg"
VERSION="1.0.0"

usage() {
    echo "Usage: ./talon.sh <command>"
    echo ""
    echo "Commands:"
    echo "  dev        Start development mode (hot-reload frontend + live Go recompilation)"
    echo "  build      Build production Mac app (Apple Silicon)"
    echo "  universal  Build universal binary (Intel + Apple Silicon)"
    echo "  dmg        Build app and create DMG installer"
    echo "  open       Launch the built .app"
    echo "  cli        Build and run CLI mode"
    echo "  doctor     Check if your system is ready for Wails development"
    echo "  release    Build DMG and create a GitHub release (usage: release [version])"
    echo ""
}

case "${1:-}" in
    dev)
        echo "Starting Talon in dev mode..."
        wails dev
        ;;
    build)
        echo "Building Talon.app (darwin/arm64)..."
        wails build -platform darwin/arm64
        echo ""
        echo "Done → build/bin/talon.app"
        ;;
    universal)
        echo "Building Talon.app (universal binary)..."
        wails build -platform darwin/universal
        echo ""
        echo "Done → build/bin/talon.app"
        ;;
    open)
        open build/bin/talon.app
        ;;
    dmg)
        PLATFORM="${2:-darwin/arm64}"
        echo "==> Building Talon.app (${PLATFORM})..."
        wails build -platform "${PLATFORM}"
        echo ""

        echo "==> Creating DMG installer..."

        # Clean previous DMG artifacts
        rm -rf /Users/bkrm/Docsumo/talon/build/*.dmg
        rm -rf "${DMG_DIR}"
        rm -f "${DMG_OUTPUT}"
        mkdir -p "${DMG_DIR}"

        # Copy the built app into the staging directory
        cp -R "${APP_BUNDLE}" "${DMG_DIR}/${APP_NAME}.app"

        # Create an Applications symlink for drag-and-drop install
        ln -s /Applications "${DMG_DIR}/Applications"

        # Determine DMG size: app size + 20 MB headroom
        APP_SIZE_KB=$(du -sk "${DMG_DIR}/${APP_NAME}.app" | awk '{print $1}')
        DMG_SIZE_KB=$(( APP_SIZE_KB + 20480 ))

        # Create a temporary read-write DMG
        TMP_DMG="build/${DMG_NAME}-tmp.dmg"
        hdiutil create \
            -srcfolder "${DMG_DIR}" \
            -volname "${APP_NAME}" \
            -fs HFS+ \
            -fsargs "-c c=64,a=16,e=16" \
            -format UDRW \
            -size "${DMG_SIZE_KB}k" \
            "${TMP_DMG}"

        # Mount the temporary DMG
        MOUNT_DIR=$(hdiutil attach -readwrite -noverify "${TMP_DMG}" | \
            grep -E '^\S+\s+Apple_HFS' | awk '{print $3}')

        if [ -z "${MOUNT_DIR}" ]; then
            echo "Error: Failed to mount DMG"
            exit 1
        fi

        # Set Finder window appearance via AppleScript
        echo "==> Configuring DMG window layout..."
        osascript <<EOF
tell application "Finder"
    tell disk "${APP_NAME}"
        open
        set current view of container window to icon view
        set toolbar visible of container window to false
        set statusbar visible of container window to false
        set bounds of container window to {100, 100, 640, 440}
        set theViewOptions to icon view options of container window
        set arrangement of theViewOptions to not arranged
        set icon size of theViewOptions to 100
        set position of item "${APP_NAME}.app" of container window to {140, 160}
        set position of item "Applications" of container window to {400, 160}
        close
        open
        update without registering applications
        delay 2
        close
    end tell
end tell
EOF

        # Ensure all writes are flushed
        sync

        # Unmount the temporary DMG
        hdiutil detach "${MOUNT_DIR}" -quiet

        # Convert to compressed read-only DMG
        hdiutil convert "${TMP_DMG}" \
            -format UDZO \
            -imagekey zlib-level=9 \
            -o "${DMG_OUTPUT}"

        # Clean up
        rm -f "${TMP_DMG}"
        rm -rf "${DMG_DIR}"

        DMG_SIZE=$(du -h "${DMG_OUTPUT}" | awk '{print $1}')
        echo ""
        echo "==> Done! DMG created at: ${DMG_OUTPUT} (${DMG_SIZE})"
        echo "    Open it with: open ${DMG_OUTPUT}"
        ;;
    cli)
        echo "Building CLI binary..."
        go build -o talon-cli ./cmd/talon
        echo "Running Talon CLI..."
        ./talon-cli --mode cli
        ;;
    release)
        REL_VERSION="${2:-$VERSION}"

        # Ensure version starts with 'v'
        case "${REL_VERSION}" in
            v*) ;;
            *)  REL_VERSION="v${REL_VERSION}" ;;
        esac

        echo "==> Preparing release ${REL_VERSION}..."

        # Preflight checks
        if ! command -v gh &>/dev/null; then
            echo "Error: GitHub CLI (gh) is not installed. Install it with: brew install gh"
            exit 1
        fi

        if ! gh auth status &>/dev/null; then
            echo "Error: Not authenticated with GitHub. Run: gh auth login"
            exit 1
        fi

        # Check for uncommitted changes
        if [ -n "$(git status --porcelain)" ]; then
            echo "Error: You have uncommitted changes. Please commit or stash them first."
            exit 1
        fi

        # Check if tag already exists
        if git rev-parse "${REL_VERSION}" &>/dev/null; then
            echo "Error: Tag ${REL_VERSION} already exists."
            echo "  Delete it with: git tag -d ${REL_VERSION} && git push origin :refs/tags/${REL_VERSION}"
            exit 1
        fi

        # Build the DMG
        PLATFORM="${3:-darwin/arm64}"
        echo "==> Building Talon.app (${PLATFORM})..."
        wails build -platform "${PLATFORM}"
        echo ""

        echo "==> Creating DMG installer..."

        rm -rf /Users/bkrm/Docsumo/talon/build/*.dmg
        rm -rf "${DMG_DIR}"
        rm -f "${DMG_OUTPUT}"
        mkdir -p "${DMG_DIR}"

        cp -R "${APP_BUNDLE}" "${DMG_DIR}/${APP_NAME}.app"
        ln -s /Applications "${DMG_DIR}/Applications"

        APP_SIZE_KB=$(du -sk "${DMG_DIR}/${APP_NAME}.app" | awk '{print $1}')
        DMG_SIZE_KB=$(( APP_SIZE_KB + 20480 ))

        TMP_DMG="build/${DMG_NAME}-tmp.dmg"
        hdiutil create \
            -srcfolder "${DMG_DIR}" \
            -volname "${APP_NAME}" \
            -fs HFS+ \
            -fsargs "-c c=64,a=16,e=16" \
            -format UDRW \
            -size "${DMG_SIZE_KB}k" \
            "${TMP_DMG}"

        MOUNT_DIR=$(hdiutil attach -readwrite -noverify "${TMP_DMG}" | \
            grep -E '^\S+\s+Apple_HFS' | awk '{print $3}')

        if [ -z "${MOUNT_DIR}" ]; then
            echo "Error: Failed to mount DMG"
            exit 1
        fi

        echo "==> Configuring DMG window layout..."
        osascript <<EOF
tell application "Finder"
    tell disk "${APP_NAME}"
        open
        set current view of container window to icon view
        set toolbar visible of container window to false
        set statusbar visible of container window to false
        set bounds of container window to {100, 100, 640, 440}
        set theViewOptions to icon view options of container window
        set arrangement of theViewOptions to not arranged
        set icon size of theViewOptions to 100
        set position of item "${APP_NAME}.app" of container window to {140, 160}
        set position of item "Applications" of container window to {400, 160}
        close
        open
        update without registering applications
        delay 2
        close
    end tell
end tell
EOF

        sync
        hdiutil detach "${MOUNT_DIR}" -quiet

        hdiutil convert "${TMP_DMG}" \
            -format UDZO \
            -imagekey zlib-level=9 \
            -o "${DMG_OUTPUT}"

        rm -f "${TMP_DMG}"
        rm -rf "${DMG_DIR}"

        DMG_SIZE=$(du -h "${DMG_OUTPUT}" | awk '{print $1}')
        echo ""
        echo "==> DMG ready: ${DMG_OUTPUT} (${DMG_SIZE})"

        # Create git tag
        echo "==> Creating git tag ${REL_VERSION}..."
        git tag -a "${REL_VERSION}" -m "Release ${REL_VERSION}"
        git push origin "${REL_VERSION}"

        # Create GitHub release and upload DMG
        echo "==> Creating GitHub release ${REL_VERSION}..."
        gh release create "${REL_VERSION}" \
            "${DMG_OUTPUT}#${DMG_NAME}.dmg" \
            --title "${APP_NAME} ${REL_VERSION}" \
            --generate-notes

        echo ""
        echo "==> Release ${REL_VERSION} published!"
        echo "    View it at: $(gh release view "${REL_VERSION}" --json url -q '.url')"
        ;;
    doctor)
        wails doctor
        ;;
    *)
        usage
        ;;
esac
