package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
	rootCmd.AddCommand(monitorCmd, recommendCmd, catalogCmd, serveCmd)
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
	monitorDuration  time.Duration
	monitorInterval  time.Duration
	monitorExport    string
	monitorOutputDir string
	monitorJobID     string
	monitorPromPort  int
	monitorHTTPURL   string
	monitorSlackURL  string
	monitorDryRun    bool
)

func init() {
	monitorCmd.Flags().DurationVar(&monitorDuration, "duration", 0, "Stop after this duration (0 = run until SIGTERM/SIGINT)")
	monitorCmd.Flags().DurationVar(&monitorInterval, "interval", 5*time.Second, "Sampling interval")
	monitorCmd.Flags().StringVar(&monitorExport, "export", "file", "Comma-separated export backends: file,otlp,prometheus,http,slack")
	monitorCmd.Flags().StringVar(&monitorOutputDir, "output-dir", ".", "Directory for file-based output")
	monitorCmd.Flags().StringVar(&monitorJobID, "job-id", "", "Job identifier (defaults to a timestamp-based ID)")
	monitorCmd.Flags().IntVar(&monitorPromPort, "prometheus-port", 9090, "Port for Prometheus /metrics endpoint")
	monitorCmd.Flags().StringVar(&monitorHTTPURL, "http-url", "", "Base URL of runright backend for http export")
	monitorCmd.Flags().StringVar(&monitorSlackURL, "slack-webhook", "", "Slack incoming webhook URL for slack export (or set RUNRIGHT_SLACK_WEBHOOK)")
	monitorCmd.Flags().BoolVar(&monitorDryRun, "dry-run", false, "Print recommendation and exit non-zero if machine is not right-sized")
}

func runMonitor(_ *cobra.Command, _ []string) error {
	backends := exporter.ParseBackends(monitorExport)

	// Allow slack webhook via env var as fallback.
	if monitorSlackURL == "" {
		monitorSlackURL = os.Getenv("RUNRIGHT_SLACK_WEBHOOK")
	}

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

	mgr, err := exporter.New(ctx, backends, monitorPromPort, monitorHTTPURL, monitorSlackURL)
	if err != nil {
		return fmt.Errorf("exporter: %w", err)
	}
	defer mgr.Shutdown(context.Background())

	col := agent.NewCollector(agent.Config{
		Interval:          monitorInterval,
		OutputDir:         monitorOutputDir,
		JobID:             monitorJobID,
		HeartbeatFilePath: filepath.Join(monitorOutputDir, "metrics-heartbeat.json"),
		FlushFn: func(summary types.MetricsSummary) error {
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
	recommendMetrics  string
	recommendProvider string
	recommendFormat   string
)

func init() {
	recommendCmd.Flags().StringVar(&recommendMetrics, "metrics", "metrics-summary.json", "Path to metrics-summary.json")
	recommendCmd.Flags().StringVar(&recommendProvider, "provider", "", "Filter by provider: aws or gcp (default: both)")
	recommendCmd.Flags().StringVar(&recommendFormat, "format", "table", "Output format: table, json, markdown")
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
	fmt.Printf("\nJob: %s | CPU p95: %.1f%% | Mem p95: %.2f GiB | Duration: %.0fs\n\n",
		s.JobID, s.CPUPercentP95, s.MemUsedGiBP95, s.DurationSeconds)
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
	fmt.Printf("| Tier | Machine | Provider | vCPUs | Memory | $/hr | $/month | Delta |\n")
	fmt.Printf("|------|---------|----------|-------|--------|------|---------|-------|\n")
	for _, r := range recs {
		fmt.Printf("| %s | `%s` | %s | %d | %.1f GiB | $%.4f | $%.2f | %+.1f%% |\n",
			r.Tier, r.Machine.ID, r.Machine.Provider,
			r.Machine.VCPUs, r.Machine.MemoryGiB,
			r.Machine.OnDemandPricePerHour, r.EstimatedMonthly, r.CostDeltaPercent)
	}
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
	catalogProvider   string
	catalogMinVCPUs   int
	catalogMaxPrice   float64
	catalogArch       string
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
