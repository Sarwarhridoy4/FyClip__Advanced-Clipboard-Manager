#!/bin/bash
# =====================================================================
# FyClip Build & Package Script (Debian + AppImage + Tarball)
# Auto-installs all necessary tools for building
# =====================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
APP_NAME="FyClip Clipboard Manager"
BIN_NAME="fyclip"
APP_ID="com.sarwar.fyclip"
PKG_NAME="fyclip"
AUTHOR="Sarwar Hossain"
EMAIL="sarwarhridoy4@gmail.com"
DIST_DIR="dist"
WORK_DIR="${DIST_DIR}/work"
FYNE_ROOT="${WORK_DIR}/fyne-root"
DEB_ROOT="${WORK_DIR}/deb-root"
APPDIR="${WORK_DIR}/FyClip.AppDir"
TARBALL_ROOT="${WORK_DIR}/tarball-root"
LOCAL_TOOLS_DIR="$(pwd)/${DIST_DIR}/.tools"

# Detect package manager
detect_package_manager() {
    local pm=""
    for p in apt-get dnf pacman zypper brew; do
        if command -v "${p}" >/dev/null 2>&1; then
            pm="${p}"
            break
        fi
    done
    echo "${pm}"
}

PM=$(detect_package_manager)

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check if running as root or have sudo access
has_sudo() {
    if [ "$(id -u)" -eq 0 ]; then return 0; fi
    if command -v sudo >/dev/null 2>&1; then
        if sudo -v 2>/dev/null; then return 0; fi
    fi
    return 1
}

# Install packages based on package manager
install_packages() {
    local packages=("$@")
    local install_cmd=""
    
    case "${PM}" in
        apt-get)
            install_cmd="apt-get update && apt-get install -y ${packages[*]}"
            ;;
        dnf)
            install_cmd="dnf install -y ${packages[*]}"
            ;;
        pacman)
            install_cmd="pacman -Sy --noconfirm ${packages[*]}"
            ;;
        zypper)
            install_cmd="zypper --non-interactive install ${packages[*]}"
            ;;
        brew)
            install_cmd="brew install ${packages[*]}"
            ;;
        *)
            log_error "No supported package manager found!"
            return 1
            ;;
    esac
    
    if has_sudo; then
        if [ "$(id -u)" -eq 0 ]; then
            bash -lc "${install_cmd}"
        else
            sudo bash -lc "${install_cmd}"
        fi
    else
        log_warn "Cannot install packages - no sudo access"
        return 1
    fi
}

# Install Go if not present
install_go() {
    if command -v go >/dev/null 2>&1; then
        log_success "Go is already installed: $(go version)"
        return 0
    fi
    
    log_info "Installing Go..."
    
    local go_version="1.26.1"
    local arch
    arch=$(uname -m)
    case "${arch}" in
        x86_64) arch="amd64" ;;
        aarch64) arch="arm64" ;;
        armv7l) arch="armv6l" ;;
    esac
    
    local go_archive="go${go_version}.linux-${arch}.tar.gz"
    local tmp_dir=$(mktemp -d)
    
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://go.dev/dl/${go_archive}" -o "${tmp_dir}/${go_archive}"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "https://go.dev/dl/${go_archive}" -O "${tmp_dir}/${go_archive}"
    else
        log_error "Neither curl nor wget found. Cannot download Go."
        return 1
    fi
    
    tar -C "${tmp_dir}" -xzf "${tmp_dir}/${go_archive}"
    
    if [ "$(id -u)" -eq 0 ]; then
        mv "${tmp_dir}/go" /usr/local/go
    else
        mkdir -p "${HOME}/.local/go"
        mv "${tmp_dir}/go" "${HOME}/.local/go"
    fi
    
    rm -rf "${tmp_dir}"
    
    # Add to PATH
    if [ "$(id -u)" -eq 0 ]; then
        echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/golang.sh
    fi
    export PATH="${HOME}/.local/go/bin:${PATH}"
    
    log_success "Go installed successfully"
    return 0
}

# Install fyne CLI
install_fyne() {
    if command -v fyne >/dev/null 2>&1; then
        log_success "fyne CLI is already installed"
        return 0
    fi
    
    log_info "Installing fyne CLI..."
    
    # Ensure Go is installed first
    if ! command -v go >/dev/null 2>&1; then
        install_go || return 1
    fi
    
    export PATH="${HOME}/.local/go/bin:${PATH}:$(go env GOPATH)/bin"
    
    if go install fyne.io/tools/cmd/fyne@latest; then
        log_success "fyne CLI installed successfully"
        return 0
    else
        log_error "Failed to install fyne CLI"
        return 1
    fi
}

# Install appimagetool
install_appimagetool() {
    if command -v appimagetool >/dev/null 2>&1; then
        log_success "appimagetool is already installed"
        return 0
    fi
    
    log_info "Installing appimagetool..."
    
    local arch="x86_64"
    case $(uname -m) in
        aarch64) arch="aarch64" ;;
        armv7l) arch="armhf" ;;
    esac
    
    local url="https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-${arch}.AppImage"
    local tmp_file=$(mktemp)
    
    mkdir -p "${LOCAL_TOOLS_DIR}/bin"
    
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "${url}" -o "${tmp_file}"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "${url}" -O "${tmp_file}"
    else
        log_error "Neither curl nor wget found"
        return 1
    fi
    
    mv "${tmp_file}" "${LOCAL_TOOLS_DIR}/bin/appimagetool"
    chmod +x "${LOCAL_TOOLS_DIR}/bin/appimagetool"
    
    log_success "appimagetool installed successfully"
    return 0
}

# Install required system packages
install_system_dependencies() {
    log_info "Installing system dependencies..."
    
    local packages=()
    
    case "${PM}" in
        apt-get)
            packages=(build-essential curl wget tar dpkg libgl1 libglib2.0-0 libxcursor1 libxrandr2 libxinerama1 libxi6 libgtk-3-0 xclip xsel wl-clipboard)
            ;;
        dnf)
            packages=( @development-tools curl wget tar dpkg glibc libXcursor libXrandr libXinerama libXi gtk3 xclip xsel wl-clipboard)
            ;;
        pacman)
            packages=(base-devel curl wget tar dpkg libglvnd libxcursor libxrandr libxinerama libxi gtk3 xclip xsel wl-clipboard)
            ;;
        zypper)
            packages=(-devel_basis curl wget tar dpkg Mesa-libGL1 glib2 libxcursor1 libXrandr2 libXinerama1 libXi6 gtk3-tools xclip xsel wl-clipboard)
            ;;
        brew)
            packages=(curl wget tar dpkg)
            ;;
    esac
    
    if [ ${#packages[@]} -gt 0 ]; then
        install_packages "${packages[@]}" || log_warn "Some packages may have failed to install"
    fi
    
    log_success "System dependencies installed"
    return 0
}

# Ensure all required tools are available
ensure_tools() {
    log_info "Checking required tools..."
    
    # Install system dependencies first
    install_system_dependencies || true
    
    # Check and install Go
    if ! command -v go >/dev/null 2>&1; then
        install_go || { log_error "Failed to install Go"; exit 1; }
    fi
    export PATH="${HOME}/.local/go/bin:${PATH}:$(go env GOPATH 2>/dev/null)/bin"
    local installed_go min_go
    installed_go="$(go env GOVERSION | sed 's/^go//')"
    min_go="1.26.1"
    if [ "$(printf '%s\n%s\n' "${min_go}" "${installed_go}" | sort -V | head -n 1)" != "${min_go}" ]; then
        log_error "Go ${min_go} or newer is required by go.mod (found ${installed_go})"
        exit 1
    fi
    log_success "Go: $(go version)"
    
    # Check and install fyne
    if ! command -v fyne >/dev/null 2>&1; then
        install_fyne || { log_error "Failed to install fyne CLI"; exit 1; }
    fi
    log_success "fyne CLI installed"
    
    # Check for tar
    if ! command -v tar >/dev/null 2>&1; then
        install_packages tar || { log_error "Failed to install tar"; exit 1; }
    fi
    
    # Check for dpkg-deb
    if ! command -v dpkg-deb >/dev/null 2>&1; then
        install_packages dpkg || { log_error "Failed to install dpkg"; exit 1; }
    fi
    
    # Check and install appimagetool
    if ! command -v appimagetool >/dev/null 2>&1; then
        install_appimagetool || { log_error "Failed to install appimagetool"; exit 1; }
    fi
    
    # Add local tools to PATH
    export PATH="${LOCAL_TOOLS_DIR}/bin:${PATH}"
    
    log_success "All required tools are available"
}

# Get version
get_version() {
    local tag
    tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
    if [ -n "${tag}" ]; then
        printf '%s\n' "${tag#v}"
    else
        printf '%s\n' "2.5.0"
    fi
}

# Get project SHA256
get_project_sha256() {
    if git rev-parse --git-dir >/dev/null 2>&1; then
        git ls-files | sort | xargs -d '\n' sha256sum 2>/dev/null | sha256sum | awk '{print $1}'
    else
        echo "unavailable"
    fi
}

# Generate properly sized hicolor icons from a single source image.
# Each generated PNG is resized to EXACTLY the target dimensions so the
# file dimensions match the hicolor directory name (fixes blurry/scaled icons).
# Arguments: <source_image> <output_hicolor_root_dir>
generate_hicolor_icons() {
    local src="$1"
    local out_root="$2"
    if [ ! -f "${src}" ]; then
        log_error "Icon source not found: ${src}"
        return 1
    fi

    local sizes=(16 24 32 48 64 128 256)
    local python_script
    python_script=$(cat <<'PYEOF'
import sys
from PIL import Image
src, out_root = sys.argv[1], sys.argv[2]
sizes = [int(x) for x in sys.argv[3:]]
img = Image.open(src).convert("RGBA")
for s in sizes:
    d = f"{out_root}/{s}x{s}/apps"
    import os
    os.makedirs(d, exist_ok=True)
    out = f"{d}/com.sarwar.fyclip.png"
    img.resize((s, s), getattr(Image, 'Resampling', Image).LANCZOS).save(out)
print("hicolor icons generated")
PYEOF
    )

    if command -v python3 >/dev/null 2>&1 && python3 -c "import PIL" >/dev/null 2>&1; then
        python3 -c "${python_script}" "${src}" "${out_root}" "${sizes[@]}"
    else
        log_warn "PIL/python3 not available - falling back to direct copy (icon sizes may mismatch)"
        for s in "${sizes[@]}"; do
            mkdir -p "${out_root}/${s}x${s}/apps"
            cp -f "${src}" "${out_root}/${s}x${s}/apps/com.sarwar.fyclip.png"
        done
    fi
}

# Generate version.go file with version embedded
generate_version_file() {
    local version="$1"
    local build_time="$(date -u +"%Y-%m-%d %H:%M:%S UTC")"
    
    cat > "internal/version/version.go" <<EOF
// File: internal/version/version.go
// This file is auto-generated during build. Do not edit manually.
package version

// Version is the application version, set during build
var Version = "${version}"

// BuildTime is the build timestamp, set during build
var BuildTime = "${build_time}"
EOF
    
    log_info "Generated version.go with version: ${version}"
}

# Main build function
main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  FyClip Build Script v2.0${NC}"
    echo -e "${BLUE}  Auto-installs all dependencies${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    # Get version
    if [ -n "${1:-}" ]; then 
        VERSION="${1#v}"
    else
        DEFAULT_VERSION=$(get_version)
        read -r -p "Enter version [${DEFAULT_VERSION}]: " VERSION
        VERSION=${VERSION:-$DEFAULT_VERSION}
    fi
    
    # Detect architecture
    ARCH_RAW=$(uname -m)
    case "$ARCH_RAW" in
        x86_64) ARCH="amd64"; APPIMAGE_ARCH="x86_64" ;;
        aarch64) ARCH="arm64"; APPIMAGE_ARCH="aarch64" ;;
        armv7l) ARCH="armhf"; APPIMAGE_ARCH="armhf" ;;
        i386|i686) ARCH="i386"; APPIMAGE_ARCH="i686" ;;
        *) ARCH="$ARCH_RAW"; APPIMAGE_ARCH="$ARCH_RAW" ;;
    esac
    
    log_info "Building ${APP_NAME} ${VERSION} (${ARCH})"
    
    PROJECT_SHA256=$(get_project_sha256)
    log_info "Project SHA256: ${PROJECT_SHA256}"
    
    # Generate version.go file with version embedded
    generate_version_file "${VERSION}"
    
    # Ensure all tools are available
    ensure_tools
    
    # Create directories
    mkdir -p "${DIST_DIR}" "${WORK_DIR}"
    rm -rf "${FYNE_ROOT}" "${DEB_ROOT}" "${APPDIR}"
    rm -f "${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb" \
           "${DIST_DIR}/${PKG_NAME}_${VERSION}_${APPIMAGE_ARCH}.AppImage" \
           "${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.tar.gz" \
           "${APP_NAME}.tar.xz"
    
    # Build with Fyne
    log_info "Building FyClip with Fyne..."
    fyne package --os linux --release --name "${APP_NAME}" --icon icon.png
    
    # Extract Fyne package
    log_info "Extracting Fyne package..."
    tar -xJf "${APP_NAME}.tar.xz" -C "${WORK_DIR}"
    
    # Detect prefix
    if [ -d "${WORK_DIR}/usr/local" ]; then
        PREFIX_REL="usr/local"
    elif [ -d "${WORK_DIR}/usr" ]; then
        PREFIX_REL="usr"
    else
        NESTED_USR=$(find "${WORK_DIR}" -maxdepth 3 -type d -name "usr" | head -n 1)
        if [ -z "${NESTED_USR}" ]; then
            log_error "No usr directory found after extraction"
            exit 1
        fi
        NESTED_DIR=$(dirname "${NESTED_USR}")
        PREFIX_REL="${NESTED_DIR#${WORK_DIR}/}/usr"
        if [ -d "${WORK_DIR}/${PREFIX_REL}/local" ]; then
            PREFIX_REL="${PREFIX_REL}/local"
        fi
    fi
    
    # Normalize directory structure
    USR_NORMALIZED="${WORK_DIR}/usr-normalized"
    mkdir -p "${USR_NORMALIZED}"
    cp -a "${WORK_DIR}/${PREFIX_REL}/." "${USR_NORMALIZED}/"
    
    BIN_DIR="${USR_NORMALIZED}/bin"
    APPS_DIR="${USR_NORMALIZED}/share/applications"
    PIXMAPS_DIR="${USR_NORMALIZED}/share/pixmaps"
    
    # Normalize binary name
    FOUND_BIN=$(find "${BIN_DIR}" -maxdepth 1 -type f -executable | head -n 1)
    if [ -z "${FOUND_BIN}" ]; then
        log_error "Binary not found"
        exit 1
    fi
    if [ "$(basename "${FOUND_BIN}")" != "${BIN_NAME}" ]; then
        mv "${FOUND_BIN}" "${BIN_DIR}/${BIN_NAME}"
    fi
    BIN_PATH="${BIN_DIR}/${BIN_NAME}"
    
    # Find desktop and icon files
    DESKTOP_PATH=$(find "${APPS_DIR}" -name '*.desktop' | head -n 1)
    ICON_PATH=$(find "${PIXMAPS_DIR}" -type f | head -n 1)
    
    if [ -z "${DESKTOP_PATH}" ]; then
        log_error "Desktop file not found"
        exit 1
    fi
    if [ -z "${ICON_PATH}" ]; then
        log_error "Icon file not found"
        exit 1
    fi
    
    # Normalize desktop and icon filenames
    DESKTOP_DIR="$(dirname "${DESKTOP_PATH}")"
    DESKTOP_NORM="${DESKTOP_DIR}/${APP_ID}.desktop"
    if [ "$(basename "${DESKTOP_PATH}")" != "${APP_ID}.desktop" ]; then
        mv "${DESKTOP_PATH}" "${DESKTOP_NORM}"
        DESKTOP_PATH="${DESKTOP_NORM}"
    fi
    
    ICON_DIR="$(dirname "${ICON_PATH}")"
    ICON_EXT="${ICON_PATH##*.}"
    ICON_NORM="${ICON_DIR}/${APP_ID}.${ICON_EXT}"
    if [ "$(basename "${ICON_PATH}")" != "${APP_ID}.${ICON_EXT}" ]; then
        mv "${ICON_PATH}" "${ICON_NORM}"
        ICON_PATH="${ICON_NORM}"
    fi
    
    # Update desktop file
    sed -i -E "s|^Exec=.*|Exec=${BIN_NAME}|" "${DESKTOP_PATH}"
    sed -i -E "s|^Icon=.*|Icon=${APP_ID}|" "${DESKTOP_PATH}"
    sed -i -E "s|^Name=.*|Name=${APP_NAME}|" "${DESKTOP_PATH}"
    grep -q '^StartupWMClass=' "${DESKTOP_PATH}" || echo "StartupWMClass=FyClip - Clipboard Manager" >> "${DESKTOP_PATH}"
    
    grep -q '^Categories=' "${DESKTOP_PATH}" || echo "Categories=Utility;" >> "${DESKTOP_PATH}"
    grep -q '^NoDisplay=' "${DESKTOP_PATH}" || echo "NoDisplay=false" >> "${DESKTOP_PATH}"
    grep -q '^Keywords=' "${DESKTOP_PATH}" || echo "Keywords=clipboard;copy;paste;history;" >> "${DESKTOP_PATH}"
    
    # Install icon in hicolor at all standard sizes (properly resized so each
    # file matches its directory name: fixes blurry/scaled icon issues).
    HICOLOR_DIR="${USR_NORMALIZED}/share/icons/hicolor"
    generate_hicolor_icons "${ICON_PATH}" "${HICOLOR_DIR}"
    ICON_PATH="${HICOLOR_DIR}/256x256/apps/${APP_ID}.png"
    
    # ---------------------------------------------------------------------
    # Build Debian Package
    # ---------------------------------------------------------------------
    log_info "Building Debian package..."
    mkdir -p "${DEB_ROOT}/DEBIAN" "${DEB_ROOT}/usr"
    cp -a "${USR_NORMALIZED}/." "${DEB_ROOT}/usr/"
    
    cat > "${DEB_ROOT}/DEBIAN/control" <<EOF
Package: ${PKG_NAME}
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Maintainer: ${AUTHOR} <${EMAIL}>
Depends: libgl1, libx11-6, libxcursor1, libxrandr2, libxinerama1, libxi6, libgtk-3-0, xclip | xsel | wl-clipboard
Description: A modular, high-performance clipboard manager
 FyClip is a powerful, feature-rich clipboard manager built with Go and Fyne.
 .
 Key features:
  * Clipboard History: Automatically saves text, images, HTML, and files
  * Pin Items: Keep important items at the top
  * Enhanced Search: Regex, case-sensitive, and fuzzy matching
  * Image Support: Preview and save clipboard images
  * Encrypted Storage: AES-256-GCM encryption at rest
  * Snippets: Save and expand text templates
  * AutoStart: Launch on system startup
  * Theme Support: Light, Dark, and System theme modes
  * Auto Update: Check for and install updates from GitHub
EOF
    
    cat > "${DEB_ROOT}/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
update-desktop-database -q /usr/share/applications || true
gtk-update-icon-cache -q /usr/share/icons/hicolor || true
EOF
    chmod 755 "${DEB_ROOT}/DEBIAN/postinst"

    cat > "${DEB_ROOT}/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -e
update-desktop-database -q /usr/share/applications || true
gtk-update-icon-cache -q /usr/share/icons/hicolor || true
EOF
    chmod 755 "${DEB_ROOT}/DEBIAN/postrm"
    
    # Remove existing package file if present
    rm -f "${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb"
    
    # Build the package
    dpkg-deb --build "${DEB_ROOT}" "${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb"
    log_success "Debian package built: ${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb"
    
    # ---------------------------------------------------------------------
    # Build AppImage
    # ---------------------------------------------------------------------
    log_info "Building AppImage..."
    mkdir -p "${APPDIR}/usr"
    cp -a "${USR_NORMALIZED}/." "${APPDIR}/usr/"
    
    # Create AppRun
    cat > "${APPDIR}/AppRun" <<EOF
#!/bin/sh
HERE="\$(dirname "\$(readlink -f "\$0")")"
export LD_LIBRARY_PATH="\$HERE/usr/lib:\$LD_LIBRARY_PATH"
exec "\$HERE/usr/bin/${BIN_NAME}" "\$@"
EOF
    chmod +x "${APPDIR}/AppRun"
    
    # Copy desktop and icon
    cp "${DESKTOP_PATH}" "${APPDIR}/${APP_ID}.desktop"
    cp "${ICON_PATH}" "${APPDIR}/${APP_ID}.${ICON_EXT}"
    
    # Create symlink
    ln -sf "${APP_ID}.${ICON_EXT}" "${APPDIR}/.DirIcon"
    
    # Update desktop file for AppImage
    sed -i -E "s|^Exec=.*|Exec=AppRun|" "${APPDIR}/${APP_ID}.desktop"
    sed -i -E "s|^Icon=.*|Icon=${APP_ID}|" "${APPDIR}/${APP_ID}.desktop"
    
    # Bundle dependencies
    mkdir -p "${APPDIR}/usr/lib"
    ldd "${BIN_PATH}" 2>/dev/null | awk '/=> \// {print $3}' | \
        grep -vE 'lib(c|pthread|rt|dl|m|gcc_s|stdc\+\+|glib|X11|xcb|wayland)' | \
        xargs -r -I '{}' cp '{}' "${APPDIR}/usr/lib/" 2>/dev/null || true
    
    # Build AppImage
    APPIMAGE_TOOL="$(command -v appimagetool 2>/dev/null || true)"
    if [ -z "${APPIMAGE_TOOL}" ] && [ -x "${LOCAL_TOOLS_DIR}/bin/appimagetool" ]; then
        APPIMAGE_TOOL="${LOCAL_TOOLS_DIR}/bin/appimagetool"
    fi
    if [ -z "${APPIMAGE_TOOL}" ]; then
        log_error "appimagetool not found"
        exit 1
    fi
    # Use absolute path since we cd to DIST_DIR
    APPDIR_ABS="$(pwd)/${WORK_DIR}"
    cd "${DIST_DIR}"
    APPIMAGE_EXTRACT_AND_RUN=1 "${APPIMAGE_TOOL}" \
        "${APPDIR_ABS}/FyClip.AppDir" \
        "${PKG_NAME}_${VERSION}_${APPIMAGE_ARCH}.AppImage"
    cd - >/dev/null
    
    log_success "AppImage built: ${DIST_DIR}/${PKG_NAME}_${VERSION}_${APPIMAGE_ARCH}.AppImage"
    
    # ---------------------------------------------------------------------
    # Build Tarball
    # ---------------------------------------------------------------------
    log_info "Building tarball..."
    rm -rf "${TARBALL_ROOT}"
    mkdir -p "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}"
    
    # Copy application files
    mkdir -p "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/bin"
    cp -f "${BIN_PATH}" "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/bin/${BIN_NAME}"
    chmod +x "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/bin/${BIN_NAME}"
    
    # Copy desktop file
    mkdir -p "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/share/applications"
    cp -f "${DESKTOP_PATH}" "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/share/applications/${APP_ID}.desktop"
    
    # Copy icon files at all standard sizes (properly resized)
    generate_hicolor_icons "${ICON_PATH}" "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/share/icons/hicolor"
    ICON_EXT="png"
    
    # Copy LICENSE file
    cp -f "Licence" "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/LICENSE"

    # Copy icon remediation helper (run by install.sh as post-install step)
    cp -f "fix-icons.sh" "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/fix-icons.sh"
    chmod +x "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/fix-icons.sh"
    
    # Create README for the tarball
    cat > "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/README.md" <<TARBALL_README
# FyClip - Advanced Clipboard Manager

A desktop clipboard manager built with Go and Fyne v2.8.1.

**Current Version**: ${VERSION}

## Features

- 📋 **Clipboard History**: Automatically saves text, images, HTML, and files
- 📌 **Pin Items**: Keep important items at the top
- ⭐ **Favorites View**: Toggle pinned-only view instantly
- 🔍 **Search**: Filter clipboard history from the search field
- ❌ **Clear Search**: One-click reset for the search box
- 🖼️ **Image Support**: Preview and save clipboard images
- 📝 **HTML Support**: Capture and preserve HTML formatting
- 📁 **File History**: Track files copied from file manager
- 📤 **Unified Export**: Export selected text or images from one action
- 📝 **Markdown Preview**: Markdown content renders correctly in preview pane
- 🕒 **Relative Time + Reuse Count**: List rows show recency and copy frequency
- 💾 **Persistent Storage**: History saved across sessions
- 🔒 **Encrypted Storage**: AES-256-GCM encryption at rest
- ☁️ **Encrypted Backup**: Password-protected backup and restore
- 📝 **Snippets**: Save and expand text templates
- 🚀 **AutoStart**: Launch on system startup
- ⏸️ **Pause Capture**: Pause monitoring for 5 minutes from toolbar/tray
- 🎨 **Theme Support**: Light, Dark, and System theme modes with easy switching
- ⚡ **Performance**: Debounced updates, async operations, O(1) lookups
- 🔒 **Thread-Safe**: Proper concurrency handling
- 🛡️ **Sensitive Data Detection**: Auto-detect credit cards, SSN, API keys
- 📦 **Bulk Operations**: Multi-select items for batch delete/pin/unpin
- 🗂️ **Automatic Categories**: Group common content types (Links, Code, Contacts, etc.)
- ⌨️ **Enhanced Keyboard Navigation**: Arrow keys, Enter, Delete, Escape, Space, Home/End, F1
- ⬆️ **Auto Update**: Check for and install updates from GitHub releases
- 📦 **Linux Packaging**: Official support for .deb, .AppImage, and tarball

## Installation

### Using DEB Package (Debian/Ubuntu)
```bash
sudo dpkg -i fyclip_<version>_amd64.deb
sudo apt-get install -f  # Install dependencies
```

### Using AppImage
```bash
chmod +x fyclip_<version>_x86_64.AppImage
./fyclip_<version>_x86_64.AppImage
```

### Using Tarball
```bash
tar -xzf fyclip_<version>_linux_amd64.tar.gz
cd fyclip_<version>_linux_amd64
sudo ./install.sh
```

## Usage

- Launch FyClip from your application menu or terminal
- Use the system tray icon to access clipboard history
- Press F1 while FyClip is focused to open the quick paste panel
- Right-click on tray icon for settings and options
- Search clipboard history from the search field
- Pin important items to keep them at the top
- Create snippets for frequently used text templates

## System Requirements

- Linux with X11 or Wayland
- GTK 3.0
- xclip, xsel, or wl-clipboard for clipboard access
- libgl1, libx11-6, libxcursor1, libxrandr2, libxinerama1, libxi6

## License

MIT License - See LICENSE file for details

## Author

Copyright (c) 2026 Sarwar Hossain
Email: sarwarhridoy4@gmail.com
TARBALL_README
    
    # Create INSTALL file
    cat > "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/INSTALL" <<'TARBALL_INSTALL'
# FyClip Installation Instructions

## Quick Install

1. Extract the tarball:
   tar -xzf fyclip_<version>_linux_amd64.tar.gz

2. Run the install script (as root):
   cd fyclip_<version>_linux_amd64
   sudo ./install.sh

3. Launch FyClip from your application menu or run 'fyclip'

## Manual Install

If you prefer manual installation:

1. Copy the binary to /usr/local/bin:
   sudo cp bin/fyclip /usr/local/bin/

2. Copy desktop file:
   sudo cp share/applications/com.sarwar.fyclip.desktop /usr/share/applications/

 3. Copy icon files:
     sudo cp -r share/icons/hicolor /usr/share/icons/

 4. Update icon cache:
    sudo gtk-update-icon-cache -f /usr/share/icons/hicolor

## Uninstall

Run (as root):
   ./uninstall.sh

Or manually remove:
   sudo rm /usr/local/bin/fyclip
   sudo rm /usr/share/applications/com.sarwar.fyclip.desktop
   sudo rm -rf /usr/share/icons/hicolor/{16x16,24x24,32x32,48x48,64x64,128x128,256x256}/apps/com.sarwar.fyclip.png
TARBALL_INSTALL
    
    # Create install/uninstall scripts
    cat > "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/install.sh" <<'INSTALL_SCRIPT'
#!/bin/bash
# FyClip Install Script

set -e

APP_NAME="FyClip Clipboard Manager"
BIN_NAME="fyclip"
APP_ID="com.sarwar.fyclip"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

install -d "${DESTDIR:-}/usr/local/bin"
install -d "${DESTDIR:-}/usr/share/applications"
install -d "${DESTDIR:-}/usr/share/icons/hicolor/128x128/apps"
install -d "${DESTDIR:-}/usr/share/icons/hicolor/256x256/apps"

install -m 755 "${SCRIPT_DIR}/bin/${BIN_NAME}" "${DESTDIR:-}/usr/local/bin/${BIN_NAME}"
install -m 644 "${SCRIPT_DIR}/share/applications/${APP_ID}.desktop" "${DESTDIR:-}/usr/share/applications/${APP_ID}.desktop"

for s in 16 24 32 48 64 128 256; do
    install -d "${DESTDIR:-}/usr/share/icons/hicolor/${s}x${s}/apps"
    install -m 644 "${SCRIPT_DIR}/share/icons/hicolor/${s}x${s}/apps/${APP_ID}.png" "${DESTDIR:-}/usr/share/icons/hicolor/${s}x${s}/apps/${APP_ID}.png"
done

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true
fi

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database /usr/share/applications 2>/dev/null || true
fi

# Remediate installed icon sizes and StartupWMClass (idempotent, may elevate)
if [ -x "${SCRIPT_DIR}/fix-icons.sh" ]; then
    "${SCRIPT_DIR}/fix-icons.sh" || true
fi

echo "FyClip installed successfully!"
INSTALL_SCRIPT
    chmod +x "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/install.sh"
    
    cat > "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/uninstall.sh" <<'UNINSTALL_SCRIPT'
#!/bin/bash
# FyClip Uninstall Script

set -e

BIN_NAME="fyclip"
APP_ID="com.sarwar.fyclip"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

rm -f "/usr/local/bin/${BIN_NAME}"
rm -f "/usr/share/applications/${APP_ID}.desktop"
for s in 16 24 32 48 64 128 256; do
    rm -f "/usr/share/icons/hicolor/${s}x${s}/apps/${APP_ID}.png"
done

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true
fi

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database /usr/share/applications 2>/dev/null || true
fi

echo "FyClip uninstalled successfully!"
UNINSTALL_SCRIPT
    chmod +x "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/uninstall.sh"
    
    # Create version info file
    cat > "${TARBALL_ROOT}/${APP_NAME}-${VERSION}-linux-${ARCH}/VERSION" <<EOF
APP_NAME=${APP_NAME}
BIN_NAME=${BIN_NAME}
APP_ID=${APP_ID}
VERSION=${VERSION}
ARCH=${ARCH}
BUILD_DATE=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
AUTHOR=${AUTHOR}
EMAIL=${EMAIL}
SHA256=${PROJECT_SHA256}
EOF
    
    # Create tarball
    # Use absolute path for DIST_DIR since we cd to TARBALL_ROOT
    DIST_DIR_ABS="$(pwd)/${DIST_DIR}"
    cd "${TARBALL_ROOT}"
    tar -czf "${DIST_DIR_ABS}/${PKG_NAME}_${VERSION}_${ARCH}.tar.gz" "${APP_NAME}-${VERSION}-linux-${ARCH}"
    cd - >/dev/null
    
    log_success "Tarball built: ${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.tar.gz"
    
    # Cleanup
    rm -rf "${WORK_DIR}" "${TARBALL_ROOT}" "${APP_NAME}.tar.xz"
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  Build Complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "Debian:   ${BLUE}${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb${NC}"
    echo -e "AppImage: ${BLUE}${DIST_DIR}/${PKG_NAME}_${VERSION}_${APPIMAGE_ARCH}.AppImage${NC}"
    echo -e "Tarball:  ${BLUE}${DIST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.tar.gz${NC}"
    echo ""

    # Remediate the live installed system icons (idempotent; auto-elevates via sudo).
    # Set FYCLIP_SKIP_ICON_FIX=1 to skip (e.g. in CI / non-interactive sudo).
    if [ -z "${FYCLIP_SKIP_ICON_FIX:-}" ]; then
        local script_dir
        script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        if [ -f "${script_dir}/fix-icons.sh" ]; then
            log_info "Remediating installed system icons (fix-icons.sh)..."
            local src_icon="${script_dir}/icon.png"
            bash "${script_dir}/fix-icons.sh" "${src_icon}" || log_warn "fix-icons.sh failed or skipped (interactive sudo required)"
        fi
    fi
}

# Run main function
main "$@"
