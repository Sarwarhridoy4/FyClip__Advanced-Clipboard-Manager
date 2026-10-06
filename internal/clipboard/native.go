// File: internal/clipboard/native.go
package clipboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.design/x/clipboard"
)

// NativeClipboard handles platform-specific clipboard operations
type NativeClipboard struct {
	available bool
	useXclip  bool
	useWlclip bool
}

type limitedOutput struct {
	buf      bytes.Buffer
	maxBytes int
	overflow bool
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	remaining := w.maxBytes - w.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = w.buf.Write(p[:remaining])
		} else {
			_, _ = w.buf.Write(p)
		}
	}
	if len(p) > remaining {
		w.overflow = true
	}
	return len(p), nil
}

func runOutputLimited(cmd *exec.Cmd, maxBytes int) ([]byte, error) {
	out := &limitedOutput{maxBytes: maxBytes}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	if out.overflow {
		return nil, fmt.Errorf("clipboard data exceeds %d byte limit", maxBytes)
	}
	return out.buf.Bytes(), nil
}

// NewNativeClipboard initializes clipboard support
func NewNativeClipboard() (*NativeClipboard, error) {
	nc := &NativeClipboard{
		available: true,
	}

	err := clipboard.Init()
	if err != nil {
		if runtime.GOOS == "linux" {
			nc.setupLinuxFallback()
		} else {
			return nil, fmt.Errorf("clipboard initialization failed: %w", err)
		}
	}

	return nc, nil
}

// setupLinuxFallback configures alternative clipboard methods for Linux
func (nc *NativeClipboard) setupLinuxFallback() {
	sessionType := os.Getenv("XDG_SESSION_TYPE")

	if sessionType == "wayland" || sessionType == "" {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			nc.useWlclip = true
			log.Println("Using wl-clipboard for Wayland")
			return
		}
		log.Println("Warning: wl-clipboard not found for Wayland")
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		nc.useXclip = true
		log.Println("Using xclip for X11")
		return
	}

	log.Println("Warning: No clipboard tool found. Install xclip or wl-clipboard")
	nc.available = false
}

// ReadText reads text from clipboard
func (nc *NativeClipboard) ReadText() []byte {
	if !nc.available {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in ReadText: %v", r)
		}
	}()

	if nc.useWlclip {
		return nc.readTextWayland()
	} else if nc.useXclip {
		return nc.readTextX11()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := clipboard.Read(ctx, clipboard.FmtText)
	if err != nil {
		return nil
	}
	return data
}

// ReadImage reads image from clipboard
func (nc *NativeClipboard) ReadImage() ([]byte, string) {
	if !nc.available {
		return nil, ""
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in ReadImage: %v", r)
		}
	}()

	if nc.useWlclip {
		return nc.readImageWayland()
	} else if nc.useXclip {
		return nc.readImageX11()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	imgData, err := clipboard.Read(ctx, clipboard.FmtImage)
	if err != nil {
		return nil, ""
	}
	if len(imgData) > 0 {
		return imgData, detectImageType(imgData)
	}

	return nil, ""
}

// WriteText writes text to clipboard
func (nc *NativeClipboard) WriteText(data []byte) error {
	if !nc.available {
		return fmt.Errorf("clipboard not available")
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in WriteText: %v", r)
		}
	}()

	if nc.useWlclip {
		return nc.writeTextWayland(data)
	} else if nc.useXclip {
		return nc.writeTextX11(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := clipboard.Write(ctx, clipboard.FmtText, data)
	return err
}

// WriteImage writes image to clipboard
func (nc *NativeClipboard) WriteImage(base64Data string) error {
	if !nc.available {
		return fmt.Errorf("clipboard not available")
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in WriteImage: %v", r)
		}
	}()

	if nc.useWlclip {
		return nc.writeImageWayland(data)
	} else if nc.useXclip {
		return nc.writeImageX11(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = clipboard.Write(ctx, clipboard.FmtImage, data)
	return err
}

// ReadHTML reads HTML from clipboard
func (nc *NativeClipboard) ReadHTML() []byte {
	if !nc.available {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in ReadHTML: %v", r)
		}
	}()

	// Try using xclip for HTML (X11)
	if nc.useXclip {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "xclip", "-o", "-selection", "clipboard", "-t", "text/html")
		out, err := runOutputLimited(cmd, MaxContentSize)
		if err == nil && len(out) > 0 {
			return out
		}
	}

	// Try using wl-paste for HTML (Wayland)
	if nc.useWlclip {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "wl-paste", "-t", "text/html", "-n")
		out, err := runOutputLimited(cmd, MaxContentSize)
		if err == nil && len(out) > 0 {
			return out
		}
	}

	return nil
}

// WriteHTML writes HTML to clipboard with plain text fallback
func (nc *NativeClipboard) WriteHTML(htmlContent, plainText string) error {
	if !nc.available {
		return fmt.Errorf("clipboard not available")
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in WriteHTML: %v", r)
		}
	}()

	// First write plain text
	if err := nc.WriteText([]byte(plainText)); err != nil {
		return err
	}

	// For platforms that support HTML clipboard format, we'd need additional handling
	// This is a simplified implementation
	log.Printf("HTML write not fully implemented for this platform")
	return nil
}

// ReadFilePaths reads file paths from clipboard (files copied in file manager)
func (nc *NativeClipboard) ReadFilePaths() []string {
	if !nc.available {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in ReadFilePaths: %v", r)
		}
	}()

	// Try reading as text (some systems send file paths as text)
	textData := nc.ReadText()
	if len(textData) > 0 {
		text := SafeString(textData)
		// Check if it looks like a file path
		if isFilePath(text) {
			return []string{text}
		}
	}

	// Try using xclip for URI list (X11)
	if nc.useXclip {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "xclip", "-o", "-selection", "clipboard", "-t", "text/uri-list")
		out, err := runOutputLimited(cmd, MaxContentSize)
		if err == nil && len(out) > 0 {
			return parseURIList(SafeString(out))
		}
	}

	// Try using wl-paste for URI list (Wayland)
	if nc.useWlclip {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "wl-paste", "-t", "text/uri-list", "-n")
		out, err := runOutputLimited(cmd, MaxContentSize)
		if err == nil && len(out) > 0 {
			return parseURIList(SafeString(out))
		}
	}

	return nil
}

// parseURIList parses URI list (file:// URLs) into file paths
func parseURIList(uriList string) []string {
	var paths []string
	lines := strings.Split(strings.TrimSpace(uriList), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "file://") {
			path := strings.TrimPrefix(line, "file://")
			// Handle URL-encoded characters
			path = strings.ReplaceAll(path, "%20", " ")
			path = strings.ReplaceAll(path, "%5C", "\\")
			path = strings.ReplaceAll(path, "%2F", "/")
			paths = append(paths, path)
		}
	}
	return paths
}

// isFilePath checks if a string looks like a file path
func isFilePath(s string) bool {
	s = strings.TrimSpace(s)
	// Check for common path indicators
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
		return true
	}
	// Check for Windows-style paths
	if len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	return false
}

// WriteFilePaths writes file paths to clipboard
func (nc *NativeClipboard) WriteFilePaths(paths []string) error {
	if !nc.available {
		return fmt.Errorf("clipboard not available")
	}

	var uriList strings.Builder
	for _, path := range paths {
		escaped := strings.ReplaceAll(path, " ", "%20")
		fmt.Fprint(&uriList, "file://", escaped, "\n")
	}

	if err := nc.WriteText([]byte(uriList.String())); err != nil {
		return err
	}

	log.Printf("File paths write not fully implemented for this platform")
	return nil
}

// Linux Wayland implementations
func (nc *NativeClipboard) readTextWayland() []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wl-paste", "-n")
	out, err := runOutputLimited(cmd, MaxContentSize)
	if err != nil {
		return nil
	}
	return out
}

func (nc *NativeClipboard) readImageWayland() ([]byte, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wl-paste", "-t", "image/png", "-n")
	out, err := runOutputLimited(cmd, MaxImageSize)
	if err == nil && len(out) > 0 {
		return out, "png"
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, "wl-paste", "-t", "image/jpeg", "-n")
	out, err = runOutputLimited(cmd2, MaxImageSize)
	if err == nil && len(out) > 0 {
		return out, "jpeg"
	}

	return nil, ""
}

func (nc *NativeClipboard) writeTextWayland(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wl-copy")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func (nc *NativeClipboard) writeImageWayland(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wl-copy", "--type", "image/png")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

// Linux X11 implementations
func (nc *NativeClipboard) readTextX11() []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xclip", "-o", "-selection", "clipboard")
	out, err := runOutputLimited(cmd, MaxContentSize)
	if err != nil {
		return nil
	}
	return out
}

func (nc *NativeClipboard) readImageX11() ([]byte, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "image/png", "-o")
	out, err := runOutputLimited(cmd, MaxImageSize)
	if err == nil && len(out) > 0 {
		return out, "png"
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, "xclip", "-selection", "clipboard", "-t", "image/jpeg", "-o")
	out, err = runOutputLimited(cmd2, MaxImageSize)
	if err == nil && len(out) > 0 {
		return out, "jpeg"
	}

	return nil, ""
}

func (nc *NativeClipboard) writeTextX11(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xclip", "-i", "-selection", "clipboard")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func (nc *NativeClipboard) writeImageX11(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "image/png", "-i")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

// detectImageType detects the image type from raw data
func detectImageType(data []byte) string {
	if len(data) < 8 {
		return "png"
	}

	// PNG signature
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "png"
	}

	// JPEG signature
	if data[0] == 0xFF && data[1] == 0xD8 {
		return "jpeg"
	}

	return "png"
}

// IsAvailable returns whether clipboard is available
func (nc *NativeClipboard) IsAvailable() bool {
	return nc.available
}

// Backend returns which mechanism is currently being used on Linux.
func (nc *NativeClipboard) Backend() string {
	if nc.useWlclip {
		return "wl-clipboard"
	}
	if nc.useXclip {
		return "xclip"
	}
	return "native"
}

// getFileInfo returns FileInfo for a given file path
func getFileInfo(path string) (*FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Name:        info.Name(),
		Path:        path,
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		IsDirectory: info.IsDir(),
	}, nil
}
