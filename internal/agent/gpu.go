package agent

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GPUVendor identifies the GPU manufacturer.
type GPUVendor string

const (
	GPUVendorNvidia GPUVendor = "nvidia"
	GPUVendorAMD    GPUVendor = "amd"
	GPUVendorIntel  GPUVendor = "intel"
	GPUVendorUnknown GPUVendor = "unknown"
)

// GPUSnapshot captures a point-in-time GPU utilization reading.
type GPUSnapshot struct {
	Index           int       `json:"index"`
	Name            string    `json:"name"`
	Vendor          GPUVendor `json:"vendor"`
	UtilizationPct  float64   `json:"utilization_pct"`   // GPU compute utilization (0-100)
	MemoryUsedMiB   float64   `json:"memory_used_mib"`
	MemoryTotalMiB  float64   `json:"memory_total_mib"`
	MemoryUtilPct   float64   `json:"memory_util_pct"`   // GPU memory utilization (0-100)
	TemperatureC    float64   `json:"temperature_c"`
	PowerDrawW      float64   `json:"power_draw_w"`
	PowerLimitW     float64   `json:"power_limit_w"`
}

// GPUSummary aggregates GPU metrics across a run.
type GPUSummary struct {
	Count                 int     `json:"count"`
	TotalMemoryGiB        float64 `json:"total_memory_gib"`
	AvgUtilizationPct     float64 `json:"avg_utilization_pct"`
	PeakUtilizationPct    float64 `json:"peak_utilization_pct"`
	P95UtilizationPct     float64 `json:"p95_utilization_pct"`
	AvgMemoryUtilPct      float64 `json:"avg_memory_util_pct"`
	PeakMemoryUtilPct     float64 `json:"peak_memory_util_pct"`
	P95MemoryUtilPct      float64 `json:"p95_memory_util_pct"`
	AvgPowerDrawW         float64 `json:"avg_power_draw_w"`
	PeakPowerDrawW        float64 `json:"peak_power_draw_w"`
	// WasteIndicators help identify GPU underutilization.
	IdleSamplesPct        float64 `json:"idle_samples_pct"`         // % of samples with <5% utilization
	UnderutilizedPct      float64 `json:"underutilized_pct"`        // % of samples with <30% utilization
	GPUType               string  `json:"gpu_type,omitempty"`       // e.g. "NVIDIA A100", "AMD MI250X"
	Vendor                string  `json:"vendor,omitempty"`         // nvidia, amd, intel
}

// detectGPUs checks for available GPUs from all supported vendors.
func detectGPUs() ([]GPUSnapshot, bool) {
	var allGPUs []GPUSnapshot

	// Try NVIDIA GPUs (nvidia-smi)
	if gpus, ok := detectNvidiaGPUs(); ok {
		allGPUs = append(allGPUs, gpus...)
	}

	// Try AMD GPUs (rocm-smi)
	if gpus, ok := detectAMDGPUs(); ok {
		allGPUs = append(allGPUs, gpus...)
	}

	// Try Intel GPUs (intel_gpu_top or xpu-smi)
	if gpus, ok := detectIntelGPUs(); ok {
		allGPUs = append(allGPUs, gpus...)
	}

	// Fallback to sysfs detection if no vendor tools available
	if len(allGPUs) == 0 {
		if gpus, ok := detectGPUsViaSysfs(); ok {
			allGPUs = append(allGPUs, gpus...)
		}
	}

	if len(allGPUs) == 0 {
		return nil, false
	}
	return allGPUs, true
}

// detectNvidiaGPUs uses nvidia-smi to query NVIDIA GPU metrics.
func detectNvidiaGPUs() ([]GPUSnapshot, bool) {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, false
	}

	cmd := exec.Command(path,
		"--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,power.limit",
		"--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var gpus []GPUSnapshot
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ", ")
		if len(fields) < 8 {
			continue
		}

		gpu := GPUSnapshot{Vendor: GPUVendorNvidia}
		gpu.Index, _ = strconv.Atoi(strings.TrimSpace(fields[0]))
		gpu.Name = strings.TrimSpace(fields[1])
		gpu.UtilizationPct, _ = strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		gpu.MemoryUsedMiB, _ = strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		gpu.MemoryTotalMiB, _ = strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		gpu.TemperatureC, _ = strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
		gpu.PowerDrawW, _ = strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)
		gpu.PowerLimitW, _ = strconv.ParseFloat(strings.TrimSpace(fields[7]), 64)

		if gpu.MemoryTotalMiB > 0 {
			gpu.MemoryUtilPct = (gpu.MemoryUsedMiB / gpu.MemoryTotalMiB) * 100
		}

		gpus = append(gpus, gpu)
	}

	if len(gpus) == 0 {
		return nil, false
	}
	return gpus, true
}

// detectAMDGPUs uses rocm-smi to query AMD GPU metrics (ROCm/HIP).
func detectAMDGPUs() ([]GPUSnapshot, bool) {
	path, err := exec.LookPath("rocm-smi")
	if err != nil {
		return nil, false
	}

	// Get GPU list with utilization
	cmd := exec.Command(path, "--showuse", "--showmemuse", "--showtemp", "--showpower", "--csv")
	output, err := cmd.Output()
	if err != nil {
		// Try alternative format
		return detectAMDGPUsAlternate(path)
	}

	var gpus []GPUSnapshot
	lines := strings.Split(string(output), "\n")
	
	// Skip header line
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		
		fields := strings.Split(line, ",")
		if len(fields) < 5 {
			continue
		}

		gpu := GPUSnapshot{Vendor: GPUVendorAMD}
		
		// Parse device index from first field (e.g., "card0" or "0")
		devStr := strings.TrimSpace(fields[0])
		devStr = strings.TrimPrefix(devStr, "card")
		gpu.Index, _ = strconv.Atoi(devStr)
		
		// Parse utilization percentage
		if len(fields) > 1 {
			utilStr := strings.TrimSpace(fields[1])
			utilStr = strings.TrimSuffix(utilStr, "%")
			gpu.UtilizationPct, _ = strconv.ParseFloat(utilStr, 64)
		}
		
		// Parse memory utilization
		if len(fields) > 2 {
			memStr := strings.TrimSpace(fields[2])
			memStr = strings.TrimSuffix(memStr, "%")
			gpu.MemoryUtilPct, _ = strconv.ParseFloat(memStr, 64)
		}
		
		// Parse temperature
		if len(fields) > 3 {
			tempStr := strings.TrimSpace(fields[3])
			tempStr = strings.TrimSuffix(tempStr, "c")
			tempStr = strings.TrimSuffix(tempStr, "C")
			gpu.TemperatureC, _ = strconv.ParseFloat(tempStr, 64)
		}
		
		// Parse power
		if len(fields) > 4 {
			powerStr := strings.TrimSpace(fields[4])
			powerStr = strings.TrimSuffix(powerStr, "W")
			powerStr = strings.TrimSuffix(powerStr, "w")
			gpu.PowerDrawW, _ = strconv.ParseFloat(powerStr, 64)
		}

		gpus = append(gpus, gpu)
	}

	if len(gpus) == 0 {
		return nil, false
	}

	// Get GPU names separately
	enrichAMDGPUNames(path, gpus)
	enrichAMDGPUMemory(path, gpus)

	return gpus, true
}

// detectAMDGPUsAlternate uses rocm-smi with JSON output as fallback.
func detectAMDGPUsAlternate(rocmPath string) ([]GPUSnapshot, bool) {
	// Try with individual queries
	cmd := exec.Command(rocmPath, "--showallinfo")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var gpus []GPUSnapshot
	var currentGPU *GPUSnapshot
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// New GPU section
		if strings.HasPrefix(line, "GPU[") {
			if currentGPU != nil {
				gpus = append(gpus, *currentGPU)
			}
			currentGPU = &GPUSnapshot{Vendor: GPUVendorAMD}
			// Extract index from GPU[0], GPU[1], etc.
			idxStr := strings.TrimPrefix(line, "GPU[")
			idxStr = strings.Split(idxStr, "]")[0]
			currentGPU.Index, _ = strconv.Atoi(idxStr)
		}
		
		if currentGPU == nil {
			continue
		}
		
		// Parse various fields
		if strings.Contains(line, "GPU use") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "%")
				currentGPU.UtilizationPct, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "GPU memory use") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "%")
				currentGPU.MemoryUtilPct, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "Temperature") && strings.Contains(line, "edge") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "c")
				valStr = strings.TrimSuffix(valStr, "C")
				currentGPU.TemperatureC, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "Average Graphics Package Power") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "W")
				currentGPU.PowerDrawW, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "Card series") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentGPU.Name = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "VRAM Total Memory") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				// Could be in MB or GB
				if strings.Contains(valStr, "GB") {
					valStr = strings.TrimSuffix(valStr, "GB")
					val, _ := strconv.ParseFloat(strings.TrimSpace(valStr), 64)
					currentGPU.MemoryTotalMiB = val * 1024
				} else {
					valStr = strings.TrimSuffix(valStr, "MB")
					currentGPU.MemoryTotalMiB, _ = strconv.ParseFloat(strings.TrimSpace(valStr), 64)
				}
			}
		}
	}
	
	if currentGPU != nil {
		gpus = append(gpus, *currentGPU)
	}

	if len(gpus) == 0 {
		return nil, false
	}
	return gpus, true
}

func enrichAMDGPUNames(rocmPath string, gpus []GPUSnapshot) {
	cmd := exec.Command(rocmPath, "--showproductname")
	output, _ := cmd.Output()
	
	lines := strings.Split(string(output), "\n")
	gpuIdx := 0
	for _, line := range lines {
		if strings.Contains(line, "Card series") || strings.Contains(line, "Card model") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 && gpuIdx < len(gpus) {
				gpus[gpuIdx].Name = strings.TrimSpace(parts[1])
				gpuIdx++
			}
		}
	}
}

func enrichAMDGPUMemory(rocmPath string, gpus []GPUSnapshot) {
	cmd := exec.Command(rocmPath, "--showmeminfo", "vram")
	output, _ := cmd.Output()
	
	lines := strings.Split(string(output), "\n")
	gpuIdx := 0
	for _, line := range lines {
		if strings.Contains(line, "Total Memory") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 && gpuIdx < len(gpus) {
				valStr := strings.TrimSpace(parts[1])
				// Parse value (typically in bytes or with unit)
				val, _ := strconv.ParseInt(strings.Fields(valStr)[0], 10, 64)
				gpus[gpuIdx].MemoryTotalMiB = float64(val) / (1024 * 1024)
			}
		}
		if strings.Contains(line, "Total Used Memory") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 && gpuIdx < len(gpus) {
				valStr := strings.TrimSpace(parts[1])
				val, _ := strconv.ParseInt(strings.Fields(valStr)[0], 10, 64)
				gpus[gpuIdx].MemoryUsedMiB = float64(val) / (1024 * 1024)
				gpuIdx++
			}
		}
	}
}

// detectIntelGPUs uses intel_gpu_top or xpu-smi for Intel GPUs (Arc, Data Center).
func detectIntelGPUs() ([]GPUSnapshot, bool) {
	// Try xpu-smi first (Intel Data Center GPUs like Max/Ponte Vecchio)
	if gpus, ok := detectIntelXPU(); ok {
		return gpus, true
	}
	
	// Try intel_gpu_top for integrated/Arc GPUs
	if gpus, ok := detectIntelGPUTop(); ok {
		return gpus, true
	}
	
	return nil, false
}

// detectIntelXPU uses xpu-smi for Intel Data Center GPUs.
func detectIntelXPU() ([]GPUSnapshot, bool) {
	path, err := exec.LookPath("xpu-smi")
	if err != nil {
		return nil, false
	}

	// List all devices
	cmd := exec.Command(path, "discovery")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var gpus []GPUSnapshot
	lines := strings.Split(string(output), "\n")
	
	for _, line := range lines {
		// Look for device entries
		if !strings.Contains(line, "Device ID") {
			continue
		}
		
		gpu := GPUSnapshot{Vendor: GPUVendorIntel}
		
		// Parse device ID
		if idx := strings.Index(line, "Device ID:"); idx >= 0 {
			valStr := strings.TrimSpace(line[idx+10:])
			gpu.Index, _ = strconv.Atoi(strings.Fields(valStr)[0])
		}
		
		gpus = append(gpus, gpu)
	}

	if len(gpus) == 0 {
		return nil, false
	}

	// Enrich with stats
	for i := range gpus {
		enrichIntelGPUStats(path, &gpus[i])
	}

	return gpus, true
}

func enrichIntelGPUStats(xpuPath string, gpu *GPUSnapshot) {
	// Get stats for specific device
	cmd := exec.Command(xpuPath, "stats", "-d", strconv.Itoa(gpu.Index))
	output, err := cmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if strings.Contains(line, "GPU Utilization") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "%")
				gpu.UtilizationPct, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "GPU Memory Used") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "MiB")
				valStr = strings.TrimSuffix(valStr, "MB")
				gpu.MemoryUsedMiB, _ = strconv.ParseFloat(strings.TrimSpace(valStr), 64)
			}
		}
		if strings.Contains(line, "GPU Memory Total") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "MiB")
				valStr = strings.TrimSuffix(valStr, "MB")
				gpu.MemoryTotalMiB, _ = strconv.ParseFloat(strings.TrimSpace(valStr), 64)
			}
		}
		if strings.Contains(line, "GPU Temperature") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "C")
				gpu.TemperatureC, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "GPU Power") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				valStr := strings.TrimSpace(parts[1])
				valStr = strings.TrimSuffix(valStr, "W")
				gpu.PowerDrawW, _ = strconv.ParseFloat(valStr, 64)
			}
		}
		if strings.Contains(line, "Device Name") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				gpu.Name = strings.TrimSpace(parts[1])
			}
		}
	}

	if gpu.MemoryTotalMiB > 0 {
		gpu.MemoryUtilPct = (gpu.MemoryUsedMiB / gpu.MemoryTotalMiB) * 100
	}
}

// detectIntelGPUTop uses intel_gpu_top for consumer/integrated Intel GPUs.
func detectIntelGPUTop() ([]GPUSnapshot, bool) {
	path, err := exec.LookPath("intel_gpu_top")
	if err != nil {
		return nil, false
	}

	// Run for 1 second to get a sample
	cmd := exec.Command(path, "-s", "1000", "-o", "-")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	// Parse intel_gpu_top output
	gpu := GPUSnapshot{
		Vendor: GPUVendorIntel,
		Index:  0,
		Name:   "Intel Integrated Graphics",
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Look for render/3D engine utilization
		if strings.Contains(line, "Render/3D") || strings.Contains(line, "render") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.HasSuffix(f, "%") {
					valStr := strings.TrimSuffix(f, "%")
					gpu.UtilizationPct, _ = strconv.ParseFloat(valStr, 64)
					break
				}
			}
		}
	}

	if gpu.UtilizationPct == 0 {
		return nil, false
	}

	return []GPUSnapshot{gpu}, true
}

// detectGPUsViaSysfs detects GPUs via /sys/class/drm (Linux only) with vendor identification.
func detectGPUsViaSysfs() ([]GPUSnapshot, bool) {
	drmPath := "/sys/class/drm"
	entries, err := os.ReadDir(drmPath)
	if err != nil {
		return nil, false
	}

	var gpus []GPUSnapshot
	seen := make(map[string]bool)
	
	for _, entry := range entries {
		name := entry.Name()
		// Look for card0, card1, etc. (not renderD* or card0-*)
		if !strings.HasPrefix(name, "card") || strings.Contains(name, "-") {
			continue
		}
		
		if seen[name] {
			continue
		}
		
		// Check if it's a real GPU by looking for device/vendor
		vendorPath := filepath.Join(drmPath, name, "device", "vendor")
		vendorData, err := os.ReadFile(vendorPath)
		if err != nil {
			continue
		}
		
		seen[name] = true
		vendorID := strings.TrimSpace(string(vendorData))
		
		// Extract card index
		cardIdx, _ := strconv.Atoi(strings.TrimPrefix(name, "card"))
		
		gpu := GPUSnapshot{
			Index: cardIdx,
		}
		
		// Identify vendor by PCI vendor ID
		switch vendorID {
		case "0x10de": // NVIDIA
			gpu.Vendor = GPUVendorNvidia
			gpu.Name = "NVIDIA GPU"
		case "0x1002": // AMD
			gpu.Vendor = GPUVendorAMD
			gpu.Name = "AMD GPU"
		case "0x8086": // Intel
			gpu.Vendor = GPUVendorIntel
			gpu.Name = "Intel GPU"
		default:
			gpu.Vendor = GPUVendorUnknown
			gpu.Name = "Unknown GPU"
		}
		
		// Try to get more details from device name
		devicePath := filepath.Join(drmPath, name, "device", "device")
		if deviceData, err := os.ReadFile(devicePath); err == nil {
			gpu.Name += " (" + strings.TrimSpace(string(deviceData)) + ")"
		}
		
		gpus = append(gpus, gpu)
	}
	
	if len(gpus) == 0 {
		return nil, false
	}
	return gpus, true
}

// collectGPUMetrics samples current GPU utilization from all available vendors.
func collectGPUMetrics() []GPUSnapshot {
	var allGPUs []GPUSnapshot

	// Collect from all vendors
	if gpus, ok := detectNvidiaGPUs(); ok {
		allGPUs = append(allGPUs, gpus...)
	}
	if gpus, ok := detectAMDGPUs(); ok {
		allGPUs = append(allGPUs, gpus...)
	}
	if gpus, ok := detectIntelGPUs(); ok {
		allGPUs = append(allGPUs, gpus...)
	}

	return allGPUs
}

// buildGPUSummary aggregates GPU snapshots into a summary.
func buildGPUSummary(snapshots [][]GPUSnapshot) *GPUSummary {
	if len(snapshots) == 0 {
		return nil
	}

	// Flatten and aggregate
	var allUtils, allMemUtils, allPower []float64
	var totalMem float64
	var gpuCount int
	var gpuType string
	var vendor GPUVendor
	var idleSamples, underutilizedSamples int

	for _, gpuList := range snapshots {
		for _, gpu := range gpuList {
			if gpuCount == 0 {
				gpuCount = len(gpuList)
				gpuType = gpu.Name
				vendor = gpu.Vendor
				totalMem = gpu.MemoryTotalMiB / 1024.0 * float64(len(gpuList)) // Convert to GiB
			}

			allUtils = append(allUtils, gpu.UtilizationPct)
			allMemUtils = append(allMemUtils, gpu.MemoryUtilPct)
			allPower = append(allPower, gpu.PowerDrawW)

			if gpu.UtilizationPct < 5 {
				idleSamples++
			}
			if gpu.UtilizationPct < 30 {
				underutilizedSamples++
			}
		}
	}

	if len(allUtils) == 0 {
		return nil
	}

	summary := &GPUSummary{
		Count:          gpuCount,
		TotalMemoryGiB: totalMem,
		GPUType:        gpuType,
		Vendor:         string(vendor),
	}

	// Compute aggregates
	summary.AvgUtilizationPct = avg(allUtils)
	summary.PeakUtilizationPct = max(allUtils)
	summary.P95UtilizationPct = percentile(allUtils, 95)

	summary.AvgMemoryUtilPct = avg(allMemUtils)
	summary.PeakMemoryUtilPct = max(allMemUtils)
	summary.P95MemoryUtilPct = percentile(allMemUtils, 95)

	summary.AvgPowerDrawW = avg(allPower)
	summary.PeakPowerDrawW = max(allPower)

	n := float64(len(allUtils))
	summary.IdleSamplesPct = float64(idleSamples) / n * 100
	summary.UnderutilizedPct = float64(underutilizedSamples) / n * 100

	return summary
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func max(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
