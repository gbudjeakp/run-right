// cmd/seed/main.go — posts realistic demo jobs to the local backend.
//
// Usage:
//
//	go run ./cmd/seed [--url http://localhost:8080] [--api-key YOUR_KEY]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/sgbudje/runright/internal/types"
)

func main() {
	url := flag.String("url", "http://localhost:8080", "Backend base URL")
	key := flag.String("api-key", "", "RUNRIGHT_API_KEY (leave blank if auth is disabled)")
	flag.Parse()

	jobs := buildJobs()
	ok, fail := 0, 0
	for _, j := range jobs {
		if err := post(*url, *key, j); err != nil {
			log.Printf("FAIL %s: %v", j.Summary.JobID, err)
			fail++
		} else {
			fmt.Printf("  ✓  %s  (%.0fs, cpu p95 %.1f%%, mem p95 %.2f GiB)\n",
				j.Summary.JobID,
				j.Summary.DurationSeconds,
				j.Summary.CPUPercentP95,
				j.Summary.MemUsedGiBP95,
			)
			ok++
		}
	}
	fmt.Printf("\nSeeded %d jobs (%d failed)\n", ok, fail)
}

// --------------------------------------------------------------------------
// HTTP helpers
// --------------------------------------------------------------------------

type payload struct {
	Summary         types.MetricsSummary   `json:"summary"`
	Recommendations []types.Recommendation `json:"recommendations"`
}

func post(base, key string, p payload) error {
	b, _ := json.Marshal(p)
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/jobs", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// --------------------------------------------------------------------------
// Seed data
// --------------------------------------------------------------------------

// awsT3Medium is the "detected" machine for most runs — a common GitHub-hosted runner size.
var awsT3Medium = types.MachineType{
	ID: "t3.medium", Provider: types.ProviderAWS,
	Family: "general-purpose", Series: "t3",
	VCPUs: 2, MemoryGiB: 4, NetworkGbps: 5, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0416,
	Tags: []string{"burstable"},
}

var awsT3Large = types.MachineType{
	ID: "t3.large", Provider: types.ProviderAWS,
	Family: "general-purpose", Series: "t3",
	VCPUs: 2, MemoryGiB: 8, NetworkGbps: 5, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0832,
	Tags: []string{"burstable"},
}

var awsC7gLarge = types.MachineType{
	ID: "c7g.large", Provider: types.ProviderAWS,
	Family: "compute-optimized", Series: "c7g",
	VCPUs: 2, MemoryGiB: 4, NetworkGbps: 12.5, StorageType: "ebs",
	Architecture: "arm64", OnDemandPricePerHour: 0.0725,
	Tags: []string{"graviton", "arm64"},
}

var awsM6iLarge = types.MachineType{
	ID: "m6i.large", Provider: types.ProviderAWS,
	Family: "general-purpose", Series: "m6i",
	VCPUs: 2, MemoryGiB: 8, NetworkGbps: 12.5, StorageType: "ebs",
	Architecture: "x86_64", OnDemandPricePerHour: 0.096,
	Tags: []string{"latest-gen"},
}

var gcpE2Medium = types.MachineType{
	ID: "e2-medium", Provider: types.ProviderGCP,
	Family: "general-purpose", Series: "e2",
	VCPUs: 2, MemoryGiB: 4, NetworkGbps: 4, StorageType: "pd-balanced",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0335,
	Tags: []string{"shared-core"},
}

var gcpN2Standard2 = types.MachineType{
	ID: "n2-standard-2", Provider: types.ProviderGCP,
	Family: "general-purpose", Series: "n2",
	VCPUs: 2, MemoryGiB: 8, NetworkGbps: 10, StorageType: "pd-balanced",
	Architecture: "x86_64", OnDemandPricePerHour: 0.0971,
	Tags: []string{"latest-gen"},
}

// rng is a seeded random source for reproducible jitter.
var rng = rand.New(rand.NewSource(42))

func jitter(base, spread float64) float64 {
	return base + (rng.Float64()*2-1)*spread
}

// buildJobs returns a realistic set of job runs spanning the past 30 days.
// Jobs are ordered oldest-first so the dashboard history looks natural.
func buildJobs() []payload {
	now := time.Now()
	var jobs []payload

	// ── "build" job: runs daily, gradually gets heavier over the month ──────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		growth := float64(30-i) / 30.0 // 0 → 1 over the month

		cpu := jitter(28+growth*22, 4)     // 28% → 50% over the month
		mem := jitter(1.1+growth*0.9, 0.1) // 1.1 → 2.0 GiB

		jobs = append(jobs, makeJob("build", "github", t, awsT3Medium, cpu, mem, 185+growth*75))
	}

	// ── "unit-tests" job: fast, low-resource, stable ────────────────────────
	for i := 29; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(5 * time.Minute)
		jobs = append(jobs, makeJob("unit-tests", "github", t, awsT3Medium,
			jitter(12, 3), jitter(0.6, 0.08), jitter(42, 8)))
	}

	// ── "integration-tests" job: medium load, runs every other day ──────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(12 * time.Minute)
		jobs = append(jobs, makeJob("integration-tests", "jenkins", t, awsT3Medium,
			jitter(55, 8), jitter(2.1, 0.2), jitter(310, 40)))
	}

	// ── "e2e-tests" job: CPU-hungry, runs twice a week ──────────────────────
	for i := 28; i >= 0; i -= 4 {
		t := now.AddDate(0, 0, -i).Add(25 * time.Minute)
		jobs = append(jobs, makeJob("e2e-tests", "jenkins", t, awsT3Medium,
			jitter(78, 6), jitter(2.8, 0.25), jitter(620, 60)))
	}

	// ── "docker-build" job: I/O heavy, runs weekly ──────────────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(2 * time.Minute)
		jobs = append(jobs, makeJob("docker-build", "github", t, awsT3Medium,
			jitter(35, 5), jitter(1.4, 0.15), jitter(240, 30)))
	}

	// ── "lint" job: trivially light, every day ──────────────────────────────
	for i := 14; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(1 * time.Minute)
		jobs = append(jobs, makeJob("lint", "github", t, awsT3Medium,
			jitter(8, 2), jitter(0.35, 0.04), jitter(22, 4)))
	}

	// ── "security-scan" job: light load, runs twice a week ──────────────────
	for i := 28; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(8 * time.Minute)
		jobs = append(jobs, makeJob("security-scan", "jenkins", t, awsT3Medium,
			jitter(14, 3), jitter(0.5, 0.06), jitter(55, 10)))
	}

	// ── "deploy-staging" job: moderate, once a day ──────────────────────────
	for i := 14; i >= 0; i-- {
		t := now.AddDate(0, 0, -i).Add(18 * time.Minute)
		jobs = append(jobs, makeJob("deploy-staging", "jenkins", t, awsT3Medium,
			jitter(30, 5), jitter(0.9, 0.1), jitter(95, 15)))
	}

	// ── "benchmark" job: CPU-heavy, weekly on GCP ───────────────────────────
	for i := 28; i >= 0; i -= 7 {
		t := now.AddDate(0, 0, -i).Add(35 * time.Minute)
		jobs = append(jobs, makeJob("benchmark", "jenkins", t, gcpN2Standard2,
			jitter(88, 5), jitter(3.2, 0.2), jitter(750, 60)))
	}

	// ── "gcp-build" job: build running on GCP, every 3 days ─────────────────
	for i := 27; i >= 0; i -= 3 {
		t := now.AddDate(0, 0, -i).Add(3 * time.Minute)
		growth := float64(27-i) / 27.0
		jobs = append(jobs, makeJob("gcp-build", "github", t, gcpN2Standard2,
			jitter(32+growth*20, 4), jitter(1.2+growth*0.7, 0.1), jitter(200+growth*80, 30)))
	}

	// ── "python-tests" job: GCP, every other day ────────────────────────────
	for i := 28; i >= 0; i -= 2 {
		t := now.AddDate(0, 0, -i).Add(7 * time.Minute)
		jobs = append(jobs, makeJob("python-tests", "jenkins", t, gcpE2Medium,
			jitter(22, 4), jitter(0.8, 0.08), jitter(68, 12)))
	}

	// ── Interrupted runs ─────────────────────────────────────────────────────
	// Status="heartbeat" simulates jobs where the agent never sent a final
	// "completed" flush — either OOM-killed (SIGKILL) or runner disconnected.

	// OOM kill: build job 3 days ago — mem was near the 4 GiB ceiling
	jobs = append(jobs, makeInterruptedJob("build", "github",
		now.AddDate(0, 0, -3).Add(10*time.Second),
		awsT3Medium, jitter(44, 3), jitter(3.88, 0.05), 47))

	// Runner disconnect: e2e-tests 9 days ago — cut off 2 min into a 10+ min run
	jobs = append(jobs, makeInterruptedJob("e2e-tests", "jenkins",
		now.AddDate(0, 0, -9).Add(25*time.Minute),
		awsT3Medium, jitter(76, 4), jitter(2.6, 0.15), 118))

	// OOM kill: benchmark (GCP) 5 days ago — memory blew past instance ceiling
	jobs = append(jobs, makeInterruptedJob("benchmark", "jenkins",
		now.AddDate(0, 0, -5).Add(35*time.Minute),
		gcpN2Standard2, jitter(93, 2), jitter(6.9, 0.2), 203))

	// Runner disconnect: python-tests 2 days ago — runner went offline mid-suite
	jobs = append(jobs, makeInterruptedJob("python-tests", "jenkins",
		now.AddDate(0, 0, -2).Add(7*time.Minute),
		gcpE2Medium, jitter(24, 3), jitter(0.78, 0.06), 31))

	return jobs
}

// makeInterruptedJob creates a job payload with Status="heartbeat", simulating
// a run where the agent was killed (OOM, SIGKILL) or the runner disconnected
// before it could send the final "completed" flush.
func makeInterruptedJob(
	jobID, ciPlatform string, at time.Time,
	detected types.MachineType,
	cpuP95, memP95GiB, durationSec float64,
) payload {
	p := makeJob(jobID, ciPlatform, at, detected, cpuP95, memP95GiB, durationSec)
	p.Summary.RunID = fmt.Sprintf("seed-%s-%x", jobID, rng.Int63())
	p.Summary.Status = "heartbeat"
	return p
}

func makeJob(
	jobID, ciPlatform string, at time.Time,
	detected types.MachineType,
	cpuP95, memP95GiB, durationSec float64,
) payload {
	start := at
	end := at.Add(time.Duration(durationSec) * time.Second)
	currentMonthly := detected.OnDemandPricePerHour * 720

	summary := types.MetricsSummary{
		JobID:           jobID,
		CIPlatform:      ciPlatform,
		StartTime:       start,
		EndTime:         end,
		DurationSeconds: durationSec,
		DetectedMachine: &detected,

		CPUPercentP95:  cpuP95,
		CPUPercentPeak: jitter(cpuP95*1.12, 3),
		CPUPercentAvg:  cpuP95 * 0.72,

		MemUsedGiBP95:  memP95GiB,
		MemUsedGiBPeak: jitter(memP95GiB*1.1, 0.05),
		MemUsedGiBAvg:  memP95GiB * 0.8,
		MemTotalGiB:    detected.MemoryGiB,

		ProcessCountPeak: int(jitter(24, 4)),
		ThreadCountPeak:  int(jitter(80, 15)),

		DiskReadMBsPeak:  jitter(12, 4),
		DiskWriteMBsPeak: jitter(8, 3),
		NetRxMBsPeak:     jitter(5, 2),
		NetTxMBsPeak:     jitter(3, 1),

		SampleCount: int(durationSec / 5),
	}

	recs := recommend(summary, detected, currentMonthly)
	return payload{Summary: summary, Recommendations: recs}
}

// recommend generates plausible recommendations based on the summary.
func recommend(s types.MetricsSummary, detected types.MachineType, currentMonthly float64) []types.Recommendation {
	cpuPct := s.CPUPercentP95
	memGiB := s.MemUsedGiBP95

	var recs []types.Recommendation

	// Right-sized: best fit for the workload
	switch {
	case cpuPct < 20 && memGiB < 0.8:
		// Very light — suggest t3.nano → not in our set, use t3.small equivalent
		m := types.MachineType{
			ID: "t3.small", Provider: types.ProviderAWS,
			Family: "general-purpose", Series: "t3",
			VCPUs: 2, MemoryGiB: 2, NetworkGbps: 5, StorageType: "ebs",
			Architecture: "x86_64", OnDemandPricePerHour: 0.0208, Tags: []string{"burstable"},
		}
		recs = append(recs, rec(m, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(gcpE2Medium, types.TierCheaper, currentMonthly))

	case cpuPct >= 60 || memGiB > detected.MemoryGiB*0.7:
		// Heavy — needs more headroom
		recs = append(recs, rec(awsM6iLarge, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(awsC7gLarge, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(awsT3Large, types.TierMoreHeadroom, currentMonthly))

	default:
		// Moderate — current is okay but cheaper options exist
		recs = append(recs, rec(awsT3Medium, types.TierRightSized, currentMonthly))
		recs = append(recs, rec(gcpE2Medium, types.TierCheaper, currentMonthly))
		recs = append(recs, rec(gcpN2Standard2, types.TierMoreHeadroom, currentMonthly))
	}

	return recs
}

func rec(m types.MachineType, tier types.RecommendationTier, currentMonthly float64) types.Recommendation {
	estimated := m.OnDemandPricePerHour * 720
	delta := (estimated - currentMonthly) / currentMonthly * 100
	spotFactor := 0.30
	if m.Provider == types.ProviderGCP {
		spotFactor = 0.20
	}
	spot := estimated * spotFactor
	spotDelta := (spot - currentMonthly) / currentMonthly * 100
	return types.Recommendation{
		Machine:           m,
		Tier:              tier,
		EstimatedMonthly:  estimated,
		SpotMonthly:       spot,
		CurrentMonthly:    currentMonthly,
		CostDeltaPercent:  delta,
		SpotDeltaPercent:  spotDelta,
		RequiredVCPUs:     m.VCPUs,
		RequiredMemoryGiB: m.MemoryGiB,
		Reasoning:         "Based on p95 CPU and memory usage patterns over recent runs.",
	}
}
