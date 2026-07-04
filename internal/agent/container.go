package agent

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// ContainerMetrics captures resource usage for a single container.
type ContainerMetrics struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Image          string  `json:"image,omitempty"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryUsedMiB  float64 `json:"memory_used_mib"`
	MemoryLimitMiB float64 `json:"memory_limit_mib,omitempty"`
	MemoryPercent  float64 `json:"memory_percent,omitempty"`
	NetRxMB        float64 `json:"net_rx_mb"`
	NetTxMB        float64 `json:"net_tx_mb"`
	BlockReadMB    float64 `json:"block_read_mb"`
	BlockWriteMB   float64 `json:"block_write_mb"`
	PIDs           int     `json:"pids"`
}

// ContainerSummary aggregates container metrics across a run.
type ContainerSummary struct {
	// Per-container aggregated metrics
	Containers []ContainerAggregates `json:"containers"`
	// Total containers observed
	TotalContainers int `json:"total_containers"`
	// Which container used the most CPU
	TopCPUContainer string `json:"top_cpu_container,omitempty"`
	// Which container used the most memory
	TopMemoryContainer string `json:"top_memory_container,omitempty"`
}

// ContainerAggregates holds aggregated metrics for a single container.
type ContainerAggregates struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Image            string  `json:"image,omitempty"`
	CPUPercentAvg    float64 `json:"cpu_percent_avg"`
	CPUPercentPeak   float64 `json:"cpu_percent_peak"`
	CPUPercentP95    float64 `json:"cpu_percent_p95"`
	MemoryUsedMiBAvg float64 `json:"memory_used_mib_avg"`
	MemoryUsedMiBPeak float64 `json:"memory_used_mib_peak"`
	MemoryLimitMiB   float64 `json:"memory_limit_mib,omitempty"`
	NetRxMBTotal     float64 `json:"net_rx_mb_total"`
	NetTxMBTotal     float64 `json:"net_tx_mb_total"`
	BlockReadMBTotal float64 `json:"block_read_mb_total"`
	BlockWriteMBTotal float64 `json:"block_write_mb_total"`
	SampleCount      int     `json:"sample_count"`
}

// detectContainers returns metrics for all running containers.
func detectContainers() ([]ContainerMetrics, bool) {
	if runtime.GOOS != "linux" {
		return nil, false
	}

	// Try docker stats first
	if containers, ok := detectDockerContainers(); ok {
		return containers, true
	}

	// Try cgroup-based detection (works inside containers and with podman)
	if containers, ok := detectCgroupContainers(); ok {
		return containers, true
	}

	return nil, false
}

// detectDockerContainers uses docker stats to get container metrics.
func detectDockerContainers() ([]ContainerMetrics, bool) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, false
	}

	// Check if docker daemon is accessible
	if err := exec.Command(dockerPath, "info").Run(); err != nil {
		return nil, false
	}

	// Get container stats in one shot
	cmd := exec.Command(dockerPath, "stats", "--no-stream",
		"--format", "{{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}\t{{.PIDs}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var containers []ContainerMetrics
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}

		container := ContainerMetrics{
			ID:   fields[0],
			Name: fields[1],
		}

		// Parse CPU percent (format: "0.50%")
		cpuStr := strings.TrimSuffix(fields[2], "%")
		container.CPUPercent, _ = strconv.ParseFloat(cpuStr, 64)

		// Parse memory usage (format: "100MiB / 1GiB")
		memParts := strings.Split(fields[3], " / ")
		if len(memParts) == 2 {
			container.MemoryUsedMiB = parseMemoryString(memParts[0])
			container.MemoryLimitMiB = parseMemoryString(memParts[1])
		}

		// Parse memory percent
		memPctStr := strings.TrimSuffix(fields[4], "%")
		container.MemoryPercent, _ = strconv.ParseFloat(memPctStr, 64)

		// Parse network I/O (format: "1.5kB / 2.3kB")
		netParts := strings.Split(fields[5], " / ")
		if len(netParts) == 2 {
			container.NetRxMB = parseSizeToMB(netParts[0])
			container.NetTxMB = parseSizeToMB(netParts[1])
		}

		// Parse block I/O
		blockParts := strings.Split(fields[6], " / ")
		if len(blockParts) == 2 {
			container.BlockReadMB = parseSizeToMB(blockParts[0])
			container.BlockWriteMB = parseSizeToMB(blockParts[1])
		}

		// Parse PIDs
		container.PIDs, _ = strconv.Atoi(fields[7])

		containers = append(containers, container)
	}

	if len(containers) == 0 {
		return nil, false
	}

	// Get image info for each container
	for i, c := range containers {
		cmd := exec.Command(dockerPath, "inspect", "--format", "{{.Config.Image}}", c.ID)
		if out, err := cmd.Output(); err == nil {
			containers[i].Image = strings.TrimSpace(string(out))
		}
	}

	return containers, true
}

// detectCgroupContainers reads cgroup data directly for container detection.
// This works inside containers and doesn't require docker CLI.
func detectCgroupContainers() ([]ContainerMetrics, bool) {
	// Check if we're in a cgroup v2 environment
	cgroupPath := "/sys/fs/cgroup"

	// Try to find container cgroups
	containers := make(map[string]*ContainerMetrics)

	// Look for docker/podman cgroup paths
	patterns := []string{
		"/sys/fs/cgroup/docker/*/",
		"/sys/fs/cgroup/system.slice/docker-*.scope/",
		"/sys/fs/cgroup/kubepods/*/pod*/*/",
		"/sys/fs/cgroup/kubepods.slice/*/pod*/*/",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			id := extractContainerID(match)
			if id == "" {
				continue
			}

			if _, exists := containers[id]; exists {
				continue
			}

			metrics := &ContainerMetrics{
				ID:   id[:12], // Short ID
				Name: id[:12],
			}

			// Read CPU stats
			if cpuUsage := readCgroupCPU(match); cpuUsage >= 0 {
				metrics.CPUPercent = cpuUsage
			}

			// Read memory stats
			if memUsed, memLimit := readCgroupMemory(match); memUsed >= 0 {
				metrics.MemoryUsedMiB = memUsed
				metrics.MemoryLimitMiB = memLimit
				if memLimit > 0 {
					metrics.MemoryPercent = (memUsed / memLimit) * 100
				}
			}

			containers[id] = metrics
		}
	}

	// Also check cgroup v2 unified hierarchy
	v2Containers := detectCgroupV2Containers(cgroupPath)
	for id, metrics := range v2Containers {
		if _, exists := containers[id]; !exists {
			containers[id] = metrics
		}
	}

	if len(containers) == 0 {
		return nil, false
	}

	result := make([]ContainerMetrics, 0, len(containers))
	for _, c := range containers {
		result = append(result, *c)
	}
	return result, true
}

// detectCgroupV2Containers finds containers in cgroup v2 unified hierarchy.
func detectCgroupV2Containers(basePath string) map[string]*ContainerMetrics {
	containers := make(map[string]*ContainerMetrics)

	// Look for container scope directories
	filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		name := d.Name()
		// Match docker-*.scope or cri-containerd-*.scope patterns
		if strings.HasPrefix(name, "docker-") && strings.HasSuffix(name, ".scope") {
			id := strings.TrimPrefix(name, "docker-")
			id = strings.TrimSuffix(id, ".scope")
			if len(id) >= 12 {
				containers[id] = &ContainerMetrics{
					ID:   id[:12],
					Name: id[:12],
				}
			}
		}
		return nil
	})

	return containers
}

func extractContainerID(path string) string {
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]
		// Docker container IDs are 64-char hex strings
		if len(p) == 64 && isHex(p) {
			return p
		}
		// Docker scope names
		if strings.HasPrefix(p, "docker-") && strings.HasSuffix(p, ".scope") {
			id := strings.TrimPrefix(p, "docker-")
			return strings.TrimSuffix(id, ".scope")
		}
	}
	return ""
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func readCgroupCPU(cgroupPath string) float64 {
	// Try cgroup v2 cpu.stat
	statPath := filepath.Join(cgroupPath, "cpu.stat")
	if data, err := os.ReadFile(statPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "usage_usec ") {
				// This is total usage, not percent - would need periodic sampling
				return 0
			}
		}
	}

	// Try cgroup v1
	usagePath := filepath.Join(cgroupPath, "cpuacct.usage")
	if data, err := os.ReadFile(usagePath); err == nil {
		usage, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		// Convert nanoseconds to something usable
		_ = usage
		return 0 // Would need delta calculation
	}

	return -1
}

func readCgroupMemory(cgroupPath string) (usedMiB, limitMiB float64) {
	// Try cgroup v2
	currentPath := filepath.Join(cgroupPath, "memory.current")
	if data, err := os.ReadFile(currentPath); err == nil {
		bytes, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		usedMiB = float64(bytes) / (1024 * 1024)
	}

	maxPath := filepath.Join(cgroupPath, "memory.max")
	if data, err := os.ReadFile(maxPath); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" {
			bytes, _ := strconv.ParseInt(s, 10, 64)
			limitMiB = float64(bytes) / (1024 * 1024)
		}
	}

	// Try cgroup v1 if v2 failed
	if usedMiB == 0 {
		usagePath := filepath.Join(cgroupPath, "memory.usage_in_bytes")
		if data, err := os.ReadFile(usagePath); err == nil {
			bytes, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
			usedMiB = float64(bytes) / (1024 * 1024)
		}

		limitPath := filepath.Join(cgroupPath, "memory.limit_in_bytes")
		if data, err := os.ReadFile(limitPath); err == nil {
			bytes, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
			// Ignore unrealistic limits (near max int64)
			if bytes < 1<<62 {
				limitMiB = float64(bytes) / (1024 * 1024)
			}
		}
	}

	return
}

func parseMemoryString(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	multiplier := 1.0
	if strings.HasSuffix(s, "GIB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "GIB")
	} else if strings.HasSuffix(s, "MIB") {
		multiplier = 1
		s = strings.TrimSuffix(s, "MIB")
	} else if strings.HasSuffix(s, "KIB") {
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "KIB")
	} else if strings.HasSuffix(s, "GB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1.0 / (1024 * 1024)
		s = strings.TrimSuffix(s, "B")
	}

	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return val * multiplier
}

func parseSizeToMB(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	multiplier := 1.0 / (1024 * 1024) // Default: bytes to MB

	if strings.HasSuffix(s, "GB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1.0 / (1024 * 1024)
		s = strings.TrimSuffix(s, "B")
	}

	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return val * multiplier
}

// buildContainerSummary aggregates container metrics across snapshots.
func buildContainerSummary(snapshots [][]ContainerMetrics) *ContainerSummary {
	if len(snapshots) == 0 {
		return nil
	}

	// Aggregate by container ID
	aggregates := make(map[string]*struct {
		metrics   ContainerAggregates
		cpuVals   []float64
		memVals   []float64
	})

	for _, snapshot := range snapshots {
		for _, c := range snapshot {
			agg, exists := aggregates[c.ID]
			if !exists {
				agg = &struct {
					metrics ContainerAggregates
					cpuVals []float64
					memVals []float64
				}{
					metrics: ContainerAggregates{
						ID:    c.ID,
						Name:  c.Name,
						Image: c.Image,
					},
				}
				aggregates[c.ID] = agg
			}

			agg.cpuVals = append(agg.cpuVals, c.CPUPercent)
			agg.memVals = append(agg.memVals, c.MemoryUsedMiB)

			if c.CPUPercent > agg.metrics.CPUPercentPeak {
				agg.metrics.CPUPercentPeak = c.CPUPercent
			}
			if c.MemoryUsedMiB > agg.metrics.MemoryUsedMiBPeak {
				agg.metrics.MemoryUsedMiBPeak = c.MemoryUsedMiB
			}
			if c.MemoryLimitMiB > agg.metrics.MemoryLimitMiB {
				agg.metrics.MemoryLimitMiB = c.MemoryLimitMiB
			}

			agg.metrics.NetRxMBTotal = c.NetRxMB // Last value (cumulative)
			agg.metrics.NetTxMBTotal = c.NetTxMB
			agg.metrics.BlockReadMBTotal = c.BlockReadMB
			agg.metrics.BlockWriteMBTotal = c.BlockWriteMB
			agg.metrics.SampleCount++
		}
	}

	summary := &ContainerSummary{
		TotalContainers: len(aggregates),
	}

	var maxCPU, maxMem float64
	for _, agg := range aggregates {
		// Calculate averages and P95
		agg.metrics.CPUPercentAvg = avg(agg.cpuVals)
		agg.metrics.CPUPercentP95 = percentile(agg.cpuVals, 95)
		agg.metrics.MemoryUsedMiBAvg = avg(agg.memVals)

		summary.Containers = append(summary.Containers, agg.metrics)

		// Track top consumers
		if agg.metrics.CPUPercentPeak > maxCPU {
			maxCPU = agg.metrics.CPUPercentPeak
			summary.TopCPUContainer = agg.metrics.Name
		}
		if agg.metrics.MemoryUsedMiBPeak > maxMem {
			maxMem = agg.metrics.MemoryUsedMiBPeak
			summary.TopMemoryContainer = agg.metrics.Name
		}
	}

	// Sort containers by peak CPU usage
	sort.Slice(summary.Containers, func(i, j int) bool {
		return summary.Containers[i].CPUPercentPeak > summary.Containers[j].CPUPercentPeak
	})

	return summary
}
