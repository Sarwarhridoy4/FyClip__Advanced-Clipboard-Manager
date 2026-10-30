# FyClip - Advanced Clipboard Manager

<p align="center">
  <img src="internal/app/assets/icon.png" alt="FyClip Logo" width="128" height="128"/>
  <br>
  <a href="https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/releases/latest">
    <img src="https://img.shields.io/github/v/release/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager?include_prereleases&style=flat" alt="GitHub release">
  </a>
  <a href="https://goreportcard.com/report/github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager">
    <img src="https://goreportcard.com/badge/github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager" alt="Go Report Card">
  </a>
  <a href="https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/blob/main/Licence">
    <img src="https://img.shields.io/github/license/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager" alt="License">
  </a>
</p>

> A desktop clipboard manager for text, images, HTML, and files, built with Go and Fyne.

**Current Version**: 2.7.0

---

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Quick Start](#quick-start)
- [Installation](#installation)
  - [Linux](#linux)
  - [Windows](#windows)
  - [macOS](#macos)
  - [Building from Source](#building-from-source)
- [Building for Release](#building-for-release)
- [Usage](#usage)
  - [Keyboard Shortcuts](#keyboard-shortcuts)
  - [Bulk Operations](#bulk-operations)
  - [Snippets](#snippets)
- [Configuration](#configuration)
- [Architecture](#architecture)
  - [Project Structure](#project-structure)
  - [Design Principles](#design-principles)
  - [Performance Optimizations](#performance-optimizations)
  - [Security](#security)
- [Development](#development)
  - [Prerequisites](#prerequisites)
  - [Makefile Targets](#makefile-targets)
  - [Adding New Features](#adding-new-features)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [Changelog](#changelog)
- [License](#license)
- [Acknowledgments](#acknowledgments)

---

## Features

FyClip is built for people who copy a lot and want fast recall, reliable history, and safer local storage.

### Highlights

- **Rich clipboard history** for text, images, HTML, and files
- **Searchable clipboard history** with a clear-search control
- **Pinned items, automatic categories, and reusable snippets** for organization
- **Encrypted local storage and backups** with safer clipboard write paths
- **Cross-platform desktop integration** with tray controls, pause capture, and auto-update support

### Feature Overview

| Area | Included |
|------|----------|
| **Capture** | Text, images, HTML, and file history |
| **Search** | Search clipboard history and clear the active query |
| **Organize** | Pinning, pinned-only view, automatic categories, snippets |
| **Actions** | Copy, export, bulk select, bulk delete, pin/unpin |
| **Date** | Sort by calendar date and filter by start/end dates |
| **Security** | AES-256-GCM encrypted history, password-protected backups, clipboard and path validation, and update asset/hash verification |
| **System** | Autostart, pause capture, system tray actions, GitHub-based auto updates |

### Auto Update Feature

The auto-update feature allows you to check for and install updates directly from GitHub releases.

#### How It Works

1. **Version Detection**: Reads the current version from embedded `internal/version/version.go` file (generated during build)
2. **GitHub Check**: Fetches the latest release from GitHub API
3. **Version Comparison**: Compares semantic versions, including `v` prefixes and pre-release handling
4. **Request Optimization**: Uses in-memory caching and rate limiting to reduce repeated GitHub API traffic
5. **Asset Selection**: Scores release assets for the active OS and architecture, preferring native installer formats like `.deb`, `.exe`, and `.dmg`
6. **Update Available**: Prompts when a newer compatible release is found
7. **Up to Date**: Reports when the current version is already the latest supported release

#### Why it is efficient

- Caches successful update checks in memory
- Applies rate limiting to repeated GitHub API calls
- Scores release assets concurrently for the active OS and architecture
- Prefers native installer formats when multiple assets are available

#### Usage

**From UI:**
- Click "Help" → "Check for Updates" in the menu

**From Terminal:**
```bash
# Check for updates
fyclip --check-update

# Download and install updates
fyclip --update
```

#### Supported Platforms

| Platform | Package Formats |
|----------|----------------|
| Linux | Check the release page for available packages, such as `.deb` or `.AppImage` |
| Windows | Check the release page for available installers |
| macOS | Check the release page for available packages |

### User Interface

| Feature | Description |
|---------|-------------|
| 🎨 **Theme Support** | Light, Dark, and System theme modes |
| 🎨 **Modern UI** | Dark theme with responsive design |
| ⌨️ **Keyboard Navigation** | Arrow keys, Enter, Delete, Escape, Space, Home/End, F1 |
| 🕒 **Relative Time** | List rows show recency and copy frequency |
| 💾 **Persistent Storage** | History saved across sessions |

### Performance

| Feature | Description |
|---------|-------------|
| ⚡ **Debounced Updates** | Batched UI updates for smooth performance |
| ⚡ **Async Operations** | Non-blocking clipboard monitoring |
| ⚡ **O(1) Lookups** | Hash map-based duplicate detection |
| 🧠 **Adaptive Memory Pressure Handling** | Multi-level cleanup responds to sustained memory growth |
| 🕒 **LRU History Retention** | Frequently used items are preserved longer than stale history |
| 🔒 **Thread-Safe** | Proper concurrency handling |
| 🖼️ **Image Thumbnails** | 150x150px thumbnails for efficient list display |
| 📦 **Compression** | Gzip compression before encryption for smaller storage |
| 💾 **Memory Optimized** | Differential indexing, object reuse, and reduced clipboard write copies |

### Security At A Glance

| Feature | CWE | Description |
|---------|-----|-------------|
| 🔒 **Encrypted Storage** | CWE-522 | Clipboard history is encrypted at rest with AES-256-GCM |
| 🔐 **PBKDF2 Key Derivation** | CWE-916 | Storage and backup keys are derived with 100,000 iterations |
| ☁️ **Encrypted Backup** | CWE-916 | Password-protected backup and restore with per-backup salting |
| 🧼 **Memory-Safer Writes** | — | Programmatic copy hashing and temporary buffer wiping where practical |
| 🚫 **Validation Guards** | CWE-20/CWE-697 | Clipboard size limits plus file-path and command validation |
| 🔍 **Security Hardening** | Multiple | Path validation, command allowlisting, ReDoS protection, UTF-8 sanitization, display sanitization, and integrity checks across update, storage, clipboard, and UI paths |
| 🔄 **Update Integrity** | CWE-494/CWE-377/CWE-601/CWE-400/CWE-409/CWE-22 | SHA-256 digest verification, private temp downloads, GitHub host-restricted redirects, size caps, and ZIP Slip prevention |

For the full security audit and remediation details (12 findings: 6 High, 6 Medium), see [`SECURITY_VULNERABILITIES.md`](./SECURITY_VULNERABILITIES.md).

---

## Screenshots

<div align="center">
  <img src="internal/app/assets/screenshots/main_window.png" alt="Main Window - Clipboard History with Search and Preview" width="800"/>
  <p><em>Main Window - Clipboard History with Search and Preview</em></p>
</div>

<div align="center">
  <img src="internal/app/assets/screenshots/quick_paste.png" alt="Quick Panel - Global Hotkey Access" width="800"/>
  <p><em>Quick Panel - Global Hotkey Access</em></p>
</div>

<div align="center">
  <img src="internal/app/assets/screenshots/updater.png" alt="Update Dialog" width="800"/>
  <p><em>Update Dialog</em></p>
</div>

---

## Quick Start

### Linux

```bash
# Install dependencies (X11)
sudo apt install xclip

# Or for Wayland
sudo apt install wl-clipboard

# Run the application
./fyclip
```

### Windows

```bash
# Simply run the executable
fyclip.exe
```

### macOS

```bash
# Run the application
./fyclip
```

---

## Installation

### Pre-built Packages

#### Linux

Download an available Linux package from [Releases](https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/releases). For a Debian package:

```bash
sudo dpkg -i fyclip_<version>_<arch>.deb
```

For an AppImage:

```bash
chmod +x fyclip_<version>_<arch>.AppImage
./fyclip_<version>_<arch>.AppImage
```

Download from [Releases](https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/releases)

#### Windows

Download the installer from [Releases](https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/releases)

#### macOS

Download an available macOS package from [Releases](https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/releases).

### Building from Source

#### Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.26.1 | Required by this checkout's `go.mod` |
| Fyne | 2.8.1 | Resolved by Go modules |

#### Linux Dependencies

**Ubuntu/Debian:**
```bash
# For X11
sudo apt install xclip

# For Wayland
sudo apt install wl-clipboard
```

**Arch Linux:**
```bash
# For X11
sudo pacman -S xclip

# For Wayland
sudo pacman -S wl-clipboard
```

**Fedora:**
```bash
# For X11
sudo dnf install xclip

# For Wayland
sudo dnf install wl-clipboard
```

#### Build Steps

```bash
# 1. Clone the repository
git clone https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager.git
cd FyClip---Advanced-Clipboard-Manager

# 2. Install dependencies
go mod download

# 3. Build
make build

# 4. Run
./fyclip
```

---

## Building for Release

### Using Build Script (Recommended for Linux)

The build script follows official Linux packaging conventions:

- **Debian**: `dpkg-buildpackage` using the existing `debian/` directory
- **AppImage**: `appimagetool` on a proper AppDir
- **Tarball**: relocatable package for Arch, Fedora, and other non-Debian distros

```bash
# Build with default version (all package types: deb, appimage, tarball)
./build.sh

# Build with explicit version
./build.sh 2.7.0

# Build only the tarball (covers Arch, Fedora, and other distros)
./build.sh 2.7.0 tarball
# or
FYCLIP_PACKAGE_TYPES=tarball ./build.sh

# Build only the Debian package
./build.sh 2.7.0 deb

# Build only the AppImage
./build.sh 2.7.0 appimage
```

This produces:
- `dist/fyclip_<version>_<arch>.deb` - Debian package (Debian/Ubuntu)
- `dist/fyclip_<version>_<arch>.AppImage` - AppImage (universal Linux)
- `dist/fyclip_<version>_<arch>.tar.gz` - Tarball (Arch, Fedora, and other distros)

**AppImage Hub submission:**

The built AppImage includes AppStream metainfo (`share/metainfo/com.sarwar.fyclip.metainfo.xml`) for software-center discovery and AppImage Hub submission. To submit to [AppImage Hub](https://appimage.github.io/appimagehub/):

1. Build the AppImage: `./build.sh 2.7.0 appimage`
2. Test it locally: `chmod +x dist/fyclip_2.7.0_x86_64.AppImage && ./dist/fyclip_2.7.0_x86_64.AppImage`
3. Fork https://github.com/AppImage/AppImageKit (or the AppImageHub repo)
4. Add your AppImage metadata following the AppImageHub submission guidelines
5. Submit a pull request with the AppImage file and its `.metainfo.xml`

**AppImage older-version compatibility:**

The AppImage bundles all shared libraries except the dynamic linker (`ld-linux`) and `linux-vdso` (kernel-provided), so it runs on older distributions. For maximum compatibility:

1. **Build on the oldest target glibc** — glibc is not forward-compatible; an AppImage built on glibc 2.34 will not run on glibc 2.28. Build on the oldest distribution you want to support (e.g., Ubuntu 18.04 / CentOS 7 for glibc 2.17-2.27).
2. **Link against older glibc** — use an older toolchain or container (e.g., `ubuntu:18.04`, `centos:7`) when building so the binary's minimum glibc version is low.
3. The bundled libraries in `AppDir/usr/lib/` take precedence via `LD_LIBRARY_PATH` in `AppRun`.


### Using Makefile

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Package for distribution
make package

# Create release
make release
```

### Using Fyne Native Packaging

```bash
# Linux tar package
fyne package --os linux --release --name fyclip --icon icon.png

# Windows installer
fyne package --os windows --release --name fyclip --icon icon.png

# macOS app bundle
fyne package --os darwin --release --name fyclip --icon icon.png
```

### Cross-Platform Build with fyne-cross

```bash
# Install fyne-cross
go install github.com/fyne-io/fyne-cross@latest

# Build for Linux
fyne-cross linux -arch=amd64

# Build for Windows
fyne-cross windows -arch=amd64

# Build for macOS
fyne-cross darwin -arch=amd64
```

---

## Usage

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `↑` / `↓` | Move through the history list |
| `Enter` or `Ctrl+C` | Copy the selected item |
| `Delete` | Delete the selected item |
| `Space` | Copy the selected item; toggle selection in selection mode |
| `Escape` | Exit selection mode |
| `Home` / `End` | Move to the first / last item |
| `F1` | Open the quick panel while FyClip is focused |
| `Ctrl+F` | Focus search |
| `Ctrl+B` / `Ctrl+R` | Open backup / restore |

### Bulk Operations

Use the toolbar's **Select** button to enter selection mode, then click items to select them. The toolbar provides select all, clear selection, pin, unpin, and delete actions.

### Features Guide

1. **Pin Items**: Select an item and click the **Pin** button to keep it at the top
2. **Search**: Type in the search bar to filter clipboard history
3. **Pinned Filter**: Click "Pinned Only" to show pinned items only
4. **Preview**: Select an item to see full content (JSON pretty-printed automatically)
5. **Export**: Click "Export" to save selected text or image
6. **Pause Monitoring**: Use "Pause 5m" to temporarily stop capturing
7. **History Limit**: Configure max unpinned history via toolbar settings
8. **Clear History**: Remove all unpinned items
9. **System Tray**: Show FyClip, open quick paste, configure autostart, or access recent items (where supported)
10. **Snippets**: Create and manage text templates
11. **Backup**: Create encrypted backups of your history
12. **Categories**: Auto-categorized content (Links, Code, Contacts, Images, Files, Text)
13. **Date tools**: Sort by date or filter with start and end dates in `YYYY-MM-DD` format
14. **Theme**: Switch between Light, Dark, and System themes
15. **Bulk Operations**: Multi-select items for batch actions
16. **Sort by Date**: Sort history by calendar date
17. **Filter by Date**: Enter start and end dates (`YYYY-MM-DD`) to filter history

### Snippets

Snippets allow you to create reusable text templates with dynamic variables.

#### Creating a Snippet

1. Click the "Snippets" button in the toolbar
2. Click "Add Snippet" button
3. Fill in the details:
   - **Title**: A descriptive name (e.g., "Email Signature")
   - **Content**: The template text with optional variables
   - **Abbreviation** (optional): A short trigger word for quick access
   - **Category** (optional): Group snippets by category

#### Available Variables

| Variable | Description | Example Output |
|----------|-------------|----------------|
| `{{date}}` | Current date | 2026-10-06 |
| `{{time}}` | Current time | 14:30:45 |
| `{{datetime}}` | Full date and time | 2026-10-06 14:30:45 |
| `{{year}}` | Current year | 2026 |
| `{{month}}` | Current month (01-12) | 10 |
| `{{day}}` | Current day (01-31) | 06 |
| `{{clipboard}}` | Current clipboard content | (varies) |

#### Example Snippet

```
Title: Email Signature
Abbreviation: sig
Category: General
Content:
Best regards,
{{clipboard}}
{{date}}
```

If the clipboard contains `John Doe`, this expands to:
```
Best regards,
John Doe
2026-10-06
```

---

## Configuration

Settings are automatically saved to:

| Platform | Path |
|----------|------|
| Linux | `~/.fyclip/` |
| Windows | `%USERPROFILE%\.fyclip\` |
| macOS | `~/.fyclip/` |

---

## Architecture

### Project Structure

```
fyclip/
├── main.go                      # Application entry point
├── Makefile                     # Build automation
├── go.mod                       # Go module dependencies
├── icon.png                     # Application icon
├── FyneApp.toml                 # Fyne application configuration
├── build.sh                     # Release build script
│
├── internal/
│   ├── app/
│   │   ├── app.go              # App initialization & lifecycle
│   │   └── single_instance.go  # Single instance enforcement
│   │
│   ├── clipboard/
│   │   ├── item.go             # Clipboard item types (Text, Image, HTML, File)
│   │   ├── manager.go          # Core manager logic & state
│   │   ├── monitor.go          # Clipboard monitoring & change detection
│   │   ├── native.go           # Platform-specific clipboard operations
│   │   ├── storage.go          # Persistence layer with encryption
│   │   ├── snippet.go          # Snippet management & expansion
│   │   ├── exclusion.go        # Pattern exclusion rules
│   │   ├── search.go           # Enhanced search (regex, fuzzy)
│   │   ├── backup.go           # Encrypted backup & restore
│   │   ├── sensitive.go        # Sensitive data detection
│   │   ├── pool.go             # Object pooling for performance
│   │   └── validation.go       # Input validation
│   │
│   ├── config/
│   │   ├── config.go           # Configuration management
│   │   └── validation.go       # Config validation
│   │
│   ├── errors/
│   │   └── errors.go           # Custom error types
│   │
│   ├── logger/
│   │   └── logger.go           # Structured logging with file rotation
│   │
│   ├── metrics/
│   │   └── metrics.go          # Application metrics
│   │
│   ├── ui/
│   │   ├── window.go           # Main window management
│   │   ├── list.go             # History list widget
│   │   ├── preview.go          # Preview pane with formatting
│   │   ├── toolbar.go          # Action buttons & controls
│   │   ├── search.go           # Search bar component
│   │   ├── status.go           # Status bar
│   │   ├── dialogs.go          # Dialogs & utilities
│   │   ├── quickpanel.go       # Quick access panel
│   │   └── update_dialogs.go   # Update notification dialogs
│   │
│   ├── platform/
│   │   ├── autostart.go        # Autostart interface
│   │   ├── autostart_linux.go  # Linux implementation
│   │   ├── autostart_windows.go # Windows implementation
│   │   ├── autostart_darwin.go # macOS implementation
│   │   └── utils.go            # Platform utilities
│   │
│   ├── tray/
│   │   └── tray.go             # System tray integration
│   │
│   ├── update/
│   │   └── checker.go          # Auto-update checking
│   │
│   └── testutil/
│       └── testutil.go         # Testing utilities
│
└── README.md
```

### Design Principles

#### Modular Architecture
- **Separation of Concerns**: Each module has a single responsibility
- **Clean Interfaces**: Well-defined APIs between components
- **Testability**: Easy to unit test individual modules

#### Code Organization
- Internal packages (`internal/`) for implementation details
- Clear dependency direction: `main.go` → `app` → `clipboard`/`ui` → `config`/`logger`
- Feature flags for optional functionality

### Performance Optimizations

| Optimization | Description |
|--------------|-------------|
| **Debounced Updates** | UI updates are batched (50ms debounce) |
| **Coalesced Saves** | History persistence requests are serialized and debounced (250ms) |
| **O(1) Lookups** | Hash maps for duplicate detection and item access |
| **Differential Index Rebuilds** | Modified indices are refreshed selectively instead of full rebuilds on every change |
| **Efficient Filtering** | Search avoids repeated lowercasing and minimizes allocation churn |
| **Object Pool** | `sync.Pool` for Item reuse to reduce GC pressure |
| **Regex Cache** | Compiled regex patterns cached for faster repeated searches (LRU eviction at 128 entries) |
| **Fuzzy Search** | Optimized subsequence matching with reduced allocations |
| **Adaptive Memory Pressure Handling** | Cleanup becomes more aggressive as allocation pressure rises (checked every 60s normally, 20s under pressure) |
| **LRU Trimming** | History eviction preserves frequently accessed items; uses `sort.Slice` + descending-index batch removal (O(n log n) instead of O(n²)) |
| **Update Check Caching** | Repeated GitHub release checks are cached and rate-limited |
| **Concurrent Asset Scoring** | Release assets are processed concurrently during platform-specific update selection |
| **Thread-Safe** | Proper mutex usage throughout |
| **Selection Fast Path** | Selecting list items avoids redundant full-window refreshes |
| **Duplicate Promotion** | Existing duplicates move to latest with notification |
| **Dirty-flag Save Skip** | `saveHistoryNow` skips marshal/gzip/AES-encrypt/write when history unchanged since last save |
| **gzip Buffer Pooling** | `sync.Pool` reuses `*gzip.Writer`/`*gzip.Reader` across save/load cycles (~59% less allocation in StorageSave) |
| **FilteredSeq** | Zero-copy iteration over filtered items without allocating a `[]Item` copy |

#### Benchmark Results

Run the benchmarks to see performance improvements:

```bash
go test -bench 'Benchmark(UpdateFilteredSearch1000|AddItemWithDuplicateScan1000|StorageSave1000)
 -benchmem ./internal/clipboard
```

Current benchmark results (measured with `go test -bench '.' -benchmem -count=3 ./internal/clipboard`):

| Benchmark | ns/op | B/op | allocs/op | Change |
|---|---|---|---|---|
| `BenchmarkUpdateFilteredSearch1000` | ~104,000 | 255 | 5 | sort.Slice replaces O(n²) bubble sort |
| `BenchmarkAddItemWithDuplicateScan1000` | ~415,000 | ~158,500 | ~1,400 | ~5.2x faster (was 2,150,222 ns/op) |
| `BenchmarkStorageSave1000` | ~3,300,000 | ~540,000 | ~2,030 | ~59% less allocation (was 1,292,749 B/op) |

Notes:
- `AddItem` improved ~5.2x from `trimHistory` using `sort.Slice` + descending-index batch removal.
- `StorageSave` allocation dropped ~59% from gzip buffer pooling and the dirty-flag save skip.
- `UpdateFilteredSearch` allocation rose slightly (0 → 255 B/op) from `sort.Slice`, but is O(n log n) instead of O(n²) for large histories.

### Security

FyClip applies defense-in-depth across update, storage, clipboard, and UI paths. Twelve findings were remediated (6 High, 6 Medium), documented in [`SECURITY_VULNERABILITIES.md`](./SECURITY_VULNERABILITIES.md):

| # | Severity | CWE | Finding |
|---|----------|-----|---------|
| 1 | High | CWE-494 | Update asset digest enforced from GitHub release metadata; fails closed if missing |
| 2 | High | CWE-377 | Private temp download dir with exclusive creation and owner-only permissions |
| 3 | High | CWE-601 / SSRF | Download URL restricted to HTTPS on github.com; redirects limited to GitHub hosts |
| 4 | High | CWE-400 / CWE-409 | Metadata capped at 2 MiB; download/expanded data at 1 GiB; history/backups/snippets at 256 MiB and 100,000 entries |
| 5 | High | CWE-22 | ZIP extraction via Go reader in private temp dir; symlinks and path escapes rejected |
| 6 | High | CWE-311 | Destructive legacy key migration removed; legacy keys loaded unchanged |
| 7 | High | CWE-409 | Image dimensions and 16M pixel area validated before full decode |
| 8 | Medium | CWE-697 | Command validation checks resolved path exists and is a file |
| 9 | Medium | CWE-20 | Backup schema: version, checksum shape, item count, and per-item validation |
| 10 | Medium | CWE-400 | Regex cache: patterns >1,000 bytes bypass compilation; reset at 128 entries |
| 11 | Medium | CWE-20 | Restored items pass `ValidateItem` with type-specific and content-size checks |
| 12 | Medium | CWE-400 | Bounded writers for xclip/wl-paste output; monitor rejects oversized values |

**Remaining risks (see `SECURITY_VULNERABILITIES.md` for details):** encryption key stored beside ciphertext, legacy SHA-256 backup fallback, digest trust depends on GitHub/TLS, platform-specific install paths not exercised, no automated dependency scan, and native clipboard allocation before monitor limits apply.

### Thread Safety

- All shared state protected with `sync.RWMutex`
- Proper locking hierarchy to prevent deadlocks
- Channel-based communication for cross-goroutine updates

---

## Development

### Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.26.1 | Required by this checkout's `go.mod` |
| Fyne | 2.8.1 | Resolved by Go modules |

### Makefile Targets

```bash
make help    # Show available targets
make build   # Build the application
make test    # Run tests
make lint    # Run linter
make clean   # Clean build artifacts
```

### Adding New Features

1. **New clipboard item types**: Extend `internal/clipboard/item.go`
2. **UI components**: Add to `internal/ui/`
3. **Platform features**: Implement in `internal/platform/`

### Code Style

- Follow Go conventions
- Use meaningful variable names
- Add comments for exported functions
- Keep functions small and focused

---

## Troubleshooting
### Icon Not showing
- Download from release
```bash
fix-icons.sh
```
- Make executable by
```bash
chmod +x ./fix-icons.sh
```
- Run
```bash
./fix-icons.sh
```
- Icon issue will be sloved.
  
### Clipboard not working on Linux

Make sure you have the required clipboard tools:

```bash
# Check for xclip
which xclip

# Check for wl-paste
which wl-paste
```

### Build errors

Make sure you have the latest Fyne dependencies:

```bash
go get -u fyne.io/fyne/v2@latest
go mod tidy
```

### Icon appears blurry, missing, or falls back to a generic icon on Linux

The icon is now fixed automatically as a **post-install step**: the
`.deb` package runs `fix-icons.sh` from its `postinst`, and the tarball's
`install.sh` runs it after deploying files. This regenerates all standard
hicolor sizes (16, 24, 32, 48, 64, 128, 256) from the source artwork, fixes
the desktop entry's `StartupWMClass`, and refreshes the icon/desktop caches.
Pillow (`python3 -m pip install pillow`) is required for proper resizing.

To remediate an already-installed system manually, run the bundled helper as
root:

```bash
sudo /usr/share/fyclip/fix-icons.sh   # if installed via .deb
# or from a source checkout:
sudo ./fix-icons.sh
```

---

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Submit a pull request

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for a history of changes.

---

## License

MIT License - See [Licence](Licence) file for details

---

## Acknowledgments

- [Fyne](https://fyne.io/) - Cross-platform GUI library
- [Go](https://golang.org/) - Programming language
- All contributors and testers

---

<p align="center">
  <strong>Made with ❤️ by <a href="https://github.com/Sarwarhridoy4">Sarwar Hossain</a></strong>
</p>
