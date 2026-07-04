package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// UpdateConfig controls auto-update behavior.
type UpdateConfig struct {
	// Enabled controls whether auto-update is active.
	Enabled bool
	// Channel is the release channel: "stable", "beta", or "nightly".
	Channel string
	// CheckInterval is how often to check for updates.
	CheckInterval time.Duration
	// GitHubRepo is the repository to check for releases.
	GitHubRepo string
	// AutoRestart controls whether to restart after update.
	AutoRestart bool
}

// DefaultUpdateConfig returns the default update configuration.
func DefaultUpdateConfig() UpdateConfig {
	return UpdateConfig{
		Enabled:       true,
		Channel:       "stable",
		CheckInterval: 24 * time.Hour,
		GitHubRepo:    "gbudjeakp/run-right",
		AutoRestart:   false, // Manual restart by default for safety
	}
}

// UpdateInfo contains information about an available update.
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseURL     string `json:"release_url"`
	DownloadURL    string `json:"download_url"`
	ReleaseNotes   string `json:"release_notes,omitempty"`
	PublishedAt    string `json:"published_at"`
	UpdateAvailable bool   `json:"update_available"`
}

// GitHubRelease represents a GitHub release.
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
	HTMLURL     string `json:"html_url"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// CheckForUpdate checks if a newer version is available.
func CheckForUpdate(cfg UpdateConfig) (*UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	release, err := fetchLatestRelease(ctx, cfg.GitHubRepo, cfg.Channel)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion: Version,
		LatestVersion:  strings.TrimPrefix(release.TagName, "v"),
		ReleaseURL:     release.HTMLURL,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt,
	}

	// Find the appropriate asset for this platform
	assetName := buildAssetName()
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			info.DownloadURL = asset.BrowserDownloadURL
			break
		}
	}

	// Compare versions
	info.UpdateAvailable = isNewerVersion(Version, info.LatestVersion)

	return info, nil
}

// fetchLatestRelease gets the latest release from GitHub.
func fetchLatestRelease(ctx context.Context, repo, channel string) (*GitHubRelease, error) {
	var url string
	if channel == "stable" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	} else {
		// For beta/nightly, get all releases and filter
		url = fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "runright-updater/"+Version)

	// Use GitHub token if available (for higher rate limits)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	if channel == "stable" {
		var release GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil, err
		}
		return &release, nil
	}

	// For non-stable channels, parse the releases list
	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	for _, r := range releases {
		switch channel {
		case "beta":
			// Include prereleases but not nightlies
			if r.Prerelease && !strings.Contains(r.TagName, "nightly") {
				return &r, nil
			}
		case "nightly":
			// Include any nightly release
			if strings.Contains(r.TagName, "nightly") {
				return &r, nil
			}
		}
	}

	// Fall back to latest stable if channel-specific not found
	for _, r := range releases {
		if !r.Prerelease {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("no releases found for channel %s", channel)
}

// buildAssetName constructs the expected asset filename for this platform.
func buildAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map GOARCH to common naming conventions
	switch arch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	case "386":
		arch = "386"
	}

	return fmt.Sprintf("runright_%s_%s.tar.gz", os, arch)
}

// isNewerVersion compares two semver strings.
func isNewerVersion(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	if current == "dev" || current == "" {
		return true // Dev builds always show update available
	}

	currentParts := strings.Split(current, ".")
	latestParts := strings.Split(latest, ".")

	for i := 0; i < 3; i++ {
		var cv, lv int
		if i < len(currentParts) {
			fmt.Sscanf(currentParts[i], "%d", &cv)
		}
		if i < len(latestParts) {
			fmt.Sscanf(latestParts[i], "%d", &lv)
		}
		if lv > cv {
			return true
		}
		if cv > lv {
			return false
		}
	}

	return false
}

// SelfUpdate downloads and installs the latest version.
func SelfUpdate(cfg UpdateConfig) error {
	info, err := CheckForUpdate(cfg)
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}

	if !info.UpdateAvailable {
		return nil // Already up to date
	}

	if info.DownloadURL == "" {
		return fmt.Errorf("no download available for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	// Download to temp file
	tmpDir, err := os.MkdirTemp("", "runright-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "runright.tar.gz")
	if err := downloadFile(info.DownloadURL, tarPath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// Extract the binary
	newBinaryPath := filepath.Join(tmpDir, "runright")
	if err := extractTarGz(tarPath, tmpDir, "runright"); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// Verify the new binary is executable
	if _, err := os.Stat(newBinaryPath); err != nil {
		return fmt.Errorf("new binary not found: %w", err)
	}

	// Backup current binary
	backupPath := execPath + ".backup"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	// Move new binary into place
	if err := copyFile(newBinaryPath, execPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, execPath)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Set executable permissions
	if err := os.Chmod(execPath, 0755); err != nil {
		os.Rename(backupPath, execPath)
		return fmt.Errorf("set permissions: %w", err)
	}

	// Clean up backup
	os.Remove(backupPath)

	fmt.Printf("Updated runright from %s to %s\n", info.CurrentVersion, info.LatestVersion)
	return nil
}

// downloadFile downloads a URL to a local file.
func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// extractTarGz extracts a specific file from a .tar.gz archive.
func extractTarGz(tarPath, destDir, targetFile string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Only extract the target file
		if filepath.Base(header.Name) != targetFile {
			continue
		}

		destPath := filepath.Join(destDir, targetFile)
		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()

		// Set file mode from tar header
		if err := os.Chmod(destPath, os.FileMode(header.Mode)); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("file %s not found in archive", targetFile)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

// Updater runs periodic update checks.
type Updater struct {
	cfg      UpdateConfig
	stopCh   chan struct{}
	updateCh chan *UpdateInfo
}

// NewUpdater creates a new background updater.
func NewUpdater(cfg UpdateConfig) *Updater {
	return &Updater{
		cfg:      cfg,
		stopCh:   make(chan struct{}),
		updateCh: make(chan *UpdateInfo, 1),
	}
}

// Start begins periodic update checks.
func (u *Updater) Start() {
	if !u.cfg.Enabled {
		return
	}

	go func() {
		// Check immediately on start
		if info, err := CheckForUpdate(u.cfg); err == nil && info.UpdateAvailable {
			select {
			case u.updateCh <- info:
			default:
			}
		}

		ticker := time.NewTicker(u.cfg.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-u.stopCh:
				return
			case <-ticker.C:
				if info, err := CheckForUpdate(u.cfg); err == nil && info.UpdateAvailable {
					select {
					case u.updateCh <- info:
					default:
					}
				}
			}
		}
	}()
}

// Stop terminates the updater.
func (u *Updater) Stop() {
	close(u.stopCh)
}

// Updates returns a channel that receives update notifications.
func (u *Updater) Updates() <-chan *UpdateInfo {
	return u.updateCh
}
