package agent

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sgbudje/runright/internal/catalog"
	"github.com/sgbudje/runright/internal/types"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Config controls how the collector behaves.
type Config struct {
	Interval          time.Duration
	HeartbeatInterval time.Duration // how often to send partial data; 0 = 30s when FlushFn or HeartbeatFilePath is set
	OutputDir         string
	JobID             string
	// HeartbeatFilePath, if non-empty, is written on every heartbeat tick so that
	// partial metrics survive an OOM-kill or force-stop of the monitor process.
	HeartbeatFilePath string
	// FlushFn is called on every heartbeat and on the final flush.
	// The summary Status will be "heartbeat" or "completed" accordingly.
	// If nil, no HTTP/callback export occurs.
	FlushFn func(types.MetricsSummary) error
}

// maxSnapshotBuffer caps the in-memory snapshot slice for long-running jobs.
// At the default 5 s interval this is ~13.8 hours of data. When the cap is
// reached the oldest half is trimmed; peaks are therefore computed over a
// sliding window rather than the full run lifetime.
const maxSnapshotBuffer = 10_000

// Collector samples system metrics at a fixed interval.
type Collector struct {
	cfg       Config
	snapshots []types.MetricSnapshot
	mu        sync.Mutex
	startTime time.Time
	runID     string // unique per invocation; used for server-side upserts

	// baseline net/disk counters captured at first tick
	baseNetIO  []net.IOCountersStat
	baseDiskIO map[string]disk.IOCountersStat
	prevTime   time.Time
}

// NewCollector creates a ready-to-run Collector.
func NewCollector(cfg Config) *Collector {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "."
	}
	if cfg.JobID == "" {
		cfg.JobID = fmt.Sprintf("job-%d", time.Now().UnixMilli())
	}
	return &Collector{cfg: cfg, runID: newRunID()}
}

// newRunID generates a random hex string used to uniquely identify this agent run.
func newRunID() string {
	b := make([]byte, 16)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b)
}

// Run starts collection and blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	c.startTime = time.Now()

	// Capture baseline I/O counters.
	netIO, _ := net.IOCounters(false)
	diskIO, _ := disk.IOCounters()
	c.baseNetIO = netIO
	c.baseDiskIO = diskIO
	c.prevTime = c.startTime

	// Heartbeat: periodically send partial data so the backend has a record even
	// if the agent is killed (OOM, runner disconnect) before the final Flush.
	hbInterval := c.cfg.HeartbeatInterval
	if hbInterval <= 0 && (c.cfg.FlushFn != nil || c.cfg.HeartbeatFilePath != "") {
		hbInterval = 30 * time.Second
	}
	var hbCh <-chan time.Time
	if hbInterval > 0 && (c.cfg.FlushFn != nil || c.cfg.HeartbeatFilePath != "") {
		hb := time.NewTicker(hbInterval)
		defer hb.Stop()
		hbCh = hb.C
	}

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return c.Flush()
		case t := <-ticker.C:
			snap, err := c.collect(t)
			if err != nil {
				continue
			}
			c.mu.Lock()
			c.snapshots = append(c.snapshots, snap)
			if len(c.snapshots) > maxSnapshotBuffer {
				// Trim oldest half; keeps memory bounded for multi-day jobs.
				c.snapshots = c.snapshots[maxSnapshotBuffer/2:]
			}
			c.mu.Unlock()
		case <-hbCh:
			c.sendHeartbeat()
		}
	}
}

// sendHeartbeat posts the current partial summary to FlushFn with Status="heartbeat".
// It also writes a metrics-heartbeat.json file if HeartbeatFilePath is configured,
// so that partial metrics survive an OOM-kill or force-stop of the monitor process.
func (c *Collector) sendHeartbeat() {
	c.mu.Lock()
	snaps := make([]types.MetricSnapshot, len(c.snapshots))
	copy(snaps, c.snapshots)
	c.mu.Unlock()

	summary := buildSummary(c.cfg.JobID, c.startTime, time.Now(), snaps)
	summary.RunID = c.runID
	summary.Status = "heartbeat"
	summary.CIPlatform = detectCIPlatform()
	if v, m := detectResources(); v > 0 && m > 0 {
		summary.DetectedMachine = catalog.DetectMachine(v, m, ciPlatformToProvider(summary.CIPlatform))
	}

	// Write partial file so OOM-killed jobs still have data for recommendations.
	if c.cfg.HeartbeatFilePath != "" {
		_ = os.MkdirAll(filepath.Dir(c.cfg.HeartbeatFilePath), 0o755)
		if f, err := os.Create(c.cfg.HeartbeatFilePath); err == nil {
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			_ = enc.Encode(summary)
			_ = f.Close()
		}
	}

	if c.cfg.FlushFn != nil {
		_ = c.cfg.FlushFn(summary) // best-effort; ignore error on heartbeat
	}
}

func (c *Collector) collect(t time.Time) (types.MetricSnapshot, error) {
	snap := types.MetricSnapshot{Timestamp: t}

	// CPU
	pcts, err := cpu.Percent(0, false)
	if err == nil && len(pcts) > 0 {
		snap.CPUPercent = pcts[0]
	}

	// Memory
	vm, err := mem.VirtualMemory()
	if err == nil {
		snap.MemUsedGiB = float64(vm.Used) / (1 << 30)
		snap.MemTotalGiB = float64(vm.Total) / (1 << 30)
	}

	// Processes + threads
	procs, err := process.Processes()
	if err == nil {
		snap.ProcessCount = len(procs)
		for _, p := range procs {
			threads, _ := p.NumThreads()
			snap.ThreadCount += int(threads)
		}
	}

	// Network I/O (delta since last tick)
	elapsed := t.Sub(c.prevTime).Seconds()
	netIO, err := net.IOCounters(false)
	if err == nil && len(netIO) > 0 && len(c.baseNetIO) > 0 {
		snap.NetRxMBs = float64(netIO[0].BytesRecv-c.baseNetIO[0].BytesRecv) / (1 << 20) / elapsed
		snap.NetTxMBs = float64(netIO[0].BytesSent-c.baseNetIO[0].BytesSent) / (1 << 20) / elapsed
		c.baseNetIO = netIO
	}

	// Disk I/O (delta since last tick)
	diskIO, err := disk.IOCounters()
	if err == nil && c.baseDiskIO != nil {
		var readBytes, writeBytes uint64
		for name, stat := range diskIO {
			if base, ok := c.baseDiskIO[name]; ok {
				readBytes += stat.ReadBytes - base.ReadBytes
				writeBytes += stat.WriteBytes - base.WriteBytes
			}
		}
		snap.DiskReadMBs = float64(readBytes) / (1 << 20) / elapsed
		snap.DiskWriteMBs = float64(writeBytes) / (1 << 20) / elapsed
		c.baseDiskIO = diskIO
	}

	c.prevTime = t
	return snap, nil
}

// Flush writes metrics.jsonl and metrics-summary.json to OutputDir, then calls
// FlushFn (if set) with Status="completed". This is called automatically when
// the Run context is cancelled — including on SIGTERM — ensuring the final
// complete record is always posted even on graceful shutdown.
func (c *Collector) Flush() error {
	c.mu.Lock()
	snaps := make([]types.MetricSnapshot, len(c.snapshots))
	copy(snaps, c.snapshots)
	c.mu.Unlock()

	if err := os.MkdirAll(c.cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("output dir: %w", err)
	}

	// Write NDJSON time-series.
	jsonlPath := filepath.Join(c.cfg.OutputDir, "metrics.jsonl")
	f, err := os.Create(jsonlPath)
	if err != nil {
		return fmt.Errorf("create metrics.jsonl: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, s := range snaps {
		if err := enc.Encode(s); err != nil {
			_ = f.Close()
			return err
		}
	}
	_ = f.Close()

	// Build and write summary.
	summary := buildSummary(c.cfg.JobID, c.startTime, time.Now(), snaps)
	summary.RunID = c.runID
	summary.Status = "completed"
	summary.CIPlatform = detectCIPlatform()

	// Detect current machine. Cgroup-aware: reads container/pod limits first
	// (K8s pods, DinD), falls back to OS-level counts on bare VMs.
	if v, m := detectResources(); v > 0 && m > 0 {
		summary.DetectedMachine = catalog.DetectMachine(v, m, ciPlatformToProvider(summary.CIPlatform))
	}

	summaryPath := filepath.Join(c.cfg.OutputDir, "metrics-summary.json")
	sf, err := os.Create(summaryPath)
	if err != nil {
		return fmt.Errorf("create metrics-summary.json: %w", err)
	}
	defer sf.Close()
	enc2 := json.NewEncoder(sf)
	enc2.SetIndent("", "  ")
	if err := enc2.Encode(summary); err != nil {
		return err
	}

	// Notify the caller (e.g. HTTP exporter) with the final completed summary.
	if c.cfg.FlushFn != nil {
		return c.cfg.FlushFn(summary)
	}
	return nil
}

func buildSummary(jobID string, start, end time.Time, snaps []types.MetricSnapshot) types.MetricsSummary {
	s := types.MetricsSummary{
		JobID:           jobID,
		StartTime:       start,
		EndTime:         end,
		DurationSeconds: end.Sub(start).Seconds(),
		SampleCount:     len(snaps),
	}
	if len(snaps) == 0 {
		return s
	}

	var (
		cpuVals, memVals []float64
		sumCPU, sumMem   float64
	)

	for _, snap := range snaps {
		cpuVals = append(cpuVals, snap.CPUPercent)
		memVals = append(memVals, snap.MemUsedGiB)
		sumCPU += snap.CPUPercent
		sumMem += snap.MemUsedGiB

		if snap.CPUPercent > s.CPUPercentPeak {
			s.CPUPercentPeak = snap.CPUPercent
		}
		if snap.MemUsedGiB > s.MemUsedGiBPeak {
			s.MemUsedGiBPeak = snap.MemUsedGiB
		}
		if snap.MemTotalGiB > s.MemTotalGiB {
			s.MemTotalGiB = snap.MemTotalGiB
		}
		if snap.ProcessCount > s.ProcessCountPeak {
			s.ProcessCountPeak = snap.ProcessCount
		}
		if snap.ThreadCount > s.ThreadCountPeak {
			s.ThreadCountPeak = snap.ThreadCount
		}
		if snap.DiskReadMBs > s.DiskReadMBsPeak {
			s.DiskReadMBsPeak = snap.DiskReadMBs
		}
		if snap.DiskWriteMBs > s.DiskWriteMBsPeak {
			s.DiskWriteMBsPeak = snap.DiskWriteMBs
		}
		if snap.NetRxMBs > s.NetRxMBsPeak {
			s.NetRxMBsPeak = snap.NetRxMBs
		}
		if snap.NetTxMBs > s.NetTxMBsPeak {
			s.NetTxMBsPeak = snap.NetTxMBs
		}
	}

	n := float64(len(snaps))
	s.CPUPercentAvg = sumCPU / n
	s.MemUsedGiBAvg = sumMem / n
	s.CPUPercentP95 = percentile(cpuVals, 95)
	s.MemUsedGiBP95 = percentile(memVals, 95)

	return s
}

func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	idx := (p / 100.0) * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (idx-float64(lo))*(sorted[hi]-sorted[lo])
}

// detectCIPlatform returns the name of the CI system that is running the current job,
// detected from well-known environment variables.
func detectCIPlatform() string {
	switch {
	case os.Getenv("GITHUB_ACTIONS") == "true":
		return "github"
	case os.Getenv("GITLAB_CI") == "true":
		return "gitlab"
	case os.Getenv("CIRCLECI") == "true":
		return "circleci"
	case os.Getenv("BITBUCKET_BUILD_NUMBER") != "" || os.Getenv("BITBUCKET_REPO_SLUG") != "":
		return "bitbucket"
	case os.Getenv("TF_BUILD") == "True":
		return "azure"
	case os.Getenv("JENKINS_URL") != "" || os.Getenv("BUILD_NUMBER") != "":
		return "jenkins"
	default:
		return "local"
	}
}

// ciPlatformToProvider converts a CI platform name (from detectCIPlatform) to
// a catalog provider hint. Returns an empty Provider when the platform is not
// a known hosted-runner environment (e.g. Jenkins on a bare AWS VM — hardware
// detection should still match against the full catalog in that case).
func ciPlatformToProvider(platform string) types.Provider {
	switch platform {
	case "github":
		return types.ProviderGitHub
	default:
		return ""
	}
}

// detectResources returns the effective vCPU count and memory GiB for the
// current execution environment. Priority:
//  1. RUNRIGHT_VCPUS / RUNRIGHT_MEMORY_GIB env vars (manual override)
//  2. cgroup v2 limits (/sys/fs/cgroup/cpu.max, memory.max)
//  3. cgroup v1 limits (cpu.cfs_quota_us / memory.limit_in_bytes)
//  4. OS-level cpu.Counts + mem.VirtualMemory (bare VM fallback)
func detectResources() (vcpus int, memGiB float64) {
	if v := os.Getenv("RUNRIGHT_VCPUS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			vcpus = n
		}
	}
	if m := os.Getenv("RUNRIGHT_MEMORY_GIB"); m != "" {
		if f, err := strconv.ParseFloat(m, 64); err == nil && f > 0 {
			memGiB = f
		}
	}
	if vcpus > 0 && memGiB > 0 {
		return
	}

	if v, m, ok := cgroupV2(); ok {
		if vcpus == 0 {
			vcpus = v
		}
		if memGiB == 0 {
			memGiB = m
		}
	}
	if vcpus == 0 || memGiB == 0 {
		if v, m, ok := cgroupV1(); ok {
			if vcpus == 0 {
				vcpus = v
			}
			if memGiB == 0 {
				memGiB = m
			}
		}
	}

	// OS-level fallback for bare VMs or containers without explicit limits.
	if vcpus == 0 {
		vcpus, _ = cpu.Counts(true)
	}
	if memGiB == 0 {
		if vm, err := mem.VirtualMemory(); err == nil {
			memGiB = float64(vm.Total) / (1 << 30)
		}
	}
	return
}

// cgroupV2 reads CPU quota and memory limit from the cgroup v2 unified
// hierarchy (/sys/fs/cgroup). Used by Kubernetes pods on modern kernels.
func cgroupV2() (vcpus int, memGiB float64, ok bool) {
	if b, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) == 2 && fields[0] != "max" {
			quota, qerr := strconv.ParseFloat(fields[0], 64)
			period, perr := strconv.ParseFloat(fields[1], 64)
			if qerr == nil && perr == nil && period > 0 && quota > 0 {
				vcpus = int(math.Ceil(quota / period))
				ok = true
			}
		}
	}
	if b, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "max" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
				memGiB = float64(n) / (1 << 30)
				ok = true
			}
		}
	}
	return
}

// cgroupV1 reads CPU quota and memory limit from the cgroup v1 hierarchy.
// Used by older kernels and some DinD setups.
func cgroupV1() (vcpus int, memGiB float64, ok bool) {
	quota, qerr := readCgroupInt64("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period, perr := readCgroupInt64("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if qerr == nil && perr == nil && quota > 0 && period > 0 {
		vcpus = int(math.Ceil(float64(quota) / float64(period)))
		ok = true
	}
	if limit, err := readCgroupInt64("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		const maxSane = int64(1) << 62 // values near MaxInt64 mean "no limit" on some kernels
		if limit > 0 && limit < maxSane {
			memGiB = float64(limit) / (1 << 30)
			ok = true
		}
	}
	return
}

func readCgroupInt64(path string) (int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
}
