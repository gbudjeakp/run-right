package engine

import (
	"fmt"
	"strings"

	"github.com/sgbudje/runright/internal/types"
)

// Explanation provides a human-readable breakdown of a recommendation.
type Explanation struct {
	Summary          string   `json:"summary"`
	CPUAnalysis      string   `json:"cpu_analysis"`
	MemoryAnalysis   string   `json:"memory_analysis"`
	CostAnalysis     string   `json:"cost_analysis"`
	Warnings         []string `json:"warnings,omitempty"`
	Suggestions      []string `json:"suggestions,omitempty"`
	DetailedBreakdown string  `json:"detailed_breakdown"`
}

// Explain generates a human-readable explanation of why a recommendation was made.
func Explain(summary types.MetricsSummary, rec types.Recommendation) Explanation {
	exp := Explanation{}

	// Summary
	if rec.Tier == "right-sized" {
		exp.Summary = fmt.Sprintf("Your current machine is right-sized. "+
			"The %s (%d vCPUs, %.1f GiB) matches your workload requirements.",
			rec.Machine.ID, rec.Machine.VCPUs, rec.Machine.MemoryGiB)
	} else if rec.CostDeltaPercent < 0 {
		exp.Summary = fmt.Sprintf("Switching to %s could save %.0f%% ($%.2f/month). "+
			"Your workload only uses %.1f%% CPU and %.2f GiB memory at p95.",
			rec.Machine.ID, -rec.CostDeltaPercent,
			rec.CurrentMonthly-rec.EstimatedMonthly,
			summary.CPUPercentP95, summary.MemUsedGiBP95)
	} else {
		exp.Summary = fmt.Sprintf("Consider %s for %.0f%% more cost but better headroom.",
			rec.Machine.ID, rec.CostDeltaPercent)
	}

	// CPU Analysis
	if summary.DetectedMachine != nil {
		detectedVCPUs := summary.DetectedMachine.VCPUs
		usedVCPUs := float64(detectedVCPUs) * (summary.CPUPercentP95 / 100.0)
		headroomVCPUs := usedVCPUs * cpuHeadroomFactor

		exp.CPUAnalysis = fmt.Sprintf(
			"CPU: You used %.1f%% of %d vCPUs at p95 (effectively %.1f vCPUs). "+
				"With 20%% headroom, you need %.1f vCPUs. "+
				"The recommended %s has %d vCPUs.",
			summary.CPUPercentP95, detectedVCPUs, usedVCPUs,
			headroomVCPUs, rec.Machine.ID, rec.Machine.VCPUs)
	} else {
		exp.CPUAnalysis = fmt.Sprintf(
			"CPU: p95 usage was %.1f%%. Machine couldn't be detected, "+
				"so recommendations are based on memory and assumed 2 vCPUs baseline.",
			summary.CPUPercentP95)
	}

	// Memory Analysis
	headroomMem := summary.MemUsedGiBP95 * memHeadroomFactor
	exp.MemoryAnalysis = fmt.Sprintf(
		"Memory: You used %.2f GiB at p95 out of %.1f GiB available. "+
			"With 30%% headroom, you need %.2f GiB. "+
			"The recommended %s has %.1f GiB.",
		summary.MemUsedGiBP95, summary.MemTotalGiB,
		headroomMem, rec.Machine.ID, rec.Machine.MemoryGiB)

	// Cost Analysis
	if rec.CurrentMonthly > 0 {
		savings := rec.CurrentMonthly - rec.EstimatedMonthly
		annualSavings := savings * 12
		if savings > 0 {
			exp.CostAnalysis = fmt.Sprintf(
				"Cost: Current machine costs $%.2f/month ($%.2f/year). "+
					"The recommended %s costs $%.2f/month, saving $%.2f/month ($%.0f/year).",
				rec.CurrentMonthly, rec.CurrentMonthly*12,
				rec.Machine.ID, rec.EstimatedMonthly,
				savings, annualSavings)
		} else {
			exp.CostAnalysis = fmt.Sprintf(
				"Cost: Current machine costs $%.2f/month. "+
					"The recommended %s costs $%.2f/month (%.0f%% more).",
				rec.CurrentMonthly, rec.Machine.ID,
				rec.EstimatedMonthly, rec.CostDeltaPercent)
		}
	}

	// Warnings
	if summary.CPUPercentP95 > 80 {
		exp.Warnings = append(exp.Warnings,
			fmt.Sprintf("High CPU utilization (%.1f%% p95) detected. Consider a larger instance to avoid throttling.",
				summary.CPUPercentP95))
	}
	if summary.MemTotalGiB > 0 && summary.MemUsedGiBP95/summary.MemTotalGiB > 0.85 {
		exp.Warnings = append(exp.Warnings,
			fmt.Sprintf("High memory pressure (%.0f%% used). Risk of OOM kills on memory spikes.",
				(summary.MemUsedGiBP95/summary.MemTotalGiB)*100))
	}
	if rec.DurationRiskNote != "" {
		exp.Warnings = append(exp.Warnings, rec.DurationRiskNote)
	}

	// Suggestions
	if rec.Machine.Architecture == "arm64" && summary.DetectedMachine != nil &&
		summary.DetectedMachine.Architecture == "x86_64" {
		exp.Suggestions = append(exp.Suggestions,
			"This ARM64 instance is cheaper. Verify your workload is ARM-compatible before switching.")
	}
	if rec.SpotMonthly > 0 && rec.SpotMonthly < rec.EstimatedMonthly*0.7 {
		spotSavings := rec.EstimatedMonthly - rec.SpotMonthly
		exp.Suggestions = append(exp.Suggestions,
			fmt.Sprintf("Spot instances available at $%.2f/month (save $%.2f/month). "+
				"Consider using spot for fault-tolerant workloads.", rec.SpotMonthly, spotSavings))
	}
	if summary.Cache != nil && summary.Cache.OverallCacheHitRate < 50 {
		exp.Suggestions = append(exp.Suggestions,
			fmt.Sprintf("Build cache hit rate is only %.0f%%. Improving caching could reduce build time and costs.",
				summary.Cache.OverallCacheHitRate))
	}

	// Detailed breakdown
	var details strings.Builder
	details.WriteString("=== RunRight Analysis ===\n\n")

	details.WriteString("WORKLOAD PROFILE:\n")
	details.WriteString(fmt.Sprintf("  Job ID:        %s\n", summary.JobID))
	details.WriteString(fmt.Sprintf("  Duration:      %.0f seconds\n", summary.DurationSeconds))
	details.WriteString(fmt.Sprintf("  Samples:       %d\n", summary.SampleCount))
	details.WriteString(fmt.Sprintf("  CI Platform:   %s\n", summary.CIPlatform))
	details.WriteString("\n")

	details.WriteString("RESOURCE USAGE (p95):\n")
	details.WriteString(fmt.Sprintf("  CPU:           %.1f%%\n", summary.CPUPercentP95))
	details.WriteString(fmt.Sprintf("  Memory:        %.2f GiB / %.1f GiB (%.0f%%)\n",
		summary.MemUsedGiBP95, summary.MemTotalGiB,
		(summary.MemUsedGiBP95/summary.MemTotalGiB)*100))
	details.WriteString(fmt.Sprintf("  Peak Procs:    %d processes, %d threads\n",
		summary.ProcessCountPeak, summary.ThreadCountPeak))
	details.WriteString("\n")

	if summary.DetectedMachine != nil {
		details.WriteString("CURRENT MACHINE:\n")
		details.WriteString(fmt.Sprintf("  Type:          %s\n", summary.DetectedMachine.ID))
		details.WriteString(fmt.Sprintf("  Provider:      %s\n", summary.DetectedMachine.Provider))
		details.WriteString(fmt.Sprintf("  vCPUs:         %d\n", summary.DetectedMachine.VCPUs))
		details.WriteString(fmt.Sprintf("  Memory:        %.1f GiB\n", summary.DetectedMachine.MemoryGiB))
		details.WriteString(fmt.Sprintf("  Cost:          $%.4f/hr ($%.2f/month)\n",
			summary.DetectedMachine.OnDemandPricePerHour,
			summary.DetectedMachine.OnDemandPricePerHour*hoursPerMonth))
		details.WriteString("\n")
	}

	details.WriteString("RECOMMENDATION:\n")
	details.WriteString(fmt.Sprintf("  Machine:       %s\n", rec.Machine.ID))
	details.WriteString(fmt.Sprintf("  Provider:      %s\n", rec.Machine.Provider))
	details.WriteString(fmt.Sprintf("  vCPUs:         %d\n", rec.Machine.VCPUs))
	details.WriteString(fmt.Sprintf("  Memory:        %.1f GiB\n", rec.Machine.MemoryGiB))
	details.WriteString(fmt.Sprintf("  Architecture:  %s\n", rec.Machine.Architecture))
	details.WriteString(fmt.Sprintf("  Cost:          $%.4f/hr ($%.2f/month)\n",
		rec.Machine.OnDemandPricePerHour, rec.EstimatedMonthly))
	details.WriteString(fmt.Sprintf("  Tier:          %s\n", rec.Tier))
	details.WriteString(fmt.Sprintf("  Cost Delta:    %+.1f%%\n", rec.CostDeltaPercent))

	exp.DetailedBreakdown = details.String()

	return exp
}

// ExplainDiff generates an explanation comparing two runs.
func ExplainDiff(before, after types.MetricsSummary) string {
	var sb strings.Builder

	sb.WriteString("=== RunRight Diff ===\n\n")

	sb.WriteString(fmt.Sprintf("Job: %s → %s\n", before.JobID, after.JobID))
	sb.WriteString(fmt.Sprintf("Duration: %.0fs → %.0fs (%+.0fs)\n",
		before.DurationSeconds, after.DurationSeconds,
		after.DurationSeconds-before.DurationSeconds))
	sb.WriteString("\n")

	sb.WriteString("CPU:\n")
	sb.WriteString(fmt.Sprintf("  p95: %.1f%% → %.1f%% (%+.1f%%)\n",
		before.CPUPercentP95, after.CPUPercentP95,
		after.CPUPercentP95-before.CPUPercentP95))
	sb.WriteString(fmt.Sprintf("  Peak: %.1f%% → %.1f%%\n",
		before.CPUPercentPeak, after.CPUPercentPeak))
	sb.WriteString("\n")

	sb.WriteString("Memory:\n")
	sb.WriteString(fmt.Sprintf("  p95: %.2f GiB → %.2f GiB (%+.2f GiB)\n",
		before.MemUsedGiBP95, after.MemUsedGiBP95,
		after.MemUsedGiBP95-before.MemUsedGiBP95))
	sb.WriteString(fmt.Sprintf("  Peak: %.2f GiB → %.2f GiB\n",
		before.MemUsedGiBPeak, after.MemUsedGiBPeak))
	sb.WriteString("\n")

	sb.WriteString("I/O:\n")
	sb.WriteString(fmt.Sprintf("  Disk Read Peak: %.2f MB/s → %.2f MB/s\n",
		before.DiskReadMBsPeak, after.DiskReadMBsPeak))
	sb.WriteString(fmt.Sprintf("  Disk Write Peak: %.2f MB/s → %.2f MB/s\n",
		before.DiskWriteMBsPeak, after.DiskWriteMBsPeak))
	sb.WriteString(fmt.Sprintf("  Net RX Peak: %.2f MB/s → %.2f MB/s\n",
		before.NetRxMBsPeak, after.NetRxMBsPeak))
	sb.WriteString(fmt.Sprintf("  Net TX Peak: %.2f MB/s → %.2f MB/s\n",
		before.NetTxMBsPeak, after.NetTxMBsPeak))

	// Highlight significant changes
	sb.WriteString("\nSummary:\n")

	cpuDelta := after.CPUPercentP95 - before.CPUPercentP95
	if cpuDelta > 10 {
		sb.WriteString(fmt.Sprintf("  ⚠️  CPU usage increased by %.1f%% - consider larger instance\n", cpuDelta))
	} else if cpuDelta < -10 {
		sb.WriteString(fmt.Sprintf("  ✓ CPU usage decreased by %.1f%% - consider smaller instance\n", -cpuDelta))
	}

	memDelta := after.MemUsedGiBP95 - before.MemUsedGiBP95
	if memDelta > 1 {
		sb.WriteString(fmt.Sprintf("  ⚠️  Memory usage increased by %.2f GiB\n", memDelta))
	} else if memDelta < -1 {
		sb.WriteString(fmt.Sprintf("  ✓ Memory usage decreased by %.2f GiB\n", -memDelta))
	}

	durationDelta := after.DurationSeconds - before.DurationSeconds
	if durationDelta > 60 {
		sb.WriteString(fmt.Sprintf("  ⚠️  Build time increased by %.0f seconds\n", durationDelta))
	} else if durationDelta < -60 {
		sb.WriteString(fmt.Sprintf("  ✓ Build time decreased by %.0f seconds\n", -durationDelta))
	}

	return sb.String()
}
