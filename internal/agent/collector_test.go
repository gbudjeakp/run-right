package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sgbudje/runright/internal/types"
)

func TestBuildSummary_Peaks(t *testing.T) {
	snaps := []types.MetricSnapshot{
		{CPUPercent: 30.0, MemUsedGiB: 1.0, MemTotalGiB: 8.0},
		{CPUPercent: 80.0, MemUsedGiB: 4.0, MemTotalGiB: 8.0},
		{CPUPercent: 50.0, MemUsedGiB: 2.0, MemTotalGiB: 8.0},
	}
	start := time.Now().Add(-30 * time.Second)
	s := buildSummary("job-peaks", start, time.Now(), snaps)

	if s.CPUPercentPeak != 80.0 {
		t.Errorf("CPUPercentPeak: got %v, want 80.0", s.CPUPercentPeak)
	}
	if s.MemUsedGiBPeak != 4.0 {
		t.Errorf("MemUsedGiBPeak: got %v, want 4.0", s.MemUsedGiBPeak)
	}
	if s.MemTotalGiB != 8.0 {
		t.Errorf("MemTotalGiB: got %v, want 8.0", s.MemTotalGiB)
	}
	if s.SampleCount != 3 {
		t.Errorf("SampleCount: got %d, want 3", s.SampleCount)
	}
	if s.JobID != "job-peaks" {
		t.Errorf("JobID: got %q, want job-peaks", s.JobID)
	}
}

func TestBuildSummary_Empty(t *testing.T) {
	s := buildSummary("job-empty", time.Now(), time.Now(), nil)
	if s.SampleCount != 0 {
		t.Errorf("SampleCount: got %d, want 0", s.SampleCount)
	}
	if s.CPUPercentPeak != 0 {
		t.Errorf("CPUPercentPeak: got %v, want 0", s.CPUPercentPeak)
	}
}

func TestBuildSummary_Averages(t *testing.T) {
	snaps := []types.MetricSnapshot{
		{CPUPercent: 20.0, MemUsedGiB: 1.0},
		{CPUPercent: 40.0, MemUsedGiB: 3.0},
	}
	s := buildSummary("job-avg", time.Now(), time.Now(), snaps)
	if s.CPUPercentAvg != 30.0 {
		t.Errorf("CPUPercentAvg: got %v, want 30.0", s.CPUPercentAvg)
	}
	if s.MemUsedGiBAvg != 2.0 {
		t.Errorf("MemUsedGiBAvg: got %v, want 2.0", s.MemUsedGiBAvg)
	}
}

func TestCollector_FlushWritesFiles(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(Config{
		Interval:  50 * time.Millisecond,
		OutputDir: dir,
		JobID:     "test-flush",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = c.Run(ctx)

	if _, err := os.Stat(filepath.Join(dir, "metrics-summary.json")); os.IsNotExist(err) {
		t.Error("metrics-summary.json not written after flush")
	}
	if _, err := os.Stat(filepath.Join(dir, "metrics.jsonl")); os.IsNotExist(err) {
		t.Error("metrics.jsonl not written after flush")
	}
}

func TestCollector_HeartbeatFile(t *testing.T) {
	dir := t.TempDir()
	hbPath := filepath.Join(dir, "metrics-heartbeat.json")

	// Use generous intervals so the test is stable under -race on loaded CI
	// runners. Each collect() invokes psutil (cpu.Percent, disk/net counters)
	// which can take 50-100 ms; the race detector adds further overhead.
	c := NewCollector(Config{
		Interval:          200 * time.Millisecond,
		HeartbeatInterval: 400 * time.Millisecond,
		OutputDir:         dir,
		JobID:             "test-heartbeat",
		HeartbeatFilePath: hbPath,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = c.Run(ctx)

	data, err := os.ReadFile(hbPath)
	if err != nil {
		t.Fatalf("heartbeat file not written: %v", err)
	}
	var summary types.MetricsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("heartbeat file contains invalid JSON: %v", err)
	}
	if summary.JobID != "test-heartbeat" {
		t.Errorf("JobID: got %q, want test-heartbeat", summary.JobID)
	}
}

func TestNewCollector_Defaults(t *testing.T) {
	c := NewCollector(Config{})
	if c.cfg.Interval != 5*time.Second {
		t.Errorf("default Interval: got %v, want 5s", c.cfg.Interval)
	}
	if c.cfg.ExpensiveSampleEvery != 6 {
		t.Errorf("default ExpensiveSampleEvery: got %d, want 6", c.cfg.ExpensiveSampleEvery)
	}
	if c.cfg.OutputDir != "." {
		t.Errorf("default OutputDir: got %q, want .", c.cfg.OutputDir)
	}
	if c.cfg.JobID == "" {
		t.Error("default JobID should be non-empty")
	}
	if c.runID == "" {
		t.Error("runID should be non-empty")
	}
}

func TestDetectRepository_GitHub(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("CI_PROJECT_PATH", "")
	t.Setenv("BITBUCKET_REPO_FULL_NAME", "")
	t.Setenv("BITBUCKET_REPO_OWNER", "")
	t.Setenv("BITBUCKET_REPO_SLUG", "")
	t.Setenv("BUILD_REPOSITORY_NAME", "")

	if got := detectRepository(); got != "owner/repo" {
		t.Fatalf("detectRepository() = %q, want %q", got, "owner/repo")
	}
}

func TestDetectRepository_BitbucketOwnerSlug(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("CI_PROJECT_PATH", "")
	t.Setenv("BITBUCKET_REPO_FULL_NAME", "")
	t.Setenv("BITBUCKET_REPO_OWNER", "my-org")
	t.Setenv("BITBUCKET_REPO_SLUG", "my-repo")
	t.Setenv("BUILD_REPOSITORY_NAME", "")

	if got := detectRepository(); got != "my-org/my-repo" {
		t.Fatalf("detectRepository() = %q, want %q", got, "my-org/my-repo")
	}
}

func TestDetectRepository_Empty(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("CI_PROJECT_PATH", "")
	t.Setenv("BITBUCKET_REPO_FULL_NAME", "")
	t.Setenv("BITBUCKET_REPO_OWNER", "")
	t.Setenv("BITBUCKET_REPO_SLUG", "")
	t.Setenv("BUILD_REPOSITORY_NAME", "")

	if got := detectRepository(); got != "" {
		t.Fatalf("detectRepository() = %q, want empty string", got)
	}
}
