package ui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/clipboard"
)

func escapeHTML(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '&':
			buf.WriteString("&amp;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&#39;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "#", "\\#")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// PreviewPane displays preview of selected item
type PreviewPane struct {
	manager   *clipboard.Manager
	text      *widget.RichText
	label     *widget.Label // For plain text display
	scroll    *container.Scroll
	image     *canvas.Image
	copyBtn   *widget.Button
	container *fyne.Container

	rawText string

	lastItemID   string
	lastItemType clipboard.ItemType
	hasSelection bool

	// Cache for image data to avoid repeated decoding
	cachedImageData     string
	cachedImageResource fyne.Resource

	// Cache for markdown content to avoid repeated parsing
	cachedMarkdown string
}

// NewPreviewPane creates a new preview pane
func NewPreviewPane(manager *clipboard.Manager) *PreviewPane {
	pp := &PreviewPane{
		manager: manager,
	}

	// Markdown-capable rich text
	pp.text = pp.newMarkdownText("_Select an item to preview..._")

	pp.scroll = container.NewVScroll(pp.text)
	pp.scroll.Hide()

	// Label for plain text display (used for HTML)
	pp.label = widget.NewLabel("")
	pp.label.Wrapping = fyne.TextWrapWord
	pp.label.Hide()

	pp.image = canvas.NewImageFromResource(theme.BrokenImageIcon())
	pp.image.FillMode = canvas.ImageFillContain
	pp.image.Hide()

	pp.copyBtn = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		if pp.rawText == "" {
			return
		}
		fyne.CurrentApp().Clipboard().SetContent(pp.rawText)
	})
	pp.copyBtn.Importance = widget.LowImportance
	pp.copyBtn.Hide()

	pp.container = container.NewBorder(
		container.NewHBox(layoutSpacer(), pp.copyBtn),
		nil,
		nil,
		nil,
		container.NewStack(pp.scroll, pp.label, pp.image),
	)

	pp.showPlaceholder()
	return pp
}

// Build returns the preview widget
func (pp *PreviewPane) Build() fyne.CanvasObject {
	return pp.container
}

// Refresh updates the preview
func (pp *PreviewPane) Refresh() {
	item, ok := pp.manager.GetSelected()
	if !ok {
		if !pp.hasSelection {
			return
		}
		pp.clear()
		return
	}

	if pp.hasSelection && item.ID == pp.lastItemID && item.Type == pp.lastItemType {
		return
	}

	switch item.Type {
	case clipboard.TypeText:
		pp.showText(item)
	case clipboard.TypeImage:
		pp.showImage(item)
	case clipboard.TypeHTML:
		pp.showCode(item)
	case clipboard.TypeFile:
		pp.showFile(item)
	}
}

// showText renders markdown text with scrollbar
func (pp *PreviewPane) showText(item clipboard.Item) {
	pp.image.Hide()
	pp.label.Hide()
	pp.scroll.Show()
	pp.copyBtn.Show()

	pp.rawText = item.Content

	// Check if content is JSON and pretty-print it
	content := item.Content
	if isJSON(content) {
		if prettyJSON, err := json.MarshalIndent(json.RawMessage(content), "", "  "); err == nil {
			content = string(prettyJSON)
			content = "**JSON (Pretty)**\n\n```json\n" + escapeMarkdown(content) + "\n```"
		}
	} else if isCodeContent(content) {
		// Detect language and wrap in code block
		lang := detectCodeLanguage(content)
		if lang != "" {
			content = "**" + strings.ToUpper(lang) + " Code**\n\n```" + lang + "\n" + escapeMarkdown(content) + "\n```"
		} else {
			content = "**Code**\n\n```\n" + escapeMarkdown(content) + "\n```"
		}
	}

	pp.setMarkdown(content)

	content += fmt.Sprintf("\n---\n\n*Copied: %s*", item.Timestamp.Format("2006-01-02 15:04:05"))

	pp.setMarkdown(content)

	pp.scroll.Show()
	pp.scroll.ScrollToTop()
	pp.markRendered(item)
}

// showCode renders HTML content as code block
func (pp *PreviewPane) showCode(item clipboard.Item) {
	pp.image.Hide()
	pp.scroll.Show()
	pp.copyBtn.Show()

	pp.rawText = item.Content

	// Show HTML as code block
	content := "```html\n" + escapeHTML(item.HTMLContent) + "\n```"

	content += fmt.Sprintf("\n---\n\n*Copied: %s*", item.Timestamp.Format("2006-01-02 15:04:05"))

	pp.setMarkdown(content)

	pp.scroll.ScrollToTop()
	pp.markRendered(item)
}

func (pp *PreviewPane) showFile(item clipboard.Item) {
	pp.image.Hide()
	pp.label.Hide()
	pp.scroll.Show()
	pp.copyBtn.Show()

	if item.FileInfo == nil {
		pp.setMarkdown("**File**\n\nNo file information available")
		pp.scroll.Show()
		pp.markRendered(item)
		return
	}

	fi := item.FileInfo
	fileType := "File"
	if fi.IsDirectory {
		fileType = "Directory"
	}

	content := fmt.Sprintf("**%s: %s**\n\n", fileType, escapeMarkdown(fi.Name))
	content += fmt.Sprintf("Path: `%s`\n", escapeMarkdown(fi.Path))
	content += fmt.Sprintf("Size: %s\n", formatFileSize(fi.Size))
	content += fmt.Sprintf("Modified: %s\n", fi.ModTime.Format("2006-01-02 15:04:05"))
	content += fmt.Sprintf("Copied: %s\n", item.Timestamp.Format("2006-01-02 15:04:05"))
	content += "\n---\n\n*Copy to clipboard: copies file path*"

	pp.rawText = fi.Path
	pp.setMarkdown(content)

	pp.scroll.Show()
	pp.scroll.ScrollToTop()
	pp.markRendered(item)
}

// isJSON checks if a string is valid JSON
func isJSON(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return false
	}
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}

// detectCodeLanguage attempts to detect the programming language from content
func detectCodeLanguage(content string) string {
	content = strings.TrimSpace(content)

	// Check for JSON
	if isJSON(content) {
		return "json"
	}

	// Check for common patterns
	lowerContent := strings.ToLower(content)

	// Go
	if strings.Contains(content, "package ") && strings.Contains(content, "func ") {
		return "go"
	}

	// Python
	if strings.Contains(content, "def ") && strings.Contains(content, ":") && !strings.Contains(content, "{") {
		return "python"
	}

	// JavaScript/TypeScript
	if strings.Contains(content, "const ") || strings.Contains(content, "let ") || strings.Contains(content, "function ") {
		if strings.Contains(content, ": string") || strings.Contains(content, ": number") || strings.Contains(content, "interface ") {
			return "typescript"
		}
		return "javascript"
	}

	// Java
	if strings.Contains(content, "public class ") || strings.Contains(content, "private ") {
		return "java"
	}

	// C/C++
	if strings.Contains(content, "#include<") || strings.Contains(content, "#include ") {
		return "cpp"
	}

	// Rust
	if strings.Contains(content, "fn main()") || strings.Contains(content, "let mut ") {
		return "rust"
	}

	// HTML
	if strings.Contains(content, "<html") || strings.Contains(content, "<!DOCTYPE") || strings.Contains(content, "<div") {
		return "html"
	}

	// CSS
	if strings.Contains(content, "{") && (strings.Contains(lowerContent, "color:") || strings.Contains(lowerContent, "margin:") || strings.Contains(lowerContent, "padding:")) {
		return "css"
	}

	// SQL
	if strings.Contains(lowerContent, "select ") || strings.Contains(lowerContent, "insert ") || strings.Contains(lowerContent, "update ") || strings.Contains(lowerContent, "create table") {
		return "sql"
	}

	// Bash/Shell
	if strings.HasPrefix(content, "#!") || strings.Contains(content, "#!/bin/bash") || strings.Contains(content, "#!/bin/sh") {
		return "bash"
	}

	// YAML
	if strings.Contains(content, "---") || (strings.Contains(content, ":") && !strings.Contains(content, "{") && !strings.Contains(content, ";")) {
		if strings.Contains(content, "  - ") {
			return "yaml"
		}
	}

	// Markdown
	if strings.Contains(content, "```") || strings.Contains(content, "[**") {
		return "markdown"
	}

	return ""
}

// isCodeContent checks if the content appears to be code
func isCodeContent(content string) bool {
	// If it's JSON, treat as code
	if isJSON(content) {
		return true
	}

	content = strings.TrimSpace(content)

	// Check for common code patterns
	codeIndicators := []string{
		"func ",
		"function ",
		"const ",
		"let ",
		"var ",
		"class ",
		"def ",
		"public ",
		"private ",
		"import ",
		"package ",
		"#include",
		"#!/bin",
		"select ",
		"from ",
		"where ",
	}

	for _, indicator := range codeIndicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}

	// Check for multiple semicolons on single line (common in code)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Count(line, ";") >= 2 {
			return true
		}
		// Check for arrow functions or lambda
		if strings.Contains(line, "=>") {
			return true
		}
		// Check for typical code brackets
		if strings.Contains(line, "{") && strings.Contains(line, "}") {
			return true
		}
	}

	return false
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

// showImage displays image preview
func (pp *PreviewPane) showImage(item clipboard.Item) {
	pp.copyBtn.Hide()
	pp.label.Hide()
	pp.scroll.Hide()
	pp.rawText = ""

	if item.ImageData == "" {
		pp.clear()
		return
	}

	// Use cached image if available
	if pp.cachedImageData == item.ImageData && pp.cachedImageResource != nil {
		pp.image.Resource = pp.cachedImageResource
		pp.image.Show()
		pp.image.Refresh()
		pp.markRendered(item)
		return
	}

	imageBytes, err := base64.StdEncoding.DecodeString(item.ImageData)
	if err != nil {
		pp.showImageError(item, "Failed to decode image data")
		return
	}

	format, err := clipboard.ValidateImage(imageBytes)
	if err != nil {
		pp.showImageError(item, "Invalid image format")
		return
	}

	size := item.Size()
	info := fmt.Sprintf(
		"**%s Image**\n\nCopied: %s\nSize: ~%d bytes\nFormat: %s",
		strings.ToUpper(format),
		item.Timestamp.Format("2006-01-02 15:04:05"),
		size,
		format,
	)

	pp.setMarkdown(info)
	pp.scroll.Show()
	pp.scroll.ScrollToTop()

	// Cache the image resource
	pp.cachedImageData = item.ImageData
	pp.cachedImageResource = fyne.NewStaticResource("preview", imageBytes)
	pp.image.Resource = pp.cachedImageResource
	pp.image.Show()
	pp.image.Refresh()
	pp.markRendered(item)
}

// showImageError displays an error for image preview
func (pp *PreviewPane) showImageError(item clipboard.Item, errMsg string) {
	pp.copyBtn.Hide()
	pp.label.Hide()
	pp.scroll.Hide()
	pp.rawText = ""

	info := fmt.Sprintf(
		"**Image Error**\n\nCopied: %s\nSize: ~%d bytes\n\n❌ %s",
		item.Timestamp.Format("2006-01-02 15:04:05"),
		item.Size(),
		errMsg,
	)

	pp.setMarkdown(info)
	pp.scroll.Show()
	pp.scroll.ScrollToTop()
	pp.image.Hide()
	pp.markRendered(item)
}

// clear clears the preview
func (pp *PreviewPane) clear() {
	pp.rawText = ""
	pp.copyBtn.Hide()
	pp.hasSelection = false
	pp.lastItemID = ""
	pp.showPlaceholder()
	pp.image.Hide()
	pp.label.Hide()
	// Clear caches
	pp.cachedImageData = ""
	pp.cachedImageResource = nil
	pp.cachedMarkdown = ""
}

func (pp *PreviewPane) showPlaceholder() {
	pp.label.Hide()
	pp.setMarkdown("_Select an item to preview..._")
	pp.scroll.Show()
	pp.scroll.ScrollToTop()
}

func (pp *PreviewPane) setMarkdown(markdown string) {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// Force update - don't use cache for HTML content
	pp.text.Segments = nil
	pp.text.ParseMarkdown(normalized)
	pp.text.Refresh()
}

func (pp *PreviewPane) newMarkdownText(markdown string) *widget.RichText {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	rt := widget.NewRichTextFromMarkdown(normalized)
	rt.Wrapping = fyne.TextWrapWord
	return rt
}

func (pp *PreviewPane) markRendered(item clipboard.Item) {
	pp.lastItemID = item.ID
	pp.lastItemType = item.Type
	pp.hasSelection = true
}

func layoutSpacer() fyne.CanvasObject {
	return container.NewHBox()
}
