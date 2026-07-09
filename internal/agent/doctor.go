package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DiagnosticCheck represents a single diagnostic check.
type DiagnosticCheck struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // "pass", "warn", "fail", "skip"
	Message     string `json:"message,omitempty"`
	Details     string `json:"details,omitempty"`
}

// DiagnosticReport contains all diagnostic results.
type DiagnosticReport struct {
	Timestamp  time.Time          `json:"timestamp"`
	Platform   string             `json:"platform"`
	GoVersion  string             `json:"go_version"`
	Checks     []DiagnosticCheck  `json:"checks"`
	PassCount  int                `json:"pass_count"`
	WarnCount  int                `json:"warn_count"`
	FailCount  int                `json:"fail_count"`
	SkipCount  int                `json:"skip_count"`
}

// RunDiagnostics performs all diagnostic checks.
func RunDiagnostics(ctx context.Context) *DiagnosticReport {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		GoVersion: runtime.Version(),
	}

	checks := []func(context.Context) DiagnosticCheck{
		checkProcFS,
		checkCgroups,
		checkDocker,
		checkNvidiaSMI,
		checkNetworkEgress,
		checkDiskAccess,
		checkProxyConfig,
		checkGitHubAPI,
		checkMemory,
		checkCPUInfo,
	}

	for _, check := range checks {
		c := check(ctx)
		report.Checks = append(report.Checks, c)
		switch c.Status {
		case "pass":
			report.PassCount++
		case "warn":
			report.WarnCount++
		case "fail":
			report.FailCount++
		case "skip":
			report.SkipCount++
		}
	}

	return report
}

func checkProcFS(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "procfs",
		Description: "Access to /proc filesystem for CPU/memory metrics",
	}

	if runtime.GOOS != "linux" {
		check.Status = "skip"
		check.Message = fmt.Sprintf("Not applicable on %s (using gopsutil fallback)", runtime.GOOS)
		return check
	}

	if _, err := os.Stat("/proc/stat"); err != nil {
		check.Status = "fail"
		check.Message = "/proc/stat not accessible"
		check.Details = err.Error()
		return check
	}

	if _, err := os.Stat("/proc/meminfo"); err != nil {
		check.Status = "fail"
		check.Message = "/proc/meminfo not accessible"
		check.Details = err.Error()
		return check
	}

	check.Status = "pass"
	check.Message = "/proc filesystem accessible"
	return check
}

func checkCgroups(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "cgroups",
		Description: "cgroup v2/v1 support for container-aware metrics",
	}

	if runtime.GOOS != "linux" {
		check.Status = "skip"
		check.Message = "cgroups only available on Linux"
		return check
	}

	// Check cgroup v2
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		check.Status = "pass"
		check.Message = "cgroup v2 detected"
		return check
	}

	// Check cgroup v1
	if _, err := os.Stat("/sys/fs/cgroup/cpu/cpu.cfs_quota_us"); err == nil {
		check.Status = "pass"
		check.Message = "cgroup v1 detected"
		return check
	}

	check.Status = "warn"
	check.Message = "No cgroup controllers found (container limits may not be detected)"
	return check
}

func checkDocker(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "docker",
		Description: "Docker socket access for container metrics",
	}

	// Check socket
	socketPath := "/var/run/docker.sock"
	if runtime.GOOS == "darwin" {
		// macOS Docker Desktop
		home, _ := os.UserHomeDir()
		altPath := home + "/.docker/run/docker.sock"
		if _, err := os.Stat(altPath); err == nil {
			socketPath = altPath
		}
	}

	if _, err := os.Stat(socketPath); err != nil {
		check.Status = "skip"
		check.Message = "Docker socket not found (container breakdown disabled)"
		return check
	}

	// Try docker info
	cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	out, err := cmd.Output()
	if err != nil {
		check.Status = "warn"
		check.Message = "Docker socket exists but docker command failed"
		check.Details = err.Error()
		return check
	}

	check.Status = "pass"
	check.Message = fmt.Sprintf("Docker %s accessible", strings.TrimSpace(string(out)))
	return check
}

func checkNvidiaSMI(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "nvidia-smi",
		Description: "NVIDIA GPU monitoring support",
	}

	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	out, err := cmd.Output()
	if err != nil {
		check.Status = "skip"
		check.Message = "nvidia-smi not found (GPU monitoring disabled)"
		return check
	}

	gpus := strings.Split(strings.TrimSpace(string(out)), "\n")
	check.Status = "pass"
	check.Message = fmt.Sprintf("%d GPU(s) detected: %s", len(gpus), strings.Join(gpus, ", "))
	return check
}

func checkNetworkEgress(ctx context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "network",
		Description: "Network egress for updates and catalog sync",
	}

	// Check if we can reach GitHub
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "HEAD", "https://api.github.com", nil)
	resp, err := client.Do(req)
	if err != nil {
		check.Status = "warn"
		check.Message = "Cannot reach GitHub API (updates may fail)"
		check.Details = err.Error()
		return check
	}
	resp.Body.Close()

	check.Status = "pass"
	check.Message = "Network egress working"
	return check
}

func checkDiskAccess(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "disk",
		Description: "Disk I/O metrics access",
	}

	// Try to read disk stats
	if runtime.GOOS == "linux" {
		if _, err := os.ReadFile("/proc/diskstats"); err != nil {
			check.Status = "warn"
			check.Message = "/proc/diskstats not readable (disk metrics limited)"
			check.Details = err.Error()
			return check
		}
	}

	// Check write access to output directory
	tmpFile := ".runright-check-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		check.Status = "warn"
		check.Message = "Cannot write to current directory"
		check.Details = err.Error()
		return check
	}
	os.Remove(tmpFile)

	check.Status = "pass"
	check.Message = "Disk access working"
	return check
}

func checkProxyConfig(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "proxy",
		Description: "HTTP/HTTPS proxy configuration",
	}

	httpProxy := os.Getenv("HTTP_PROXY")
	if httpProxy == "" {
		httpProxy = os.Getenv("http_proxy")
	}
	httpsProxy := os.Getenv("HTTPS_PROXY")
	if httpsProxy == "" {
		httpsProxy = os.Getenv("https_proxy")
	}
	noProxy := os.Getenv("NO_PROXY")
	if noProxy == "" {
		noProxy = os.Getenv("no_proxy")
	}

	if httpProxy == "" && httpsProxy == "" {
		check.Status = "pass"
		check.Message = "No proxy configured (direct connection)"
		return check
	}

	// Proxy is configured, test it
	details := []string{}
	if httpProxy != "" {
		details = append(details, "HTTP_PROXY="+httpProxy)
	}
	if httpsProxy != "" {
		details = append(details, "HTTPS_PROXY="+httpsProxy)
	}
	if noProxy != "" {
		details = append(details, "NO_PROXY="+noProxy)
	}

	check.Status = "pass"
	check.Message = "Proxy configured"
	check.Details = strings.Join(details, ", ")
	return check
}

func checkGitHubAPI(ctx context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "github-releases",
		Description: "GitHub Releases API for auto-updates",
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://api.github.com/repos/gbudjeakp/run-right/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		check.Status = "warn"
		check.Message = "Cannot reach GitHub Releases API"
		check.Details = err.Error()
		return check
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		check.Status = "warn"
		check.Message = "GitHub API rate limited"
		return check
	}

	if resp.StatusCode != 200 {
		check.Status = "warn"
		check.Message = fmt.Sprintf("GitHub API returned status %d", resp.StatusCode)
		return check
	}

	check.Status = "pass"
	check.Message = "GitHub Releases API accessible"
	return check
}

func checkMemory(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "memory",
		Description: "System memory information",
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Get system memory (platform-specific)
	var totalMem uint64
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "MemTotal:") {
					var kb uint64
					fmt.Sscanf(line, "MemTotal: %d kB", &kb)
					totalMem = kb * 1024
					break
				}
			}
		}
	}

	if totalMem > 0 {
		check.Status = "pass"
		check.Message = fmt.Sprintf("%.1f GiB total memory", float64(totalMem)/(1024*1024*1024))
	} else {
		check.Status = "pass"
		check.Message = "Memory metrics available"
	}

	return check
}

func checkCPUInfo(_ context.Context) DiagnosticCheck {
	check := DiagnosticCheck{
		Name:        "cpu",
		Description: "CPU information and architecture",
	}

	numCPU := runtime.NumCPU()
	arch := runtime.GOARCH

	isARM := arch == "arm64" || arch == "arm"
	archLabel := "x86_64"
	if isARM {
		archLabel = "ARM64"
	}

	check.Status = "pass"
	check.Message = fmt.Sprintf("%d cores, %s architecture", numCPU, archLabel)

	// Detect specific ARM chips
	if isARM {
		if runtime.GOOS == "darwin" {
			check.Details = "Apple Silicon detected"
		} else if runtime.GOOS == "linux" {
			// Check for Graviton
			data, _ := os.ReadFile("/proc/cpuinfo")
			if strings.Contains(string(data), "AWS Graviton") {
				check.Details = "AWS Graviton detected"
			} else if strings.Contains(string(data), "Ampere") {
				check.Details = "Ampere Altra detected"
			}
		}
	}

	return check
}

// CheckConnectivity tests connectivity to a specific endpoint.
func CheckConnectivity(ctx context.Context, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
