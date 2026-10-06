// File: internal/clipboard/search.go
package clipboard

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SearchOptions contains search configuration
type SearchOptions struct {
	CaseSensitive bool // Case-sensitive search
	RegexEnabled  bool // Enable regex matching
	FuzzyEnabled  bool // Enable fuzzy matching
}

const (
	regexMatchTimeout = 100 * time.Millisecond
	regexMaxInputLen  = 10000
)

// DefaultSearchOptions returns default search options
func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		CaseSensitive: false,
		RegexEnabled:  false,
		FuzzyEnabled:  false,
	}
}

// regexCache stores compiled regex patterns for performance
var (
	regexCacheMu sync.RWMutex
	regexCache   = make(map[string]*regexp.Regexp)
)

// ClearRegexCache clears the regex cache (useful for testing)
func ClearRegexCache() {
	regexCacheMu.Lock()
	defer regexCacheMu.Unlock()
	regexCache = make(map[string]*regexp.Regexp)
}

// SearchItem searches an item with the given query and options
func SearchItem(item *Item, query string, opts *SearchOptions) bool {
	if query == "" {
		return true
	}

	// Get searchable content
	searchContent := item.SearchContent()
	if searchContent == "" && item.Content != "" {
		item.PrepareForSearch()
		searchContent = item.SearchContent()
	}

	if searchContent == "" {
		return false
	}

	// Apply search based on options
	if opts.RegexEnabled {
		return searchWithRegex(searchContent, query)
	}

	if opts.FuzzyEnabled {
		return searchWithFuzzy(searchContent, query, opts.CaseSensitive)
	}

	// Default: substring search
	return searchWithSubstring(searchContent, query, opts.CaseSensitive)
}

// searchWithSubstring performs a basic substring search
func searchWithSubstring(content, query string, caseSensitive bool) bool {
	if !caseSensitive {
		content = strings.ToLower(content)
		query = strings.ToLower(query)
	}
	return strings.Contains(content, query)
}

// searchWithRegex performs regex-based search
func searchWithRegex(content, pattern string) bool {
	if len(pattern) > 1000 {
		return strings.Contains(content, pattern)
	}
	regexCacheMu.Lock()
	defer regexCacheMu.Unlock()

	re, exists := regexCache[pattern]

	if !exists {
		if err := validateRegexPattern(pattern); err != nil {
			return strings.Contains(content, pattern)
		}
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			// If regex is invalid, fall back to substring search
			return strings.Contains(content, pattern)
		}
		if len(regexCache) >= 128 {
			regexCache = make(map[string]*regexp.Regexp)
		}
		regexCache[pattern] = re
	}

	if len(content) > regexMaxInputLen {
		content = content[:regexMaxInputLen]
	}

	ctx, cancel := context.WithTimeout(context.Background(), regexMatchTimeout)
	defer cancel()

	type result struct {
		matched bool
	}
	done := make(chan result, 1)
	go func() {
		done <- result{matched: re.MatchString(content)}
	}()

	select {
	case res := <-done:
		return res.matched
	case <-ctx.Done():
		return false
	}
}

var nestedQuantifierRegex = regexp.MustCompile(`\([^)]*[+*][^)]*\)[+*]`)

func validateRegexPattern(pattern string) error {
	if nestedQuantifierRegex.MatchString(pattern) {
		return fmt.Errorf("regex pattern contains nested quantifier which may cause catastrophic backtracking")
	}
	return nil
}

// searchWithFuzzy performs fuzzy matching (simple Levenshtein-based)
func searchWithFuzzy(content, query string, caseSensitive bool) bool {
	if !caseSensitive {
		content = strings.ToLower(content)
		query = strings.ToLower(query)
	}

	// First try exact substring match
	if strings.Contains(content, query) {
		return true
	}

	// Check if query is a subsequence of content (for typo tolerance)
	return isSubsequence(query, content)
}

// isSubsequence checks if query is a subsequence of text
func isSubsequence(query, text string) bool {
	if len(query) == 0 {
		return true
	}
	// Convert query to rune slice once to avoid allocations in loop
	q := []rune(query)
	qIdx := 0
	for _, ch := range text {
		if qIdx < len(q) && ch == q[qIdx] {
			qIdx++
			if qIdx == len(q) {
				return true
			}
		}
	}
	return false
}

// SearchHistoryItem searches an item for history display
func SearchHistoryItem(item *Item, query string) bool {
	// Default search without options (substring, case-insensitive)
	opts := DefaultSearchOptions()
	return SearchItem(item, query, opts)
}
