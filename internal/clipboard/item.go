// File: internal/clipboard/item.go
package clipboard

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// Thumbnail dimensions
	ThumbnailWidth  = 150
	ThumbnailHeight = 150
	// JPEG quality for thumbnails
	ThumbnailQuality  = 70
	maxImageDimension = 10000
	maxImagePixels    = int64(16_000_000)
)

// ItemType represents the type of clipboard content
type ItemType int

const (
	TypeText ItemType = iota
	TypeImage
	TypeHTML
	TypeFile
)

func (t ItemType) String() string {
	switch t {
	case TypeText:
		return "Text"
	case TypeImage:
		return "Image"
	case TypeHTML:
		return "HTML"
	case TypeFile:
		return "File"
	default:
		return "Unknown"
	}
}

// FileInfo represents metadata for file clipboard items
type FileInfo struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	IsDirectory bool      `json:"is_directory"`
}

// Item represents a clipboard history item
type Item struct {
	ID           string    `json:"id"`
	Type         ItemType  `json:"type"`
	Content      string    `json:"content"`
	ImageData    string    `json:"image_data,omitempty"`
	ImageType    string    `json:"image_type,omitempty"`
	Thumbnail    string    `json:"thumbnail,omitempty"` // Compressed thumbnail for list display
	HTMLContent  string    `json:"html_content,omitempty"`
	FileInfo     *FileInfo `json:"file_info,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	LastAccessed time.Time `json:"last_accessed,omitempty"` // Tracks when item was last accessed for LRU
	Pinned       bool      `json:"pinned"`
	CopyCount    int       `json:"copy_count,omitempty"`
	Hash         string    `json:"hash,omitempty"`
	Category     string    `json:"category,omitempty"`
	Tags         []string  `json:"tags,omitempty"`

	searchContent string `json:"-"`
}

// PrepareForSearch builds cached normalized text used for filtering.
func (i *Item) PrepareForSearch() {
	i.searchContent = strings.ToLower(i.Content)
}

// SearchContent returns cached normalized text used for filtering.
func (i *Item) SearchContent() string {
	return i.searchContent
}

// UpdateLastAccessed updates the last accessed timestamp for LRU tracking
func (i *Item) UpdateLastAccessed() {
	i.LastAccessed = time.Now()
}

// DisplayText returns a truncated version of the content for display
func (i *Item) DisplayText(maxLen int) string {
	if i.Type == TypeImage {
		return fmt.Sprintf("%s Image (%s)",
			strings.ToUpper(i.ImageType),
			i.Timestamp.Format("15:04:05"))
	}

	if i.Type == TypeFile {
		if i.FileInfo != nil {
			safeName := sanitizeDisplayText(i.FileInfo.Name)
			if safeName == "" {
				safeName = "unnamed"
			}
			return fmt.Sprintf("[FILE] %s (%s)", safeName, formatFileSize(i.FileInfo.Size))
		}
		return fmt.Sprintf("File: %s", i.Timestamp.Format("15:04:05"))
	}

	text := sanitizeDisplayText(i.Content)
	text = strings.TrimSpace(text)

	runes := []rune(text)
	if len(runes) > maxLen {
		runes = append(runes[:maxLen-3], []rune("...")...)
	}
	return string(runes)
}

func sanitizeDisplayText(s string) string {
	var buf strings.Builder
	prevRune := rune(0)
	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		if r == '\r' || r == '\n' {
			if prevRune != ' ' && prevRune != '\n' && prevRune != '\r' {
				buf.WriteRune(' ')
			}
			prevRune = ' '
			continue
		}
		if r == '\x00' || (r >= 0x00 && r <= 0x1F) || (r >= 0x7F && r <= 0x9F) {
			prevRune = r
			continue
		}
		buf.WriteRune(r)
		prevRune = r
	}
	return buf.String()
}

// Size returns approximate size in bytes
func (i *Item) Size() int {
	if i.Type == TypeImage {
		return len(i.ImageData) * 3 / 4 // base64 overhead
	}
	if i.Type == TypeFile && i.FileInfo != nil {
		return int(i.FileInfo.Size)
	}
	return len(i.Content)
}

// GetDisplayImage returns the appropriate image for display
// For list view, returns thumbnail (if available), for preview returns full image
func (i *Item) GetDisplayImage(forList bool) string {
	if i.Type != TypeImage {
		return ""
	}

	// For list view, prefer thumbnail
	if forList && i.Thumbnail != "" {
		return i.Thumbnail
	}

	// Otherwise return full image data
	return i.ImageData
}

// HasThumbnail returns true if the item has a thumbnail
func (i *Item) HasThumbnail() bool {
	return i.Type == TypeImage && i.Thumbnail != ""
}

// EnsureThumbnail generates thumbnail if missing (for backward compatibility)
func (i *Item) EnsureThumbnail() {
	if i.Type == TypeImage && i.Thumbnail == "" && i.ImageData != "" {
		i.Thumbnail = GenerateThumbnail(i.ImageData)
	}
}

// HasHTML returns true if the item has HTML content
func (i *Item) HasHTML() bool {
	return i.Type == TypeHTML && i.HTMLContent != ""
}

// IsFile returns true if the item is a file
func (i *Item) IsFile() bool {
	return i.Type == TypeFile && i.FileInfo != nil
}

// GetDisplayContent returns the appropriate content for display based on type
func (i *Item) GetDisplayContent() string {
	if i.Type == TypeHTML && i.HTMLContent != "" {
		// Return plain text version of HTML
		return stripHTMLTags(i.HTMLContent)
	}
	return i.Content
}

// stripHTMLTags removes HTML tags from content
func stripHTMLTags(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// formatFileSize formats file size in human-readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), suffixes[exp])
}

// TimeAgo returns a human-readable time difference
func (i *Item) TimeAgo() string {
	diff := time.Since(i.Timestamp)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}

	days := int(diff.Hours() / 24)
	if days == 1 {
		return "yesterday"
	}
	return fmt.Sprintf("%dd ago", days)
}

// AutoDetectCategory detects and sets the category based on content patterns
func (i *Item) AutoDetectCategory() {
	if i.Category != "" {
		return // Already has a category
	}

	content := i.Content

	// URL detection
	if isURL(content) {
		i.Category = "Links"
		return
	}

	// Email detection
	if isEmail(content) {
		i.Category = "Contacts"
		return
	}

	// Phone number detection
	if isPhoneNumber(content) {
		i.Category = "Contacts"
		return
	}

	// Code detection
	if isCodeSnippet(content) {
		i.Category = "Code"
		return
	}

	// File path detection
	if isContentFilePath(content) {
		i.Category = "Files"
		return
	}

	// JSON detection
	if isJSON(content) {
		i.Category = "Data"
		return
	}

	// Default category based on type
	switch i.Type {
	case TypeImage:
		i.Category = "Images"
	case TypeFile:
		i.Category = "Files"
	case TypeHTML:
		i.Category = "Web"
	default:
		i.Category = "Text"
	}
}

// AddTag adds a tag to the item
func (i *Item) AddTag(tag string) {
	for _, t := range i.Tags {
		if t == tag {
			return // Tag already exists
		}
	}
	i.Tags = append(i.Tags, tag)
}

// RemoveTag removes a tag from the item
func (i *Item) RemoveTag(tag string) {
	newTags := make([]string, 0)
	for _, t := range i.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}
	i.Tags = newTags
}

// HasTag returns true if the item has the specified tag
func (i *Item) HasTag(tag string) bool {
	for _, t := range i.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// isURL checks if the content is a URL
func isURL(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "http://") ||
		strings.HasPrefix(content, "https://") ||
		strings.HasPrefix(content, "ftp://") ||
		strings.HasPrefix(content, "file://")
}

// isEmail checks if the content is an email address
func isEmail(content string) bool {
	content = strings.TrimSpace(content)
	match, _ := regexp.MatchString(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, content)
	return match
}

// isPhoneNumber checks if the content is a phone number
func isPhoneNumber(content string) bool {
	content = strings.TrimSpace(content)
	// Match common phone number formats
	match, _ := regexp.MatchString(`^[+]?[0-9\s()-]{7,20}$`, content)
	return match
}

// isCodeSnippet checks if the content looks like code
func isCodeSnippet(content string) bool {
	// Common code patterns
	codeIndicators := []string{"func ", "function ", "def ", "class ", "const ", "let ", "var ",
		"import ", "export ", "package ", "#include", "public ", "private ",
		"if (", "for (", "while (", "switch (", "=> {", "-> {",
		"{}", "};", "();", "[]", "<>", "<!--", "//", "/*", "#"}

	count := 0
	for _, indicator := range codeIndicators {
		if strings.Contains(content, indicator) {
			count++
			if count >= 2 {
				return true
			}
		}
	}
	return false
}

// isContentFilePath checks if the content looks like a file path
func isContentFilePath(content string) bool {
	content = strings.TrimSpace(content)
	// Unix or Windows path
	match, _ := regexp.MatchString(`^([A-Za-z]:\\|/)[^\x00]+$`, content)
	return match
}

// isJSON checks if the content is valid JSON
func isJSON(content string) bool {
	content = strings.TrimSpace(content)
	if (strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}")) ||
		(strings.HasPrefix(content, "[") && strings.HasSuffix(content, "]")) {
		// Try to parse as JSON
		var js interface{}
		return json.Unmarshal([]byte(content), &js) == nil
	}
	return false
}

// GenerateThumbnail creates a compressed thumbnail from base64 image data
// Returns empty string if generation fails
func GenerateThumbnail(base64Data string) string {
	if base64Data == "" {
		return ""
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return ""
	}

	// Reject extreme dimensions before the decoder allocates a full image.
	img, format, err := DecodeImage(data)
	if err != nil {
		return ""
	}

	// Calculate thumbnail dimensions while maintaining aspect ratio
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var thumbW, thumbH int
	if w > h {
		thumbW = ThumbnailWidth
		thumbH = (h * ThumbnailWidth) / w
	} else {
		thumbH = ThumbnailHeight
		thumbW = (w * ThumbnailHeight) / h
	}
	if thumbW < 1 {
		thumbW = 1
	}
	if thumbH < 1 {
		thumbH = 1
	}

	// Create thumbnail using nearest neighbor for speed
	thumb := image.NewNRGBA(image.Rect(0, 0, thumbW, thumbH))

	// Scale down the image
	sx := float64(w) / float64(thumbW)
	sy := float64(h) / float64(thumbH)

	for y := 0; y < thumbH; y++ {
		for x := 0; x < thumbW; x++ {
			sxInt := int(float64(x) * sx)
			syInt := int(float64(y) * sy)
			if sxInt >= w {
				sxInt = w - 1
			}
			if syInt >= h {
				syInt = h - 1
			}
			thumb.Set(x, y, img.At(sxInt, syInt))
		}
	}

	// Encode thumbnail
	var buf bytes.Buffer
	if format == "png" {
		err = png.Encode(&buf, thumb)
	} else {
		err = jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: ThumbnailQuality})
	}

	if err != nil {
		return ""
	}

	// Return base64 encoded thumbnail
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// DecodeImage validates image dimensions before fully decoding untrusted data.
func DecodeImage(data []byte) (image.Image, string, error) {
	format, err := ValidateImage(data)
	if err != nil {
		return nil, "", err
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	if decodedFormat == "" {
		decodedFormat = format
	}
	return img, decodedFormat, nil
}

// ValidateImage checks image dimensions without allocating the decoded pixel buffer.
func ValidateImage(data []byte) (string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageDimension || cfg.Height > maxImageDimension || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return "", fmt.Errorf("image dimensions exceed safe limits")
	}
	return format, nil
}

// CompressImage compresses the image data to reduce memory footprint
// Returns the compressed base64 string
func CompressImage(base64Data string, maxWidth, maxHeight int, quality int) string {
	if base64Data == "" {
		return ""
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return base64Data // Return original if decode fails
	}

	// Decode image
	img, format, err := DecodeImage(data)
	if err != nil {
		return base64Data // Return original if decode fails
	}

	// Check if resizing is needed
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w <= maxWidth && h <= maxHeight {
		return base64Data // No resize needed
	}

	// Calculate new dimensions maintaining aspect ratio
	var newW, newH int
	if w > h {
		newW = maxWidth
		newH = (h * maxWidth) / w
	} else {
		newH = maxHeight
		newW = (w * maxHeight) / h
	}

	// Resize image
	resized := image.NewNRGBA(image.Rect(0, 0, newW, newH))

	sx := float64(w) / float64(newW)
	sy := float64(h) / float64(newH)

	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			sxInt := int(float64(x) * sx)
			syInt := int(float64(y) * sy)
			if sxInt >= w {
				sxInt = w - 1
			}
			if syInt >= h {
				syInt = h - 1
			}
			resized.Set(x, y, img.At(sxInt, syInt))
		}
	}

	// Encode compressed image
	var buf bytes.Buffer
	if format == "png" {
		err = png.Encode(&buf, resized)
	} else {
		err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality})
	}

	if err != nil {
		return base64Data // Return original if encode fails
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
