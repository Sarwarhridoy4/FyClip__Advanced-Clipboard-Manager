# FyClip Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Quick Paste Overlay**: Replaced modal quick-paste dialog with a larger floating popup overlay
  - Shows up to 15 recent items instead of 9
  - Opens centered at top of window like Windows+V
  - Keyboard navigation: ↑↓ to move, Enter to select, Esc to close
  - Selected item is highlighted with primary theme color
- **Quick Paste from System Tray**: Added Quick Paste option to system tray menu for fast access while app is running
- **Equal-Width Toolbar Buttons**: Toolbar rows now use grid layout so buttons share equal width
- **Simplified Date Filtering**: Replaced custom calendar widget with straightforward From/To date inputs in YYYY-MM-DD format

### Fixed
- **Quick Paste Keyboard Handling**: Navigation keys no longer fall through to main list when quick paste popup is open
- **Clipboard Image Limits**: Reject oversized image dimensions before thumbnail generation and preview decoding
- **Bounded Clipboard Reads**: Limit Linux clipboard subprocess output and reject oversized monitored text, image, and HTML payloads
- **Backup Validation**: Validate backup versions, checksums, item counts, and item contents before restore
- **Stored Item Validation**: Validate restored items and stored history records before using or saving them
- **Update Download Safety**: Restrict release URLs and redirects to approved HTTPS GitHub hosts, cap download and extraction sizes, and reject unsafe archive paths
- **Update Integrity**: Require and verify the SHA-256 digest published for a release asset before installation
- **Regex Cache Limits**: Bound regex pattern length and cache growth
- **Linux Packaging**: Correct hicolor icon paths and install only screenshots that exist in the source tree
- **Build Version Metadata**: Pass version and build-time values through the correct Go linker variable names

### Security
- Keep legacy storage encryption keys intact instead of replacing them without re-encrypting the saved history
- Create update downloads in private temporary directories and extract archives without following unsafe paths
- Bound reads for history, snippets, backup files, and release metadata

## [2.3.0] - 2026-07-13

### Added
- **Calendar Date Sorting**: Sort clipboard history by calendar date from the toolbar or View menu
- **Date Range Filtering**: Filter history by exact date or date range using a custom selectable calendar picker
  - Open the date picker from the toolbar `Filter Date` button
  - Click once to set start date, click again to set end date
  - Apply filters to show only history from selected range
  - Pinned items remain on top while unpinned items are filtered by date
- **Date Filter UI State**: Toolbar button shows active state when a date filter is applied

### Fixed
- **Tray Icon Persistence**: Re-apply tray icon when the system tray menu is rebuilt to prevent icon from disappearing on some Linux desktops
- **Storage Migration**: Fixed undeclared `newKey` variable in storage key migration path
- **Calendar Month Navigation**: Month/year header now updates correctly when navigating between months in the date picker

## [2.2.3] - 2026-07-04

### Fixed
- **DBus Tray Menu Error**: Fixed "wire format error: input has a null char('\000') in string" in StatusNotifierItem menu
  - Strip null bytes from `DisplayText()` output before passing to system tray menu items

### Added
- **Preview Panel Timestamp**: Show copied item date/time in preview pane for all item types
  - Text, HTML/code, and file preview panes now display "Copied: YYYY-MM-DD HH:MM:SS"
  - Image preview already had this; consistent display across all types

## [2.2.2] - 2026-04-30

### Changed
- Updated release metadata and packaging version references across the project
- Refreshed embedded application version to `2.2.2`

## [2.2.1] - 2026-04-04

### Fixed
- **Notification Crash**: Fixed crash when showing notification if command exits quickly (nil pointer dereference)
- **False Duplicate Detection**: Fixed false "already running" detection when lock file contains PID of different system process
- **Stale Lock Detection**: Improved stale lock file detection on Linux by validating /proc/PID/exe symlink exists

## [2.2.0] - 2026-03-31

### Added
- **Snap Store Support**: Added Snap package configuration for Ubuntu Snap Store
  - Created `snap/snapcraft.yaml` with complete metadata and dependencies
  - Added desktop entry file and application icon for snap package
  - Included 5 application screenshots for Snap Store listing
  - Added `SNAP_STORE_SUBMISSION.md` with build and publish instructions
  - Updated `.gitignore` to exclude `*.snap` build artifacts
  - Updated `readme.md` with Snap installation instructions

## [2.2.0] - 2026-03-25

### Fixed
- **Update Checker**: Fixed update checker to read version from embedded version.go file instead of FyneApp.toml
  - Created `internal/version/version.go` file that gets generated during build with version embedded
  - Updated `main.go` to use version from `internal/version` package
  - Updated `internal/ui/update_dialogs.go` to use version from `internal/version` package
  - Updated `build.sh` to generate `version.go` file with version embedded during build process
  - Version comparison now uses exact string matching instead of semantic version parsing
  - Shows "You are using the latest version!" when current version matches GitHub tag
  - Shows "Update Available!" when versions don't match
  - Fixed issue where installed Debian package showed version as "dev" instead of actual version
- **Update Installation**: Fixed update installation not showing logs or completing
  - Installation commands now capture output instead of sending to stdout/stderr
  - Added installation progress dialog with real-time log display
  - Fixed window reference issue causing popup to disappear
  - Added `GetOutput()` method to retrieve captured installation logs
  - Updated `installDeb()` to use `pkexec` (graphical sudo) for password prompt in UI
  - Updated `installExe()`, `installMsi()`, `installDmg()`, `installZip()` to capture output
  - Added logging to track download and installation progress

### Added
- **Single Instance Protection**: Prevents multiple FyClip instances from running simultaneously
  - Uses lock file mechanism to detect existing instances
  - Lock file location: `~/.local/share/FyClip/.fyclip.lock` (Linux), `%LOCALAPPDATA%/FyClip/` (Windows), `~/Library/Application Support/FyClip/` (macOS)
  - Automatically handles stale lock files from crashed instances
  - Shows system notification when another instance is already running
    - Linux: Critical urgency notification via `shownotification`
    - Windows: PowerShell toast notification
    - macOS: `osascript` notification

### Performance
- **Image Thumbnails**: Added 150x150px thumbnails for efficient memory usage
  - List view displays thumbnails instead of full images
  - Preview pane shows full images when requested
  - Generates thumbnails automatically for captured images
  - Backward compatibility: auto-generates thumbnails for existing items on load
  - Expected: 50-80% memory reduction for image-heavy clipboard history
- **Storage Compression**: Added gzip compression before encryption
  - Compresses JSON data before AES-256-GCM encryption
  - Decompresses after decryption on load
  - Backward compatibility: handles both compressed and uncompressed data
  - Expected: 30-60% reduction in storage file size
- **JSON Optimization**: Removed indentation whitespace from saved JSON
  - Changed from `json.MarshalIndent()` to `json.Marshal()`
  - Expected: ~10-15% additional reduction in storage file size
- **Channel Buffer Optimization**: Reduced update channel buffer from 100 to 10
  - Updates are debounced anyway, so smaller buffer is sufficient
  - Minor memory reduction for channel buffers

## [2.1.3] - 2026-03-24

### Added
- **Auto Update Feature**: Check for and install application updates automatically
  - GitHub release detection for checking latest version
  - Automatic download of platform-specific packages
  - Terminal commands: `fyclip --check-update` and `fyclip --update`
  - UI integration: Help menu "Check for Updates" option
  - Feature guide documentation
  - Supports Linux (.deb, .AppImage), Windows (.exe, .msi), macOS (.dmg)

## [2.1.2] - 2026-03-24

### Added
- **Theme Support**: Light, Dark, and System theme modes with centered popup selection
  - Toggle between Light, Dark, and System (follows OS) themes
  - Theme selection popup centered on window for better UX

### Fixed
- **HTML Preview**: Auto-detect HTML content and display as code block
  - Added fast HTML detection in clipboard monitor
  - Any content starting with `<` followed by a letter is detected as HTML
  - HTML content displays as code block in preview pane using `showCode()` function
- **Unused Function Warning**: Exported `clearRegexCache` to `ClearRegexCache` for test usage
- **Race Condition**: Fixed thread-safety issue in `searchWithRegex` by using single lock with defer
- **Unused Theme Select**: Removed unused `onThemeSelect` method and incomplete dropdown code
- **Theme Popup Position**: Centered theme selection popup menu on the window

### Performance
- **Object Pool Integration**: Added sync.Pool for Item reuse to reduce GC pressure
- **Regex Cache**: Compiled regex patterns cached for faster repeated searches
- **Fuzzy Search Optimization**: Optimized subsequence matching with reduced allocations

## [2.1.1] - 2026-03-24

### Added
- **Bulk Operations**: Multi-select clipboard items with checkboxes
  - Toggle selection mode via toolbar button
  - Bulk delete (skips pinned items)
  - Bulk pin/unpin functionality
  - Select all/none options

- **Keyboard Navigation**: Enhanced keyboard shortcuts
  - Arrow keys for navigation
  - Enter to copy, Delete to delete
  - Space to toggle selection or copy
  - Escape to exit selection mode
  - Home/End to jump to top/bottom
  - F1 for quick panel

- **Smart Categories & Tags**: Automatic content categorization
  - Auto-detection based on content patterns:
    - URLs → "Links"
    - Emails → "Contacts"
    - Phone numbers → "Contacts"
    - Code snippets → "Code"
    - File paths → "Files"
    - JSON → "Data"
    - Images → "Images"
    - HTML → "Web"
  - Manual tag support (AddTag, RemoveTag, HasTag)

- **Quick Panel**: Global hotkey quick access popup (Ctrl+Shift+V)

- **Snippets/Templates**: Text templates with variables
  - Support for {{date}}, {{time}}, {{datetime}}, {{year}}, {{month}}, {{day}}, {{clipboard}}

- **Pattern Exclusion**: Regex, app, and size-based content filtering

- **Hash Maps**: O(1) duplicate detection and item lookup

- **Encrypted Backup**: Password-protected backup with AES-256-GCM

- **Rich Text/HTML**: Capture and preserve HTML clipboard content

- **File History**: Track files copied from file manager

- **Enhanced Search**: Regex, case-sensitive, and fuzzy matching

- **Sensitive Data Detection**: Auto-detect credit cards, SSN, API keys

- **Structured Logging**: slog-based logging with file rotation

- **Graceful Shutdown**: Context-based shutdown with hooks

- **System Tray**: Recent items submenu, Clear History action

- **Preview Enhancements**: JSON pretty-printing, file info display

### Changed
- Improved clipboard monitoring performance
- Enhanced search functionality
- Better item deduplication

### Fixed
- Fixed unused write warnings in test files (improved test coverage)
- Added missing field assertions in TestItemIDField, TestItemJSONFields, and TestItemCopyCount
- Resolved import metadata issue with internal/ui package (gopls cache)
- Updated .gitignore with proper build artifact patterns
- Synced dependencies in go.sum

### Performance
- **Object Pool Integration**: Added sync.Pool for Item reuse to reduce GC pressure
- **Regex Cache**: Compiled regex patterns cached for faster repeated searches
- **Fuzzy Search Optimization**: Optimized subsequence matching with reduced allocations

---

## [Previous Versions]

See the [release tags](https://github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/tags) for version history.
