// File: internal/clipboard/validation.go
package clipboard

import (
	"errors"
	"unicode/utf8"
)

const (
	// MaxContentSize is the maximum allowed content size (10MB)
	MaxContentSize = 10 * 1024 * 1024
	// MaxImageSize is the maximum allowed image data size (50MB)
	MaxImageSize = 50 * 1024 * 1024
)

var (
	// ErrContentTooLarge is returned when content exceeds maximum size
	ErrContentTooLarge = errors.New("content exceeds maximum size")
	// ErrInvalidUTF8 is returned when content is not valid UTF-8
	ErrInvalidUTF8 = errors.New("content is not valid UTF-8")
	// ErrInvalidType is returned when item type is invalid
	ErrInvalidType = errors.New("invalid item type")
	// ErrEmptyContent is returned when content is empty
	ErrEmptyContent = errors.New("content cannot be empty")
	// ErrNilItem is returned when item is nil
	ErrNilItem = errors.New("item cannot be nil")
	// ErrInvalidMaxHistory is returned when max history value is invalid
	ErrInvalidMaxHistory = errors.New("max history must be between 1 and 100000")
)

// ValidateItem validates a clipboard item
func ValidateItem(item *Item) error {
	if item == nil {
		return ErrNilItem
	}

	// Validate item type
	if item.Type < TypeText || item.Type > TypeFile {
		return ErrInvalidType
	}
	if len(item.Content) > MaxContentSize {
		return ErrContentTooLarge
	}

	// Validate content size based on type
	switch item.Type {
	case TypeText, TypeHTML:
		if len(item.Content) > MaxContentSize {
			return ErrContentTooLarge
		}
		// Validate UTF-8 for text content
		if len(item.Content) > 0 && !utf8.ValidString(item.Content) {
			return ErrInvalidUTF8
		}
	case TypeImage:
		if len(item.ImageData) > MaxImageSize {
			return ErrContentTooLarge
		}
		if item.ImageData == "" {
			return errors.New("image item must have image data")
		}
	case TypeFile:
		if item.FileInfo == nil {
			return errors.New("file item must have file info")
		}
		if item.FileInfo.Path == "" {
			return errors.New("file item must have a path")
		}
	}

	// Validate HTML content if present
	if item.Type == TypeHTML && item.HTMLContent != "" {
		if len(item.HTMLContent) > MaxContentSize {
			return ErrContentTooLarge
		}
		if !utf8.ValidString(item.HTMLContent) {
			return ErrInvalidUTF8
		}
	}

	return nil
}

// SafeString converts byte slice to string with UTF-8 validation
// Returns empty string if data is not valid UTF-8
func SafeString(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if !utf8.ValidString(string(data)) {
		return ""
	}
	return string(data)
}

// ValidateMaxHistory validates the max history value
func ValidateMaxHistory(max int) error {
	if max < 1 || max > 100000 {
		return ErrInvalidMaxHistory
	}
	return nil
}

// ValidateSearchQuery validates a search query
func ValidateSearchQuery(query string) error {
	// Empty search is valid
	if query == "" {
		return nil
	}

	// Check for unreasonably long search queries
	if len(query) > 1000 {
		return errors.New("search query is too long (max 1000 characters)")
	}

	return nil
}

// ValidateSnippet validates a snippet
func ValidateSnippet(snippet *Snippet) error {
	if snippet == nil {
		return errors.New("snippet cannot be nil")
	}
	if snippet.Title == "" {
		return errors.New("snippet title cannot be empty")
	}
	if snippet.Content == "" {
		return errors.New("snippet content cannot be empty")
	}
	if len(snippet.Content) > MaxContentSize {
		return errors.New("snippet content exceeds maximum size")
	}
	return nil
}

// ValidateExclusionRule validates an exclusion rule
func ValidateExclusionRule(rule *ExclusionRule) error {
	if rule == nil {
		return errors.New("exclusion rule cannot be nil")
	}
	if rule.Pattern == "" {
		return errors.New("exclusion pattern cannot be empty")
	}
	validTypes := map[ExclusionRuleType]bool{
		ExclusionTypeRegex: true,
		ExclusionTypeApp:   true,
		ExclusionTypeSize:  true,
	}
	if !validTypes[rule.Type] {
		return errors.New("invalid exclusion type (must be regex, app, or size)")
	}
	return nil
}
