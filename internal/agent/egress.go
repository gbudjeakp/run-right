package agent

import (
	"os"
	"strings"
)

// EgressPricing contains network egress pricing per GB for different providers.
// Prices are approximate and should be updated periodically.
var EgressPricing = map[string]EgressTiers{
	"aws": {
		// AWS data transfer out to internet (US regions)
		FirstTierGB:    10 * 1024,    // First 10 TB
		FirstTierPrice: 0.09,         // $0.09/GB
		SecondTierGB:   40 * 1024,    // Next 40 TB
		SecondTierPrice: 0.085,       // $0.085/GB
		ThirdTierGB:    100 * 1024,   // Next 100 TB
		ThirdTierPrice: 0.07,         // $0.07/GB
		FinalTierPrice: 0.05,         // >150 TB
		// Inter-region pricing
		InterRegionPrice: 0.02,       // $0.02/GB
		// Same region (AZ to AZ)
		SameRegionPrice: 0.01,        // $0.01/GB
	},
	"gcp": {
		// GCP network egress (Worldwide destinations)
		FirstTierGB:    1 * 1024,     // First 1 TB
		FirstTierPrice: 0.12,         // $0.12/GB
		SecondTierGB:   10 * 1024,    // 1-10 TB
		SecondTierPrice: 0.11,        // $0.11/GB
		ThirdTierGB:    100 * 1024,   // 10+ TB
		ThirdTierPrice: 0.08,         // $0.08/GB
		FinalTierPrice: 0.08,
		InterRegionPrice: 0.01,
		SameRegionPrice: 0.01,
	},
	"github": {
		// GitHub Actions has included data transfer
		// But large artifacts/packages can incur storage costs
		FirstTierGB:    100,          // ~100GB/month included
		FirstTierPrice: 0,            // Free
		SecondTierGB:   100 * 1024,
		SecondTierPrice: 0.09,        // Similar to AWS for overages
		ThirdTierGB:    100 * 1024,
		ThirdTierPrice: 0.09,
		FinalTierPrice: 0.09,
		InterRegionPrice: 0,
		SameRegionPrice: 0,
	},
	"azure": {
		// Azure data transfer (Zone 1 - Americas, Europe)
		FirstTierGB:    100,          // First 100 GB free
		FirstTierPrice: 0,
		SecondTierGB:   10 * 1024,    // 100GB - 10 TB
		SecondTierPrice: 0.087,       // $0.087/GB
		ThirdTierGB:    50 * 1024,    // 10-50 TB
		ThirdTierPrice: 0.083,        // $0.083/GB
		FinalTierPrice: 0.07,         // >50 TB
		InterRegionPrice: 0.02,
		SameRegionPrice: 0.01,
	},
}

// EgressTiers defines tiered pricing structure.
type EgressTiers struct {
	FirstTierGB      float64
	FirstTierPrice   float64
	SecondTierGB     float64
	SecondTierPrice  float64
	ThirdTierGB      float64
	ThirdTierPrice   float64
	FinalTierPrice   float64
	InterRegionPrice float64
	SameRegionPrice  float64
}

// EgressMetrics captures network egress cost estimation.
type EgressMetrics struct {
	// Total bytes transferred out
	TotalEgressGB float64 `json:"total_egress_gb"`
	// Breakdown by destination type
	InternetEgressGB   float64 `json:"internet_egress_gb,omitempty"`
	InterRegionGB      float64 `json:"inter_region_gb,omitempty"`
	SameRegionGB       float64 `json:"same_region_gb,omitempty"`
	// Estimated costs
	EstimatedCostUSD   float64 `json:"estimated_cost_usd"`
	MonthlyProjection  float64 `json:"monthly_projection_usd,omitempty"`
	// Provider used for calculation
	Provider           string  `json:"provider"`
	// Breakdown by traffic type
	DockerPullGB       float64 `json:"docker_pull_gb,omitempty"`
	ArtifactUploadGB   float64 `json:"artifact_upload_gb,omitempty"`
	PackageDownloadGB  float64 `json:"package_download_gb,omitempty"`
	OtherEgressGB      float64 `json:"other_egress_gb,omitempty"`
}

// calculateEgressCost estimates the cost for a given amount of egress.
func calculateEgressCost(egressGB float64, provider string) float64 {
	tiers, ok := EgressPricing[strings.ToLower(provider)]
	if !ok {
		tiers = EgressPricing["aws"] // Default to AWS pricing
	}

	var cost float64
	remaining := egressGB

	// First tier
	if remaining <= 0 {
		return 0
	}
	firstAmount := min(remaining, tiers.FirstTierGB)
	cost += firstAmount * tiers.FirstTierPrice
	remaining -= firstAmount

	// Second tier
	if remaining <= 0 {
		return cost
	}
	secondAmount := min(remaining, tiers.SecondTierGB-tiers.FirstTierGB)
	cost += secondAmount * tiers.SecondTierPrice
	remaining -= secondAmount

	// Third tier
	if remaining <= 0 {
		return cost
	}
	thirdAmount := min(remaining, tiers.ThirdTierGB-tiers.SecondTierGB)
	cost += thirdAmount * tiers.ThirdTierPrice
	remaining -= thirdAmount

	// Final tier
	if remaining > 0 {
		cost += remaining * tiers.FinalTierPrice
	}

	return cost
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// estimateEgressMetrics analyzes network usage and estimates costs.
func estimateEgressMetrics(netTxMBTotal float64, ciPlatform string, durationSeconds float64) *EgressMetrics {
	// Convert MB to GB
	totalEgressGB := netTxMBTotal / 1024.0

	if totalEgressGB < 0.001 { // Less than 1 MB
		return nil
	}

	provider := mapCIPlatformToProvider(ciPlatform)

	metrics := &EgressMetrics{
		TotalEgressGB: totalEgressGB,
		Provider:      provider,
	}

	// Estimate traffic breakdown based on common CI patterns
	// These are heuristics; actual breakdown would require packet inspection
	metrics.DockerPullGB = estimateDockerPullTraffic()
	metrics.ArtifactUploadGB = estimateArtifactTraffic()
	metrics.PackageDownloadGB = totalEgressGB * 0.3 // Rough estimate: 30% is package downloads

	// Remaining is "other"
	accounted := metrics.DockerPullGB + metrics.ArtifactUploadGB + metrics.PackageDownloadGB
	if accounted < totalEgressGB {
		metrics.OtherEgressGB = totalEgressGB - accounted
	}

	// Most CI egress goes to internet (artifact uploads, registry pushes)
	metrics.InternetEgressGB = totalEgressGB * 0.8
	metrics.InterRegionGB = totalEgressGB * 0.1
	metrics.SameRegionGB = totalEgressGB * 0.1

	// Calculate cost
	metrics.EstimatedCostUSD = calculateEgressCost(metrics.InternetEgressGB, provider)
	metrics.EstimatedCostUSD += calculateInterRegionCost(metrics.InterRegionGB, provider)
	metrics.EstimatedCostUSD += calculateSameRegionCost(metrics.SameRegionGB, provider)

	// Project monthly cost based on this run
	if durationSeconds > 0 {
		runsPerMonth := (30 * 24 * 3600) / durationSeconds // Estimate runs per month
		metrics.MonthlyProjection = metrics.EstimatedCostUSD * runsPerMonth
	}

	return metrics
}

func calculateInterRegionCost(egressGB float64, provider string) float64 {
	tiers, ok := EgressPricing[strings.ToLower(provider)]
	if !ok {
		tiers = EgressPricing["aws"]
	}
	return egressGB * tiers.InterRegionPrice
}

func calculateSameRegionCost(egressGB float64, provider string) float64 {
	tiers, ok := EgressPricing[strings.ToLower(provider)]
	if !ok {
		tiers = EgressPricing["aws"]
	}
	return egressGB * tiers.SameRegionPrice
}

func mapCIPlatformToProvider(ciPlatform string) string {
	switch strings.ToLower(ciPlatform) {
	case "github":
		return "github"
	case "gitlab":
		// GitLab.com runs on GCP
		return "gcp"
	case "azure":
		return "azure"
	case "circleci", "bitbucket":
		// These often run on AWS
		return "aws"
	default:
		// Check environment for hints
		if os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_DEFAULT_REGION") != "" {
			return "aws"
		}
		if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" || os.Getenv("GCLOUD_PROJECT") != "" {
			return "gcp"
		}
		if os.Getenv("AZURE_SUBSCRIPTION_ID") != "" {
			return "azure"
		}
		return "aws" // Default to AWS pricing
	}
}

// estimateDockerPullTraffic estimates Docker image pull traffic based on env vars.
func estimateDockerPullTraffic() float64 {
	// Check for Docker-related environment variables
	// This is a rough heuristic; actual tracking requires Docker daemon events
	
	// Check if there are signs of Docker usage
	if _, err := os.Stat("/.dockerenv"); err == nil {
		// Running inside a container - likely pulled an image
		return 0.5 // Estimate 500MB average image size
	}

	// Check for GitHub Container Registry usage
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		if os.Getenv("DOCKER_BUILDKIT") != "" || os.Getenv("DOCKER_CONFIG") != "" {
			return 0.8 // Estimate 800MB for typical CI images
		}
	}

	return 0
}

// estimateArtifactTraffic estimates artifact upload traffic.
func estimateArtifactTraffic() float64 {
	// Check for artifact-related environment variables
	artifactPath := os.Getenv("GITHUB_WORKSPACE")
	if artifactPath == "" {
		artifactPath = os.Getenv("CI_PROJECT_DIR")
	}
	if artifactPath == "" {
		return 0.1 // Default small estimate
	}

	// Could walk artifact directories here, but that's intrusive
	// Use a conservative estimate
	return 0.2 // 200MB average artifact uploads
}

// EgressSummary provides run-level egress analysis.
type EgressSummary struct {
	TotalEgressGB        float64 `json:"total_egress_gb"`
	EstimatedCostUSD     float64 `json:"estimated_cost_usd"`
	CostPerRunUSD        float64 `json:"cost_per_run_usd"`
	MonthlyProjectionUSD float64 `json:"monthly_projection_usd,omitempty"`
	Provider             string  `json:"provider"`
	// Recommendations
	RecommendCaching     bool    `json:"recommend_caching,omitempty"`
	RecommendCompression bool    `json:"recommend_compression,omitempty"`
	PotentialSavingsUSD  float64 `json:"potential_savings_usd,omitempty"`
}

// buildEgressSummary creates an egress cost summary for a run.
func buildEgressSummary(netTxMBPeak float64, ciPlatform string, durationSeconds float64, sampleCount int) *EgressSummary {
	// Estimate total transfer based on peak rate and duration
	// This is an approximation; actual would require cumulative tracking
	avgRate := netTxMBPeak * 0.3 // Assume average is ~30% of peak
	totalMB := avgRate * (durationSeconds / 60) // MB per minute * minutes
	totalGB := totalMB / 1024.0

	if totalGB < 0.01 { // Less than 10 MB
		return nil
	}

	provider := mapCIPlatformToProvider(ciPlatform)
	cost := calculateEgressCost(totalGB, provider)

	summary := &EgressSummary{
		TotalEgressGB:    totalGB,
		EstimatedCostUSD: cost,
		CostPerRunUSD:    cost,
		Provider:         provider,
	}

	// Calculate monthly projection
	if durationSeconds > 0 {
		// Assume 20 builds per day
		buildsPerDay := 20.0
		summary.MonthlyProjectionUSD = cost * buildsPerDay * 30
	}

	// Recommendations
	if totalGB > 1.0 {
		summary.RecommendCaching = true
		summary.PotentialSavingsUSD = cost * 0.5 // Caching could save ~50%
	}
	if totalGB > 5.0 {
		summary.RecommendCompression = true
		summary.PotentialSavingsUSD = cost * 0.7 // Compression + caching could save ~70%
	}

	return summary
}
