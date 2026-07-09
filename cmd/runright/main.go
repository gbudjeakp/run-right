package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sgbudje/runright/internal/agent"
	"github.com/sgbudje/runright/internal/catalog"
	"github.com/sgbudje/runright/internal/engine"
	"github.com/sgbudje/runright/internal/exporter"
	"github.com/sgbudje/runright/internal/types"
)

// Version is set at build time
var Version = "dev"

// Global output flags
var (
	quietMode   bool
	oneLiner    bool
	noColor     bool
	jsonOutput  bool
)

var rootCmd = &cobra.Command{
	Use:   "runright",
	Short: "Compute sizing tool for CI/CD workloads",
	Long: `runright monitors your CI job's resource usage and recommends
the right AWS, GCP, or Azure machine type so you stop guessing and start saving.`,
	Version: Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if noColor {
			color.NoColor = true
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-essential output")
	rootCmd.PersistentFlags().BoolVar(&oneLiner, "one-liner", false, "Output only the top recommended machine ID")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}

func main() {
	rootCmd.AddCommand(
		monitorCmd, recommendCmd, catalogCmd, verifyCmd, updateCmd, setupCmd,
		initCmd, historyCmd, diffCmd, doctorCmd, explainCmd, completionCmd,
	)
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
	// Exit code policies
	monitorFailIfOversized      float64
	monitorMaxCostPerHour       float64
	monitorAlertIfOver          float64
	// History
	monitorRecordHistory        bool
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
	// Exit code policies
	monitorCmd.Flags().Float64Var(&monitorFailIfOversized, "fail-if-oversized", 0, "Exit non-zero if wasting more than N%% (e.g., 50 for 50%%)")
	monitorCmd.Flags().Float64Var(&monitorMaxCostPerHour, "max-cost-per-hour", 0, "Exit non-zero if recommended machine costs more than $N/hr")
	monitorCmd.Flags().Float64Var(&monitorAlertIfOver, "alert-if-over", 0, "Print warning if monthly cost exceeds $N")
	// History
	monitorCmd.Flags().BoolVar(&monitorRecordHistory, "record-history", true, "Record run in local history database")
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

			// Record to history on completion
			if summary.Status == "completed" && monitorRecordHistory {
				if db, err := agent.OpenHistory(""); err == nil {
					var topRec *types.Recommendation
					if len(recs) > 0 {
						topRec = &recs[0]
					}
					_ = db.RecordRun(summary, topRec)
					db.Close()
				}
			}

			if monitorDryRun && summary.Status == "completed" {
				return runDryRun(summary, recs)
			}
			return mgr.PublishSummary(context.Background(), summary, recs)
		},
	})

	if !quietMode {
		fmt.Printf("runright: monitoring started (interval=%s, export=%s)\n", monitorInterval, monitorExport)
	}
	return col.Run(ctx)
}

// runDryRun prints the recommendation table and exits non-zero if the machine is
// not right-sized. It is used when --dry-run is set.
func runDryRun(summary types.MetricsSummary, recs []types.Recommendation) error {
	if !quietMode {
		printTable(recs, summary)
	}

	// One-liner mode: just print the top recommendation
	if oneLiner && len(recs) > 0 {
		fmt.Println(recs[0].Machine.ID)
		return nil
	}

	// Exit code policies
	if len(recs) > 0 {
		top := recs[0]

		// Check fail-if-oversized policy
		if monitorFailIfOversized > 0 && top.CostDeltaPercent < -monitorFailIfOversized {
			fmt.Fprintf(os.Stderr, "\nrunright: POLICY VIOLATION - wasting %.0f%% (threshold: %.0f%%)\n",
				-top.CostDeltaPercent, monitorFailIfOversized)
			fmt.Fprintf(os.Stderr, "Recommended: %s (saves $%.2f/month)\n",
				top.Machine.ID, top.CurrentMonthly-top.EstimatedMonthly)
			os.Exit(2)
		}

		// Check max-cost-per-hour policy
		if monitorMaxCostPerHour > 0 && top.Machine.OnDemandPricePerHour > monitorMaxCostPerHour {
			fmt.Fprintf(os.Stderr, "\nrunright: POLICY VIOLATION - recommended machine costs $%.4f/hr (max: $%.4f/hr)\n",
				top.Machine.OnDemandPricePerHour, monitorMaxCostPerHour)
			os.Exit(2)
		}

		// Check alert-if-over (warning only, no exit)
		if monitorAlertIfOver > 0 && top.EstimatedMonthly > monitorAlertIfOver {
			fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: Monthly cost ($%.2f) exceeds threshold ($%.2f)\n",
				top.EstimatedMonthly, monitorAlertIfOver)
		}
	}

	for _, r := range recs {
		if r.Tier != "right-sized" {
			if !quietMode {
				fmt.Fprintf(os.Stderr, "\nrunright: machine is %s — change instance type to cut costs\n", recs[0].Tier)
			}
			os.Exit(1)
		}
	}
	if !quietMode {
		fmt.Println("\nrunright: machine is right-sized")
	}
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

// ── setup ─────────────────────────────────────────────────────────────────────

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard for RunRight",
	Long: `setup walks you through configuring RunRight, including SSO providers
for enterprise authentication.

Examples:
  runright setup                   # Full interactive setup
  runright setup --sso             # Configure SSO only
  runright setup --sso google      # Configure Google SSO
  runright setup --sso github      # Configure GitHub SSO
  runright setup --sso okta        # Configure Okta SSO
  runright setup --sso azuread     # Configure Azure AD SSO`,
	RunE: runSetup,
}

var (
	setupSSO      string
	setupURL      string
	setupAPIKey   string
)

func init() {
	setupCmd.Flags().StringVar(&setupSSO, "sso", "", "Configure SSO (google, github, okta, azuread, oidc, saml)")
	setupCmd.Flags().StringVar(&setupURL, "url", "http://localhost:8080", "RunRight server URL")
	setupCmd.Flags().StringVar(&setupAPIKey, "api-key", "", "API key for authentication (or RUNRIGHT_API_KEY)")
}

func runSetup(_ *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Get API key
	apiKey := setupAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("RUNRIGHT_API_KEY")
	}
	if apiKey == "" {
		fmt.Print("Enter your RUNRIGHT_API_KEY: ")
		input, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(input)
	}
	if apiKey == "" {
		return fmt.Errorf("API key required for setup")
	}

	// Get server URL
	serverURL := setupURL
	if serverURL == "" {
		serverURL = os.Getenv("RUNRIGHT_URL")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║           RunRight Setup Wizard                       ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
	fmt.Println()

	// Determine what to configure
	ssoProvider := setupSSO
	if len(args) > 0 && ssoProvider == "" {
		ssoProvider = args[0]
	}

	if ssoProvider == "" {
		fmt.Println("What would you like to configure?")
		fmt.Println()
		fmt.Println("  [1] SSO (Google, GitHub, Okta, Azure AD, or SAML)")
		fmt.Println("  [2] Exit")
		fmt.Println()
		fmt.Print("Enter choice [1-2]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1", "sso":
			return setupSSOInteractive(reader, serverURL, apiKey)
		default:
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	return setupSSOProvider(reader, serverURL, apiKey, ssoProvider)
}

func setupSSOInteractive(reader *bufio.Reader, serverURL, apiKey string) error {
	fmt.Println()
	fmt.Println("Select your identity provider:")
	fmt.Println()
	fmt.Println("  [1] Google Workspace")
	fmt.Println("  [2] GitHub")
	fmt.Println("  [3] Okta")
	fmt.Println("  [4] Azure AD (Microsoft Entra)")
	fmt.Println("  [5] Generic OIDC")
	fmt.Println("  [6] SAML 2.0")
	fmt.Println()
	fmt.Print("Enter choice [1-6]: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	providerMap := map[string]string{
		"1": "google", "2": "github", "3": "okta",
		"4": "azuread", "5": "oidc", "6": "saml",
	}
	provider, ok := providerMap[choice]
	if !ok {
		return fmt.Errorf("invalid choice")
	}

	return setupSSOProvider(reader, serverURL, apiKey, provider)
}

func setupSSOProvider(reader *bufio.Reader, serverURL, apiKey, provider string) error {
	fmt.Println()
	fmt.Printf("Configuring %s SSO...\n", strings.ToUpper(provider))
	fmt.Println()

	config := map[string]interface{}{
		"provider_type": provider,
		"enabled":       true,
		"default_role":  "viewer",
	}

	// Provider-specific setup instructions and prompts
	switch provider {
	case "google":
		fmt.Println("Google OAuth Setup:")
		fmt.Println("  1. Go to https://console.cloud.google.com/apis/credentials")
		fmt.Println("  2. Create OAuth 2.0 Client ID (Web application)")
		fmt.Printf("  3. Add redirect URI: %s/api/v1/sso/callback/google\n", serverURL)
		fmt.Println()
		config["name"] = "Google"
		config["scopes"] = "email,profile,openid"

	case "github":
		fmt.Println("GitHub OAuth Setup:")
		fmt.Println("  1. Go to https://github.com/settings/developers")
		fmt.Println("  2. Create a new OAuth App")
		fmt.Printf("  3. Set callback URL: %s/api/v1/sso/callback/github\n", serverURL)
		fmt.Println()
		config["name"] = "GitHub"
		config["scopes"] = "user:email,read:org"

	case "okta":
		fmt.Println("Okta OIDC Setup:")
		fmt.Println("  1. In Okta Admin Console, go to Applications → Create App")
		fmt.Println("  2. Select OIDC - OpenID Connect, then Web Application")
		fmt.Printf("  3. Set redirect URI: %s/api/v1/sso/callback/okta\n", serverURL)
		fmt.Println()
		config["name"] = "Okta"
		config["scopes"] = "openid,profile,email"

		fmt.Print("Enter your Okta domain (e.g., yourcompany.okta.com): ")
		domain, _ := reader.ReadString('\n')
		domain = strings.TrimSpace(domain)
		if domain == "" {
			return fmt.Errorf("Okta domain required")
		}
		config["issuer_url"] = fmt.Sprintf("https://%s", domain)

	case "azuread":
		fmt.Println("Azure AD Setup:")
		fmt.Println("  1. Go to Azure Portal → Azure Active Directory → App registrations")
		fmt.Println("  2. Create a new registration")
		fmt.Printf("  3. Add redirect URI: %s/api/v1/sso/callback/azuread\n", serverURL)
		fmt.Println()
		config["name"] = "Azure AD"
		config["scopes"] = "openid,profile,email"

	case "oidc":
		fmt.Println("Generic OIDC Setup:")
		fmt.Printf("Redirect URI for your IdP: %s/api/v1/sso/callback/oidc\n", serverURL)
		fmt.Println()

		fmt.Print("Enter display name: ")
		name, _ := reader.ReadString('\n')
		config["name"] = strings.TrimSpace(name)

		fmt.Print("Enter OIDC issuer URL (e.g., https://idp.example.com): ")
		issuer, _ := reader.ReadString('\n')
		config["issuer_url"] = strings.TrimSpace(issuer)
		config["scopes"] = "openid,profile,email"

	case "saml":
		fmt.Println("SAML 2.0 Setup:")
		fmt.Printf("ACS URL: %s/api/v1/sso/callback/saml\n", serverURL)
		fmt.Printf("Entity ID: %s\n", serverURL)
		fmt.Println()
		fmt.Println("Note: SAML requires X.509 certificates. Set RUNRIGHT_SAML_CERT and")
		fmt.Println("RUNRIGHT_SAML_KEY environment variables on the server.")
		fmt.Println()

		fmt.Print("Enter display name: ")
		name, _ := reader.ReadString('\n')
		config["name"] = strings.TrimSpace(name)

		fmt.Print("Enter IDP metadata URL: ")
		metadataURL, _ := reader.ReadString('\n')
		config["idp_metadata_url"] = strings.TrimSpace(metadataURL)
		config["sp_entity_id"] = serverURL

		// SAML doesn't use client_id/client_secret
		config["client_id"] = "saml"
		config["client_secret"] = "saml"

		return saveAndEnableSSO(serverURL, apiKey, config)
	}

	// Common OAuth prompts
	fmt.Println()
	fmt.Print("Enter Client ID: ")
	clientID, _ := reader.ReadString('\n')
	config["client_id"] = strings.TrimSpace(clientID)

	fmt.Print("Enter Client Secret: ")
	clientSecret, _ := reader.ReadString('\n')
	config["client_secret"] = strings.TrimSpace(clientSecret)

	// Optional: allowed domains
	fmt.Println()
	fmt.Print("Restrict to email domains? (comma-separated, or leave blank for all): ")
	domains, _ := reader.ReadString('\n')
	domains = strings.TrimSpace(domains)
	if domains != "" {
		config["allowed_domains"] = domains
	}

	return saveAndEnableSSO(serverURL, apiKey, config)
}

func saveAndEnableSSO(serverURL, apiKey string, config map[string]interface{}) error {
	fmt.Println()
	fmt.Println("Saving SSO configuration...")

	// Create HTTP client and make request
	body, _ := json.Marshal(config)
	req, err := http.NewRequest("PUT", serverURL+"/api/v1/sso/configs", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", fmt.Sprintf("runright_session=%s", apiKey))

	// First, authenticate with API key to get session
	authBody, _ := json.Marshal(map[string]string{"api_key": apiKey})
	authReq, _ := http.NewRequest("POST", serverURL+"/api/v1/auth", bytes.NewReader(authBody))
	authReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	authResp, err := client.Do(authReq)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	authResp.Body.Close()

	if authResp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed: invalid API key")
	}

	// Get session cookie
	var sessionCookie string
	for _, cookie := range authResp.Cookies() {
		if cookie.Name == "runright_session" {
			sessionCookie = cookie.Value
			break
		}
	}

	// Now save the SSO config
	req.Header.Set("Cookie", fmt.Sprintf("runright_session=%s", sessionCookie))
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to save SSO config (status %d)", resp.StatusCode)
	}

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║           SSO Configuration Complete!                 ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Provider: %s\n", config["provider_type"])
	fmt.Printf("Name: %s\n", config["name"])
	fmt.Printf("Status: Enabled\n")
	fmt.Println()
	fmt.Println("Users can now sign in with SSO at your RunRight login page.")
	fmt.Println()

	return nil
}

// ── init ──────────────────────────────────────────────────────────────────────

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize RunRight in the current project",
	Long: `init auto-detects your CI platform, generates a config file,
and provides integration snippets for your CI system.

Examples:
  runright init                   # Auto-detect and generate config
  runright init --platform github # Force GitHub Actions config
  runright init --dry-run         # Show what would be created`,
	RunE: runInit,
}

var (
	initPlatform string
	initDryRun   bool
	initForce    bool
)

func init() {
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Force specific platform: github, gitlab, jenkins, circleci, azure, bitbucket")
	initCmd.Flags().BoolVar(&initDryRun, "dry-run", false, "Show what would be created without writing files")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing config files")
}

func runInit(_ *cobra.Command, _ []string) error {
	if !quietMode {
		fmt.Println()
		fmt.Println("╔═══════════════════════════════════════════════════════╗")
		fmt.Println("║              RunRight Project Setup                   ║")
		fmt.Println("╚═══════════════════════════════════════════════════════╝")
		fmt.Println()
	}

	// Detect CI platform
	detection := agent.DetectCI()
	if initPlatform != "" {
		detection.Platform = agent.CIPlatform(initPlatform)
	}

	if !quietMode {
		fmt.Printf("Detected CI Platform: %s\n", detection.Platform)
		if detection.Repository != "" {
			fmt.Printf("Repository: %s\n", detection.Repository)
		}
	}

	// Detect project type
	project := agent.DetectProjectType()
	if !quietMode && project.Language != "" {
		fmt.Printf("Project Type: %s (%s)\n", project.Language, project.BuildTool)
	}

	// Generate config
	cfg := agent.DefaultInitConfig(detection)
	configContent, _ := agent.GenerateConfigFile(cfg)

	if initDryRun {
		fmt.Println()
		fmt.Println("Would create .runright.yaml:")
		fmt.Println("─────────────────────────────")
		fmt.Println(configContent)
		fmt.Println()
		fmt.Println("CI Integration snippet:")
		fmt.Println("───────────────────────")
		fmt.Println(agent.GenerateCISnippet(detection.Platform, cfg))
		return nil
	}

	// Check if config exists
	if _, err := os.Stat(".runright.yaml"); err == nil && !initForce {
		return fmt.Errorf(".runright.yaml already exists. Use --force to overwrite")
	}

	// Write config file
	if err := agent.WriteConfigFile(".runright.yaml", configContent); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if !quietMode {
		fmt.Println("✓ Created .runright.yaml")
	}

	// Create output directory
	if err := agent.EnsureOutputDir(cfg.OutputDir); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if !quietMode {
		fmt.Printf("✓ Created %s/ directory\n", cfg.OutputDir)
	}

	// Update .gitignore
	if cfg.GitIgnore {
		if err := agent.WriteGitIgnore(cfg.OutputDir); err != nil {
			if !quietMode {
				fmt.Printf("⚠ Could not update .gitignore: %v\n", err)
			}
		} else if !quietMode {
			fmt.Println("✓ Updated .gitignore")
		}
	}

	// Show CI integration snippet
	if !quietMode {
		fmt.Println()
		fmt.Println("Add this to your CI configuration:")
		fmt.Println("───────────────────────────────────")
		fmt.Println(agent.GenerateCISnippet(detection.Platform, cfg))
	}

	if oneLiner {
		fmt.Println(".runright.yaml")
	}

	return nil
}

// ── history ───────────────────────────────────────────────────────────────────

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View local run history",
	Long: `history shows past monitoring runs stored in a local SQLite database.
Use this to track trends without needing the RunRight platform.

Examples:
  runright history                    # Show recent runs
  runright history --job-id build     # Filter by job ID
  runright history --stats            # Show aggregate statistics
  runright history --prune 30d        # Remove runs older than 30 days`,
	RunE: runHistory,
}

var (
	historyJobID     string
	historyRepo      string
	historyLimit     int
	historyStats     bool
	historyPrune     string
	historyDBPath    string
)

func init() {
	historyCmd.Flags().StringVar(&historyJobID, "job-id", "", "Filter by job ID")
	historyCmd.Flags().StringVar(&historyRepo, "repository", "", "Filter by repository")
	historyCmd.Flags().IntVar(&historyLimit, "limit", 20, "Maximum number of runs to show")
	historyCmd.Flags().BoolVar(&historyStats, "stats", false, "Show aggregate statistics")
	historyCmd.Flags().StringVar(&historyPrune, "prune", "", "Remove runs older than duration (e.g., 30d, 90d)")
	historyCmd.Flags().StringVar(&historyDBPath, "db", "", "Path to history database (default: ~/.runright/history.db)")
}

func runHistory(_ *cobra.Command, _ []string) error {
	db, err := agent.OpenHistory(historyDBPath)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	defer db.Close()

	// Handle prune
	if historyPrune != "" {
		duration, err := time.ParseDuration(historyPrune)
		if err != nil {
			// Try parsing as days
			var days int
			if _, err := fmt.Sscanf(historyPrune, "%dd", &days); err == nil {
				duration = time.Duration(days) * 24 * time.Hour
			} else {
				return fmt.Errorf("invalid prune duration: %s (use e.g., 30d or 720h)", historyPrune)
			}
		}
		deleted, err := db.Prune(duration)
		if err != nil {
			return fmt.Errorf("prune: %w", err)
		}
		fmt.Printf("Pruned %d runs older than %s\n", deleted, historyPrune)
		return nil
	}

	// Handle stats
	if historyStats {
		stats, err := db.Stats()
		if err != nil {
			return fmt.Errorf("get stats: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(stats)
		}

		fmt.Println()
		fmt.Println("RunRight History Statistics")
		fmt.Println("───────────────────────────")
		fmt.Printf("Total Runs:          %d\n", stats.TotalRuns)
		fmt.Printf("Unique Jobs:         %d\n", stats.UniqueJobs)
		fmt.Printf("Unique Repos:        %d\n", stats.UniqueRepos)
		fmt.Printf("Total Duration:      %.1f minutes\n", stats.TotalDurationMin)
		fmt.Printf("Avg CPU (p95):       %.1f%%\n", stats.AvgCPUP95)
		fmt.Printf("Avg Memory (p95):    %.2f GiB\n", stats.AvgMemP95)
		fmt.Printf("Oversized Runs:      %d\n", stats.OversizedRuns)
		if stats.PotentialSavings > 0 {
			fmt.Printf("Est. Monthly Savings: $%.2f\n", stats.PotentialSavings)
		}
		return nil
	}

	// List runs
	runs, err := db.ListRuns(agent.ListOptions{
		JobID:      historyJobID,
		Repository: historyRepo,
		Limit:      historyLimit,
	})
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No runs found. Run 'runright monitor' to record your first run.")
		return nil
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(runs)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tJOB\tDURATION\tCPU p95\tMEM p95\tRECOMMEND\tDELTA")
	fmt.Fprintln(w, "----\t---\t--------\t-------\t-------\t---------\t-----")
	for _, r := range runs {
		delta := ""
		if r.CostDelta != 0 {
			delta = fmt.Sprintf("%+.0f%%", r.CostDelta)
		}
		fmt.Fprintf(w, "%s\t%s\t%.0fs\t%.1f%%\t%.2f GiB\t%s\t%s\n",
			r.StartTime.Format("2006-01-02 15:04"),
			truncate(r.JobID, 20),
			r.DurationSeconds,
			r.CPUPercentP95,
			r.MemUsedGiBP95,
			r.TopRecommend,
			delta,
		)
	}
	return w.Flush()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ── diff ──────────────────────────────────────────────────────────────────────

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare two monitoring runs",
	Long: `diff compares two metrics summaries to highlight changes in resource usage.
Useful for comparing before/after optimizations or PR comparisons.

Examples:
  runright diff --before metrics-old.json --after metrics-new.json
  runright diff --run1 abc123 --run2 def456   # Compare by run ID from history`,
	RunE: runDiff,
}

var (
	diffBefore string
	diffAfter  string
	diffRun1   string
	diffRun2   string
)

func init() {
	diffCmd.Flags().StringVar(&diffBefore, "before", "", "Path to before metrics-summary.json")
	diffCmd.Flags().StringVar(&diffAfter, "after", "", "Path to after metrics-summary.json")
	diffCmd.Flags().StringVar(&diffRun1, "run1", "", "First run ID from history")
	diffCmd.Flags().StringVar(&diffRun2, "run2", "", "Second run ID from history")
}

func runDiff(_ *cobra.Command, _ []string) error {
	var before, after types.MetricsSummary

	if diffBefore != "" && diffAfter != "" {
		// Load from files
		data1, err := os.ReadFile(diffBefore)
		if err != nil {
			return fmt.Errorf("read before: %w", err)
		}
		if err := json.Unmarshal(data1, &before); err != nil {
			return fmt.Errorf("parse before: %w", err)
		}

		data2, err := os.ReadFile(diffAfter)
		if err != nil {
			return fmt.Errorf("read after: %w", err)
		}
		if err := json.Unmarshal(data2, &after); err != nil {
			return fmt.Errorf("parse after: %w", err)
		}
	} else if diffRun1 != "" && diffRun2 != "" {
		// Load from history
		db, err := agent.OpenHistory("")
		if err != nil {
			return fmt.Errorf("open history: %w", err)
		}
		defer db.Close()

		entry1, err := db.GetRun(diffRun1)
		if err != nil {
			return fmt.Errorf("get run1: %w", err)
		}
		if entry1 == nil || entry1.Summary == nil {
			return fmt.Errorf("run %s not found or has no summary", diffRun1)
		}
		before = *entry1.Summary

		entry2, err := db.GetRun(diffRun2)
		if err != nil {
			return fmt.Errorf("get run2: %w", err)
		}
		if entry2 == nil || entry2.Summary == nil {
			return fmt.Errorf("run %s not found or has no summary", diffRun2)
		}
		after = *entry2.Summary
	} else {
		return fmt.Errorf("specify --before/--after or --run1/--run2")
	}

	output := engine.ExplainDiff(before, after)
	fmt.Println(output)
	return nil
}

// ── doctor ────────────────────────────────────────────────────────────────────

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose system configuration and capabilities",
	Long: `doctor runs a series of diagnostic checks to verify RunRight
can access system metrics, network, and optional features like
GPU monitoring and Docker container metrics.

Examples:
  runright doctor                 # Run all diagnostics
  runright doctor --json          # Output as JSON`,
	RunE: runDoctor,
}

func runDoctor(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report := agent.RunDiagnostics(ctx)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║              RunRight System Diagnostics              ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Platform: %s\n", report.Platform)
	fmt.Printf("Go Version: %s\n", report.GoVersion)
	fmt.Printf("RunRight Version: %s\n", Version)
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CHECK\tSTATUS\tMESSAGE")
	fmt.Fprintln(w, "-----\t------\t-------")

	for _, c := range report.Checks {
		status := c.Status
		switch c.Status {
		case "pass":
			status = color.GreenString("✓ PASS")
		case "warn":
			status = color.YellowString("⚠ WARN")
		case "fail":
			status = color.RedString("✗ FAIL")
		case "skip":
			status = color.HiBlackString("○ SKIP")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, status, c.Message)
	}
	_ = w.Flush()

	fmt.Println()
	fmt.Printf("Summary: %d passed, %d warnings, %d failed, %d skipped\n",
		report.PassCount, report.WarnCount, report.FailCount, report.SkipCount)

	if report.FailCount > 0 {
		os.Exit(1)
	}
	return nil
}

// ── explain ───────────────────────────────────────────────────────────────────

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain a recommendation in human-readable terms",
	Long: `explain provides a detailed breakdown of why a recommendation
was made, including CPU/memory analysis and cost calculations.

Examples:
  runright explain --metrics metrics-summary.json
  runright explain --metrics metrics-summary.json --machine t3.medium`,
	RunE: runExplain,
}

var (
	explainMetrics  string
	explainMachine  string
)

func init() {
	explainCmd.Flags().StringVar(&explainMetrics, "metrics", "metrics-summary.json", "Path to metrics-summary.json")
	explainCmd.Flags().StringVar(&explainMachine, "machine", "", "Specific machine to explain (default: top recommendation)")
}

func runExplain(_ *cobra.Command, _ []string) error {
	data, err := os.ReadFile(explainMetrics)
	if err != nil {
		return fmt.Errorf("read metrics: %w", err)
	}

	var summary types.MetricsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("parse metrics: %w", err)
	}

	machines := catalog.Query(catalog.QueryOptions{})
	recs := engine.Recommend(summary, machines)

	if len(recs) == 0 {
		return fmt.Errorf("no recommendations available")
	}

	// Find the recommendation to explain
	var rec types.Recommendation
	if explainMachine != "" {
		found := false
		for _, r := range recs {
			if r.Machine.ID == explainMachine {
				rec = r
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("machine %s not in recommendations", explainMachine)
		}
	} else {
		rec = recs[0]
	}

	explanation := engine.Explain(summary, rec)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(explanation)
	}

	fmt.Println(explanation.DetailedBreakdown)

	if len(explanation.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range explanation.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}

	if len(explanation.Suggestions) > 0 {
		fmt.Println("\nSuggestions:")
		for _, s := range explanation.Suggestions {
			fmt.Printf("  💡 %s\n", s)
		}
	}

	return nil
}

// ── completion ────────────────────────────────────────────────────────────────

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for RunRight.

To load completions:

Bash:
  $ source <(runright completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ runright completion bash > /etc/bash_completion.d/runright
  # macOS:
  $ runright completion bash > /usr/local/etc/bash_completion.d/runright

Zsh:
  $ source <(runright completion zsh)
  # To load completions for each session, execute once:
  $ runright completion zsh > "${fpath[1]}/_runright"

Fish:
  $ runright completion fish | source
  # To load completions for each session, execute once:
  $ runright completion fish > ~/.config/fish/completions/runright.fish

PowerShell:
  PS> runright completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, execute once:
  PS> runright completion powershell > runright.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

// Suppress unused import warning
var _ = runtime.GOOS
