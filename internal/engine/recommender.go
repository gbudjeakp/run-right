package engine

import (
	"fmt"
	"math"
	"sort"

	"github.com/sgbudje/runright/internal/types"
)

const (
	cpuHeadroomFactor = 1.20 // 20% headroom above p95 CPU — tolerates brief spikes without thrashing
	memHeadroomFactor = 1.30 // 30% headroom above p95 memory — OOM kills are more disruptive than slowdown
	hoursPerMonth     = 720.0
)

// Recommend returns a ranked list of machine recommendations based on observed
// job metrics and the provided machine catalog.
func Recommend(summary types.MetricsSummary, catalog []types.MachineType) []types.Recommendation {
	detected := summary.DetectedMachine

	// Determine the vCPU and memory baselines.
	var detectedVCPUs int
	var currentPricePerHour float64

	if detected != nil {
		detectedVCPUs = detected.VCPUs
		currentPricePerHour = detected.OnDemandPricePerHour
	} else {
		// Fallback: use a sensible default if machine couldn't be detected.
		detectedVCPUs = 2
	}

	// Required resources with headroom.
	// CPU uses p95 usage as a fraction of the detected machine's vCPUs.
	// Memory gets a larger headroom factor because OOM kills are more disruptive than CPU throttling.
	requiredVCPUs := int(math.Ceil(float64(detectedVCPUs) * (summary.CPUPercentP95 / 100.0) * cpuHeadroomFactor))
	if requiredVCPUs < 1 {
		requiredVCPUs = 1
	}
	requiredMemGiB := summary.MemUsedGiBP95 * memHeadroomFactor

	// Ensure minimums make sense.
	if requiredMemGiB < 0.5 {
		requiredMemGiB = 0.5
	}

	currentMonthly := currentPricePerHour * hoursPerMonth

	var results []types.Recommendation

	for _, m := range catalog {
		if m.VCPUs < requiredVCPUs || m.MemoryGiB < requiredMemGiB {
			continue
		}
		tier := classifyTier(m, detected, requiredVCPUs, requiredMemGiB)
		estimatedMonthly := m.OnDemandPricePerHour * hoursPerMonth
		deltaPercent := 0.0
		if currentMonthly > 0 {
			deltaPercent = ((estimatedMonthly - currentMonthly) / currentMonthly) * 100
		}
		results = append(results, types.Recommendation{
			Machine:           m,
			Tier:              tier,
			EstimatedMonthly:  estimatedMonthly,
			CurrentMonthly:    currentMonthly,
			CostDeltaPercent:  deltaPercent,
			RequiredVCPUs:     requiredVCPUs,
			RequiredMemoryGiB: requiredMemGiB,
			Reasoning:         buildReasoning(m, summary, requiredVCPUs, requiredMemGiB),
		})
	}

	// Sort: right-sized first, then by price ascending.
	sort.Slice(results, func(i, j int) bool {
		if tierOrder(results[i].Tier) != tierOrder(results[j].Tier) {
			return tierOrder(results[i].Tier) < tierOrder(results[j].Tier)
		}
		return results[i].EstimatedMonthly < results[j].EstimatedMonthly
	})

	// Return at most top 10 results per tier to keep output readable.
	return cap(results)
}

func classifyTier(m types.MachineType, detected *types.MachineType, reqVCPUs int, reqMemGiB float64) types.RecommendationTier {
	vcpuRatio := float64(m.VCPUs) / float64(reqVCPUs)
	memRatio := m.MemoryGiB / reqMemGiB

	// Significantly over-provisioned on either dimension — useful if you need burst room.
	// Threshold of 4x CPU or 3x memory to avoid mislabeling the smallest viable machine.
	if vcpuRatio >= 4.0 || memRatio >= 3.0 {
		return types.TierMoreHeadroom
	}
	// Cheaper than the machine currently in use.
	if detected != nil && m.OnDemandPricePerHour < detected.OnDemandPricePerHour {
		return types.TierCheaper
	}
	return types.TierRightSized
}

func buildReasoning(m types.MachineType, s types.MetricsSummary, reqVCPUs int, reqMemGiB float64) string {
	cpuSlack := float64(m.VCPUs-reqVCPUs) / float64(reqVCPUs) * 100
	memSlack := (m.MemoryGiB - reqMemGiB) / reqMemGiB * 100
	return fmt.Sprintf(
		"Job used %.1f%% CPU (p95; avg %.1f%%, peak %.1f%%) and %.2f GiB memory (p95; avg %.2f GiB) across %d samples. "+
			"With %d%% CPU headroom and %d%% memory headroom, requires %d vCPUs and %.1f GiB. "+
			"%s (%d vCPUs, %.1f GiB) gives %.0f%% CPU slack and %.0f%% memory slack.",
		s.CPUPercentP95, s.CPUPercentAvg, s.CPUPercentPeak, s.MemUsedGiBP95, s.MemUsedGiBAvg, s.SampleCount,
		int((cpuHeadroomFactor-1)*100), int((memHeadroomFactor-1)*100),
		reqVCPUs, reqMemGiB,
		m.ID, m.VCPUs, m.MemoryGiB, cpuSlack, memSlack,
	)
}

func tierOrder(t types.RecommendationTier) int {
	switch t {
	case types.TierRightSized:
		return 0
	case types.TierCheaper:
		return 1
	case types.TierMoreHeadroom:
		return 2
	}
	return 99
}

func cap(results []types.Recommendation) []types.Recommendation {
	const maxPerTier = 5
	counts := make(map[types.RecommendationTier]int)
	var out []types.Recommendation
	for _, r := range results {
		if counts[r.Tier] < maxPerTier {
			out = append(out, r)
			counts[r.Tier]++
		}
	}
	return out
}
