// File: internal/clipboard/manager.go
package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MaxHistoryItems = 1000
	UpdateDebounce  = 200 * time.Millisecond
	SaveDebounce    = 250 * time.Millisecond

	// Shutdown timeout for graceful shutdown
	ShutdownTimeout = 10 * time.Second

	// Security limits for clipboard content
	MaxClipboardTextSize  = 10 * 1024 * 1024 // 10MB max text content
	MaxClipboardImageSize = 50 * 1024 * 1024 // 50MB max image content
	MaxClipboardPathSize  = 4096             // 4KB max file path length
)

// AddItemResult describes the result of adding a clipboard item.
type AddItemResult struct {
	Added          bool
	MovedDuplicate bool
}

// ShutdownHook is a function that runs during shutdown
type ShutdownHook func(ctx context.Context) error

// Manager handles clipboard history and operations
type Manager struct {
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	history    []Item
	filtered   []*Item // Changed to pointers to reduce memory copies
	storage    *Storage
	snippets   *SnippetManager
	exclusions *ExclusionManager
	native     *NativeClipboard
	monitor    *Monitor
	backup     *BackupManager

	// Shutdown hooks
	shutdownHooks []ShutdownHook

	// Index maps for O(1) lookups
	hashIndexMap map[string]int // hash+type -> index in history
	idIndexMap   map[string]int // id -> index in history

	// Differential tracking for optimized index updates
	indexNeedsFullRebuild bool         // Flag to track if full rebuild is needed
	modifiedIndices       map[int]bool // Track which indices have been modified

	selectedIndex   int
	searchQuery     string
	searchOptions   *SearchOptions
	showPinnedOnly  bool
	sortByDate      bool
	dateFrom        time.Time
	dateTo          time.Time
	filterByDate    bool
	lastCopied      time.Time
	maxHistoryItems int

	updateChan   chan struct{}
	saveChan     chan struct{}
	shutdownChan chan struct{}
	running      bool

	// Memory tracking
	lastMemoryCheck     time.Time
	memoryPressureLevel int            // 0=none, 1=moderate, 2=high, 3=critical
	memorySamples       []memorySample // Track memory usage over time

	// Callbacks
	onUpdate func()
	onError  func(error)
	onInfo   func(string)
}

type memorySample struct {
	timestamp time.Time
	alloc     uint64
	gcCycles  uint32
}

// Config holds manager configuration
type Config struct {
	StoragePath string
	OnUpdate    func()
	OnError     func(error)
	OnInfo      func(string)
}

// NewManager creates a new clipboard manager
func NewManager(cfg Config) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	storage, err := NewStorage(cfg.StoragePath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	native, err := NewNativeClipboard()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize clipboard: %w", err)
	}

	m := &Manager{
		ctx:                   ctx,
		cancel:                cancel,
		storage:               storage,
		snippets:              NewSnippetManager(storage),
		exclusions:            NewExclusionManager(),
		native:                native,
		backup:                NewBackupManager(storage),
		hashIndexMap:          make(map[string]int),
		idIndexMap:            make(map[string]int),
		indexNeedsFullRebuild: false,
		modifiedIndices:       make(map[int]bool),
		selectedIndex:         -1,
		updateChan:            make(chan struct{}, 10), // Reduced from 100 - updates are debounced
		saveChan:              make(chan struct{}, 1),
		shutdownChan:          make(chan struct{}),
		running:               true,
		maxHistoryItems:       MaxHistoryItems,
		searchOptions:         DefaultSearchOptions(),
		memorySamples:         make([]memorySample, 0, 10), // Keep last 10 samples
		onUpdate:              cfg.OnUpdate,
		onError:               cfg.OnError,
		onInfo:                cfg.OnInfo,
	}

	// Load snippets
	if err := m.snippets.LoadSnippets(); err != nil {
		// Log but don't fail - snippets are optional
		if cfg.OnError != nil {
			cfg.OnError(fmt.Errorf("failed to load snippets: %w", err))
		}
	}

	// Load history
	if err := m.loadHistory(); err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	// Start update dispatcher
	go m.updateDispatcher()
	go m.saveDispatcher()

	// Create and start monitor
	m.monitor = NewMonitor(m, native)
	m.monitor.Start()

	return m, nil
}

// loadHistory loads history from storage
func (m *Manager) loadHistory() error {
	items, err := m.storage.Load()
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.history = items
	m.buildIndexMaps()
	m.mu.Unlock()

	m.updateFiltered()
	return nil
}

// buildIndexMaps rebuilds the hash and ID index maps from history
func (m *Manager) buildIndexMaps() {
	m.hashIndexMap = make(map[string]int, len(m.history))
	m.idIndexMap = make(map[string]int, len(m.history))
	for i, item := range m.history {
		// Create composite key with type for hash to handle same content different types
		hashKey := item.Hash + ":" + strconv.Itoa(int(item.Type))
		m.hashIndexMap[hashKey] = i
		m.idIndexMap[item.ID] = i
	}
	// Reset differential tracking after full rebuild
	m.indexNeedsFullRebuild = false
	m.modifiedIndices = make(map[int]bool)
}

// rebuildModifiedIndices performs differential index rebuild for only modified indices
func (m *Manager) rebuildModifiedIndices() {
	if m.indexNeedsFullRebuild {
		m.buildIndexMaps()
		return
	}

	if len(m.modifiedIndices) == 0 {
		return // No changes to rebuild
	}

	// Rebuild only the modified indices
	for index := range m.modifiedIndices {
		if index >= len(m.history) {
			continue // Index no longer valid
		}
		item := m.history[index]
		hashKey := item.Hash + ":" + strconv.Itoa(int(item.Type))
		m.hashIndexMap[hashKey] = index
		m.idIndexMap[item.ID] = index
	}

	// Clear the modified indices tracking
	m.modifiedIndices = make(map[int]bool)
}

// ensureIndicesUpToDate ensures that index maps are up to date before operations that depend on them
func (m *Manager) ensureIndicesUpToDate() {
	m.rebuildModifiedIndices()
}

// queueSaveHistory schedules a debounced save of history.
func (m *Manager) queueSaveHistory() {
	if !m.running {
		return
	}

	select {
	case m.saveChan <- struct{}{}:
	default:
		// Save already queued.
	}
}

// saveHistory persists history to storage (internal)
func (m *Manager) saveHistory() {
	m.queueSaveHistory()
}

// saveHistoryNow writes a snapshot to storage immediately.
func (m *Manager) saveHistoryNow() {
	m.mu.RLock()
	items := make([]Item, len(m.history))
	copy(items, m.history)
	m.mu.RUnlock()

	if err := m.storage.Save(items); err != nil {
		if m.onError != nil {
			m.onError(fmt.Errorf("failed to save history: %w", err))
		}
	}
}

// SaveHistory is a public method to force save history
func (m *Manager) SaveHistory() {
	m.saveHistory()
}

// ReloadHistory reloads history from storage
func (m *Manager) ReloadHistory() error {
	return m.loadHistory()
}

// AddItem adds a new item to history
func (m *Manager) AddItem(item Item) AddItemResult {
	if ValidateItem(&item) != nil {
		return AddItemResult{}
	}
	if item.Content == "" && item.ImageData == "" {
		return AddItemResult{}
	}

	// Generate hash if not provided
	if item.Hash == "" {
		if item.Type == TypeText {
			hash := sha256.Sum256([]byte(item.Content))
			item.Hash = hex.EncodeToString(hash[:])
		} else {
			hash := sha256.Sum256([]byte(item.ImageData))
			item.Hash = hex.EncodeToString(hash[:])
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure indices are up to date before lookups
	m.ensureIndicesUpToDate()

	// Create composite key with type for hash
	hashKey := item.Hash + ":" + strconv.Itoa(int(item.Type))

	// Check for duplicate at end (last item check)
	if len(m.history) > 0 {
		last := m.history[len(m.history)-1]
		lastHashKey := last.Hash + ":" + strconv.Itoa(int(last.Type))
		if lastHashKey == hashKey {
			return AddItemResult{}
		}
	}

	// O(1) lookup for existing duplicate using hashIndexMap
	movedDuplicate := false
	if idx, exists := m.hashIndexMap[hashKey]; exists {
		// Remove the existing item
		m.removeAtIndex(idx)
		movedDuplicate = true
	}

	// Add new item
	newItem := GetFromPool()
	*newItem = item
	newItem.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	if newItem.Timestamp.IsZero() {
		newItem.Timestamp = time.Now()
	}

	// Auto-detect category based on content
	newItem.AutoDetectCategory()

	// Add to history
	newIndex := len(m.history)
	m.history = append(m.history, *newItem)

	// Mark differential index update needed
	m.modifiedIndices[newIndex] = true

	// Trim unpinned items
	m.trimHistory()
	m.lastCopied = time.Now()

	return AddItemResult{
		Added:          true,
		MovedDuplicate: movedDuplicate,
	}
}

// removeAtIndex removes an item at the given index and updates index maps
// Caller must hold the lock
func (m *Manager) removeAtIndex(idx int) {
	if idx < 0 || idx >= len(m.history) {
		return
	}

	item := m.history[idx]

	// Remove from index maps
	hashKey := item.Hash + ":" + strconv.Itoa(int(item.Type))
	delete(m.hashIndexMap, hashKey)
	delete(m.idIndexMap, item.ID)

	// Remove from history
	m.history = append(m.history[:idx], m.history[idx+1:]...)

	// Return removed item to pool
	ReturnToPool(&item)

	// Mark all indices from the removal point onward as modified
	// This is necessary because all subsequent indices shift by 1
	for i := idx; i < len(m.history); i++ {
		m.modifiedIndices[i] = true
	}
}

// trimHistory keeps only the most recently used MaxHistoryItems unpinned items (LRU)
func (m *Manager) trimHistory() {
	// First, ensure all items have LastAccessed set (for backward compatibility)
	now := time.Now()
	for i := range m.history {
		if m.history[i].LastAccessed.IsZero() {
			// For existing items without LastAccessed, use creation timestamp as approximation
			if !m.history[i].Timestamp.IsZero() {
				m.history[i].LastAccessed = m.history[i].Timestamp
			} else {
				m.history[i].LastAccessed = now
			}
		}
	}

	// Count unpinned items
	unpinnedCount := 0
	for _, item := range m.history {
		if !item.Pinned {
			unpinnedCount++
		}
	}

	// If we're within limits, no trimming needed
	if unpinnedCount <= m.maxHistoryItems {
		return
	}

	// Collect unpinned items with their indices for sorting by LRU
	type itemWithIndex struct {
		index int
		item  *Item
	}

	var unpinnedItems []itemWithIndex
	for i := range m.history {
		if !m.history[i].Pinned {
			unpinnedItems = append(unpinnedItems, itemWithIndex{index: i, item: &m.history[i]})
		}
	}

	// Sort by LastAccessed (oldest first = least recently used)
	for i := 0; i < len(unpinnedItems)-1; i++ {
		for j := i + 1; j < len(unpinnedItems); j++ {
			if unpinnedItems[i].item.LastAccessed.After(unpinnedItems[j].item.LastAccessed) {
				unpinnedItems[i], unpinnedItems[j] = unpinnedItems[j], unpinnedItems[i]
			}
		}
	}

	// Remove excess items (least recently used first)
	itemsToRemove := unpinnedCount - m.maxHistoryItems
	for i := 0; i < itemsToRemove && i < len(unpinnedItems); i++ {
		// Find the current index of this item (it may have shifted due to previous removals)
		currentIndex := -1
		for j := range m.history {
			if m.history[j].ID == unpinnedItems[i].item.ID {
				currentIndex = j
				break
			}
		}
		if currentIndex >= 0 {
			m.removeAtIndex(currentIndex)
		}
	}
}

// updateFiltered updates the filtered list based on search
func (m *Manager) updateFiltered() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cap(m.filtered) < len(m.history) {
		m.filtered = make([]*Item, 0, len(m.history))
	} else {
		m.filtered = m.filtered[:0]
	}

	query := m.searchQuery
	searchOpts := m.searchOptions
	sortByDate := m.sortByDate
	filterByDate := m.filterByDate
	dateFrom := m.dateFrom
	dateTo := m.dateTo

	match := func(item *Item) bool {
		if query != "" && !SearchItem(item, query, searchOpts) {
			return false
		}
		if filterByDate {
			itemDate := item.Timestamp
			if !dateFrom.IsZero() && itemDate.Before(startOfDay(dateFrom)) {
				return false
			}
			if !dateTo.IsZero() && itemDate.After(endOfDay(dateTo)) {
				return false
			}
		}
		return true
	}

	var unpinned []*Item
	for i := range m.history {
		item := &m.history[i]
		if item.Pinned {
			continue
		}
		if m.showPinnedOnly {
			continue
		}
		if match(item) {
			unpinned = append(unpinned, item)
		}
	}

	if sortByDate {
		for i := 0; i < len(unpinned)-1; i++ {
			for j := i + 1; j < len(unpinned); j++ {
				if unpinned[i].Timestamp.Before(unpinned[j].Timestamp) {
					unpinned[i], unpinned[j] = unpinned[j], unpinned[i]
				}
			}
		}
	}

	m.filtered = m.filtered[:0]
	for i := range m.history {
		item := &m.history[i]
		if !item.Pinned {
			continue
		}
		if match(item) {
			m.filtered = append(m.filtered, item)
		}
	}
	for _, item := range unpinned {
		m.filtered = append(m.filtered, item)
	}

	m.selectedIndex = -1
}

func startOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return startOfDay(t).Add(24 * time.Hour).Add(-time.Nanosecond)
}

// CheckMemoryPressure checks memory usage and triggers cleanup if needed
func (m *Manager) CheckMemoryPressure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	// Check more frequently under pressure, less frequently when stable
	checkInterval := 30 * time.Second
	if m.memoryPressureLevel > 0 {
		checkInterval = 10 * time.Second
	}

	if now.Sub(m.lastMemoryCheck) < checkInterval {
		return
	}
	m.lastMemoryCheck = now

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Add current sample to history
	sample := memorySample{
		timestamp: now,
		alloc:     memStats.Alloc,
		gcCycles:  memStats.NumGC,
	}
	m.memorySamples = append(m.memorySamples, sample)

	// Keep only last 10 samples
	if len(m.memorySamples) > 10 {
		m.memorySamples = m.memorySamples[1:]
	}

	// Determine memory pressure level based on allocation and trends
	newPressureLevel := m.calculateMemoryPressure(memStats)

	// Act on pressure changes
	if newPressureLevel != m.memoryPressureLevel {
		m.handleMemoryPressureChange(newPressureLevel, memStats)
		m.memoryPressureLevel = newPressureLevel
	}
}

// calculateMemoryPressure determines the current memory pressure level
func (m *Manager) calculateMemoryPressure(memStats runtime.MemStats) int {
	allocMB := float64(memStats.Alloc) / (1024 * 1024)

	// Base pressure on allocation size
	var level int
	switch {
	case allocMB > 200: // Critical
		level = 3
	case allocMB > 150: // High
		level = 2
	case allocMB > 100: // Moderate
		level = 1
	default: // Normal
		level = 0
	}

	// Increase pressure if GC is running frequently
	if memStats.NumGC > 50 && len(m.memorySamples) > 1 {
		// Check if allocations are consistently high
		recentHigh := 0
		for i := len(m.memorySamples) - 1; i >= 0 && i >= len(m.memorySamples)-3; i-- {
			if float64(m.memorySamples[i].alloc)/(1024*1024) > 120 {
				recentHigh++
			}
		}
		if recentHigh >= 2 && level < 2 {
			level++
		}
	}

	return level
}

// handleMemoryPressureChange responds to memory pressure level changes
func (m *Manager) handleMemoryPressureChange(newLevel int, memStats runtime.MemStats) {
	allocMB := float64(memStats.Alloc) / (1024 * 1024)

	switch newLevel {
	case 0: // Normal - no action needed
		if m.onInfo != nil {
			m.onInfo(fmt.Sprintf("Memory usage normal: %.1f MB", allocMB))
		}

	case 1: // Moderate - light cleanup
		oldMax := m.maxHistoryItems
		m.maxHistoryItems = int(float64(m.maxHistoryItems) * 0.9)
		if m.maxHistoryItems < 200 {
			m.maxHistoryItems = 200
		}
		if m.maxHistoryItems < oldMax {
			m.trimHistory()
			if m.onInfo != nil {
				m.onInfo(fmt.Sprintf("Light memory cleanup: reduced history from %d to %d (%.1f MB used)",
					oldMax, m.maxHistoryItems, allocMB))
			}
		}

	case 2: // High - moderate cleanup
		oldMax := m.maxHistoryItems
		m.maxHistoryItems = int(float64(m.maxHistoryItems) * 0.75)
		if m.maxHistoryItems < 150 {
			m.maxHistoryItems = 150
		}
		if m.maxHistoryItems < oldMax {
			m.trimHistory()
			runtime.GC() // Force garbage collection
			if m.onInfo != nil {
				m.onInfo(fmt.Sprintf("Memory pressure: reduced history from %d to %d (%.1f MB used)",
					oldMax, m.maxHistoryItems, allocMB))
			}
		}

	case 3: // Critical - aggressive cleanup
		oldMax := m.maxHistoryItems
		m.maxHistoryItems = int(float64(m.maxHistoryItems) * 0.5)
		if m.maxHistoryItems < 100 {
			m.maxHistoryItems = 100
		}
		if m.maxHistoryItems < oldMax {
			m.trimHistory()
			runtime.GC() // Force garbage collection
			if m.onInfo != nil {
				m.onInfo(fmt.Sprintf("Critical memory pressure: reduced history from %d to %d (%.1f MB used)",
					oldMax, m.maxHistoryItems, allocMB))
			}
		}
	}
}

// SetPinnedOnly controls whether only pinned items are shown.
func (m *Manager) SetPinnedOnly(enabled bool) {
	m.mu.Lock()
	m.showPinnedOnly = enabled
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
}

// TogglePinnedOnly toggles pinned-only filtering and returns the new state.
func (m *Manager) TogglePinnedOnly() bool {
	m.mu.Lock()
	m.showPinnedOnly = !m.showPinnedOnly
	enabled := m.showPinnedOnly
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
	return enabled
}

// IsPinnedOnly returns whether pinned-only filter is enabled.
func (m *Manager) IsPinnedOnly() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.showPinnedOnly
}

// ToggleSortByDate toggles calendar date sorting and returns the new state.
func (m *Manager) ToggleSortByDate() bool {
	m.mu.Lock()
	m.sortByDate = !m.sortByDate
	enabled := m.sortByDate
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
	return enabled
}

// SetSortByDate sets calendar date sorting.
func (m *Manager) SetSortByDate(enabled bool) {
	m.mu.Lock()
	m.sortByDate = enabled
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
}

// IsSortByDate returns whether calendar date sorting is enabled.
func (m *Manager) IsSortByDate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sortByDate
}

// SetDateFilter sets a date range filter.
// Pass zero time values to clear the filter.
func (m *Manager) SetDateFilter(from, to time.Time) {
	m.mu.Lock()
	if from.IsZero() && to.IsZero() {
		m.filterByDate = false
		m.dateFrom = time.Time{}
		m.dateTo = time.Time{}
	} else {
		m.filterByDate = true
		m.dateFrom = from
		m.dateTo = to
	}
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
}

// GetDateFilter returns the current date filter range and whether it is active.
func (m *Manager) GetDateFilter() (from, to time.Time, active bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dateFrom, m.dateTo, m.filterByDate
}

// ClearDateFilter removes any active date filter.
func (m *Manager) ClearDateFilter() {
	m.SetDateFilter(time.Time{}, time.Time{})
}

// IsDateFilterActive returns whether a date filter is currently applied.
func (m *Manager) IsDateFilterActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filterByDate
}

// SetMaxHistory updates the max number of unpinned items retained.
func (m *Manager) SetMaxHistory(limit int) bool {
	if limit <= 0 {
		return false
	}

	m.mu.Lock()
	m.maxHistoryItems = limit
	m.trimHistory()
	m.mu.Unlock()

	m.updateFiltered()
	m.saveHistory()
	m.triggerUpdate()
	return true
}

// GetMaxHistory returns the configured max unpinned history count.
func (m *Manager) GetMaxHistory() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxHistoryItems
}

// PauseMonitoringFor pauses clipboard capture for the specified duration.
func (m *Manager) PauseMonitoringFor(d time.Duration) {
	if m.monitor == nil {
		return
	}
	m.monitor.PauseFor(d)
}

// ResumeMonitoring resumes clipboard capture immediately.
func (m *Manager) ResumeMonitoring() {
	if m.monitor == nil {
		return
	}
	m.monitor.Resume()
}

// IsMonitoringPaused reports whether capture is paused.
func (m *Manager) IsMonitoringPaused() bool {
	if m.monitor == nil {
		return false
	}
	return m.monitor.IsPaused()
}

// GetBackupManager returns the backup manager
func (m *Manager) GetBackupManager() *BackupManager {
	return m.backup
}

// SetSearch updates the search query
func (m *Manager) SetSearch(query string) {
	m.mu.Lock()
	m.searchQuery = query
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
}

// SetSearchOptions updates the search options
func (m *Manager) SetSearchOptions(opts *SearchOptions) {
	m.mu.Lock()
	m.searchOptions = opts
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
}

// GetSearchOptions returns the current search options
func (m *Manager) GetSearchOptions() *SearchOptions {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.searchOptions
}

// ToggleRegexSearch toggles regex search mode
func (m *Manager) ToggleRegexSearch() bool {
	m.mu.Lock()
	m.searchOptions.RegexEnabled = !m.searchOptions.RegexEnabled
	enabled := m.searchOptions.RegexEnabled
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
	return enabled
}

// ToggleCaseSensitive toggles case-sensitive search
func (m *Manager) ToggleCaseSensitive() bool {
	m.mu.Lock()
	m.searchOptions.CaseSensitive = !m.searchOptions.CaseSensitive
	enabled := m.searchOptions.CaseSensitive
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
	return enabled
}

// ToggleFuzzySearch toggles fuzzy search mode
func (m *Manager) ToggleFuzzySearch() bool {
	m.mu.Lock()
	m.searchOptions.FuzzyEnabled = !m.searchOptions.FuzzyEnabled
	enabled := m.searchOptions.FuzzyEnabled
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
	return enabled
}

// GetFiltered returns the filtered items (thread-safe)
func (m *Manager) GetFiltered() []Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]Item, len(m.filtered))
	for i, p := range m.filtered {
		items[i] = *p
	}
	return items
}

// GetFilteredCount returns the count of filtered items
func (m *Manager) GetFilteredCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.filtered)
}

// GetItem returns an item by index (thread-safe)
func (m *Manager) GetItem(index int) (Item, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if index < 0 || index >= len(m.filtered) {
		return Item{}, false
	}

	// Update last accessed time for LRU tracking
	item := m.filtered[index]
	if idx, exists := m.idIndexMap[item.ID]; exists {
		m.history[idx].UpdateLastAccessed()
	}

	return *item, true
}

// GetSelected returns the currently selected item
func (m *Manager) GetSelected() (Item, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.selectedIndex < 0 || m.selectedIndex >= len(m.filtered) {
		return Item{}, false
	}

	// Update last accessed time for LRU tracking
	item := m.filtered[m.selectedIndex]
	if idx, exists := m.idIndexMap[item.ID]; exists {
		m.history[idx].UpdateLastAccessed()
	}

	return *item, true
}

// GetSelectedIndex returns the selected index
func (m *Manager) GetSelectedIndex() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selectedIndex
}

// SetSelected sets the selected index
func (m *Manager) SetSelected(index int) {
	m.mu.Lock()
	m.selectedIndex = index
	m.mu.Unlock()
}

// TogglePin toggles the pin status of an item
func (m *Manager) TogglePin(index int) bool {
	m.mu.Lock()

	if index < 0 || index >= len(m.filtered) {
		m.mu.Unlock()
		return false
	}

	// Ensure indices are up to date before lookups
	m.ensureIndicesUpToDate()

	targetID := m.filtered[index].ID

	// O(1) lookup using idIndexMap
	idx, exists := m.idIndexMap[targetID]
	if !exists {
		m.mu.Unlock()
		return false
	}

	m.history[idx].Pinned = !m.history[idx].Pinned

	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()

	return true
}

// FindIndexByID finds the current index of an item by its ID in the filtered list
func (m *Manager) FindIndexByID(id string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i, item := range m.filtered {
		if item.ID == id {
			return i
		}
	}
	return -1
}

// Delete removes an item
func (m *Manager) Delete(index int) error {
	m.mu.Lock()

	if index < 0 || index >= len(m.filtered) {
		m.mu.Unlock()
		return fmt.Errorf("invalid index")
	}

	// Ensure indices are up to date before lookups
	m.ensureIndicesUpToDate()

	targetItem := m.filtered[index]
	if targetItem.Pinned {
		m.mu.Unlock()
		return fmt.Errorf("cannot delete pinned items")
	}

	// O(1) lookup using idIndexMap
	idx, exists := m.idIndexMap[targetItem.ID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("item not found")
	}

	m.removeAtIndex(idx)

	m.selectedIndex = -1
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()

	return nil
}

// ClearUnpinned removes all unpinned items
func (m *Manager) ClearUnpinned() {
	m.mu.Lock()

	var pinned []Item
	for _, item := range m.history {
		if item.Pinned {
			pinned = append(pinned, item)
		}
	}

	m.history = pinned
	m.buildIndexMaps()
	m.selectedIndex = -1
	m.mu.Unlock()

	m.updateFiltered()
	m.triggerUpdate()
}

// BulkDelete removes multiple items by their IDs
// Returns the count of successfully deleted items
func (m *Manager) BulkDelete(ids []string) int {
	m.mu.Lock()
	deleted := 0

	for _, id := range ids {
		// O(1) lookup using idIndexMap
		idx, exists := m.idIndexMap[id]
		if !exists {
			continue
		}

		// Skip pinned items
		if m.history[idx].Pinned {
			continue
		}

		m.removeAtIndex(idx)
		deleted++
	}

	m.selectedIndex = -1
	m.mu.Unlock()

	if deleted > 0 {
		m.updateFiltered()
		m.triggerUpdate()
	}

	return deleted
}

// BulkPin pins multiple items by their IDs
// Returns the count of successfully pinned items
func (m *Manager) BulkPin(ids []string) int {
	m.mu.Lock()
	pinned := 0

	for _, id := range ids {
		// O(1) lookup using idIndexMap
		idx, exists := m.idIndexMap[id]
		if !exists {
			continue
		}

		if !m.history[idx].Pinned {
			m.history[idx].Pinned = true
			pinned++
		}
	}

	m.mu.Unlock()

	if pinned > 0 {
		m.updateFiltered()
		m.triggerUpdate()
	}

	return pinned
}

// BulkUnpin unpins multiple items by their IDs
// Returns the count of successfully unpinned items
func (m *Manager) BulkUnpin(ids []string) int {
	m.mu.Lock()
	unpinned := 0

	for _, id := range ids {
		// O(1) lookup using idIndexMap
		idx, exists := m.idIndexMap[id]
		if !exists {
			continue
		}

		if m.history[idx].Pinned {
			m.history[idx].Pinned = false
			unpinned++
		}
	}

	m.mu.Unlock()

	if unpinned > 0 {
		m.updateFiltered()
		m.triggerUpdate()
	}

	return unpinned
}

// BulkTogglePin toggles pin status for multiple items
// Returns the count of items that were toggled
func (m *Manager) BulkTogglePin(ids []string) int {
	m.mu.Lock()
	toggled := 0

	for _, id := range ids {
		// O(1) lookup using idIndexMap
		idx, exists := m.idIndexMap[id]
		if !exists {
			continue
		}

		m.history[idx].Pinned = !m.history[idx].Pinned
		toggled++
	}

	m.mu.Unlock()

	if toggled > 0 {
		m.updateFiltered()
		m.triggerUpdate()
	}

	return toggled
}

// BulkCopy copies multiple items to clipboard sequentially
// Returns the count of successfully copied items
func (m *Manager) BulkCopy(indices []int) int {
	copied := 0

	for _, index := range indices {
		if err := m.CopyToClipboard(index); err == nil {
			copied++
		}
	}

	return copied
}

// CopyToClipboard copies an item to the system clipboard
func (m *Manager) CopyToClipboard(index int) error {
	item, ok := m.GetItem(index)
	if !ok {
		return fmt.Errorf("invalid index")
	}

	return m.copyItemToClipboard(&item, false)
}

// CopyToClipboardAsHTML copies an item to clipboard as HTML (if available)
func (m *Manager) CopyToClipboardAsHTML(index int) error {
	item, ok := m.GetItem(index)
	if !ok {
		return fmt.Errorf("invalid index")
	}

	return m.copyItemToClipboard(&item, true)
}

// CopyToClipboardAsPlainText copies an item to clipboard as plain text
func (m *Manager) CopyToClipboardAsPlainText(index int) error {
	item, ok := m.GetItem(index)
	if !ok {
		return fmt.Errorf("invalid index")
	}

	return m.copyItemToClipboard(&item, false)
}

// validateClipboardContent validates content size limits for security
func (m *Manager) validateClipboardContent(item *Item) error {
	switch item.Type {
	case TypeText:
		if len(item.Content) > MaxClipboardTextSize {
			return fmt.Errorf("clipboard content exceeds maximum size limit (%d bytes)", MaxClipboardTextSize)
		}
	case TypeHTML:
		if len(item.Content) > MaxClipboardTextSize || len(item.HTMLContent) > MaxClipboardTextSize {
			return fmt.Errorf("clipboard content exceeds maximum size limit (%d bytes)", MaxClipboardTextSize)
		}
	case TypeImage:
		if len(item.ImageData) > MaxClipboardImageSize {
			return fmt.Errorf("clipboard image exceeds maximum size limit (%d bytes)", MaxClipboardImageSize)
		}
	case TypeFile:
		if item.FileInfo != nil && len(item.FileInfo.Path) > MaxClipboardPathSize {
			return fmt.Errorf("file path exceeds maximum length limit (%d characters)", MaxClipboardPathSize)
		}
	}
	return nil
}

func (m *Manager) clipboardWriteHash(item *Item, _ bool) string {
	if item == nil {
		return ""
	}

	switch item.Type {
	case TypeText:
		sum := sha256.Sum256([]byte(item.Content))
		return hex.EncodeToString(sum[:])
	case TypeHTML:
		// Current HTML writes fall back to plain text, so track the plain-text hash.
		sum := sha256.Sum256([]byte(item.Content))
		return hex.EncodeToString(sum[:])
	case TypeImage:
		if item.Hash != "" {
			return item.Hash
		}
		sum := sha256.Sum256([]byte(item.ImageData))
		return hex.EncodeToString(sum[:])
	case TypeFile:
		if item.FileInfo == nil {
			return ""
		}
		sum := sha256.Sum256([]byte(item.FileInfo.Path))
		return hex.EncodeToString(sum[:])
	default:
		return ""
	}
}

// secureCopyToMonitor safely notifies the monitor about a programmatic copy
// without passing the clipboard content itself through monitor state.
func (m *Manager) secureCopyToMonitor(item *Item, asHTML bool) {
	if m.monitor == nil {
		return
	}

	if hash := m.clipboardWriteHash(item, asHTML); hash != "" {
		m.monitor.SetProgrammaticHash(hash)
	}
}

// copyItemToClipboard is the internal method for copying items
func (m *Manager) copyItemToClipboard(item *Item, asHTML bool) error {
	// Validate content size limits for security
	if err := m.validateClipboardContent(item); err != nil {
		return fmt.Errorf("clipboard content validation failed: %w", err)
	}

	// Securely notify monitor about programmatic copy (without exposing content)
	m.secureCopyToMonitor(item, asHTML)

	m.mu.Lock()
	m.lastCopied = time.Now()
	m.mu.Unlock()

	// Memory-safe clipboard operations
	switch item.Type {
	case TypeText:
		// Create a copy of the content to avoid holding references to the original data
		content := make([]byte, len(item.Content))
		copy(content, item.Content)
		defer wipeSensitiveData(content) // Securely wipe from memory after use

		if err := m.native.WriteText(content); err != nil {
			return err
		}
	case TypeHTML:
		// If asHTML is true and we have HTML content, write as HTML
		if asHTML && item.HasHTML() {
			// Create secure copies of HTML content
			htmlContent := make([]byte, len(item.HTMLContent))
			copy(htmlContent, item.HTMLContent)
			textContent := make([]byte, len(item.Content))
			copy(textContent, item.Content)
			defer func() {
				wipeSensitiveData(htmlContent)
				wipeSensitiveData(textContent)
			}()

			if err := m.native.WriteHTML(string(htmlContent), string(textContent)); err != nil {
				return err
			}
		} else {
			// Write plain text
			content := make([]byte, len(item.Content))
			copy(content, item.Content)
			defer wipeSensitiveData(content)

			if err := m.native.WriteText(content); err != nil {
				return err
			}
		}
	case TypeImage:
		// Decode and copy image data securely
		imageData := make([]byte, len(item.ImageData))
		copy(imageData, item.ImageData)
		defer wipeSensitiveData(imageData)

		if err := m.native.WriteImage(string(imageData)); err != nil {
			return err
		}
	case TypeFile:
		if item.FileInfo != nil {
			// Validate file path before copying
			if err := validateFilePath(item.FileInfo.Path); err != nil {
				return fmt.Errorf("file path validation failed: %w", err)
			}

			if err := m.native.WriteFilePaths([]string{item.FileInfo.Path}); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported item type")
	}

	m.incrementCopyCount(item.ID)
	m.saveHistory()
	m.triggerUpdate()
	return nil
}

// validateFilePath validates a file path for security
func validateFilePath(path string) error {
	// Check for directory traversal attempts
	if strings.Contains(path, "..") {
		return fmt.Errorf("directory traversal detected in path: %s", path)
	}

	// Check for absolute paths only (relative paths are safer)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("relative paths not allowed: %s", path)
	}

	// Check for suspicious characters
	if strings.ContainsAny(path, "<>|;&$`") {
		return fmt.Errorf("suspicious characters in path: %s", path)
	}

	// Verify the path exists and is accessible
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	return nil
}

// OpenFileLocation opens the file location in the system file manager
func (m *Manager) OpenFileLocation(index int) error {
	item, ok := m.GetItem(index)
	if !ok {
		return fmt.Errorf("invalid index")
	}

	if !item.IsFile() || item.FileInfo == nil {
		return fmt.Errorf("item is not a file")
	}

	path := item.FileInfo.Path
	// Get directory path
	dirPath := path
	if !item.FileInfo.IsDirectory {
		// Get the directory containing the file
		dirPath = filepath.Dir(path)
	}

	// Resolve and validate the final directory path
	absDirPath, err := filepath.Abs(dirPath)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	// Validate the directory path before executing command
	if err := validateFilePath(absDirPath); err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", absDirPath)
	case "darwin":
		cmd = exec.Command("open", absDirPath)
	case "windows":
		cmd = exec.Command("explorer", absDirPath)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}

// GetStats returns statistics about the clipboard
func (m *Manager) GetStats() (total, pinned, filtered int, lastCopied time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total = len(m.history)
	filtered = len(m.filtered)
	lastCopied = m.lastCopied

	for _, item := range m.history {
		if item.Pinned {
			pinned++
		}
	}

	return
}

// triggerUpdate signals that the UI should be updated
func (m *Manager) triggerUpdate() {
	if !m.running {
		return
	}

	select {
	case m.updateChan <- struct{}{}:
	default:
		// Channel full, update already queued
	}
}

// updateDispatcher handles debounced UI updates
func (m *Manager) updateDispatcher() {
	var timer *time.Timer

	for {
		select {
		case <-m.updateChan:
			// Drain channel
			for len(m.updateChan) > 0 {
				<-m.updateChan
			}

			// Reset debounce timer
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(UpdateDebounce, func() {
				if m.running && m.onUpdate != nil {
					m.onUpdate()
				}
			})

		case <-m.shutdownChan:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// saveDispatcher coalesces frequent save requests into fewer disk writes.
func (m *Manager) saveDispatcher() {
	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(SaveDebounce)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(SaveDebounce)
		timerC = timer.C
	}

	for {
		select {
		case <-m.saveChan:
			for len(m.saveChan) > 0 {
				<-m.saveChan
			}
			resetTimer()

		case <-timerC:
			m.saveHistoryNow()
			timerC = nil

		case <-m.shutdownChan:
			if timer != nil {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			flush := false
			select {
			case <-m.saveChan:
				flush = true
			default:
			}
			for len(m.saveChan) > 0 {
				<-m.saveChan
				flush = true
			}
			if flush {
				m.saveHistoryNow()
			}
			return
		}
	}
}

// Shutdown stops the manager gracefully with timeout
func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	// Cancel context to stop all goroutines
	if m.cancel != nil {
		m.cancel()
	}

	// Stop the monitor
	if m.monitor != nil {
		m.monitor.Stop()
	}

	// Run shutdown hooks with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	m.runShutdownHooks(ctx)

	// Close channels
	close(m.shutdownChan)

	// Final save
	m.saveHistoryNow()
}

// runShutdownHooks executes all registered shutdown hooks
func (m *Manager) runShutdownHooks(ctx context.Context) {
	m.mu.RLock()
	hooks := make([]ShutdownHook, len(m.shutdownHooks))
	copy(hooks, m.shutdownHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(ctx); err != nil {
			// Log error but continue with other hooks
			if m.onError != nil {
				m.onError(fmt.Errorf("shutdown hook error: %w", err))
			}
		}
	}
}

// AddShutdownHook adds a function to be called during shutdown
func (m *Manager) AddShutdownHook(hook ShutdownHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownHooks = append(m.shutdownHooks, hook)
}

// RemoveShutdownHook removes a shutdown hook by index
func (m *Manager) RemoveShutdownHook(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index >= 0 && index < len(m.shutdownHooks) {
		m.shutdownHooks = append(m.shutdownHooks[:index], m.shutdownHooks[index+1:]...)
	}
}

// ClearShutdownHooks removes all shutdown hooks
func (m *Manager) ClearShutdownHooks() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownHooks = nil
}

// GetContext returns the manager's context
func (m *Manager) GetContext() context.Context {
	return m.ctx
}

func (m *Manager) incrementCopyCount(itemID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure indices are up to date before lookups
	m.ensureIndicesUpToDate()

	// O(1) lookup using idIndexMap
	if idx, exists := m.idIndexMap[itemID]; exists {
		m.history[idx].CopyCount++
		m.history[idx].UpdateLastAccessed() // Update LRU timestamp
	}
}

func (m *Manager) notifyInfo(message string) {
	if m.onInfo != nil {
		m.onInfo(message)
	}
}

// GetSnippets returns all snippets
func (m *Manager) GetSnippets() []Snippet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snippets.GetSnippets()
}

// GetSnippetByAbbreviation returns a snippet by its abbreviation
func (m *Manager) GetSnippetByAbbreviation(abbr string) (Snippet, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snippets.GetSnippetByAbbreviation(abbr)
}

// AddSnippet adds a new snippet
func (m *Manager) AddSnippet(snippet Snippet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snippets.AddSnippet(snippet)
}

// UpdateSnippet updates an existing snippet
func (m *Manager) UpdateSnippet(snippet Snippet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snippets.UpdateSnippet(snippet)
}

// DeleteSnippet deletes a snippet
func (m *Manager) DeleteSnippet(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snippets.DeleteSnippet(id)
}

// ExpandSnippet expands template variables in content
func (m *Manager) ExpandSnippet(content string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get current clipboard content if available
	clipContent := ""
	if len(m.history) > 0 {
		clipContent = m.history[0].Content
	}
	return m.snippets.ExpandSnippet(content, clipContent)
}

// ExpandSnippetWithClipboard expands template variables with provided clipboard content
func (m *Manager) ExpandSnippetWithClipboard(content string, clipboardContent string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.snippets.ExpandSnippet(content, clipboardContent)
}

// GetExclusionRules returns all exclusion rules
func (m *Manager) GetExclusionRules() []ExclusionRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.exclusions.GetRules()
}

// AddExclusionRule adds a new exclusion rule
func (m *Manager) AddExclusionRule(rule ExclusionRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exclusions.AddRule(rule)
}

// RemoveExclusionRule removes an exclusion rule
func (m *Manager) RemoveExclusionRule(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exclusions.RemoveRule(id)
}

// UpdateExclusionRule updates an exclusion rule
func (m *Manager) UpdateExclusionRule(rule ExclusionRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exclusions.UpdateRule(rule)
}

// ShouldExcludeContent checks if content should be excluded
func (m *Manager) ShouldExcludeContent(content string, contentSize int, appName string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.exclusions.ShouldExclude(content, contentSize, appName)
}

// DeleteMultiple deletes multiple items by their indices in the filtered list
// Returns the number of items successfully deleted
func (m *Manager) DeleteMultiple(indices []int) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(indices) == 0 {
		return 0
	}

	// Sort indices in reverse order to delete from end to start
	// This prevents index shifting issues
	sortedIndices := make([]int, len(indices))
	copy(sortedIndices, indices)
	for i := 0; i < len(sortedIndices)-1; i++ {
		for j := i + 1; j < len(sortedIndices); j++ {
			if sortedIndices[i] > sortedIndices[j] {
				sortedIndices[i], sortedIndices[j] = sortedIndices[j], sortedIndices[i]
			}
		}
	}

	deleted := 0
	seen := make(map[int]bool)
	for _, idx := range sortedIndices {
		// Skip duplicates
		if seen[idx] {
			continue
		}
		seen[idx] = true

		if idx < 0 || idx >= len(m.filtered) {
			continue
		}

		targetItem := m.filtered[idx]
		if targetItem.Pinned {
			continue
		}

		// O(1) lookup using idIndexMap
		histIdx, exists := m.idIndexMap[targetItem.ID]
		if !exists {
			continue
		}

		m.removeAtIndex(histIdx)
		deleted++
	}

	m.selectedIndex = -1

	if deleted > 0 {
		m.queueSaveHistory()
	}

	return deleted
}

// TogglePinMultiple toggles the pin status of multiple items
// Returns the number of items successfully toggled
func (m *Manager) TogglePinMultiple(indices []int) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(indices) == 0 {
		return 0
	}

	toggled := 0
	seen := make(map[int]bool)
	for _, idx := range indices {
		// Skip duplicates
		if seen[idx] {
			continue
		}
		seen[idx] = true

		if idx < 0 || idx >= len(m.filtered) {
			continue
		}

		histIdx, exists := m.idIndexMap[m.filtered[idx].ID]
		if !exists {
			continue
		}

		m.history[histIdx].Pinned = !m.history[histIdx].Pinned
		toggled++
	}

	if toggled > 0 {
		m.queueSaveHistory()
	}

	return toggled
}

// ValidateItem validates an item before adding
func (m *Manager) ValidateItem(item *Item) error {
	return ValidateItem(item)
}
