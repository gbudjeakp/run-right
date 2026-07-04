package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CacheStats captures build cache efficiency metrics.
type CacheStats struct {
	// Docker layer cache stats
	DockerLayerCacheHits   int `json:"docker_layer_cache_hits,omitempty"`
	DockerLayerCacheMisses int `json:"docker_layer_cache_misses,omitempty"`

	// Package manager caches
	NPMCacheHitRate    float64 `json:"npm_cache_hit_rate,omitempty"`
	PIPCacheHitRate    float64 `json:"pip_cache_hit_rate,omitempty"`
	GoCacheHitRate     float64 `json:"go_cache_hit_rate,omitempty"`
	MavenCacheHitRate  float64 `json:"maven_cache_hit_rate,omitempty"`
	GradleCacheHitRate float64 `json:"gradle_cache_hit_rate,omitempty"`

	// CI platform cache stats (when available)
	CIPlatformCacheHit  bool    `json:"ci_platform_cache_hit,omitempty"`
	CIPlatformCacheSize int64   `json:"ci_platform_cache_size_bytes,omitempty"`
	CacheRestoreTimeMs  int64   `json:"cache_restore_time_ms,omitempty"`
	CacheSaveTimeMs     int64   `json:"cache_save_time_ms,omitempty"`

	// Aggregated metrics
	OverallCacheHitRate float64 `json:"overall_cache_hit_rate,omitempty"` // 0-100
	EstimatedTimeSaved  float64 `json:"estimated_time_saved_sec,omitempty"`
}

// detectCacheStats collects cache efficiency metrics from various sources.
func detectCacheStats() *CacheStats {
	stats := &CacheStats{}
	var hitRates []float64

	// Docker build cache
	if hits, misses := detectDockerCache(); hits > 0 || misses > 0 {
		stats.DockerLayerCacheHits = hits
		stats.DockerLayerCacheMisses = misses
		if total := hits + misses; total > 0 {
			hitRates = append(hitRates, float64(hits)/float64(total)*100)
		}
	}

	// Go module cache
	if rate := detectGoCacheHitRate(); rate >= 0 {
		stats.GoCacheHitRate = rate
		hitRates = append(hitRates, rate)
	}

	// NPM cache
	if rate := detectNPMCacheHitRate(); rate >= 0 {
		stats.NPMCacheHitRate = rate
		hitRates = append(hitRates, rate)
	}

	// Maven cache
	if rate := detectMavenCacheHitRate(); rate >= 0 {
		stats.MavenCacheHitRate = rate
		hitRates = append(hitRates, rate)
	}

	// Gradle cache
	if rate := detectGradleCacheHitRate(); rate >= 0 {
		stats.GradleCacheHitRate = rate
		hitRates = append(hitRates, rate)
	}

	// GitHub Actions cache
	if hit, size := detectGitHubActionsCache(); size > 0 {
		stats.CIPlatformCacheHit = hit
		stats.CIPlatformCacheSize = size
	}

	// Calculate overall hit rate
	if len(hitRates) > 0 {
		var sum float64
		for _, r := range hitRates {
			sum += r
		}
		stats.OverallCacheHitRate = sum / float64(len(hitRates))
	}

	// Return nil if no cache data found
	if stats.DockerLayerCacheHits == 0 && stats.DockerLayerCacheMisses == 0 &&
		stats.GoCacheHitRate == 0 && stats.NPMCacheHitRate == 0 &&
		stats.MavenCacheHitRate == 0 && stats.GradleCacheHitRate == 0 &&
		stats.CIPlatformCacheSize == 0 {
		return nil
	}

	return stats
}

// detectDockerCache parses docker build output for cache statistics.
func detectDockerCache() (hits, misses int) {
	// Check for BuildKit cache stats in environment
	if cacheStats := os.Getenv("DOCKER_BUILD_CACHE_STATS"); cacheStats != "" {
		// Format: "hits:misses"
		parts := strings.Split(cacheStats, ":")
		if len(parts) == 2 {
			hits, _ = strconv.Atoi(parts[0])
			misses, _ = strconv.Atoi(parts[1])
			return
		}
	}

	// Try to read from BuildKit cache metadata
	cacheDir := os.Getenv("DOCKER_BUILDKIT_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = filepath.Join(os.Getenv("HOME"), ".docker", "buildkit")
	}
	metaPath := filepath.Join(cacheDir, "cache", "metadata.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta struct {
			Hits   int `json:"hits"`
			Misses int `json:"misses"`
		}
		if json.Unmarshal(data, &meta) == nil {
			return meta.Hits, meta.Misses
		}
	}

	return 0, 0
}

// detectGoCacheHitRate estimates Go build cache hit rate.
func detectGoCacheHitRate() float64 {
	// Check GOCACHE location
	cacheDir := os.Getenv("GOCACHE")
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".cache", "go-build")
	}

	// Try 'go env GOCACHE' as fallback
	if cacheDir == "" {
		if out, err := exec.Command("go", "env", "GOCACHE").Output(); err == nil {
			cacheDir = strings.TrimSpace(string(out))
		}
	}

	if cacheDir == "" {
		return -1
	}

	// Estimate based on cache directory contents
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return -1
	}

	// If cache has content, assume some hit rate based on age
	if len(entries) > 10 {
		// Heuristic: more entries = higher cache effectiveness
		// This is a rough estimate; actual hit rate requires build output parsing
		return 70.0
	} else if len(entries) > 0 {
		return 30.0
	}

	return 0
}

// detectNPMCacheHitRate estimates NPM cache hit rate.
func detectNPMCacheHitRate() float64 {
	// Check for npm cache stats from environment (set by CI plugins)
	if statsEnv := os.Getenv("NPM_CACHE_STATS"); statsEnv != "" {
		if rate, err := strconv.ParseFloat(statsEnv, 64); err == nil {
			return rate
		}
	}

	// Check npm cache directory
	cacheDir := os.Getenv("npm_config_cache")
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".npm")
	}

	// Check _cacache directory (npm's content-addressable cache)
	cacachePath := filepath.Join(cacheDir, "_cacache")
	if info, err := os.Stat(cacachePath); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(filepath.Join(cacachePath, "content-v2", "sha512"))
		if len(entries) > 100 {
			return 80.0 // Well-populated cache
		} else if len(entries) > 10 {
			return 50.0
		}
	}

	return -1
}

// detectMavenCacheHitRate estimates Maven cache hit rate.
func detectMavenCacheHitRate() float64 {
	// Check for Maven wrapper or local repo
	home, _ := os.UserHomeDir()
	m2Repo := filepath.Join(home, ".m2", "repository")

	if info, err := os.Stat(m2Repo); err == nil && info.IsDir() {
		// Count artifact directories
		count := 0
		filepath.WalkDir(m2Repo, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				count++
				if count > 1000 {
					return filepath.SkipAll
				}
			}
			return nil
		})
		if count > 500 {
			return 85.0
		} else if count > 100 {
			return 60.0
		} else if count > 10 {
			return 30.0
		}
	}

	return -1
}

// detectGradleCacheHitRate estimates Gradle cache hit rate.
func detectGradleCacheHitRate() float64 {
	// Check for Gradle cache directory
	home, _ := os.UserHomeDir()
	gradleCache := filepath.Join(home, ".gradle", "caches")

	if info, err := os.Stat(gradleCache); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(gradleCache)
		if len(entries) > 5 {
			// Check for build-cache directory (remote cache hits)
			buildCache := filepath.Join(gradleCache, "build-cache-1")
			if bcInfo, err := os.Stat(buildCache); err == nil && bcInfo.IsDir() {
				bcEntries, _ := os.ReadDir(buildCache)
				if len(bcEntries) > 100 {
					return 80.0
				}
				return 50.0
			}
			return 40.0
		}
	}

	return -1
}

// detectGitHubActionsCache checks for GitHub Actions cache usage.
func detectGitHubActionsCache() (hit bool, sizeBytes int64) {
	// GitHub Actions sets these when cache is restored
	if os.Getenv("ACTIONS_CACHE_HIT") == "true" {
		hit = true
	}

	// Check for cache size from environment
	if sizeStr := os.Getenv("ACTIONS_CACHE_SIZE"); sizeStr != "" {
		sizeBytes, _ = strconv.ParseInt(sizeStr, 10, 64)
	}

	// Try to read from GitHub Actions cache state file
	stateDir := os.Getenv("GITHUB_STATE")
	if stateDir != "" {
		statePath := filepath.Join(stateDir, "cache")
		if data, err := os.ReadFile(statePath); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "cache-hit=") {
					hit = strings.TrimPrefix(line, "cache-hit=") == "true"
				}
				if strings.HasPrefix(line, "cache-size=") {
					sizeBytes, _ = strconv.ParseInt(strings.TrimPrefix(line, "cache-size="), 10, 64)
				}
			}
		}
	}

	// Check RUNNER_TOOL_CACHE for tool cache info
	if toolCache := os.Getenv("RUNNER_TOOL_CACHE"); toolCache != "" {
		if info, err := os.Stat(toolCache); err == nil && info.IsDir() {
			// Walk to get total size
			var totalSize int64
			filepath.WalkDir(toolCache, func(_ string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if fi, err := d.Info(); err == nil {
					totalSize += fi.Size()
				}
				return nil
			})
			if totalSize > sizeBytes {
				sizeBytes = totalSize
			}
		}
	}

	return
}

// parseBuildLog scans a build log for cache-related output.
// This is useful when the build output is captured to a file.
func parseBuildLog(logPath string) *CacheStats {
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	stats := &CacheStats{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		// Docker BuildKit cache patterns
		if strings.Contains(line, "CACHED") {
			stats.DockerLayerCacheHits++
		}
		if strings.Contains(line, "DONE") && !strings.Contains(line, "CACHED") {
			stats.DockerLayerCacheMisses++
		}

		// Go build cache patterns
		if strings.Contains(line, "cached") && strings.Contains(line, "go build") {
			// This is a heuristic; Go doesn't output detailed cache stats
		}

		// npm patterns
		if strings.Contains(line, "reusing") || strings.Contains(line, "cache hit") {
			// npm cache hit indicator
		}
	}

	return stats
}
