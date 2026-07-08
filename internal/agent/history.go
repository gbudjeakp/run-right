package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sgbudje/runright/internal/types"
)

// HistoryDB manages local run history in SQLite.
type HistoryDB struct {
	db   *sql.DB
	path string
}

// HistoryEntry represents a single historical run.
type HistoryEntry struct {
	ID              int64              `json:"id"`
	RunID           string             `json:"run_id"`
	JobID           string             `json:"job_id"`
	Repository      string             `json:"repository,omitempty"`
	CIPlatform      string             `json:"ci_platform,omitempty"`
	StartTime       time.Time          `json:"start_time"`
	EndTime         time.Time          `json:"end_time"`
	DurationSeconds float64            `json:"duration_seconds"`
	CPUPercentP95   float64            `json:"cpu_percent_p95"`
	MemUsedGiBP95   float64            `json:"mem_used_gib_p95"`
	DetectedMachine string             `json:"detected_machine,omitempty"`
	TopRecommend    string             `json:"top_recommend,omitempty"`
	CostDelta       float64            `json:"cost_delta_percent,omitempty"`
	Summary         *types.MetricsSummary `json:"summary,omitempty"`
}

// DefaultHistoryPath returns the default path for the history database.
func DefaultHistoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".runright", "history.db")
}

// OpenHistory opens or creates a history database.
func OpenHistory(path string) (*HistoryDB, error) {
	if path == "" {
		path = DefaultHistoryPath()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create history dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open history db: %w", err)
	}

	h := &HistoryDB{db: db, path: path}
	if err := h.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate history db: %w", err)
	}

	return h, nil
}

// migrate creates the schema if it doesn't exist.
func (h *HistoryDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT UNIQUE NOT NULL,
		job_id TEXT NOT NULL,
		repository TEXT,
		ci_platform TEXT,
		start_time TEXT NOT NULL,
		end_time TEXT NOT NULL,
		duration_seconds REAL NOT NULL,
		cpu_percent_p95 REAL NOT NULL,
		mem_used_gib_p95 REAL NOT NULL,
		detected_machine TEXT,
		top_recommend TEXT,
		cost_delta_percent REAL,
		summary_json TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_runs_job_id ON runs(job_id);
	CREATE INDEX IF NOT EXISTS idx_runs_repository ON runs(repository);
	CREATE INDEX IF NOT EXISTS idx_runs_start_time ON runs(start_time);
	`
	_, err := h.db.Exec(schema)
	return err
}

// RecordRun stores a completed run in history.
func (h *HistoryDB) RecordRun(summary types.MetricsSummary, topRec *types.Recommendation) error {
	summaryJSON, _ := json.Marshal(summary)

	detected := ""
	if summary.DetectedMachine != nil {
		detected = summary.DetectedMachine.ID
	}

	topMachine := ""
	costDelta := 0.0
	if topRec != nil {
		topMachine = topRec.Machine.ID
		costDelta = topRec.CostDeltaPercent
	}

	_, err := h.db.Exec(`
		INSERT OR REPLACE INTO runs (
			run_id, job_id, repository, ci_platform,
			start_time, end_time, duration_seconds,
			cpu_percent_p95, mem_used_gib_p95,
			detected_machine, top_recommend, cost_delta_percent,
			summary_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		summary.RunID, summary.JobID, summary.Repository, summary.CIPlatform,
		summary.StartTime.Format(time.RFC3339), summary.EndTime.Format(time.RFC3339),
		summary.DurationSeconds,
		summary.CPUPercentP95, summary.MemUsedGiBP95,
		detected, topMachine, costDelta,
		string(summaryJSON),
	)
	return err
}

// ListRuns returns recent runs, optionally filtered.
type ListOptions struct {
	JobID      string
	Repository string
	Limit      int
	Since      time.Time
}

// ListRuns returns historical runs matching the options.
func (h *HistoryDB) ListRuns(opts ListOptions) ([]HistoryEntry, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	query := `
		SELECT id, run_id, job_id, repository, ci_platform,
			start_time, end_time, duration_seconds,
			cpu_percent_p95, mem_used_gib_p95,
			detected_machine, top_recommend, cost_delta_percent
		FROM runs
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.JobID != "" {
		query += " AND job_id = ?"
		args = append(args, opts.JobID)
	}
	if opts.Repository != "" {
		query += " AND repository = ?"
		args = append(args, opts.Repository)
	}
	if !opts.Since.IsZero() {
		query += " AND start_time >= ?"
		args = append(args, opts.Since.Format(time.RFC3339))
	}

	query += " ORDER BY start_time DESC LIMIT ?"
	args = append(args, opts.Limit)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var startStr, endStr string
		var detected, topRec sql.NullString
		var costDelta sql.NullFloat64
		var repo, ci sql.NullString

		err := rows.Scan(
			&e.ID, &e.RunID, &e.JobID, &repo, &ci,
			&startStr, &endStr, &e.DurationSeconds,
			&e.CPUPercentP95, &e.MemUsedGiBP95,
			&detected, &topRec, &costDelta,
		)
		if err != nil {
			return nil, err
		}

		e.StartTime, _ = time.Parse(time.RFC3339, startStr)
		e.EndTime, _ = time.Parse(time.RFC3339, endStr)
		e.Repository = repo.String
		e.CIPlatform = ci.String
		e.DetectedMachine = detected.String
		e.TopRecommend = topRec.String
		e.CostDelta = costDelta.Float64

		entries = append(entries, e)
	}

	return entries, nil
}

// GetRun retrieves a single run by run_id with full summary.
func (h *HistoryDB) GetRun(runID string) (*HistoryEntry, error) {
	var e HistoryEntry
	var startStr, endStr string
	var detected, topRec sql.NullString
	var costDelta sql.NullFloat64
	var repo, ci sql.NullString
	var summaryJSON sql.NullString

	err := h.db.QueryRow(`
		SELECT id, run_id, job_id, repository, ci_platform,
			start_time, end_time, duration_seconds,
			cpu_percent_p95, mem_used_gib_p95,
			detected_machine, top_recommend, cost_delta_percent,
			summary_json
		FROM runs WHERE run_id = ?
	`, runID).Scan(
		&e.ID, &e.RunID, &e.JobID, &repo, &ci,
		&startStr, &endStr, &e.DurationSeconds,
		&e.CPUPercentP95, &e.MemUsedGiBP95,
		&detected, &topRec, &costDelta,
		&summaryJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	e.StartTime, _ = time.Parse(time.RFC3339, startStr)
	e.EndTime, _ = time.Parse(time.RFC3339, endStr)
	e.Repository = repo.String
	e.CIPlatform = ci.String
	e.DetectedMachine = detected.String
	e.TopRecommend = topRec.String
	e.CostDelta = costDelta.Float64

	if summaryJSON.Valid {
		var summary types.MetricsSummary
		if err := json.Unmarshal([]byte(summaryJSON.String), &summary); err == nil {
			e.Summary = &summary
		}
	}

	return &e, nil
}

// Stats returns aggregate statistics.
type HistoryStats struct {
	TotalRuns        int     `json:"total_runs"`
	UniqueJobs       int     `json:"unique_jobs"`
	UniqueRepos      int     `json:"unique_repos"`
	TotalDurationMin float64 `json:"total_duration_min"`
	AvgCPUP95        float64 `json:"avg_cpu_p95"`
	AvgMemP95        float64 `json:"avg_mem_p95_gib"`
	OversizedRuns    int     `json:"oversized_runs"`
	PotentialSavings float64 `json:"potential_savings_monthly"`
}

// Stats returns aggregate statistics across all runs.
func (h *HistoryDB) Stats() (*HistoryStats, error) {
	var s HistoryStats

	row := h.db.QueryRow(`
		SELECT 
			COUNT(*) as total_runs,
			COUNT(DISTINCT job_id) as unique_jobs,
			COUNT(DISTINCT repository) as unique_repos,
			COALESCE(SUM(duration_seconds) / 60.0, 0) as total_duration_min,
			COALESCE(AVG(cpu_percent_p95), 0) as avg_cpu_p95,
			COALESCE(AVG(mem_used_gib_p95), 0) as avg_mem_p95,
			COALESCE(SUM(CASE WHEN cost_delta_percent < -10 THEN 1 ELSE 0 END), 0) as oversized_runs
		FROM runs
	`)

	err := row.Scan(
		&s.TotalRuns, &s.UniqueJobs, &s.UniqueRepos,
		&s.TotalDurationMin, &s.AvgCPUP95, &s.AvgMemP95,
		&s.OversizedRuns,
	)
	if err != nil {
		return nil, err
	}

	// Calculate potential monthly savings (rough estimate)
	// Assume avg job costs $0.10/hr and runs 100 times/month
	// If oversized by avg 30%, savings = oversized_runs * 0.03 * 100
	if s.OversizedRuns > 0 {
		s.PotentialSavings = float64(s.OversizedRuns) * 3.0 // rough estimate
	}

	return &s, nil
}

// Close closes the database.
func (h *HistoryDB) Close() error {
	return h.db.Close()
}

// Prune removes runs older than the given duration.
func (h *HistoryDB) Prune(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339)
	result, err := h.db.Exec(`DELETE FROM runs WHERE start_time < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
