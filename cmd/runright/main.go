package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sgbudje/runright/internal/agent"
	"github.com/sgbudje/runright/internal/catalog"
	"github.com/sgbudje/runright/internal/engine"
	"github.com/sgbudje/runright/internal/exporter"
	"github.com/sgbudje/runright/internal/server"
	"github.com/sgbudje/runright/internal/types"
)

var rootCmd = &cobra.Command{
	Use:   "runright",
	Short: "Compute sizing tool for CI/CD workloads",
	Long: `runright monitors your CI job's resource usage and recommends
the right AWS or GCP machine type so you stop guessing and start saving.`,
}

func main() {
	rootCmd.AddCommand(monitorCmd, recommendCmd, catalogCmd, serveCmd, verifyCmd, updateCmd)
	cobra.OnInitialize(initConfig)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	viper.SetEnvPrefix("RUNRIGHT")
	viper.AutomaticEnv()
	viper.SetConfigName(".runright")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(".")
	_ = viper.ReadInConfig()
}

// ── monitor ──────────────────────────────────────────────────────────────────

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Collect system metrics during a job",
	Long: `monitor runs a background metrics collector and writes results to
metrics.jsonl and metrics-summary.json when it exits.

Examples:
  runright monitor --duration 60s
  runright monitor --export file,otlp --duration 5m
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 runright monitor --export otlp`,
	RunE: runMonitor,
}

var (
	monitorDuration             time.Duration
	monitorInterval             time.Duration
	monitorExpensiveSampleEvery int
	monitorExport               string
	monitorOutputDir            string
	monitorJobID                string
	monitorPromPort             int
	monitorHTTPURL              string
	monitorSlackURL             string
	monitorTeamsURL             string
	monitorDryRun               bool
	monitorAllowedMachineIDs    string
	monitorAllowedSeries        string
	monitorAllowedFamilies      string
)

func init() {
	monitorCmd.Flags().DurationVar(&monitorDuration, "duration", 0, "Stop after this duration (0 = run until SIGTERM/SIGINT)")
	monitorCmd.Flags().DurationVar(&monitorInterval, "interval", 5*time.Second, "Sampling interval")
	monitorCmd.Flags().IntVar(&monitorExpensiveSampleEvery, "expensive-sample-every", 6, "Run expensive host probes (process/thread + disk/net) every N ticks")
	monitorCmd.Flags().StringVar(&monitorExport, "export", "file", "Comma-separated export backends: file,otlp,prometheus,http,slack,teams")
	monitorCmd.Flags().StringVar(&monitorOutputDir, "output-dir", ".", "Directory for file-based output")
	monitorCmd.Flags().StringVar(&monitorJobID, "job-id", "", "Job identifier (defaults to a timestamp-based ID)")
	monitorCmd.Flags().IntVar(&monitorPromPort, "prometheus-port", 9090, "Port for Prometheus /metrics endpoint")
	monitorCmd.Flags().StringVar(&monitorHTTPURL, "http-url", "", "Base URL of runright backend for http export")
	monitorCmd.Flags().StringVar(&monitorSlackURL, "slack-webhook", "", "Slack incoming webhook URL for slack export (or set RUNRIGHT_SLACK_WEBHOOK)")
	monitorCmd.Flags().StringVar(&monitorTeamsURL, "teams-webhook", "", "Microsoft Teams incoming webhook URL for teams export (or set RUNRIGHT_TEAMS_WEBHOOK)")
	monitorCmd.Flags().BoolVar(&monitorDryRun, "dry-run", false, "Print recommendation and exit non-zero if machine is not right-sized")
	monitorCmd.Flags().StringVar(&monitorAllowedMachineIDs, "allowed-machine-ids", "", "Comma-separated machine allow-list (or RUNRIGHT_ALLOWED_MACHINE_IDS)")
	monitorCmd.Flags().StringVar(&monitorAllowedSeries, "allowed-series", "", "Comma-separated series allow-list, e.g. c7g,m7i,n2,e2 (or RUNRIGHT_ALLOWED_SERIES)")
	monitorCmd.Flags().StringVar(&monitorAllowedFamilies, "allowed-families", "", "Comma-separated family prefixes, e.g. c,m,r,n,e (or RUNRIGHT_ALLOWED_FAMILIES)")
}

func runMonitor(_ *cobra.Command, _ []string) error {
	backends := exporter.ParseBackends(monitorExport)

	// Allow slack/teams webhook via env var as fallback.
	if monitorSlackURL == "" {
		monitorSlackURL = os.Getenv("RUNRIGHT_SLACK_WEBHOOK")
	}
	if monitorTeamsURL == "" {
		monitorTeamsURL = os.Getenv("RUNRIGHT_TEAMS_WEBHOOK")
	}
	if monitorAllowedMachineIDs == "" {
		monitorAllowedMachineIDs = os.Getenv("RUNRIGHT_ALLOWED_MACHINE_IDS")
	}
	if monitorAllowedSeries == "" {
		monitorAllowedSeries = os.Getenv("RUNRIGHT_ALLOWED_SERIES")
	}
	if monitorAllowedFamilies == "" {
		monitorAllowedFamilies = os.Getenv("RUNRIGHT_ALLOWED_FAMILIES")
	}
	allowedMachineIDs := parseCSV(monitorAllowedMachineIDs)
	allowedSeries := parseCSV(monitorAllowedSeries)
	allowedFamilies := parseCSV(monitorAllowedFamilies)

	ctx, cancel := context.WithCancel(context.Background())
	if monitorDuration > 0 {
		ctx, cancel = context.WithTimeout(ctx, monitorDuration)
	}
	defer cancel()

	// Handle OS signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	mgr, err := exporter.New(ctx, backends, monitorPromPort, monitorHTTPURL, monitorSlackURL, monitorTeamsURL)
	if err != nil {
		return fmt.Errorf("exporter: %w", err)
	}
	defer mgr.Shutdown(context.Background())

	col := agent.NewCollector(agent.Config{
		Interval:             monitorInterval,
		ExpensiveSampleEvery: monitorExpensiveSampleEvery,
		OutputDir:            monitorOutputDir,
		JobID:                monitorJobID,
		HeartbeatFilePath:    filepath.Join(monitorOutputDir, "metrics-heartbeat.json"),
		FlushFn: func(summary types.MetricsSummary) error {
			summary.AllowedMachineIDs = allowedMachineIDs
			summary.AllowedSeries = allowedSeries
			summary.AllowedFamilies = allowedFamilies
			// Compute recommendations and post to the HTTP backend (if configured).
			// This runs on every heartbeat AND on the final completed flush, so
			// the backend always has an up-to-date record for this run.
			machines := catalog.Query(catalog.QueryOptions{})
			recs := engine.Recommend(summary, machines)
			if monitorDryRun && summary.Status == "completed" {
				return runDryRun(summary, recs)
			}
			return mgr.PublishSummary(context.Background(), summary, recs)
		},
	})

	fmt.Printf("runright: monitoring started (interval=%s, export=%s)\n", monitorInterval, monitorExport)
	return col.Run(ctx)
}

// runDryRun prints the recommendation table and exits non-zero if the machine is
// not right-sized. It is used when --dry-run is set.
func runDryRun(summary types.MetricsSummary, recs []types.Recommendation) error {
	printTable(recs, summary)
	for _, r := range recs {
		if r.Tier != "right-sized" {
			fmt.Fprintf(os.Stderr, "\nrunright: machine is %s — change instance type to cut costs\n", recs[0].Tier)
			os.Exit(1)
		}
	}
	fmt.Println("\nrunright: machine is right-sized")
	return nil
}

// ── recommend ────────────────────────────────────────────────────────────────

var recommendCmd = &cobra.Command{
	Use:   "recommend",
	Short: "Analyse a metrics summary and recommend machine types",
	Long: `recommend reads a metrics-summary.json file and outputs a ranked list
of AWS/GCP machine recommendations.

Examples:
  runright recommend --metrics metrics-summary.json
  runright recommend --metrics metrics-summary.json --provider aws --format json`,
	RunE: runRecommend,
}

var (
	recommendMetrics           string
	recommendProvider          string
	recommendFormat            string
	recommendAllowedMachineIDs string
	recommendAllowedSeries     string
	recommendAllowedFamilies   string
)

func init() {
	recommendCmd.Flags().StringVar(&recommendMetrics, "metrics", "metrics-summary.json", "Path to metrics-summary.json")
	recommendCmd.Flags().StringVar(&recommendProvider, "provider", "", "Filter by provider: aws, gcp, or github (default: all)")
	recommendCmd.Flags().StringVar(&recommendFormat, "format", "table", "Output format: table, json, markdown")
	recommendCmd.Flags().StringVar(&recommendAllowedMachineIDs, "allowed-machine-ids", "", "Comma-separated machine allow-list (or RUNRIGHT_ALLOWED_MACHINE_IDS)")
	recommendCmd.Flags().StringVar(&recommendAllowedSeries, "allowed-series", "", "Comma-separated series allow-list, e.g. c7g,m7i,n2,e2 (or RUNRIGHT_ALLOWED_SERIES)")
	recommendCmd.Flags().StringVar(&recommendAllowedFamilies, "allowed-families", "", "Comma-separated family prefixes, e.g. c,m,r,n,e (or RUNRIGHT_ALLOWED_FAMILIES)")
}

func runRecommend(_ *cobra.Command, _ []string) error {
	data, err := os.ReadFile(recommendMetrics)
	if err != nil {
		return fmt.Errorf("reading %s: %w", recommendMetrics, err)
	}
	var summary types.MetricsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("parsing metrics summary: %w", err)
	}
	if recommendAllowedMachineIDs == "" {
		recommendAllowedMachineIDs = os.Getenv("RUNRIGHT_ALLOWED_MACHINE_IDS")
	}
	if recommendAllowedSeries == "" {
		recommendAllowedSeries = os.Getenv("RUNRIGHT_ALLOWED_SERIES")
	}
	if recommendAllowedFamilies == "" {
		recommendAllowedFamilies = os.Getenv("RUNRIGHT_ALLOWED_FAMILIES")
	}
	if ids := parseCSV(recommendAllowedMachineIDs); len(ids) > 0 {
		summary.AllowedMachineIDs = ids
	}
	if series := parseCSV(recommendAllowedSeries); len(series) > 0 {
		summary.AllowedSeries = series
	}
	if families := parseCSV(recommendAllowedFamilies); len(families) > 0 {
		summary.AllowedFamilies = families
	}

	opts := catalog.QueryOptions{Provider: types.Provider(recommendProvider)}
	machines := catalog.Query(opts)
	recs := engine.Recommend(summary, machines)

	switch recommendFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(recs)
	case "markdown":
		printMarkdown(recs, summary)
	default:
		printTable(recs, summary)
	}
	return nil
}

func printTable(recs []types.Recommendation, s types.MetricsSummary) {
	fmt.Printf("\nJob: %s | CPU p95: %.1f%% | Mem p95: %.2f GiB | Duration: %.0fs\n",
		s.JobID, s.CPUPercentP95, s.MemUsedGiBP95, s.DurationSeconds)

	// Print the detected machine block so users can see the current baseline.
	if s.DetectedMachine != nil {
		m := s.DetectedMachine
		currentMonthly := m.OnDemandPricePerHour * 720.0
		fmt.Printf("\nDetected:  %s (%s, %d vCPUs, %.1f GiB)  $%.4f/hr  $%.2f/mo\n",
			m.ID, m.Provider, m.VCPUs, m.MemoryGiB, m.OnDemandPricePerHour, currentMonthly)
	}
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIER\tMACHINE\tPROVIDER\tvCPUs\tMEMORY\t$/HR\t$/MO\tDELTA")
	fmt.Fprintln(w, "----\t-------\t--------\t-----\t------\t----\t----\t-----")
	for _, r := range recs {
		delta := fmt.Sprintf("%+.1f%%", r.CostDeltaPercent)
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.1f GiB\t$%.4f\t$%.2f\t%s\n",
			r.Tier, r.Machine.ID, r.Machine.Provider,
			r.Machine.VCPUs, r.Machine.MemoryGiB,
			r.Machine.OnDemandPricePerHour, r.EstimatedMonthly, delta)
	}
	_ = w.Flush()
}

func printMarkdown(recs []types.Recommendation, s types.MetricsSummary) {
	fmt.Printf("## RunRight Recommendations\n\n")
	fmt.Printf("**Job:** `%s` | **CPU p95:** %.1f%% | **Mem p95:** %.2f GiB | **Duration:** %.0fs\n\n",
		s.JobID, s.CPUPercentP95, s.MemUsedGiBP95, s.DurationSeconds)
	fmt.Printf("| Tier | Machine | Provider | vCPUs | Memory | $/hr | $/month | $/year | Delta | Spot Risk |\n")
	fmt.Printf("|------|---------|----------|-------|--------|------|---------|--------|-------|-----------|\n")
	for _, r := range recs {
		annualCost := r.EstimatedMonthly * 12
		spotRisk := r.SpotRisk
		if spotRisk == "" {
			spotRisk = "—"
		}
		fmt.Printf("| %s | `%s` | %s | %d | %.1f GiB | $%.4f | $%.2f | $%.0f | %+.1f%% | %s |\n",
			r.Tier, r.Machine.ID, r.Machine.Provider,
			r.Machine.VCPUs, r.Machine.MemoryGiB,
			r.Machine.OnDemandPricePerHour, r.EstimatedMonthly, annualCost, r.CostDeltaPercent, spotRisk)
	}

	// Annual savings projection for the top cheaper recommendation.
	for _, r := range recs {
		if r.CostDeltaPercent < -0.5 && r.CurrentMonthly > 0 {
			annualSaving := (r.CurrentMonthly - r.EstimatedMonthly) * 12
			if annualSaving > 0 {
				fmt.Printf("\n> Switching to `%s` could save **~$%.0f/year** ($%.2f/month).\n",
					r.Machine.ID, annualSaving, r.CurrentMonthly-r.EstimatedMonthly)
			}
			break
		}
	}

	// Duration risk notes.
	for _, r := range recs {
		if r.DurationRiskNote != "" {
			fmt.Printf("\n> WARNING: **Duration risk** (`%s`): %s\n", r.Machine.ID, r.DurationRiskNote)
			break
		}
	}
}

func parseCSV(in string) []string {
	if strings.TrimSpace(in) == "" {
		return nil
	}
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		norm := strings.TrimSpace(strings.ToLower(p))
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ── catalog ──────────────────────────────────────────────────────────────────

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Browse and update the machine catalog",
}

var catalogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List machines in the catalog",
	RunE:  runCatalogList,
}

var (
	catalogProvider string
	catalogMinVCPUs int
	catalogMaxPrice float64
	catalogArch     string
)

func init() {
	catalogListCmd.Flags().StringVar(&catalogProvider, "provider", "", "Filter by provider: aws or gcp")
	catalogListCmd.Flags().IntVar(&catalogMinVCPUs, "min-vcpus", 0, "Minimum vCPU count")
	catalogListCmd.Flags().Float64Var(&catalogMaxPrice, "max-price", 0, "Maximum on-demand price per hour")
	catalogListCmd.Flags().StringVar(&catalogArch, "arch", "", "Filter by architecture: x86_64 or arm64")
	catalogCmd.AddCommand(catalogListCmd)
}

func runCatalogList(_ *cobra.Command, _ []string) error {
	machines := catalog.Query(catalog.QueryOptions{
		Provider:        types.Provider(catalogProvider),
		MinVCPUs:        catalogMinVCPUs,
		MaxPricePerHour: catalogMaxPrice,
		Architecture:    catalogArch,
	})
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROVIDER\tFAMILY\tvCPUs\tMEMORY\tARCH\t$/HR")
	fmt.Fprintln(w, "--\t--------\t------\t-----\t------\t----\t----")
	for _, m := range machines {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.1f GiB\t%s\t$%.4f\n",
			m.ID, m.Provider, m.Family, m.VCPUs, m.MemoryGiB, m.Architecture, m.OnDemandPricePerHour)
	}
	return w.Flush()
}

// ── serve ─────────────────────────────────────────────────────────────────────

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the RunRight dashboard backend",
	RunE:  runServe,
}

var servePort int

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "HTTP port")
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg := server.ConfigFromEnv()
	cfg.Port = servePort
	srv, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("server init: %w", err)
	}
	return srv.Run(servePort)
}

// ── verify ────────────────────────────────────────────────────────────────────

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Compare a previous recommendation against actual job metrics",
	Long: `verify loads a recommendation JSON (produced by a prior run) and a new
metrics-summary.json from a job that ran on the recommended machine. It checks
whether the recommendation was accurate and reports a pass/fail result.

Examples:
  runright verify --previous-recommendation recs.json --actual-metrics metrics-summary.json`,
	RunE: runVerify,
}

var (
	verifyPreviousRec   string
	verifyActualMetrics string
)

func init() {
	verifyCmd.Flags().StringVar(&verifyPreviousRec, "previous-recommendation", "", "Path to recommendations JSON from a prior run (required)")
	verifyCmd.Flags().StringVar(&verifyActualMetrics, "actual-metrics", "", "Path to metrics-summary.json from the run on the recommended machine (required)")
	_ = verifyCmd.MarkFlagRequired("previous-recommendation")
	_ = verifyCmd.MarkFlagRequired("actual-metrics")
}

func runVerify(_ *cobra.Command, _ []string) error {
	recData, err := os.ReadFile(verifyPreviousRec)
	if err != nil {
		return fmt.Errorf("reading %s: %w", verifyPreviousRec, err)
	}
	var recs []types.Recommendation
	if err := json.Unmarshal(recData, &recs); err != nil {
		return fmt.Errorf("parsing recommendations: %w", err)
	}
	if len(recs) == 0 {
		return fmt.Errorf("no recommendations found in %s", verifyPreviousRec)
	}

	metricData, err := os.ReadFile(verifyActualMetrics)
	if err != nil {
		return fmt.Errorf("reading %s: %w", verifyActualMetrics, err)
	}
	var actual types.MetricsSummary
	if err := json.Unmarshal(metricData, &actual); err != nil {
		return fmt.Errorf("parsing metrics summary: %w", err)
	}

	top := recs[0]
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Printf("\nRunRight Verify — was the recommendation accurate?\n")
	fmt.Printf("Recommended machine: %s (%d vCPUs, %.1f GiB)\n\n",
		top.Machine.ID, top.Machine.VCPUs, top.Machine.MemoryGiB)

	cpuOK := actual.CPUPercentP95 <= float64(top.Machine.VCPUs)/float64(top.RequiredVCPUs)*float64(top.RequiredVCPUs)
	memOK := actual.MemUsedGiBP95 <= top.Machine.MemoryGiB

	// Headroom checks: p95 usage should be within the recommended machine's capacity.
	cpuFit := actual.CPUPercentP95 <= 80.0
	memFit := actual.MemUsedGiBP95 <= top.Machine.MemoryGiB*0.85

	fmt.Fprintln(w, "CHECK\tRESULT\tACTUAL\tRECOMMENDED")
	fmt.Fprintln(w, "-----\t------\t------\t-----------")

	cpuResult := "PASS"
	if !cpuFit {
		cpuResult = "FAIL - machine is CPU-saturated, consider larger"
	} else if !cpuOK {
		cpuResult = "WARN"
	}
	fmt.Fprintf(w, "CPU p95\t%s\t%.1f%%\t%.1f%% headroom target\n",
		cpuResult, actual.CPUPercentP95, 80.0)

	memResult := "PASS"
	if !memFit {
		memResult = "FAIL - machine is memory-saturated, consider larger"
	} else if !memOK {
		memResult = "WARN"
	}
	fmt.Fprintf(w, "Memory p95\t%s\t%.2f GiB\t%.1f GiB available\n",
		memResult, actual.MemUsedGiBP95, top.Machine.MemoryGiB)

	_ = w.Flush()

	if !cpuFit || !memFit {
		fmt.Fprintf(os.Stderr, "\nrunright verify: recommendation was too aggressive — the machine was saturated.\n")
		os.Exit(2)
	}
	fmt.Println("\nrunright verify: recommendation validated")
	return nil
}

// ── update ────────────────────────────────────────────────────────────────────

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates to runright",
	Long: `update checks for a newer version of runright and optionally installs it.

Examples:
  runright update --check          # Check for updates without installing
  runright update                  # Check and install if available
  runright update --channel beta   # Check beta channel for updates`,
	RunE: runUpdate,
}

var (
	updateCheck   bool
	updateChannel string
	updateForce   bool
)

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Only check for updates, don't install")
	updateCmd.Flags().StringVar(&updateChannel, "channel", "stable", "Release channel: stable, beta, or nightly")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if already on latest version")
}

func runUpdate(_ *cobra.Command, _ []string) error {
	cfg := agent.UpdateConfig{
		Enabled:  true,
		Channel:  updateChannel,
		GitHubRepo: "gbudjeakp/run-right",
	}

	fmt.Printf("runright: checking for updates (channel: %s)...\n", updateChannel)

	info, err := agent.CheckForUpdate(cfg)
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}

	fmt.Printf("Current version: %s\n", info.CurrentVersion)
	fmt.Printf("Latest version:  %s\n", info.LatestVersion)

	if !info.UpdateAvailable && !updateForce {
		fmt.Println("runright: already up to date")
		return nil
	}

	if updateCheck {
		if info.UpdateAvailable {
			fmt.Printf("\nUpdate available: %s -> %s\n", info.CurrentVersion, info.LatestVersion)
			fmt.Printf("Release notes: %s\n", info.ReleaseURL)
			if info.DownloadURL != "" {
				fmt.Printf("Download: %s\n", info.DownloadURL)
			}
		}
		return nil
	}

	if info.DownloadURL == "" {
		return fmt.Errorf("no download available for this platform")
	}

	fmt.Printf("Downloading and installing %s...\n", info.LatestVersion)

	if err := agent.SelfUpdate(cfg); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Println("runright: update complete. Please restart to use the new version.")
	return nil
}
