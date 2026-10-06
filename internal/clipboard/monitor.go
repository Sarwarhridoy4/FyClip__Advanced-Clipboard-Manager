// File: internal/clipboard/monitor.go
package clipboard

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultMonitorInterval = 300 * time.Millisecond
const waylandHelperInterval = 750 * time.Millisecond

// Monitor watches the clipboard for changes
type Monitor struct {
	manager  *Manager
	native   *NativeClipboard
	interval time.Duration

	mu            sync.RWMutex
	lastTextHash  string
	lastImageHash string
	lastHTMLHash  string
	lastFileHash  string

	programmaticCopy bool
	programmaticHash string
	pausedUntil      time.Time

	stopChan     chan struct{}
	running      bool
	restartCount int
}

// NewMonitor creates a new clipboard monitor
func NewMonitor(manager *Manager, native *NativeClipboard) *Monitor {
	interval := defaultMonitorInterval

	// When running on GNOME Wayland without native clipboard integration (fallback to `wl-paste`),
	// aggressive polling can cause visible stutter/flicker. Throttle slightly in that case.
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" && native != nil && native.Backend() == "wl-clipboard" {
		interval = waylandHelperInterval
	}

	return &Monitor{
		manager:  manager,
		native:   native,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins monitoring the clipboard
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.monitorLoop()
}

// Stop stops monitoring the clipboard
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false
	select {
	case <-m.stopChan:
	default:
		close(m.stopChan)
	}
}

// IsRunning returns whether the monitor is running
func (m *Monitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// PauseFor pauses clipboard capture for the provided duration.
func (m *Monitor) PauseFor(d time.Duration) {
	if d <= 0 {
		return
	}
	m.mu.Lock()
	m.pausedUntil = time.Now().Add(d)
	m.mu.Unlock()
}

// Resume clears any active monitor pause.
func (m *Monitor) Resume() {
	m.mu.Lock()
	m.pausedUntil = time.Time{}
	m.mu.Unlock()
}

// IsPaused reports whether monitoring is currently paused.
func (m *Monitor) IsPaused() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.pausedUntil.IsZero() && time.Now().Before(m.pausedUntil)
}

// SetProgrammaticHash marks the next clipboard change using its content hash.
func (m *Monitor) SetProgrammaticHash(hash string) {
	if hash == "" {
		return
	}

	m.mu.Lock()
	m.programmaticCopy = true
	m.programmaticHash = hash
	m.mu.Unlock()

	time.AfterFunc(200*time.Millisecond, func() {
		m.mu.Lock()
		m.programmaticCopy = false
		m.programmaticHash = ""
		m.mu.Unlock()
	})
}

// SetProgrammaticCopy marks the next clipboard change as programmatic
func (m *Monitor) SetProgrammaticCopy(data []byte) {
	defer wipeSensitiveData(data)

	hash := sha256.Sum256(data)
	m.SetProgrammaticHash(hex.EncodeToString(hash[:]))
}

// monitorLoop continuously checks the clipboard
func (m *Monitor) monitorLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in monitor loop: %v", r)
			m.mu.Lock()
			restartCount := m.restartCount
			m.restartCount++
			running := m.running
			m.mu.Unlock()

			// Limit restart attempts to prevent infinite loop
			if running && restartCount < 3 {
				time.Sleep(1 * time.Second)
				go m.monitorLoop()
			} else {
				log.Printf("Monitor loop stopped after %d panic restarts", restartCount)
			}
		}
	}()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkClipboard()

		case <-m.stopChan:
			return
		}
	}
}

// checkClipboard checks for clipboard changes
func (m *Monitor) checkClipboard() {
	m.mu.RLock()
	isProgrammatic := m.programmaticCopy
	programmaticHash := m.programmaticHash
	pausedUntil := m.pausedUntil
	m.mu.RUnlock()

	if isProgrammatic {
		return
	}

	if !pausedUntil.IsZero() {
		if time.Now().Before(pausedUntil) {
			return
		}
		m.mu.Lock()
		m.pausedUntil = time.Time{}
		m.mu.Unlock()
	}

	// Try reading text first
	if textData := m.native.ReadText(); len(textData) > 0 {
		if len(textData) > MaxContentSize {
			return
		}
		m.handleText(textData, programmaticHash)
		return
	}

	// Try reading image
	if imageData, imageType := m.native.ReadImage(); len(imageData) > 0 {
		if len(imageData) > MaxImageSize {
			return
		}
		m.handleImage(imageData, imageType, programmaticHash)
		return
	}

	// Try reading HTML
	if htmlData := m.native.ReadHTML(); len(htmlData) > 0 {
		if len(htmlData) > MaxContentSize {
			return
		}
		m.handleHTML(htmlData, programmaticHash)
		return
	}

	// Try reading file paths
	if filePaths := m.native.ReadFilePaths(); len(filePaths) > 0 {
		m.handleFiles(filePaths, programmaticHash)
	}
}

// handleText processes text clipboard content
func (m *Monitor) handleText(data []byte, programmaticHash string) {
	content := SafeString(data)
	if len(content) == 0 {
		return
	}

	// Check if content looks like HTML - auto-detect HTML
	if looksLikeHTML(content) {
		m.handleHTML(data, programmaticHash)
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	m.mu.Lock()
	lastHash := m.lastTextHash
	m.mu.Unlock()

	if hashStr == lastHash || hashStr == programmaticHash {
		return
	}

	m.mu.Lock()
	m.lastTextHash = hashStr
	m.lastImageHash = "" // Clear image hash when text is copied
	m.mu.Unlock()

	item := Item{
		Type:    TypeText,
		Content: content,
		Hash:    hashStr,
	}

	result := m.manager.AddItem(item)
	if result.Added {
		if result.MovedDuplicate {
			m.manager.notifyInfo("Duplicate item moved to latest")
		}
		m.manager.saveHistory()
		m.manager.updateFiltered()
		m.manager.triggerUpdate()
	}
}

// looksLikeHTML checks if the content appears to be HTML
// Uses a fast, lightweight check for common HTML tag patterns
func looksLikeHTML(content string) bool {
	content = strings.TrimSpace(content)
	if len(content) < 3 {
		return false
	}
	// Must start with <
	if content[0] != '<' {
		return false
	}
	// Check if it starts with common HTML patterns
	if strings.HasPrefix(content, "<html") ||
		strings.HasPrefix(content, "<!DOCTYPE") ||
		strings.HasPrefix(content, "<!--") {
		return true
	}
	// Check for single tags like <div>, <span>, <p>, <a>, <img>, etc.
	// Just check if first char is < and second is a letter
	if content[1] >= 'a' && content[1] <= 'z' {
		return true
	}
	return false
}

// handleImage processes image clipboard content
func (m *Monitor) handleImage(data []byte, imageType, programmaticHash string) {
	if len(data) == 0 {
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	m.mu.Lock()
	lastHash := m.lastImageHash
	m.mu.Unlock()

	if hashStr == lastHash || hashStr == programmaticHash {
		return
	}

	m.mu.Lock()
	m.lastImageHash = hashStr
	m.lastTextHash = "" // Clear text hash when image is copied
	m.lastHTMLHash = "" // Clear HTML hash when image is copied
	m.lastFileHash = "" // Clear file hash when image is copied
	m.mu.Unlock()

	// Encode image data to base64
	base64Data := base64.StdEncoding.EncodeToString(data)

	// Generate thumbnail for efficient list display
	thumbnail := GenerateThumbnail(base64Data)

	item := Item{
		Type:      TypeImage,
		Content:   fmt.Sprintf("Image (%d bytes)", len(data)),
		ImageData: base64Data,
		ImageType: imageType,
		Thumbnail: thumbnail,
		Hash:      hashStr,
	}

	result := m.manager.AddItem(item)
	if result.Added {
		if result.MovedDuplicate {
			m.manager.notifyInfo("Duplicate item moved to latest")
		}
		m.manager.saveHistory()
		m.manager.updateFiltered()
		m.manager.triggerUpdate()
	}
}

// handleHTML processes HTML clipboard content
func (m *Monitor) handleHTML(data []byte, programmaticHash string) {
	if len(data) == 0 {
		return
	}

	htmlContent := SafeString(data)
	if len(htmlContent) == 0 {
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	m.mu.Lock()
	lastHash := m.lastHTMLHash
	m.mu.Unlock()

	if hashStr == lastHash || hashStr == programmaticHash {
		return
	}

	m.mu.Lock()
	m.lastHTMLHash = hashStr
	m.lastTextHash = ""  // Clear text hash when HTML is copied
	m.lastImageHash = "" // Clear image hash when HTML is copied
	m.lastFileHash = ""  // Clear file hash when HTML is copied
	m.mu.Unlock()

	// Extract plain text from HTML for searchability
	plainText := stripHTMLForSearch(htmlContent)

	item := Item{
		Type:        TypeHTML,
		Content:     plainText,
		HTMLContent: htmlContent,
		Hash:        hashStr,
	}

	result := m.manager.AddItem(item)
	if result.Added {
		if result.MovedDuplicate {
			m.manager.notifyInfo("Duplicate item moved to latest")
		}
		m.manager.saveHistory()
		m.manager.updateFiltered()
		m.manager.triggerUpdate()
	}
}

// handleFiles processes file path clipboard content
func (m *Monitor) handleFiles(filePaths []string, programmaticHash string) {
	if len(filePaths) == 0 {
		return
	}

	// Create a combined hash for all file paths
	combinedPaths := strings.Join(filePaths, "|")
	hash := sha256.Sum256([]byte(combinedPaths))
	hashStr := hex.EncodeToString(hash[:])

	m.mu.Lock()
	lastHash := m.lastFileHash
	m.mu.Unlock()

	if hashStr == lastHash || hashStr == programmaticHash {
		return
	}

	m.mu.Lock()
	m.lastFileHash = hashStr
	m.lastTextHash = ""  // Clear text hash when files are copied
	m.lastImageHash = "" // Clear image hash when files are copied
	m.lastHTMLHash = ""  // Clear HTML hash when files are copied
	m.mu.Unlock()

	// Get info for the first file
	fileInfo, err := getFileInfo(filePaths[0])
	if err != nil {
		log.Printf("Failed to get file info: %v", err)
		return
	}

	// Create content description from all files
	content := fmt.Sprintf("%d file(s)", len(filePaths))
	if len(filePaths) == 1 {
		content = fileInfo.Name
	}

	item := Item{
		Type:     TypeFile,
		Content:  content,
		FileInfo: fileInfo,
		Hash:     hashStr,
	}

	result := m.manager.AddItem(item)
	if result.Added {
		if result.MovedDuplicate {
			m.manager.notifyInfo("Duplicate item moved to latest")
		}
		m.manager.saveHistory()
		m.manager.updateFiltered()
		m.manager.triggerUpdate()
	}
}

// stripHTMLForSearch extracts plain text from HTML for search indexing
func stripHTMLForSearch(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
			result.WriteRune(' ') // Replace tags with space
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}
