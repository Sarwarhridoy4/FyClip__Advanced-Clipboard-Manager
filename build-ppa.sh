#!/bin/bash
# =====================================================================
# FyClip PPA Build Script
# Builds source package for Ubuntu PPA submission
# =====================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
APP_NAME="FyClip"
BIN_NAME="fyclip"
APP_ID="com.sarwar.fyclip"
PKG_NAME="fyclip"
AUTHOR="Sarwar Hossain"
EMAIL="sarwarhridoy4@gmail.com"
DIST_DIR="dist"
PPA_BUILD_DIR="${DIST_DIR}/ppa-build"
DEBIAN_REVISION="1"
DEBIAN_DISTRIBUTION="${DEBIAN_DISTRIBUTION:-jammy}"
DEB_VERSION=""

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Get version from git or use default
get_version() {
    local tag
    tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
    if [ -n "${tag}" ]; then
        printf '%s\n' "${tag#v}"
    else
        printf '%s\n' "2.5.0"
    fi
}

# Check if required tools are installed
check_dependencies() {
    log_info "Checking dependencies..."
    
    local missing=()
    
    if ! command -v go >/dev/null 2>&1; then
        missing+=("golang-go")
    fi
    
    if ! command -v dpkg-buildpackage >/dev/null 2>&1; then
        missing+=("dpkg-dev")
    fi
    
    if ! command -v debuild >/dev/null 2>&1; then
        missing+=("devscripts")
    fi

    command -v rsync >/dev/null 2>&1 || missing+=("rsync")
    command -v tar >/dev/null 2>&1 || missing+=("tar")
    if ! command -v dpkg-source >/dev/null 2>&1 || ! command -v dpkg-parsechangelog >/dev/null 2>&1; then
        missing+=("dpkg-dev")
    fi
    
    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing dependencies: ${missing[*]}"
        log_info "Install them with:"
        log_info "  sudo apt-get install ${missing[*]}"
        exit 1
    fi
    
    log_success "All dependencies are installed"
}

# Clean previous builds
clean_build() {
    log_info "Cleaning previous builds..."
    rm -rf "${PPA_BUILD_DIR}"
    rm -f "${DIST_DIR}"/*.deb
    rm -f "${DIST_DIR}"/*.build
    rm -f "${DIST_DIR}"/*.buildinfo
    rm -f "${DIST_DIR}"/*.changes
    rm -f "${DIST_DIR}"/*.dsc
    rm -f "${DIST_DIR}"/*.tar.gz
    rm -f "${DIST_DIR}"/*.tar.xz
    log_success "Cleaned previous builds"
}

# Prepare source for PPA build
prepare_source() {
    log_info "Preparing source for PPA build..."
    
    mkdir -p "${PPA_BUILD_DIR}"
    
    # Copy source files (excluding dist directory to avoid copying into itself)
    rsync -a --exclude='dist' --exclude='.git' --exclude='bin' --exclude='snap' --exclude='.github' . "${PPA_BUILD_DIR}/"

    local current_deb_version old_changelog
    current_deb_version=$(cd "${PPA_BUILD_DIR}" && dpkg-parsechangelog -S Version)
    if [ "${current_deb_version}" != "${DEB_VERSION}" ]; then
        old_changelog="${PPA_BUILD_DIR}/debian/changelog.old"
        mv "${PPA_BUILD_DIR}/debian/changelog" "${old_changelog}"
        {
            printf 'fyclip (%s) %s; urgency=medium\n\n' "${DEB_VERSION}" "${DEBIAN_DISTRIBUTION}"
            printf '  * Prepare FyClip %s for PPA upload.\n\n' "${VERSION}"
            printf ' -- Sarwar Hossain <sarwarhridoy4@gmail.com>  %s\n\n' "$(date -R)"
            cat "${old_changelog}"
        } > "${PPA_BUILD_DIR}/debian/changelog"
        rm -f "${old_changelog}"
    fi
    
    # Create orig tarball
    cd "${PPA_BUILD_DIR}"
    tar -czf "../${PKG_NAME}_${VERSION}.orig.tar.gz" --exclude='debian' .
    cd - > /dev/null
    
    log_success "Source prepared"
}

# Build source package for PPA
build_ppa_package() {
    log_info "Building source package for PPA..."
    
    cd "${PPA_BUILD_DIR}"
    
    # Build source package (no binary)
    debuild -S -sa
    
    cd - > /dev/null
    
    log_success "Source package built for PPA"
}

# Verify the package
verify_package() {
    log_info "Verifying package..."
    
    local dsc_file changes_file
    dsc_file=$(find "${DIST_DIR}" -maxdepth 1 -name "${PKG_NAME}_${DEB_VERSION}*.dsc" -print -quit)
    changes_file=$(find "${DIST_DIR}" -maxdepth 1 -name "${PKG_NAME}_${DEB_VERSION}*.changes" -print -quit)
    
    if [ -z "${dsc_file}" ]; then
        log_error "DSC file not found"
        return 1
    fi
    
    if [ -z "${changes_file}" ]; then
        log_error "Changes file not found"
        return 1
    fi
    
    log_info "DSC file: ${dsc_file}"
    log_info "Changes file: ${changes_file}"
    
    # Show package info
    dpkg-source --info "${dsc_file}"
    
    log_success "Package verified"
}

# Show upload instructions
show_upload_instructions() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  PPA Package Ready!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    log_info "To upload to your PPA:"
    echo ""
    echo "  1. Create a PPA on Launchpad:"
    echo "     https://launchpad.net/~YOUR_USERNAME"
    echo ""
    echo "  2. Upload the package:"
    echo "     dput ppa:YOUR_USERNAME/fyclip ${DIST_DIR}/${PKG_NAME}_${DEB_VERSION}_source.changes"
    echo ""
    echo "  3. Wait for build to complete on Launchpad"
    echo ""
    echo "  4. Share with users:"
    echo "     sudo add-apt-repository ppa:YOUR_USERNAME/fyclip"
    echo "     sudo apt-get update"
    echo "     sudo apt-get install fyclip"
    echo ""
    log_info "For more details, see APP_CENTER_SUBMISSION.md"
}

# Main function
main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  FyClip PPA Package Builder${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    # Get version
    if [ -n "${1:-}" ]; then
        VERSION="${1#v}"
    else
        VERSION=$(get_version)
    fi
    DEB_VERSION="${VERSION}-${DEBIAN_REVISION}"
    
    log_info "Building ${APP_NAME} ${VERSION} PPA package"
    
    # Check dependencies
    check_dependencies
    
    # Create dist directory
    mkdir -p "${DIST_DIR}"
    
    # Clean previous builds
    clean_build
    
    # Prepare source
    prepare_source
    
    # Build package
    build_ppa_package
    
    # Verify package
    verify_package
    
    # Show upload instructions
    show_upload_instructions
}

# Run main function
main "$@"
