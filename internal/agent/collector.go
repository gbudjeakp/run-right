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
	"runtime"
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
	// ExpensiveSampleEvery controls how often expensive host-wide probes run.
	// 1 = every tick, 6 = every 6 ticks (default, ~30s at 5s interval).
	ExpensiveSampleEvery int
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
	prevIOTime time.Time
	tickCount  int

	// Tier 3 feature: GPU metrics snapshots
	gpuSnapshots [][]GPUSnapshot
	// Tier 3 feature: Container metrics snapshots
	containerSnapshots [][]ContainerMetrics
	// Tier 3 feature: Cache stats (captured once at end)
	cacheStats *CacheStats
}

// NewCollector creates a ready-to-run Collector.
func NewCollector(cfg Config) *Collector {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.ExpensiveSampleEvery <= 0 {
		cfg.ExpensiveSampleEvery = 6
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
	c.prevIOTime = c.startTime

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
	gpuSnaps := make([][]GPUSnapshot, len(c.gpuSnapshots))
	copy(gpuSnaps, c.gpuSnapshots)
	containerSnaps := make([][]ContainerMetrics, len(c.containerSnapshots))
	copy(containerSnaps, c.containerSnapshots)
	c.mu.Unlock()

	summary := buildSummary(c.cfg.JobID, c.startTime, time.Now(), snaps)
	summary.RunID = c.runID
	summary.Status = "heartbeat"
	summary.CIPlatform = detectCIPlatform()
	summary.Repository = detectRepository()
	applyDetectedMachineMetadata(&summary)
	summary.RuntimeStorageClass = detectRuntimeStorageClass()

	// Tier 3: Add GPU summary
	if gpuSummary := buildGPUSummary(gpuSnaps); gpuSummary != nil {
		summary.GPU = convertGPUSummary(gpuSummary)
	}

	// Tier 3: Add container summary
	if containerSummary := buildContainerSummary(containerSnaps); containerSummary != nil {
		summary.Containers = convertContainerSummary(containerSummary)
	}

	// Tier 3: Add egress estimate
	if egressSummary := buildEgressSummary(summary.NetTxMBsPeak, summary.CIPlatform, summary.DurationSeconds, summary.SampleCount); egressSummary != nil {
		summary.Egress = convertEgressSummary(egressSummary)
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
	c.tickCount++
	runExpensive := c.tickCount == 1 || c.tickCount%c.cfg.ExpensiveSampleEvery == 0

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

	if runExpensive {
		// Processes + threads are expensive on large runners; sample less frequently.
		procs, err := process.Processes()
		if err == nil {
			snap.ProcessCount = len(procs)
			for _, p := range procs {
				threads, _ := p.NumThreads()
				snap.ThreadCount += int(threads)
			}
		}

		// Network I/O (delta since previous expensive sample)
		elapsed := t.Sub(c.prevIOTime).Seconds()
		if elapsed <= 0 {
			elapsed = c.cfg.Interval.Seconds()
		}
		netIO, err := net.IOCounters(false)
		if err == nil && len(netIO) > 0 && len(c.baseNetIO) > 0 {
			snap.NetRxMBs = float64(netIO[0].BytesRecv-c.baseNetIO[0].BytesRecv) / (1 << 20) / elapsed
			snap.NetTxMBs = float64(netIO[0].BytesSent-c.baseNetIO[0].BytesSent) / (1 << 20) / elapsed
			c.baseNetIO = netIO
		}

		// Disk I/O (delta since previous expensive sample)
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
		c.prevIOTime = t

		// Tier 3: GPU metrics (sample during expensive probes)
		if gpuSnaps := collectGPUMetrics(); len(gpuSnaps) > 0 {
			c.mu.Lock()
			c.gpuSnapshots = append(c.gpuSnapshots, gpuSnaps)
			c.mu.Unlock()
		}

		// Tier 3: Container metrics (sample during expensive probes)
		if containers, ok := detectContainers(); ok && len(containers) > 0 {
			c.mu.Lock()
			c.containerSnapshots = append(c.containerSnapshots, containers)
			c.mu.Unlock()
		}
	}

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
	gpuSnaps := make([][]GPUSnapshot, len(c.gpuSnapshots))
	copy(gpuSnaps, c.gpuSnapshots)
	containerSnaps := make([][]ContainerMetrics, len(c.containerSnapshots))
	copy(containerSnaps, c.containerSnapshots)
	c.mu.Unlock()

	// Tier 3: Collect cache stats at the end of the run
	cacheStats := detectCacheStats()

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
	summary.Repository = detectRepository()
	applyDetectedMachineMetadata(&summary)
	summary.RuntimeStorageClass = detectRuntimeStorageClass()

	// Tier 3: Add GPU summary
	if gpuSummary := buildGPUSummary(gpuSnaps); gpuSummary != nil {
		summary.GPU = convertGPUSummary(gpuSummary)
	}

	// Tier 3: Add container summary
	if containerSummary := buildContainerSummary(containerSnaps); containerSummary != nil {
		summary.Containers = convertContainerSummary(containerSummary)
	}

	// Tier 3: Add cache stats
	if cacheStats != nil {
		summary.Cache = convertCacheStats(cacheStats)
	}

	// Tier 3: Add egress estimate
	if egressSummary := buildEgressSummary(summary.NetTxMBsPeak, summary.CIPlatform, summary.DurationSeconds, summary.SampleCount); egressSummary != nil {
		summary.Egress = convertEgressSummary(egressSummary)
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

// detectRepository returns the repository slug (owner/repo) when available.
// This is used to group runs in repo-centric views.
func detectRepository() string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("CI_PROJECT_PATH")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("BITBUCKET_REPO_FULL_NAME")); v != "" {
		return v
	}
	if owner := strings.TrimSpace(os.Getenv("BITBUCKET_REPO_OWNER")); owner != "" {
		if slug := strings.TrimSpace(os.Getenv("BITBUCKET_REPO_SLUG")); slug != "" {
			return owner + "/" + slug
		}
	}
	if v := strings.TrimSpace(os.Getenv("BUILD_REPOSITORY_NAME")); v != "" {
		return v
	}
	return ""
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

func applyDetectedMachineMetadata(summary *types.MetricsSummary) {
	if summary == nil {
		return
	}
	v, m := detectResources()
	if v <= 0 || m <= 0 {
		summary.DetectedMachineConfidenceLevel = "unknown"
		summary.DetectedMachineMatchReason = "insufficient runtime resources for machine detection"
		return
	}

	machine, confidence, reason := catalog.DetectMachineWithConfidence(v, m, ciPlatformToProvider(summary.CIPlatform))
	summary.DetectedMachine = machine
	summary.DetectedMachineConfidence = confidence
	summary.DetectedMachineMatchReason = reason
	summary.DetectedMachineConfidenceLevel = confidenceLevel(confidence)
	if machine == nil {
		summary.DetectedMachine = fallbackDetectedMachine(summary.CIPlatform, v, m)
		summary.DetectedMachineConfidence = 0
		summary.DetectedMachineConfidenceLevel = "unknown"
		summary.DetectedMachineMatchReason = "no catalog match found; using runtime self-hosted machine metadata"
	}
}

func fallbackDetectedMachine(ciPlatform string, vcpus int, memGiB float64) *types.MachineType {
	id := "self-hosted"
	if runner := strings.TrimSpace(os.Getenv("RUNNER_NAME")); runner != "" {
		id = runner
	}

	provider := types.Provider("")
	if ciPlatform == "github" {
		provider = types.ProviderGitHub
	}

	return &types.MachineType{
		ID:                   id,
		Provider:             provider,
		Family:               "self-hosted",
		Series:               "self-hosted",
		VCPUs:                vcpus,
		MemoryGiB:            math.Round(memGiB*100) / 100,
		StorageType:          "unknown",
		Architecture:         runtime.GOARCH,
		OnDemandPricePerHour: 0,
		Tags:                 []string{"self-hosted", "runtime-detected"},
	}
}

func confidenceLevel(score float64) string {
	switch {
	case score >= 0.85:
		return "high"
	case score >= 0.65:
		return "medium"
	case score > 0:
		return "low"
	default:
		return "unknown"
	}
}

// detectRuntimeStorageClass probes the local runtime device type as a
// best-effort hint. It does not identify cloud provider volume products.
func detectRuntimeStorageClass() string {
	if runtime.GOOS != "linux" {
		return "unknown"
	}

	dev, ok := linuxRootBlockDeviceName()
	if !ok || dev == "" {
		return "unknown"
	}

	rotPath := filepath.Join("/sys/class/block", dev, "queue/rotational")
	b, err := os.ReadFile(rotPath)
	if err != nil {
		return "unknown"
	}
	v := strings.TrimSpace(string(b))
	switch v {
	case "0":
		return "ssd"
	case "1":
		return "hdd"
	default:
		return "unknown"
	}
}

func linuxRootBlockDeviceName() (string, bool) {
	b, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "/" {
			continue
		}
		src := fields[0]
		if !strings.HasPrefix(src, "/dev/") {
			return "", false
		}
		name := strings.TrimPrefix(src, "/dev/")
		if strings.HasPrefix(name, "mapper/") {
			return "", false
		}
		base := trimLinuxPartitionSuffix(name)
		if base == "" {
			return "", false
		}
		return base, true
	}
	return "", false
}

func trimLinuxPartitionSuffix(name string) string {
	// NVMe partitions end with pN (example: nvme0n1p1).
	if i := strings.LastIndex(name, "p"); i > 0 {
		suffix := name[i+1:]
		if suffix != "" && isDigits(suffix) {
			return name[:i]
		}
	}

	// SATA/xvd/virtio partition names typically end with digits (example: xvda1).
	i := len(name) - 1
	for i >= 0 && name[i] >= '0' && name[i] <= '9' {
		i--
	}
	if i < len(name)-1 {
		return name[:i+1]
	}
	return name
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
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

// Tier 3: Converter functions to map agent-internal types to types package

// convertGPUSummary converts the agent GPUSummary to types.GPUSummary.
func convertGPUSummary(src *GPUSummary) *types.GPUSummary {
	if src == nil {
		return nil
	}
	return &types.GPUSummary{
		Count:              src.Count,
		TotalMemoryGiB:     src.TotalMemoryGiB,
		AvgUtilizationPct:  src.AvgUtilizationPct,
		PeakUtilizationPct: src.PeakUtilizationPct,
		P95UtilizationPct:  src.P95UtilizationPct,
		AvgMemoryUtilPct:   src.AvgMemoryUtilPct,
		PeakMemoryUtilPct:  src.PeakMemoryUtilPct,
		P95MemoryUtilPct:   src.P95MemoryUtilPct,
		AvgPowerDrawW:      src.AvgPowerDrawW,
		PeakPowerDrawW:     src.PeakPowerDrawW,
		IdleSamplesPct:     src.IdleSamplesPct,
		UnderutilizedPct:   src.UnderutilizedPct,
		GPUType:            src.GPUType,
	}
}

// convertContainerSummary converts the agent ContainerSummary to types.ContainerSummary.
func convertContainerSummary(src *ContainerSummary) *types.ContainerSummary {
	if src == nil {
		return nil
	}
	containers := make([]types.ContainerAggregates, len(src.Containers))
	for i, c := range src.Containers {
		containers[i] = types.ContainerAggregates{
			ID:                c.ID,
			Name:              c.Name,
			Image:             c.Image,
			CPUPercentAvg:     c.CPUPercentAvg,
			CPUPercentPeak:    c.CPUPercentPeak,
			CPUPercentP95:     c.CPUPercentP95,
			MemoryUsedMiBAvg:  c.MemoryUsedMiBAvg,
			MemoryUsedMiBPeak: c.MemoryUsedMiBPeak,
			MemoryLimitMiB:    c.MemoryLimitMiB,
			NetRxMBTotal:      c.NetRxMBTotal,
			NetTxMBTotal:      c.NetTxMBTotal,
			BlockReadMBTotal:  c.BlockReadMBTotal,
			BlockWriteMBTotal: c.BlockWriteMBTotal,
			SampleCount:       c.SampleCount,
		}
	}
	return &types.ContainerSummary{
		Containers:         containers,
		TotalContainers:    src.TotalContainers,
		TopCPUContainer:    src.TopCPUContainer,
		TopMemoryContainer: src.TopMemoryContainer,
	}
}

// convertCacheStats converts the agent CacheStats to types.CacheStats.
func convertCacheStats(src *CacheStats) *types.CacheStats {
	if src == nil {
		return nil
	}
	return &types.CacheStats{
		DockerLayerCacheHits:   src.DockerLayerCacheHits,
		DockerLayerCacheMisses: src.DockerLayerCacheMisses,
		NPMCacheHitRate:        src.NPMCacheHitRate,
		PIPCacheHitRate:        src.PIPCacheHitRate,
		GoCacheHitRate:         src.GoCacheHitRate,
		MavenCacheHitRate:      src.MavenCacheHitRate,
		GradleCacheHitRate:     src.GradleCacheHitRate,
		CIPlatformCacheHit:     src.CIPlatformCacheHit,
		CIPlatformCacheSize:    src.CIPlatformCacheSize,
		CacheRestoreTimeMs:     src.CacheRestoreTimeMs,
		CacheSaveTimeMs:        src.CacheSaveTimeMs,
		OverallCacheHitRate:    src.OverallCacheHitRate,
		EstimatedTimeSaved:     src.EstimatedTimeSaved,
	}
}

// convertEgressSummary converts the agent EgressSummary to types.EgressSummary.
func convertEgressSummary(src *EgressSummary) *types.EgressSummary {
	if src == nil {
		return nil
	}
	return &types.EgressSummary{
		TotalEgressGB:        src.TotalEgressGB,
		EstimatedCostUSD:     src.EstimatedCostUSD,
		CostPerRunUSD:        src.CostPerRunUSD,
		MonthlyProjectionUSD: src.MonthlyProjectionUSD,
		Provider:             src.Provider,
		RecommendCaching:     src.RecommendCaching,
		RecommendCompression: src.RecommendCompression,
		PotentialSavingsUSD:  src.PotentialSavingsUSD,
	}
}
