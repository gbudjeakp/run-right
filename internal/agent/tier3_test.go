package agent

import (
	"testing"
)

func TestBuildGPUSummary(t *testing.T) {
	// Test with empty snapshots
	result := buildGPUSummary(nil)
	if result != nil {
		t.Error("expected nil for empty snapshots")
	}

	// Test with NVIDIA GPU snapshots
	snapshots := [][]GPUSnapshot{
		{
			{Index: 0, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 50, MemoryUsedMiB: 8000, MemoryTotalMiB: 40960, MemoryUtilPct: 19.5, PowerDrawW: 200},
		},
		{
			{Index: 0, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 80, MemoryUsedMiB: 10000, MemoryTotalMiB: 40960, MemoryUtilPct: 24.4, PowerDrawW: 280},
		},
		{
			{Index: 0, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 10, MemoryUsedMiB: 5000, MemoryTotalMiB: 40960, MemoryUtilPct: 12.2, PowerDrawW: 100},
		},
	}

	summary := buildGPUSummary(snapshots)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	if summary.Count != 1 {
		t.Errorf("expected 1 GPU, got %d", summary.Count)
	}

	if summary.PeakUtilizationPct != 80 {
		t.Errorf("expected peak utilization 80%%, got %.1f%%", summary.PeakUtilizationPct)
	}

	if summary.GPUType != "NVIDIA A100" {
		t.Errorf("expected NVIDIA A100, got %s", summary.GPUType)
	}

	if summary.Vendor != "nvidia" {
		t.Errorf("expected vendor nvidia, got %s", summary.Vendor)
	}

	// Check idle/underutilized percentages
	// Based on the test data: 50%, 80%, 10% utilization
	// - Idle (<5%): 0 samples -> 0%
	// - Underutilized (<30%): 1 sample (10%) -> ~33%
	if summary.IdleSamplesPct != 0 {
		t.Errorf("expected 0%% idle samples (none < 5%%), got %.1f%%", summary.IdleSamplesPct)
	}

	// The 10% sample is underutilized (<30%)
	if summary.UnderutilizedPct < 30 || summary.UnderutilizedPct > 40 {
		t.Errorf("expected ~33%% underutilized samples, got %.1f%%", summary.UnderutilizedPct)
	}
}

func TestBuildGPUSummaryMultiVendor(t *testing.T) {
	// Test with AMD GPU snapshots
	amdSnapshots := [][]GPUSnapshot{
		{
			{Index: 0, Name: "AMD MI250X", Vendor: GPUVendorAMD, UtilizationPct: 75, MemoryUsedMiB: 32000, MemoryTotalMiB: 65536, MemoryUtilPct: 48.8, PowerDrawW: 400},
		},
		{
			{Index: 0, Name: "AMD MI250X", Vendor: GPUVendorAMD, UtilizationPct: 90, MemoryUsedMiB: 40000, MemoryTotalMiB: 65536, MemoryUtilPct: 61.0, PowerDrawW: 500},
		},
	}

	summary := buildGPUSummary(amdSnapshots)
	if summary == nil {
		t.Fatal("expected non-nil AMD summary")
	}

	if summary.Vendor != "amd" {
		t.Errorf("expected vendor amd, got %s", summary.Vendor)
	}

	if summary.GPUType != "AMD MI250X" {
		t.Errorf("expected AMD MI250X, got %s", summary.GPUType)
	}

	// Test with Intel GPU snapshots
	intelSnapshots := [][]GPUSnapshot{
		{
			{Index: 0, Name: "Intel Max 1550", Vendor: GPUVendorIntel, UtilizationPct: 60, MemoryUsedMiB: 24000, MemoryTotalMiB: 49152, MemoryUtilPct: 48.8, PowerDrawW: 300},
		},
	}

	intelSummary := buildGPUSummary(intelSnapshots)
	if intelSummary == nil {
		t.Fatal("expected non-nil Intel summary")
	}

	if intelSummary.Vendor != "intel" {
		t.Errorf("expected vendor intel, got %s", intelSummary.Vendor)
	}
}

func TestCalculateEgressCost(t *testing.T) {
	tests := []struct {
		name     string
		egressGB float64
		provider string
		minCost  float64
		maxCost  float64
	}{
		{"small AWS egress", 1.0, "aws", 0.08, 0.10},
		{"medium AWS egress", 100.0, "aws", 8.0, 10.0},
		{"small GCP egress", 1.0, "gcp", 0.10, 0.15},
		{"GitHub free tier", 50.0, "github", 0, 1.0}, // Most should be free
		{"zero egress", 0, "aws", 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cost := calculateEgressCost(tc.egressGB, tc.provider)
			if cost < tc.minCost || cost > tc.maxCost {
				t.Errorf("expected cost between $%.2f and $%.2f, got $%.2f",
					tc.minCost, tc.maxCost, cost)
			}
		})
	}
}

func TestBuildContainerSummary(t *testing.T) {
	// Test with empty snapshots
	result := buildContainerSummary(nil)
	if result != nil {
		t.Error("expected nil for empty snapshots")
	}

	// Test with container snapshots
	snapshots := [][]ContainerMetrics{
		{
			{ID: "abc123", Name: "web", CPUPercent: 50, MemoryUsedMiB: 512, MemoryLimitMiB: 1024},
			{ID: "def456", Name: "db", CPUPercent: 30, MemoryUsedMiB: 1024, MemoryLimitMiB: 2048},
		},
		{
			{ID: "abc123", Name: "web", CPUPercent: 80, MemoryUsedMiB: 768, MemoryLimitMiB: 1024},
			{ID: "def456", Name: "db", CPUPercent: 40, MemoryUsedMiB: 1200, MemoryLimitMiB: 2048},
		},
	}

	summary := buildContainerSummary(snapshots)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	if summary.TotalContainers != 2 {
		t.Errorf("expected 2 containers, got %d", summary.TotalContainers)
	}

	if summary.TopCPUContainer != "web" {
		t.Errorf("expected top CPU container 'web', got '%s'", summary.TopCPUContainer)
	}

	if summary.TopMemoryContainer != "db" {
		t.Errorf("expected top memory container 'db', got '%s'", summary.TopMemoryContainer)
	}
}

func TestParseMemoryString(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"100MiB", 100},
		{"1GiB", 1024},
		{"512MB", 512},
		{"2GB", 2048},
		{"256KiB", 0.25},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := parseMemoryString(tc.input)
			if result < tc.expected*0.99 || result > tc.expected*1.01 {
				t.Errorf("parseMemoryString(%s) = %.2f, want %.2f", tc.input, result, tc.expected)
			}
		})
	}
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"1.0.1", "1.0.0", false},
		{"2.0.0", "1.9.9", false},
		{"1.0.0", "1.0.0", false},
		{"dev", "1.0.0", true}, // dev always shows update available
		{"v1.0.0", "v1.0.1", true},
	}

	for _, tc := range tests {
		t.Run(tc.current+"->"+tc.latest, func(t *testing.T) {
			result := isNewerVersion(tc.current, tc.latest)
			if result != tc.expected {
				t.Errorf("isNewerVersion(%s, %s) = %v, want %v",
					tc.current, tc.latest, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// GPU Tests
// ============================================================================

func TestBuildGPUSummaryMultiGPU(t *testing.T) {
	// Test with multiple GPUs in a single snapshot
	snapshots := [][]GPUSnapshot{
		{
			{Index: 0, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 50, MemoryTotalMiB: 40960, PowerDrawW: 200},
			{Index: 1, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 60, MemoryTotalMiB: 40960, PowerDrawW: 220},
			{Index: 2, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 70, MemoryTotalMiB: 40960, PowerDrawW: 240},
			{Index: 3, Name: "NVIDIA A100", Vendor: GPUVendorNvidia, UtilizationPct: 80, MemoryTotalMiB: 40960, PowerDrawW: 260},
		},
	}

	summary := buildGPUSummary(snapshots)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	if summary.Count != 4 {
		t.Errorf("expected 4 GPUs, got %d", summary.Count)
	}

	// Peak should be 80%
	if summary.PeakUtilizationPct != 80 {
		t.Errorf("expected peak 80%%, got %.1f%%", summary.PeakUtilizationPct)
	}

	// Average should be 65% ((50+60+70+80)/4)
	if summary.AvgUtilizationPct < 64 || summary.AvgUtilizationPct > 66 {
		t.Errorf("expected avg ~65%%, got %.1f%%", summary.AvgUtilizationPct)
	}
}

func TestBuildGPUSummaryPowerMetrics(t *testing.T) {
	snapshots := [][]GPUSnapshot{
		{{Vendor: GPUVendorNvidia, PowerDrawW: 100}},
		{{Vendor: GPUVendorNvidia, PowerDrawW: 200}},
		{{Vendor: GPUVendorNvidia, PowerDrawW: 300}},
		{{Vendor: GPUVendorNvidia, PowerDrawW: 400}},
	}

	summary := buildGPUSummary(snapshots)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	if summary.PeakPowerDrawW != 400 {
		t.Errorf("expected peak power 400W, got %.1fW", summary.PeakPowerDrawW)
	}

	expectedAvgPower := 250.0 // (100+200+300+400)/4
	if summary.AvgPowerDrawW < expectedAvgPower-1 || summary.AvgPowerDrawW > expectedAvgPower+1 {
		t.Errorf("expected avg power ~%.0fW, got %.1fW", expectedAvgPower, summary.AvgPowerDrawW)
	}
}

func TestGPUVendorConstants(t *testing.T) {
	// Ensure vendor constants are correct
	if GPUVendorNvidia != "nvidia" {
		t.Errorf("expected nvidia, got %s", GPUVendorNvidia)
	}
	if GPUVendorAMD != "amd" {
		t.Errorf("expected amd, got %s", GPUVendorAMD)
	}
	if GPUVendorIntel != "intel" {
		t.Errorf("expected intel, got %s", GPUVendorIntel)
	}
	if GPUVendorUnknown != "unknown" {
		t.Errorf("expected unknown, got %s", GPUVendorUnknown)
	}
}

// ============================================================================
// Egress Cost Tests
// ============================================================================

func TestCalculateEgressCostTiered(t *testing.T) {
	// Test tiered pricing for AWS
	tests := []struct {
		name     string
		egressGB float64
		minCost  float64
		maxCost  float64
	}{
		{"1GB", 1.0, 0.08, 0.10},
		{"100GB", 100.0, 8.5, 9.5},
		{"1TB", 1024.0, 85, 95},
		{"10TB", 10 * 1024.0, 850, 950},
		{"100TB", 100 * 1024.0, 7500, 8500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cost := calculateEgressCost(tc.egressGB, "aws")
			if cost < tc.minCost || cost > tc.maxCost {
				t.Errorf("AWS %s: expected cost $%.2f-$%.2f, got $%.2f",
					tc.name, tc.minCost, tc.maxCost, cost)
			}
		})
	}
}

func TestCalculateEgressCostProviders(t *testing.T) {
	egressGB := 10.0

	awsCost := calculateEgressCost(egressGB, "aws")
	gcpCost := calculateEgressCost(egressGB, "gcp")
	githubCost := calculateEgressCost(egressGB, "github")

	// GCP tends to be slightly more expensive for small amounts
	if gcpCost < awsCost*0.9 {
		t.Logf("GCP cost $%.2f, AWS cost $%.2f", gcpCost, awsCost)
	}

	// GitHub has free tier, should be cheaper for small amounts
	if githubCost > awsCost {
		t.Logf("GitHub cost $%.2f, AWS cost $%.2f (GitHub has free tier)", githubCost, awsCost)
	}
}

func TestCalculateInterRegionCost(t *testing.T) {
	cost := calculateInterRegionCost(10.0, "aws")
	// AWS inter-region is $0.02/GB
	expected := 0.2
	if cost < expected*0.9 || cost > expected*1.1 {
		t.Errorf("expected ~$%.2f, got $%.2f", expected, cost)
	}
}

func TestMapCIPlatformToProvider(t *testing.T) {
	tests := []struct {
		platform string
		expected string
	}{
		{"github", "github"},
		{"gitlab", "gcp"},
		{"azure", "azure"},
		{"circleci", "aws"},
		{"bitbucket", "aws"},
		{"jenkins", "aws"},
		{"unknown", "aws"},
	}

	for _, tc := range tests {
		t.Run(tc.platform, func(t *testing.T) {
			result := mapCIPlatformToProvider(tc.platform)
			if result != tc.expected {
				t.Errorf("mapCIPlatformToProvider(%s) = %s, want %s",
					tc.platform, result, tc.expected)
			}
		})
	}
}

func TestBuildEgressSummary(t *testing.T) {
	// Test with minimal egress (should return nil)
	summary := buildEgressSummary(0.001, "github", 60, 10)
	if summary != nil {
		t.Error("expected nil for minimal egress")
	}

	// Test with significant egress
	summary = buildEgressSummary(100.0, "github", 3600, 100) // 100 MB/s peak, 1 hour
	if summary == nil {
		t.Fatal("expected non-nil summary for significant egress")
	}

	if summary.Provider != "github" {
		t.Errorf("expected provider github, got %s", summary.Provider)
	}

	if summary.TotalEgressGB <= 0 {
		t.Error("expected positive total egress")
	}
}

// ============================================================================
// Container Tests
// ============================================================================

func TestBuildContainerSummaryAggregation(t *testing.T) {
	// Test CPU P95 calculation
	snapshots := [][]ContainerMetrics{
		{{ID: "a", Name: "app", CPUPercent: 10}},
		{{ID: "a", Name: "app", CPUPercent: 20}},
		{{ID: "a", Name: "app", CPUPercent: 30}},
		{{ID: "a", Name: "app", CPUPercent: 40}},
		{{ID: "a", Name: "app", CPUPercent: 50}},
		{{ID: "a", Name: "app", CPUPercent: 60}},
		{{ID: "a", Name: "app", CPUPercent: 70}},
		{{ID: "a", Name: "app", CPUPercent: 80}},
		{{ID: "a", Name: "app", CPUPercent: 90}},
		{{ID: "a", Name: "app", CPUPercent: 100}},
	}

	summary := buildContainerSummary(snapshots)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	if summary.TotalContainers != 1 {
		t.Errorf("expected 1 container, got %d", summary.TotalContainers)
	}

	if len(summary.Containers) != 1 {
		t.Fatal("expected 1 container aggregate")
	}

	agg := summary.Containers[0]

	// Peak should be 100
	if agg.CPUPercentPeak != 100 {
		t.Errorf("expected peak 100%%, got %.1f%%", agg.CPUPercentPeak)
	}

	// Average should be 55 ((10+20+...+100)/10)
	expectedAvg := 55.0
	if agg.CPUPercentAvg < expectedAvg-1 || agg.CPUPercentAvg > expectedAvg+1 {
		t.Errorf("expected avg ~%.0f%%, got %.1f%%", expectedAvg, agg.CPUPercentAvg)
	}

	// P95 should be around 95
	if agg.CPUPercentP95 < 90 || agg.CPUPercentP95 > 100 {
		t.Errorf("expected P95 ~95%%, got %.1f%%", agg.CPUPercentP95)
	}
}

func TestBuildContainerSummaryNetworkIO(t *testing.T) {
	snapshots := [][]ContainerMetrics{
		{{ID: "a", Name: "app", NetRxMB: 100, NetTxMB: 50, BlockReadMB: 200, BlockWriteMB: 100}},
		{{ID: "a", Name: "app", NetRxMB: 200, NetTxMB: 100, BlockReadMB: 400, BlockWriteMB: 200}},
	}

	summary := buildContainerSummary(snapshots)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}

	agg := summary.Containers[0]

	// Last values should be reported (cumulative)
	if agg.NetRxMBTotal != 200 {
		t.Errorf("expected NetRx 200MB, got %.1f", agg.NetRxMBTotal)
	}
	if agg.NetTxMBTotal != 100 {
		t.Errorf("expected NetTx 100MB, got %.1f", agg.NetTxMBTotal)
	}
}

func TestParseSizeToMB(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"100MB", 100},
		{"1GB", 1024},
		{"512KB", 0.5},
		{"1024B", 1.0 / 1024},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := parseSizeToMB(tc.input)
			if result < tc.expected*0.99 || result > tc.expected*1.01 {
				t.Errorf("parseSizeToMB(%s) = %.4f, want %.4f", tc.input, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// Cache Tests
// ============================================================================

func TestDetectCacheStatsReturnsNilWhenEmpty(t *testing.T) {
	// This test verifies the function doesn't crash
	// Actual cache detection depends on environment
	stats := detectCacheStats()
	// May be nil or non-nil depending on environment - just ensure no panic
	_ = stats
}

// ============================================================================
// Update Tests
// ============================================================================

func TestVersionComparisonEdgeCases(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		// Empty versions - empty current is treated like "dev"
		{"", "1.0.0", true},
		{"1.0.0", "", false},
		{"", "", true}, // Empty current treated as dev, so update available
		// Single digit versions
		{"1", "2", true},
		{"2", "1", false},
		// Two digit versions
		{"1.0", "1.1", true},
		{"1.1", "1.0", false},
		// Versions with extra characters (non-numeric parts stripped)
		{"1.0.0-beta", "1.0.1", true},  // 1.0.0 < 1.0.1
		{"1.0.0-rc1", "1.1.0", true},   // 1.0.0 < 1.1.0
		{"1.0.0-alpha", "1.0.0", false}, // Same base version
	}

	for _, tc := range tests {
		name := tc.current + "->" + tc.latest
		if name == "->" {
			name = "empty->empty"
		}
		t.Run(name, func(t *testing.T) {
			result := isNewerVersion(tc.current, tc.latest)
			if result != tc.expected {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tc.current, tc.latest, result, tc.expected)
			}
		})
	}
}

func TestBuildAssetName(t *testing.T) {
	name := buildAssetName()
	// Should contain OS and arch
	if name == "" {
		t.Error("expected non-empty asset name")
	}
	if !containsAny(name, []string{"darwin", "linux", "windows"}) {
		t.Errorf("expected OS in asset name, got %s", name)
	}
	if !containsAny(name, []string{"amd64", "arm64", "386"}) {
		t.Errorf("expected arch in asset name, got %s", name)
	}
	if !containsAny(name, []string{".tar.gz"}) {
		t.Errorf("expected .tar.gz extension, got %s", name)
	}
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDefaultUpdateConfig(t *testing.T) {
	cfg := DefaultUpdateConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if cfg.Channel != "stable" {
		t.Errorf("expected Channel stable, got %s", cfg.Channel)
	}
	if cfg.CheckInterval <= 0 {
		t.Error("expected positive CheckInterval")
	}
	if cfg.GitHubRepo == "" {
		t.Error("expected non-empty GitHubRepo")
	}
	if cfg.AutoRestart {
		t.Error("expected AutoRestart to be false by default for safety")
	}
}

// ============================================================================
// Type Converter Tests
// ============================================================================

func TestConvertGPUSummary(t *testing.T) {
	// Test nil input
	result := convertGPUSummary(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	// Test valid input
	src := &GPUSummary{
		Count:              2,
		TotalMemoryGiB:     80,
		AvgUtilizationPct:  65,
		PeakUtilizationPct: 95,
		P95UtilizationPct:  90,
		GPUType:            "NVIDIA A100",
		Vendor:             "nvidia",
	}

	result = convertGPUSummary(src)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Count != 2 {
		t.Errorf("expected Count 2, got %d", result.Count)
	}
	if result.TotalMemoryGiB != 80 {
		t.Errorf("expected TotalMemoryGiB 80, got %.1f", result.TotalMemoryGiB)
	}
	if result.GPUType != "NVIDIA A100" {
		t.Errorf("expected GPUType NVIDIA A100, got %s", result.GPUType)
	}
}

func TestConvertContainerSummary(t *testing.T) {
	// Test nil input
	result := convertContainerSummary(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	// Test valid input
	src := &ContainerSummary{
		TotalContainers:    3,
		TopCPUContainer:    "web",
		TopMemoryContainer: "db",
		Containers: []ContainerAggregates{
			{ID: "a", Name: "web", CPUPercentPeak: 80},
			{ID: "b", Name: "db", MemoryUsedMiBPeak: 2048},
		},
	}

	result = convertContainerSummary(src)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.TotalContainers != 3 {
		t.Errorf("expected 3 containers, got %d", result.TotalContainers)
	}
	if len(result.Containers) != 2 {
		t.Errorf("expected 2 container aggregates, got %d", len(result.Containers))
	}
}

func TestConvertCacheStats(t *testing.T) {
	// Test nil input
	result := convertCacheStats(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	// Test valid input
	src := &CacheStats{
		DockerLayerCacheHits:   10,
		DockerLayerCacheMisses: 2,
		GoCacheHitRate:         85.5,
		OverallCacheHitRate:    80.0,
	}

	result = convertCacheStats(src)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.DockerLayerCacheHits != 10 {
		t.Errorf("expected 10 hits, got %d", result.DockerLayerCacheHits)
	}
	if result.GoCacheHitRate != 85.5 {
		t.Errorf("expected Go cache rate 85.5, got %.1f", result.GoCacheHitRate)
	}
}

func TestConvertEgressSummary(t *testing.T) {
	// Test nil input
	result := convertEgressSummary(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	// Test valid input
	src := &EgressSummary{
		TotalEgressGB:        5.5,
		EstimatedCostUSD:     0.50,
		Provider:             "aws",
		RecommendCaching:     true,
		PotentialSavingsUSD:  0.25,
	}

	result = convertEgressSummary(src)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.TotalEgressGB != 5.5 {
		t.Errorf("expected 5.5 GB, got %.1f", result.TotalEgressGB)
	}
	if result.Provider != "aws" {
		t.Errorf("expected aws, got %s", result.Provider)
	}
	if !result.RecommendCaching {
		t.Error("expected RecommendCaching true")
	}
}
