// File: internal/update/checker.go
package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/logger"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt string        `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a release asset
type GitHubAsset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"browser_download_url"`
	ContentType string `json:"content_type"`
	Digest      string `json:"digest"`
}

type assetCandidate struct {
	asset GitHubAsset
	score int
	index int
}

// UpdateInfo contains information about available updates
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	ReleaseNotes   string
	DownloadURL    string
	AssetName      string
	AssetSize      int64
	IsPrerelease   bool
	ExpectedHash   string
}

// Cache entry for update check responses
type cacheEntry struct {
	updateInfo *UpdateInfo
	cachedAt   time.Time
	expiresAt  time.Time
}

// Rate limit tracker for API requests
type rateLimitTracker struct {
	requests    []time.Time
	lastCleanup time.Time
	mu          sync.Mutex
}

// Update check caching constants
const (
	updateCacheDuration        = 1 * time.Hour   // Cache successful responses for 1 hour
	updateCacheMaxEntries      = 10              // Maximum cache entries
	updateRateLimitWindow      = 5 * time.Minute // Rate limit window
	updateMaxRequestsPerWindow = 5               // Max requests per window
)

var (
	safeAssetNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

func validateAssetName(name string) error {
	if name == "" {
		return fmt.Errorf("asset name is empty")
	}
	if !safeAssetNameRegex.MatchString(name) {
		return fmt.Errorf("asset name contains invalid characters: %s", name)
	}
	return nil
}

func validatePathInTemp(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	absTemp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return fmt.Errorf("failed to resolve temp directory: %w", err)
	}
	if !strings.HasPrefix(absPath, absTemp+string(filepath.Separator)) && absPath != absTemp {
		return fmt.Errorf("path is outside temp directory: %s", absPath)
	}
	return nil
}

func validateCommandExists(cmd string) error {
	path, err := exec.LookPath(cmd)
	if err != nil {
		return fmt.Errorf("command not found: %s", cmd)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve command path: %w", err)
	}
	if info, err := os.Stat(absPath); err != nil || info.IsDir() {
		return fmt.Errorf("command is not an executable file: %s", absPath)
	}
	return nil
}

func computeFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute file hash: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Global cache and rate limiter (shared across all checkers)
var (
	updateCache   = make(map[string]*cacheEntry)
	updateCacheMu sync.RWMutex
	rateLimiter   = &rateLimitTracker{}
)

// Checker handles update checking functionality
type Checker struct {
	owner           string
	repo            string
	currentVersion  string
	httpClient      *http.Client
	downloadTimeout time.Duration
	log             *logger.Logger
	cacheEnabled    bool // Enable/disable caching for this checker
}

// Option is a functional option for Checker
type Option func(*Checker)

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) Option {
	return func(c *Checker) {
		c.httpClient = client
	}
}

// WithDownloadTimeout sets the download timeout
func WithDownloadTimeout(timeout time.Duration) Option {
	return func(c *Checker) {
		c.downloadTimeout = timeout
	}
}

// WithLogger sets a custom logger
func WithLogger(log *logger.Logger) Option {
	return func(c *Checker) {
		c.log = log
	}
}

// WithCache enables or disables caching for update checks
func WithCache(enabled bool) Option {
	return func(c *Checker) {
		c.cacheEnabled = enabled
	}
}

// NewChecker creates a new update checker
func NewChecker(owner, repo, currentVersion string, opts ...Option) *Checker {
	c := &Checker{
		owner:           owner,
		repo:            repo,
		currentVersion:  currentVersion,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		downloadTimeout: 5 * time.Minute,
		log:             logger.Get(),
		cacheEnabled:    true, // Enable caching by default
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// getCacheKey generates a cache key for the current checker configuration
func (c *Checker) getCacheKey() string {
	key := fmt.Sprintf("%s/%s", c.owner, c.repo)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:])
}

// checkCache looks up a cached update check result
func (c *Checker) checkCache() (*UpdateInfo, bool) {
	if !c.cacheEnabled {
		return nil, false
	}

	updateCacheMu.RLock()
	defer updateCacheMu.RUnlock()

	cacheKey := c.getCacheKey()
	entry, exists := updateCache[cacheKey]
	if !exists {
		return nil, false
	}

	// Check if cache entry is still valid
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	c.log.Debug("Using cached update check result")
	return entry.updateInfo, true
}

// storeCache saves an update check result to cache
func (c *Checker) storeCache(updateInfo *UpdateInfo) {
	if !c.cacheEnabled {
		return
	}

	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	cacheKey := c.getCacheKey()
	now := time.Now()

	// Clean up expired entries if cache is getting full
	if len(updateCache) >= updateCacheMaxEntries {
		c.cleanupExpiredCache(now)
	}

	entry := &cacheEntry{
		updateInfo: updateInfo,
		cachedAt:   now,
		expiresAt:  now.Add(updateCacheDuration),
	}

	updateCache[cacheKey] = entry
	c.log.Debug("Cached update check result")
}

// cleanupExpiredCache removes expired cache entries
func (c *Checker) cleanupExpiredCache(now time.Time) {
	for key, entry := range updateCache {
		if now.After(entry.expiresAt) {
			delete(updateCache, key)
		}
	}
}

// checkRateLimit checks if the request should be rate limited
func (c *Checker) checkRateLimit() error {
	rateLimiter.mu.Lock()
	defer rateLimiter.mu.Unlock()

	now := time.Now()

	// Clean up old requests outside the window
	windowStart := now.Add(-updateRateLimitWindow)
	validRequests := make([]time.Time, 0, len(rateLimiter.requests))
	for _, reqTime := range rateLimiter.requests {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}
	rateLimiter.requests = validRequests

	// Check if we're over the limit
	if len(rateLimiter.requests) >= updateMaxRequestsPerWindow {
		// Calculate when the next request will be allowed
		oldestRequest := rateLimiter.requests[0]
		nextAllowed := oldestRequest.Add(updateRateLimitWindow)
		waitTime := nextAllowed.Sub(now)

		if waitTime > 0 {
			return fmt.Errorf("rate limit exceeded, try again in %v", waitTime.Round(time.Second))
		}
	}

	// Add this request to the tracker
	rateLimiter.requests = append(rateLimiter.requests, now)
	return nil
}

// CheckForUpdate checks if a new version is available
func (c *Checker) CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	c.log.Info("Checking for updates...")

	// Check cache first
	if cachedResult, found := c.checkCache(); found {
		return cachedResult, nil
	}

	// Apply rate limiting
	if err := c.checkRateLimit(); err != nil {
		c.log.Warn(fmt.Sprintf("Update check rate limited: %v", err))
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", c.owner, c.repo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "FyClip-Update-Checker")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if len(body) > 2<<20 {
		return nil, fmt.Errorf("GitHub release response exceeds size limit")
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	// Find the appropriate asset for the current platform
	asset := c.findAsset(release.Assets)
	if asset == nil {
		return nil, fmt.Errorf("no compatible asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	releaseURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", c.owner, c.repo, release.TagName)

	updateInfo := &UpdateInfo{
		CurrentVersion: c.currentVersion,
		LatestVersion:  strings.TrimPrefix(release.TagName, "v"),
		ReleaseURL:     releaseURL,
		ReleaseNotes:   release.Body,
		DownloadURL:    asset.DownloadURL,
		AssetName:      asset.Name,
		AssetSize:      asset.Size,
		IsPrerelease:   release.Prerelease,
		ExpectedHash:   strings.TrimPrefix(asset.Digest, "sha256:"),
	}

	c.log.Info(fmt.Sprintf("Current version: %s, Latest version: %s", c.currentVersion, updateInfo.LatestVersion))

	// Cache successful result
	c.storeCache(updateInfo)

	return updateInfo, nil
}

// findAsset finds the appropriate asset for the current platform
func (c *Checker) findAsset(assets []GitHubAsset) *GitHubAsset {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map runtime GOOS to asset naming conventions
	osMap := map[string][]string{
		"linux":   {"linux", "Linux", "deb", "AppImage"},
		"windows": {"windows", "Windows", "exe", "msi"},
		"darwin":  {"darwin", "Darwin", "macos", "macOS", "dmg"},
	}

	archMap := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64"},
		"arm64": {"arm64", "aarch64", "arm64"},
		"386":   {"386", "i386", "i686"},
	}

	candidates := c.collectAssetCandidates(assets, osMap[goos], archMap[goarch], goos)
	if len(candidates) == 0 {
		return nil
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.score > best.score {
			best = candidate
			continue
		}
		if candidate.score == best.score && candidate.index < best.index {
			best = candidate
		}
	}

	asset := best.asset
	return &asset
}

func (c *Checker) collectAssetCandidates(assets []GitHubAsset, osAliases, archAliases []string, goos string) []assetCandidate {
	if len(assets) == 0 {
		return nil
	}

	results := make(chan assetCandidate, len(assets))
	var waitGroup sync.WaitGroup

	for index, asset := range assets {
		waitGroup.Add(1)
		go func(idx int, current GitHubAsset) {
			defer waitGroup.Done()
			score := scoreAssetForPlatform(current, osAliases, archAliases, goos)
			if score < 0 {
				return
			}
			results <- assetCandidate{
				asset: current,
				score: score,
				index: idx,
			}
		}(index, asset)
	}

	go func() {
		waitGroup.Wait()
		close(results)
	}()

	candidates := make([]assetCandidate, 0, len(assets))
	for candidate := range results {
		candidates = append(candidates, candidate)
	}

	return candidates
}

func scoreAssetForPlatform(asset GitHubAsset, osAliases, archAliases []string, goos string) int {
	name := strings.ToLower(asset.Name)
	if name == "" || asset.DownloadURL == "" {
		return -1
	}

	if !containsAnyAlias(name, osAliases) || !containsAnyAlias(name, archAliases) {
		return -1
	}

	score := 100

	preferredExt := map[string]string{
		"linux":   ".deb",
		"windows": ".exe",
		"darwin":  ".dmg",
	}
	if ext := preferredExt[goos]; ext != "" && strings.HasSuffix(name, ext) {
		score += 50
	}

	secondaryExts := map[string][]string{
		"linux":   {".appimage", ".rpm", ".tar.gz", ".zip"},
		"windows": {".msi", ".zip"},
		"darwin":  {".pkg", ".zip"},
	}
	for _, ext := range secondaryExts[goos] {
		if strings.HasSuffix(name, ext) {
			score += 25
			break
		}
	}

	if asset.Size > 0 {
		score += 5
	}

	if asset.ContentType != "" {
		score += 5
	}

	return score
}

func containsAnyAlias(name string, aliases []string) bool {
	for _, alias := range aliases {
		if strings.Contains(name, strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

// IsUpdateAvailable checks if an update is available
func (c *Checker) IsUpdateAvailable(ctx context.Context) (bool, error) {
	updateInfo, err := c.CheckForUpdate(ctx)
	if err != nil {
		return false, err
	}

	return CompareVersions(updateInfo.LatestVersion, c.currentVersion) > 0, nil
}

// ClearCache clears the update check cache for this checker
func (c *Checker) ClearCache() {
	if !c.cacheEnabled {
		return
	}

	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	cacheKey := c.getCacheKey()
	delete(updateCache, cacheKey)
	c.log.Debug("Cleared update check cache")
}

// GetCacheStats returns cache statistics for debugging
func GetCacheStats() (entries int, rateLimitRequests int) {
	updateCacheMu.RLock()
	entries = len(updateCache)
	updateCacheMu.RUnlock()

	rateLimiter.mu.Lock()
	rateLimitRequests = len(rateLimiter.requests)
	rateLimiter.mu.Unlock()

	return entries, rateLimitRequests
}

// CompareVersions compares two semantic versions
// Returns: 1 if v1 > v2, 0 if v1 == v2, -1 if v1 < v2
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Handle "dev" version - treat it as equal to any version
	// This prevents showing "update available" when running development builds
	if v1 == "dev" || v2 == "dev" {
		return 0
	}

	// Handle pre-release versions: 1.0.0-beta < 1.0.0
	v1IsPrerelease := strings.Contains(v1, "-")
	v2IsPrerelease := strings.Contains(v2, "-")

	// If both are pre-release or both are not, compare normally
	// If one is pre-release and other is not, non-prerelease is greater
	if v1IsPrerelease && !v2IsPrerelease {
		return -1
	}
	if !v1IsPrerelease && v2IsPrerelease {
		return 1
	}

	// Strip pre-release suffix for numeric comparison
	if v1IsPrerelease {
		v1 = strings.Split(v1, "-")[0]
	}
	if v2IsPrerelease {
		v2 = strings.Split(v2, "-")[0]
	}

	v1Parts := strings.Split(v1, ".")
	v2Parts := strings.Split(v2, ".")

	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(v1Parts) {
			fmt.Sscanf(v1Parts[i], "%d", &n1)
		}
		if i < len(v2Parts) {
			fmt.Sscanf(v2Parts[i], "%d", &n2)
		}

		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	return 0
}

// Downloader handles downloading and installing updates
type Downloader struct {
	checker      *Checker
	updateInfo   *UpdateInfo
	downloadPath string
	downloadDir  string
	log          *logger.Logger
}

// NewDownloader creates a new downloader
func NewDownloader(checker *Checker, updateInfo *UpdateInfo) (*Downloader, error) {
	if checker == nil || updateInfo == nil {
		return nil, fmt.Errorf("checker and update info are required")
	}
	if err := validateAssetName(updateInfo.AssetName); err != nil {
		return nil, fmt.Errorf("invalid asset name: %w", err)
	}
	u, err := url.Parse(updateInfo.DownloadURL)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") || u.User != nil {
		return nil, fmt.Errorf("download URL must be an HTTPS GitHub URL")
	}
	downloadDir, err := os.MkdirTemp("", "fyclip-update-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create private download directory: %w", err)
	}
	downloadPath := filepath.Join(downloadDir, updateInfo.AssetName)
	if err := validatePathInTemp(downloadPath); err != nil {
		_ = os.RemoveAll(downloadDir)
		return nil, fmt.Errorf("invalid download path: %w", err)
	}
	return &Downloader{
		checker:      checker,
		updateInfo:   updateInfo,
		downloadPath: downloadPath,
		downloadDir:  downloadDir,
		log:          logger.Get(),
	}, nil
}

// Download downloads the update asset
func (d *Downloader) Download(ctx context.Context, progressFunc func(downloaded, total int64)) (retErr error) {
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(d.downloadDir)
		}
	}()
	d.log.Info(fmt.Sprintf("Downloading %s...", d.updateInfo.AssetName))

	req, err := http.NewRequestWithContext(ctx, "GET", d.updateInfo.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "FyClip-Update-Downloader")

	resp, err := d.checker.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}
	finalURL := resp.Request.URL
	if finalURL == nil || finalURL.Scheme != "https" || !allowedAssetHost(finalURL.Hostname()) {
		return fmt.Errorf("download redirected to an untrusted URL")
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 {
		totalSize = d.updateInfo.AssetSize
	}

	// Create file
	file, err := os.OpenFile(d.downloadPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download with progress
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if d.updateInfo.AssetSize > 0 && downloaded+int64(n) > d.updateInfo.AssetSize {
				return fmt.Errorf("download exceeds declared asset size")
			}
			if downloaded+int64(n) > 1<<30 {
				return fmt.Errorf("download exceeds 1 GiB limit")
			}
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("failed to write: %w", writeErr)
			}
			downloaded += int64(n)
			if progressFunc != nil {
				progressFunc(downloaded, totalSize)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read error: %w", err)
		}
	}
	if err := d.Verify(); err != nil {
		return err
	}

	d.log.Info("Download complete")
	return nil
}

func allowedAssetHost(host string) bool {
	switch strings.ToLower(host) {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	default:
		return false
	}
}

// GetDownloadPath returns the path to the downloaded file
func (d *Downloader) GetDownloadPath() string {
	return d.downloadPath
}

// ExpectedHash returns the SHA-256 digest advertised by GitHub for this asset.
func (d *Downloader) ExpectedHash() string { return d.updateInfo.ExpectedHash }

// Verify verifies the downloaded file integrity
func (d *Downloader) Verify() error {
	if d.downloadPath == "" {
		return fmt.Errorf("no download path set")
	}

	hash, err := computeFileHash(d.downloadPath)
	if err != nil {
		return err
	}

	d.log.Info(fmt.Sprintf("Downloaded file SHA-256: %s", hash))

	if len(d.updateInfo.ExpectedHash) != sha256.Size*2 {
		return fmt.Errorf("release does not provide a trusted SHA-256 digest; refusing to install")
	}
	if _, err := hex.DecodeString(d.updateInfo.ExpectedHash); err != nil {
		return fmt.Errorf("invalid expected SHA-256 digest")
	}
	if !strings.EqualFold(hash, d.updateInfo.ExpectedHash) {
		return fmt.Errorf("downloaded asset SHA-256 does not match release digest")
	}
	d.log.Info("Hash verification passed")

	return nil
}

// Installer handles installing the update
type Installer struct {
	downloadPath string
	appName      string
	expectedHash string
	log          *logger.Logger
	output       strings.Builder
}

// NewInstaller creates a new installer
func NewInstaller(downloadPath, appName string, expectedHash ...string) *Installer {
	i := &Installer{
		downloadPath: downloadPath,
		appName:      appName,
		log:          logger.Get(),
	}
	if len(expectedHash) > 0 {
		i.expectedHash = expectedHash[0]
	}
	return i
}

// GetOutput returns the captured installation output
func (i *Installer) GetOutput() string {
	return i.output.String()
}

// Install installs the update based on the platform and file type
func (i *Installer) Install() error {
	filename := filepath.Base(i.downloadPath)
	ext := strings.ToLower(filepath.Ext(filename))

	if err := validatePathInTemp(i.downloadPath); err != nil {
		return fmt.Errorf("invalid download path: %w", err)
	}

	hash, err := computeFileHash(i.downloadPath)
	if err != nil {
		return fmt.Errorf("failed to verify download: %w", err)
	}
	i.log.Info(fmt.Sprintf("Downloaded file SHA-256: %s", hash))
	if len(i.expectedHash) != sha256.Size*2 || !strings.EqualFold(hash, i.expectedHash) {
		return fmt.Errorf("downloaded asset does not match the trusted release SHA-256 digest")
	}

	i.log.Info(fmt.Sprintf("Installing %s...", filename))

	switch runtime.GOOS {
	case "linux":
		return i.installLinux(ext)
	case "windows":
		return i.installWindows(ext)
	case "darwin":
		return i.installDarwin(ext)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// installLinux handles Linux installation
func (i *Installer) installLinux(ext string) error {
	switch ext {
	case ".deb":
		return i.installDeb()
	case ".AppImage", ".appimage":
		return i.installAppImage()
	default:
		return fmt.Errorf("unsupported Linux package format: %s", ext)
	}
}

// installDeb installs a .deb package
func (i *Installer) installDeb() error {
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pkexec"); err == nil {
		if err := validateCommandExists("pkexec"); err != nil {
			return err
		}
		cmd = exec.Command("pkexec", "dpkg", "-i", i.downloadPath)
	} else {
		if err := validateCommandExists("sudo"); err != nil {
			return err
		}
		cmd = exec.Command("sudo", "dpkg", "-i", i.downloadPath)
	}
	if err := validateCommandExists("dpkg"); err != nil {
		return err
	}
	cmd.Stdout = &i.output
	cmd.Stderr = &i.output

	i.log.Info("Installing .deb package with dpkg...")
	err := cmd.Run()
	if err != nil {
		i.log.Error(fmt.Sprintf("dpkg installation failed: %v", err))
		i.log.Error(fmt.Sprintf("Installation output: %s", i.output.String()))
	}
	return err
}

// installAppImage makes an AppImage executable
func (i *Installer) installAppImage() error {
	// Make executable
	err := os.Chmod(i.downloadPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	i.log.Info("AppImage is ready to run. You can execute it directly.")
	i.log.Info(fmt.Sprintf("Location: %s", i.downloadPath))

	return nil
}

// installWindows handles Windows installation
func (i *Installer) installWindows(ext string) error {
	switch ext {
	case ".exe":
		return i.installExe()
	case ".msi":
		return i.installMsi()
	default:
		return fmt.Errorf("unsupported Windows package format: %s", ext)
	}
}

// installExe runs the installer executable
func (i *Installer) installExe() error {
	if err := validatePathInTemp(i.downloadPath); err != nil {
		return fmt.Errorf("invalid installer path: %w", err)
	}
	cmd := exec.Command(i.downloadPath, "/S") // Silent install
	cmd.Stdout = &i.output
	cmd.Stderr = &i.output

	i.log.Info("Running installer...")
	err := cmd.Run()
	if err != nil {
		i.log.Error(fmt.Sprintf("Installer execution failed: %v", err))
		i.log.Error(fmt.Sprintf("Installation output: %s", i.output.String()))
	}
	return err
}

// installMsi installs an MSI package
func (i *Installer) installMsi() error {
	if err := validatePathInTemp(i.downloadPath); err != nil {
		return fmt.Errorf("invalid installer path: %w", err)
	}
	if err := validateCommandExists("msiexec"); err != nil {
		return err
	}
	cmd := exec.Command("msiexec", "/i", i.downloadPath, "/quiet")
	cmd.Stdout = &i.output
	cmd.Stderr = &i.output

	i.log.Info("Installing MSI package...")
	err := cmd.Run()
	if err != nil {
		i.log.Error(fmt.Sprintf("MSI installation failed: %v", err))
		i.log.Error(fmt.Sprintf("Installation output: %s", i.output.String()))
	}
	return err
}

// installDarwin handles macOS installation
func (i *Installer) installDarwin(ext string) error {
	switch ext {
	case ".dmg":
		return i.installDmg()
	case ".zip":
		return i.installZip()
	default:
		return fmt.Errorf("unsupported macOS package format: %s", ext)
	}
}

// installDmg mounts and installs from DMG
func (i *Installer) installDmg() error {
	if err := validatePathInTemp(i.downloadPath); err != nil {
		return fmt.Errorf("invalid download path: %w", err)
	}
	if err := validateCommandExists("hdiutil"); err != nil {
		return err
	}
	if err := validateCommandExists("cp"); err != nil {
		return err
	}

	// Mount DMG
	mountCmd := exec.Command("hdiutil", "attach", i.downloadPath, "-nobrowse")
	mountOut, err := mountCmd.Output()
	if err != nil {
		i.output.WriteString(fmt.Sprintf("Failed to mount DMG: %v\n", err))
		return fmt.Errorf("failed to mount DMG: %w", err)
	}

	// Parse mount point from output
	mountPoint := strings.TrimSpace(string(mountOut))
	i.log.Info(fmt.Sprintf("DMG mounted at: %s", mountPoint))
	i.output.WriteString(fmt.Sprintf("DMG mounted at: %s\n", mountPoint))

	// Find .app in mounted volume
	appPath := filepath.Join(mountPoint, i.appName+".app")
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		files, _ := filepath.Glob(filepath.Join(mountPoint, "*.app"))
		if len(files) > 0 {
			appPath = files[0]
		}
	}

	// Validate appPath is within the mounted volume
	if !strings.HasPrefix(appPath, mountPoint) {
		return fmt.Errorf("app path is outside mount point: %s", appPath)
	}

	// Copy to Applications
	if _, err := os.Stat(appPath); err == nil {
		copyCmd := exec.Command("cp", "-R", appPath, "/Applications/")
		copyCmd.Stdout = &i.output
		copyCmd.Stderr = &i.output
		if err := copyCmd.Run(); err != nil {
			i.output.WriteString(fmt.Sprintf("Failed to copy to Applications: %v\n", err))
			return fmt.Errorf("failed to copy to Applications: %w", err)
		}
		i.log.Info("Application installed to /Applications/")
		i.output.WriteString("Application installed to /Applications/\n")
	}

	// Unmount DMG
	detachCmd := exec.Command("hdiutil", "detach", mountPoint)
	detachCmd.Stdout = &i.output
	detachCmd.Stderr = &i.output
	detachCmd.Run()

	return nil
}

// installZip extracts and installs from ZIP
func (i *Installer) installZip() error {
	if err := validatePathInTemp(i.downloadPath); err != nil {
		return fmt.Errorf("invalid download path: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "fyclip-extract-*")
	if err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := extractZipSafely(i.downloadPath, tmpDir); err != nil {
		return err
	}

	// Find .app
	appPath := filepath.Join(tmpDir, i.appName+".app")
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		files, _ := filepath.Glob(filepath.Join(tmpDir, "*.app"))
		if len(files) > 0 {
			appPath = files[0]
		}
	}

	// Validate appPath is within tmpDir
	if !strings.HasPrefix(appPath, tmpDir) {
		return fmt.Errorf("app path is outside temp directory: %s", appPath)
	}

	// Copy to Applications
	if _, err := os.Stat(appPath); err == nil {
		copyCmd := exec.Command("cp", "-R", appPath, "/Applications/")
		copyCmd.Stdout = &i.output
		copyCmd.Stderr = &i.output
		if err := copyCmd.Run(); err != nil {
			i.output.WriteString(fmt.Sprintf("Failed to copy to Applications: %v\n", err))
			return fmt.Errorf("failed to copy to Applications: %w", err)
		}
		i.log.Info("Application installed to /Applications/")
		i.output.WriteString("Application installed to /Applications/\n")
	}

	return nil
}

func extractZipSafely(archivePath, destination string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open update archive: %w", err)
	}
	defer r.Close()
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	var total uint64
	for _, f := range r.File {
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("update archive contains a symbolic link")
		}
		entry := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(entry) || filepath.VolumeName(entry) != "" {
			return fmt.Errorf("update archive contains an absolute path")
		}
		target := filepath.Join(root, entry)
		relative, err := filepath.Rel(root, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("update archive contains an unsafe path")
		}
		if f.UncompressedSize64 > (1<<30)-total {
			return fmt.Errorf("update archive exceeds 1 GiB expanded limit")
		}
		total += f.UncompressedSize64
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.CopyN(out, src, int64(f.UncompressedSize64))
		closeErr := out.Close()
		srcErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if srcErr != nil {
			return srcErr
		}
	}
	return nil
}

// AutoUpdater provides a complete update workflow
type AutoUpdater struct {
	checker     *Checker
	downloadDir string
	log         *logger.Logger
}

// NewAutoUpdater creates a new auto updater
func NewAutoUpdater(owner, repo, currentVersion string, opts ...Option) *AutoUpdater {
	checker := NewChecker(owner, repo, currentVersion, opts...)
	return &AutoUpdater{
		checker:     checker,
		downloadDir: os.TempDir(),
		log:         logger.Get(),
	}
}

// CheckAndDownload checks for updates and downloads if available
func (a *AutoUpdater) CheckAndDownload(ctx context.Context) (*UpdateInfo, *Downloader, error) {
	updateInfo, err := a.checker.CheckForUpdate(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Check if update is available
	if CompareVersions(updateInfo.LatestVersion, updateInfo.CurrentVersion) <= 0 {
		a.log.Info("No update available")
		return updateInfo, nil, nil
	}

	a.log.Info(fmt.Sprintf("Update available: %s -> %s", updateInfo.CurrentVersion, updateInfo.LatestVersion))

	// Download the update
	downloader, err := NewDownloader(a.checker, updateInfo)
	if err != nil {
		return nil, nil, err
	}
	if err := downloader.Download(ctx, nil); err != nil {
		return nil, nil, err
	}

	return updateInfo, downloader, nil
}

// CheckAndInstall checks for updates, downloads and installs
func (a *AutoUpdater) CheckAndInstall(ctx context.Context, appName string) error {
	_, downloader, err := a.CheckAndDownload(ctx)
	if err != nil {
		return err
	}

	if downloader == nil {
		a.log.Info("Already on latest version")
		return nil
	}

	// Install the update
	installer := NewInstaller(downloader.GetDownloadPath(), appName, downloader.updateInfo.ExpectedHash)
	if err := installer.Install(); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(downloader.GetDownloadPath()), ".AppImage") {
		_ = os.RemoveAll(downloader.downloadDir)
	}

	a.log.Info("Update installed successfully")
	return nil
}

// ParseRepoFromURL parses owner and repo from a GitHub URL
func ParseRepoFromURL(githubURL string) (owner, repo string, err error) {
	// Handle both HTTPS and SSH URLs
	// https://github.com/owner/repo
	// git@github.com:owner/repo.git

	githubURL = strings.TrimSuffix(githubURL, ".git")
	githubURL = strings.TrimPrefix(githubURL, "git@github.com:")
	githubURL = strings.TrimPrefix(githubURL, "https://github.com/")

	parts := strings.Split(githubURL, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid GitHub URL: %s", githubURL)
	}

	return parts[0], parts[1], nil
}

// ParseRepoFromModule parses owner and repo from go.mod module name
func ParseRepoFromModule(moduleName string) (owner, repo string, err error) {
	// Remove https:// or git@ prefix
	moduleName = strings.TrimPrefix(moduleName, "https://")
	moduleName = strings.TrimPrefix(moduleName, "git@github.com:")
	moduleName = strings.TrimSuffix(moduleName, ".git")

	parts := strings.Split(moduleName, "/")
	if len(parts) < 3 {
		// Could be just owner/repo
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
		return "", "", fmt.Errorf("invalid module name: %s", moduleName)
	}

	// Find the position where owner/repo starts
	// Usually it's the last two parts for github.com
	owner = parts[len(parts)-2]
	repo = parts[len(parts)-1]

	return owner, repo, nil
}

// GetAssetDownloadURL constructs a direct download URL for a specific release
func GetAssetDownloadURL(owner, repo, version, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, version, assetName)
}

// ValidateDownloadURL validates if a download URL is accessible
func ValidateDownloadURL(downloadURL string) error {
	resp, err := http.Head(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("URL returned status %d", resp.StatusCode)
	}

	return nil
}

// GetLatestReleaseTag gets the latest release tag from GitHub
func GetLatestReleaseTag(ctx context.Context, owner, repo string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

// URLEncode encodes a string for use in URLs
func URLEncode(s string) string {
	return url.QueryEscape(s)
}
