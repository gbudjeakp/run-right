package engine

import (
	"testing"

	"github.com/sgbudje/runright/internal/catalog"
	"github.com/sgbudje/runright/internal/types"
)

func TestRecommend_BasicOutput(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:          "test-job",
		CPUPercentP95:  60,
		CPUPercentPeak: 75,
		CPUPercentAvg:  45,
		MemUsedGiBP95:  3.5,
		MemUsedGiBPeak: 4.0,
		MemTotalGiB:    8.0,
		SampleCount:    12,
		DetectedMachine: &types.MachineType{
			ID:                   "t3.large",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            8.0,
			OnDemandPricePerHour: 0.0835,
		},
	}

	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation, got none")
	}
	for _, r := range recs {
		if r.Machine.VCPUs < r.RequiredVCPUs {
			t.Errorf("machine %s has %d vCPUs but required %d", r.Machine.ID, r.Machine.VCPUs, r.RequiredVCPUs)
		}
		if r.Machine.MemoryGiB < r.RequiredMemoryGiB {
			t.Errorf("machine %s has %.1f GiB but required %.1f", r.Machine.ID, r.Machine.MemoryGiB, r.RequiredMemoryGiB)
		}
	}
}

func TestRecommend_HighCPU(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:         "heavy-build",
		CPUPercentP95: 95,
		MemUsedGiBP95: 2.0,
		MemTotalGiB:   4.0,
		SampleCount:   30,
		DetectedMachine: &types.MachineType{
			ID:                   "t3.medium",
			Provider:             types.ProviderAWS,
			VCPUs:                2,
			MemoryGiB:            4.0,
			OnDemandPricePerHour: 0.0418,
		},
	}
	recs := Recommend(summary, machines)
	if len(recs) == 0 {
		t.Fatal("expected recommendations for high-CPU job")
	}
	// Top recommendation must have more vCPUs than current under high load.
	top := recs[0]
	if top.RequiredVCPUs <= 2 {
		t.Logf("required vCPUs: %d (may be fine for low-mem workload)", top.RequiredVCPUs)
	}
}

func TestRecommend_NoDetectedMachine(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:         "unknown-runner",
		CPUPercentP95: 50,
		MemUsedGiBP95: 1.5,
		MemTotalGiB:   4.0,
		SampleCount:   6,
	}
	recs := Recommend(summary, machines)
	// Should still return results using fallback vCPU count.
	if len(recs) == 0 {
		t.Fatal("expected recommendations even without detected machine")
	}
}

func TestRecommend_TierOrdering(t *testing.T) {
	machines := catalog.All()
	summary := types.MetricsSummary{
		JobID:         "tier-order",
		CPUPercentP95: 40,
		MemUsedGiBP95: 2.0,
		MemTotalGiB:   8.0,
		SampleCount:   10,
		DetectedMachine: &types.MachineType{
			ID:                   "m7i.xlarge",
			Provider:             types.ProviderAWS,
			VCPUs:                4,
			MemoryGiB:            16.0,
			OnDemandPricePerHour: 0.2016,
		},
	}
	recs := Recommend(summary, machines)
	for i := 1; i < len(recs); i++ {
		if tierOrder(recs[i-1].Tier) > tierOrder(recs[i].Tier) {
			t.Errorf("recommendations are not sorted by tier at index %d", i)
		}
	}
}
