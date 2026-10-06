// File: internal/clipboard/snippet.go
package clipboard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Snippet represents a text snippet/template
type Snippet struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Abbreviation string    `json:"abbreviation,omitempty"` // Short trigger like "sig"
	Category     string    `json:"category,omitempty"`
	IsSystem     bool      `json:"is_system,omitempty"` // System snippets cannot be deleted
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SnippetManager handles snippet storage and operations
type SnippetManager struct {
	snippets []Snippet
	storage  *Storage
}

// NewSnippetManager creates a new snippet manager
func NewSnippetManager(storage *Storage) *SnippetManager {
	return &SnippetManager{
		snippets: []Snippet{},
		storage:  storage,
	}
}

// LoadSnippets loads snippets from storage
func (sm *SnippetManager) LoadSnippets() error {
	// Try to load snippets - they may not exist yet
	// We'll store snippets in a separate file
	path := filepath.Join(sm.storage.GetDir(), "snippets.json")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No snippets file yet, start with defaults
			sm.snippets = getDefaultSnippets()
			return sm.SaveSnippets()
		}
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxStoredHistoryBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read snippets: %w", err)
	}
	if len(data) > maxStoredHistoryBytes {
		return fmt.Errorf("snippet file exceeds size limit")
	}

	if err := json.Unmarshal(data, &sm.snippets); err != nil {
		return fmt.Errorf("failed to parse snippets: %w", err)
	}

	for i := range sm.snippets {
		if sm.snippets[i].ID == "" {
			sm.snippets[i].ID = fmt.Sprintf("snippet-%d", i)
		}
	}

	return validateSnippets(sm.snippets)
}

func validateSnippets(snippets []Snippet) error {
	for i, s := range snippets {
		if i >= maxHistoryItems {
			return fmt.Errorf("too many snippets")
		}
		if s.ID == "" {
			return fmt.Errorf("snippet %d has empty ID", i)
		}
		if s.Title == "" {
			return fmt.Errorf("snippet %d has empty title", i)
		}
		if len(s.Title) > 4096 || len(s.Content) > MaxContentSize || len(s.Abbreviation) > 256 || len(s.Category) > 256 {
			return fmt.Errorf("snippet %d exceeds field size limits", i)
		}
	}
	return nil
}

// SaveSnippets saves snippets to storage
func (sm *SnippetManager) SaveSnippets() error {
	path := filepath.Join(sm.storage.GetDir(), "snippets.json")
	data, err := json.MarshalIndent(sm.snippets, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// GetSnippets returns all snippets
func (sm *SnippetManager) GetSnippets() []Snippet {
	return sm.snippets
}

// GetSnippetByID returns a snippet by ID
func (sm *SnippetManager) GetSnippetByID(id string) (Snippet, bool) {
	for _, s := range sm.snippets {
		if s.ID == id {
			return s, true
		}
	}
	return Snippet{}, false
}

// GetSnippetByAbbreviation returns a snippet by abbreviation
func (sm *SnippetManager) GetSnippetByAbbreviation(abbr string) (Snippet, bool) {
	for _, s := range sm.snippets {
		if s.Abbreviation == abbr {
			return s, true
		}
	}
	return Snippet{}, false
}

// AddSnippet adds a new snippet
func (sm *SnippetManager) AddSnippet(snippet Snippet) error {
	snippet.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	snippet.CreatedAt = time.Now()
	snippet.UpdatedAt = time.Now()

	sm.snippets = append(sm.snippets, snippet)
	return sm.SaveSnippets()
}

// UpdateSnippet updates an existing snippet
func (sm *SnippetManager) UpdateSnippet(snippet Snippet) error {
	for i, s := range sm.snippets {
		if s.ID == snippet.ID {
			snippet.UpdatedAt = time.Now()
			snippet.CreatedAt = s.CreatedAt
			sm.snippets[i] = snippet
			return sm.SaveSnippets()
		}
	}
	return fmt.Errorf("snippet not found")
}

// DeleteSnippet deletes a snippet by ID
func (sm *SnippetManager) DeleteSnippet(id string) error {
	for i, s := range sm.snippets {
		if s.ID == id {
			sm.snippets = append(sm.snippets[:i], sm.snippets[i+1:]...)
			return sm.SaveSnippets()
		}
	}
	return fmt.Errorf("snippet not found")
}

// ExpandSnippet expands template variables in snippet content
func (sm *SnippetManager) ExpandSnippet(content string, clipboardContent string) string {
	result := content

	// Replace template variables
	now := time.Now()

	result = strings.ReplaceAll(result, "{{date}}", now.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "{{time}}", now.Format("15:04:05"))
	result = strings.ReplaceAll(result, "{{datetime}}", now.Format("2006-01-02 15:04:05"))
	result = strings.ReplaceAll(result, "{{clipboard}}", clipboardContent)

	// Add more date/time formats
	result = strings.ReplaceAll(result, "{{year}}", fmt.Sprintf("%d", now.Year()))
	result = strings.ReplaceAll(result, "{{month}}", now.Format("01"))
	result = strings.ReplaceAll(result, "{{day}}", now.Format("02"))

	return result
}

// GetCategories returns all unique categories
func (sm *SnippetManager) GetCategories() []string {
	catMap := make(map[string]bool)
	for _, s := range sm.snippets {
		if s.Category != "" {
			catMap[s.Category] = true
		}
	}

	categories := []string{}
	for cat := range catMap {
		categories = append(categories, cat)
	}

	return categories
}

// getDefaultSnippets returns some default snippets (system snippets that cannot be deleted)
func getDefaultSnippets() []Snippet {
	now := time.Now()
	return []Snippet{
		{
			ID:           "system-email-signature",
			Title:        "Email Signature",
			Content:      "Best regards,\n{{date}}",
			Abbreviation: "sig",
			Category:     "General",
			IsSystem:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "system-current-date",
			Title:        "Current Date",
			Content:      "{{date}}",
			Abbreviation: "date",
			Category:     "Utility",
			IsSystem:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "system-current-datetime",
			Title:        "Current DateTime",
			Content:      "{{datetime}}",
			Abbreviation: "dt",
			Category:     "Utility",
			IsSystem:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "system-current-time",
			Title:        "Current Time",
			Content:      "{{time}}",
			Abbreviation: "time",
			Category:     "Utility",
			IsSystem:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "system-clipboard-content",
			Title:        "Clipboard Content",
			Content:      "{{clipboard}}",
			Abbreviation: "clip",
			Category:     "Utility",
			IsSystem:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}
