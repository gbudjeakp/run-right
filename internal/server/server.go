package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/sgbudje/runright/internal/catalog"
	"github.com/sgbudje/runright/internal/types"
)

// Server wraps the Gin engine and database connection.
type Server struct {
	router       *gin.Engine
	db           *sql.DB
	slackWebhook string
}

// Config holds server configuration.
type Config struct {
	Port         int
	DSN          string // Postgres DSN, e.g. postgres://user:pass@localhost/scalecidb
	APIKey       string
	SlackWebhook string // optional; if set, weekly savings digests are posted here
}

// New creates a Server, runs migrations, and wires up routes.
func New(cfg Config) (*Server, error) {
	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migration: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	s := &Server{router: r, db: db, slackWebhook: cfg.SlackWebhook}

	// Auth endpoint — no middleware applied here.
	r.POST("/api/v1/auth", authLogin(cfg.APIKey))
	r.POST("/api/v1/auth/logout", authLogout())

	v1 := r.Group("/api/v1")
	v1.Use(authMiddleware(cfg.APIKey))
	{
		v1.POST("/jobs", s.createJob)
		v1.GET("/jobs", s.listJobs)
		v1.GET("/jobs/:id", s.getJob)
		v1.GET("/catalog", s.getCatalog)
		v1.GET("/savings", s.getSavings)
	}

	// Badge endpoint — intentionally unauthenticated for embedding in READMEs.
	r.GET("/badge/:jobId", s.getBadge)

	// Health check — no auth required.
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// Start weekly Slack digest if a webhook is configured.
	if cfg.SlackWebhook != "" {
		go s.weeklyDigestLoop()
	}

	return s, nil
}

// Run starts the HTTP server on the configured port.
func (s *Server) Run(port int) error {
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("runright server listening on %s\n", addr)
	return srv.ListenAndServe()
}

// --- Route handlers ---

type jobPayload struct {
	Summary         types.MetricsSummary  `json:"summary"`
	Recommendations []types.Recommendation `json:"recommendations"`
}

func (s *Server) createJob(c *gin.Context) {
	var p jobPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	summaryJSON, _ := json.Marshal(p.Summary)
	recsJSON, _ := json.Marshal(p.Recommendations)

	// Determine run_id (may be empty for old agents or the seed script).
	var runID *string
	if p.Summary.RunID != "" {
		runID = &p.Summary.RunID
	}

	// Status defaults to "completed" if not set (e.g. seed data).
	status := p.Summary.Status
	if status == "" {
		status = "completed"
	}

	var id string
	var err error

	if runID != nil {
		// Upsert by run_id: heartbeats create/update the record; the final
		// "completed" flush overwrites it. A completed record is never
		// downgraded back to "heartbeat" (WHERE clause guards this).
		err = s.db.QueryRowContext(c.Request.Context(), `
			INSERT INTO jobs (job_id, run_id, start_time, end_time, duration_seconds, summary, recommendations, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (run_id) WHERE run_id IS NOT NULL DO UPDATE SET
				end_time         = EXCLUDED.end_time,
				duration_seconds = EXCLUDED.duration_seconds,
				summary          = EXCLUDED.summary,
				recommendations  = EXCLUDED.recommendations,
				status           = EXCLUDED.status
			WHERE jobs.status != 'completed'
			RETURNING id`,
			p.Summary.JobID, runID,
			p.Summary.StartTime, p.Summary.EndTime, p.Summary.DurationSeconds,
			summaryJSON, recsJSON, status,
		).Scan(&id)
	} else {
		err = s.db.QueryRowContext(c.Request.Context(),
			`INSERT INTO jobs (job_id, start_time, end_time, duration_seconds, summary, recommendations, status)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			p.Summary.JobID,
			p.Summary.StartTime, p.Summary.EndTime, p.Summary.DurationSeconds,
			summaryJSON, recsJSON, status,
		).Scan(&id)
	}

	if err == sql.ErrNoRows {
		// ON CONFLICT matched but WHERE blocked the update (already completed).
		c.JSON(http.StatusOK, gin.H{"status": "already completed"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Regression detection: compare the new job's duration against the rolling
	// average of the previous 10 completed runs for the same job_id. If it is
	// more than 15% slower, annotate the first recommendation with the delta.
	if status == "completed" && p.Summary.DurationSeconds > 0 {
		go s.checkDurationRegression(p.Summary.JobID, p.Summary.DurationSeconds, id)
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// checkDurationRegression computes the rolling avg duration and, if the new run
// is >15% slower, updates the first recommendation's duration_regression_pct field.
func (s *Server) checkDurationRegression(jobID string, newDuration float64, rowID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var avgDuration float64
	err := s.db.QueryRowContext(ctx,
		`SELECT AVG(duration_seconds) FROM (
			SELECT duration_seconds FROM jobs
			WHERE job_id = $1 AND status = 'completed' AND id::text != $2
			ORDER BY created_at DESC LIMIT 10
		) t`, jobID, rowID).Scan(&avgDuration)
	if err != nil || avgDuration <= 0 {
		return
	}
	regressionPct := ((newDuration - avgDuration) / avgDuration) * 100
	if regressionPct < 15 {
		return
	}
	// Fetch the current recommendations and annotate.
	var recsJSON json.RawMessage
	if err := s.db.QueryRowContext(ctx,
		`SELECT recommendations FROM jobs WHERE id::text = $1`, rowID).Scan(&recsJSON); err != nil {
		return
	}
	var recs []types.Recommendation
	if err := json.Unmarshal(recsJSON, &recs); err != nil || len(recs) == 0 {
		return
	}
	pct := regressionPct
	recs[0].DurationRegressionPct = &pct
	updated, err := json.Marshal(recs)
	if err != nil {
		return
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE jobs SET recommendations = $1 WHERE id::text = $2`, updated, rowID)
}

type jobRow struct {
	ID              int             `json:"id"`
	JobID           string          `json:"job_id"`
	StartTime       time.Time       `json:"start_time"`
	EndTime         time.Time       `json:"end_time"`
	DurationSeconds float64         `json:"duration_seconds"`
	Summary         json.RawMessage `json:"summary"`
	Recommendations json.RawMessage `json:"recommendations"`
	Status          string          `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
}

func (s *Server) listJobs(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT id, job_id, start_time, end_time, duration_seconds, summary, recommendations, status, created_at
		 FROM jobs ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var jobs []jobRow
	for rows.Next() {
		var j jobRow
		if err := rows.Scan(&j.ID, &j.JobID, &j.StartTime, &j.EndTime, &j.DurationSeconds,
			&j.Summary, &j.Recommendations, &j.Status, &j.CreatedAt); err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	c.JSON(http.StatusOK, jobs)
}

func (s *Server) getJob(c *gin.Context) {
	id := c.Param("id")
	var j jobRow
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT id, job_id, start_time, end_time, duration_seconds, summary, recommendations, status, created_at
		 FROM jobs WHERE id = $1`, id).
		Scan(&j.ID, &j.JobID, &j.StartTime, &j.EndTime, &j.DurationSeconds,
			&j.Summary, &j.Recommendations, &j.Status, &j.CreatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, j)
}

func (s *Server) getCatalog(c *gin.Context) {
	provider := types.Provider(c.Query("provider"))
	machines := catalog.Query(catalog.QueryOptions{Provider: provider})
	c.JSON(http.StatusOK, machines)
}

// getSavings aggregates potential monthly savings across all completed jobs.
func (s *Server) getSavings(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT recommendations, duration_seconds FROM jobs WHERE status = 'completed' ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var (
		totalJobs      int
		jobsWithSaving int
		totalSavingUSD float64
		totalWastePct  float64
	)
	for rows.Next() {
		var recsJSON json.RawMessage
		var durSec float64
		if err := rows.Scan(&recsJSON, &durSec); err != nil {
			continue
		}
		var recs []types.Recommendation
		if err := json.Unmarshal(recsJSON, &recs); err != nil || len(recs) == 0 {
			continue
		}
		totalJobs++
		best := recs[0]
		if best.CostDeltaPercent < -0.5 { // cheaper recommendation exists
			jobsWithSaving++
			saving := best.CurrentMonthly - best.EstimatedMonthly
			if saving > 0 {
				totalSavingUSD += saving
			}
			totalWastePct += -best.CostDeltaPercent
		}
	}

	avgWastePct := 0.0
	if jobsWithSaving > 0 {
		avgWastePct = totalWastePct / float64(jobsWithSaving)
	}
	c.JSON(http.StatusOK, gin.H{
		"total_jobs":                  totalJobs,
		"jobs_with_savings":           jobsWithSaving,
		"estimated_monthly_savings":   totalSavingUSD,
		"projected_annual_savings":    totalSavingUSD * 12,
		"avg_waste_percent":           avgWastePct,
	})
}

// getBadge returns a shields.io-style SVG badge showing the latest tier for a job.
// This endpoint is intentionally unauthenticated so teams can embed it in READMEs.
func (s *Server) getBadge(c *gin.Context) {
	jobId := c.Param("jobId")
	var recsJSON json.RawMessage
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT recommendations FROM jobs WHERE job_id = $1 AND status = 'completed'
		 ORDER BY created_at DESC LIMIT 1`, jobId).Scan(&recsJSON)
	if err == sql.ErrNoRows {
		c.Header("Content-Type", "image/svg+xml")
		c.String(http.StatusOK, badgeSVG("runright", "unknown", "#9f9f9f"))
		return
	}
	if err != nil {
		c.Header("Content-Type", "image/svg+xml")
		c.String(http.StatusInternalServerError, badgeSVG("runright", "error", "#e05d44"))
		return
	}
	var recs []types.Recommendation
	if err := json.Unmarshal(recsJSON, &recs); err != nil || len(recs) == 0 {
		c.Header("Content-Type", "image/svg+xml")
		c.String(http.StatusOK, badgeSVG("runright", "unknown", "#9f9f9f"))
		return
	}
	tier := string(recs[0].Tier)
	color := map[string]string{
		"right-sized":    "#44cc11",
		"cheaper-option": "#dfb317",
		"more-headroom":  "#e05d44",
	}[tier]
	if color == "" {
		color = "#9f9f9f"
	}
	c.Header("Content-Type", "image/svg+xml")
	c.Header("Cache-Control", "no-cache, max-age=3600")
	c.String(http.StatusOK, badgeSVG("runright", tier, color))
}

// badgeSVG generates a minimal shields.io-compatible badge SVG.
func badgeSVG(label, message, color string) string {
	lw := len(label)*6 + 10
	mw := len(message)*6 + 10
	tw := lw + mw
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">`+
		`<linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>`+
		`<rect rx="3" width="%d" height="20" fill="#555"/>`+
		`<rect rx="3" x="%d" width="%d" height="20" fill="%s"/>`+
		`<rect x="%d" width="4" height="20" fill="%s"/>`+
		`<rect rx="3" width="%d" height="20" fill="url(#s)"/>`+
		`<g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">`+
		`<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>`+
		`<text x="%d" y="14">%s</text>`+
		`<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>`+
		`<text x="%d" y="14">%s</text>`+
		`</g></svg>`,
		tw, tw, lw, mw, color, lw, color, tw,
		lw/2+1, label, lw/2, label,
		lw+mw/2+1, message, lw+mw/2, message,
	)
}

// weeklyDigestLoop fires every Monday at 09:00 UTC and posts a savings summary to Slack.
func (s *Server) weeklyDigestLoop() {
	for {
		now := time.Now().UTC()
		// Next Monday 09:00 UTC.
		daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
		if daysUntilMonday == 0 && now.Hour() >= 9 {
			daysUntilMonday = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 9, 0, 0, 0, time.UTC)
		time.Sleep(time.Until(next))
		s.postWeeklyDigest()
	}
}

func (s *Server) postWeeklyDigest() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT recommendations FROM jobs
		 WHERE status = 'completed' AND created_at >= NOW() - INTERVAL '7 days'`)
	if err != nil {
		return
	}
	defer rows.Close()

	var totalJobs int
	var savingJobs int
	var totalSaving float64
	for rows.Next() {
		var recsJSON json.RawMessage
		if err := rows.Scan(&recsJSON); err != nil {
			continue
		}
		var recs []types.Recommendation
		if err := json.Unmarshal(recsJSON, &recs); err != nil || len(recs) == 0 {
			continue
		}
		totalJobs++
		best := recs[0]
		if best.CostDeltaPercent < -0.5 {
			savingJobs++
			if saving := best.CurrentMonthly - best.EstimatedMonthly; saving > 0 {
				totalSaving += saving
			}
		}
	}
	if totalJobs == 0 {
		return
	}

	msg := fmt.Sprintf(
		":moneybag: *RunRight weekly digest*\n"+
			"• %d CI jobs ran this week\n"+
			"• %d are over-provisioned\n"+
			"• Potential savings: *$%.2f/month* ($%.0f/year)",
		totalJobs, savingJobs, totalSaving, totalSaving*12,
	)
	body, _ := json.Marshal(map[string]string{"text": msg})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.slackWebhook, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// --- Auth handlers + middleware ---

const sessionCookie = "runright_session"

// authLogin validates the API key and issues an HttpOnly session cookie.
func authLogin(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if apiKey == "" {
			// Auth not configured — nothing to log in to.
			c.JSON(http.StatusOK, gin.H{"status": "no auth configured"})
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_key required"})
			return
		}
		if body.APIKey != apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api_key"})
			return
		}
		// Set HttpOnly, SameSite=Strict cookie. JS cannot read this.
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(sessionCookie, apiKey, 86400*30, "/", "", false, true /* httpOnly */)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// authLogout clears the session cookie.
func authLogout() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(sessionCookie, "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"status": "logged out"})
	}
}

// authMiddleware accepts either the HttpOnly cookie or a Bearer token (for CLI/curl).
func authMiddleware(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no API key is configured, skip auth (dev mode).
		if apiKey == "" {
			c.Next()
			return
		}
		// Check HttpOnly cookie first (dashboard).
		if cookie, err := c.Cookie(sessionCookie); err == nil && cookie == apiKey {
			c.Next()
			return
		}
		// Fall back to Bearer token (CLI / programmatic access).
		if c.GetHeader("Authorization") == "Bearer "+apiKey {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
}

// --- Migrations ---

func migrate(db *sql.DB) error {
	stmts := []string{
		// Core table (idempotent).
		`CREATE TABLE IF NOT EXISTS jobs (
			id               SERIAL PRIMARY KEY,
			job_id           TEXT NOT NULL,
			run_id           TEXT,
			start_time       TIMESTAMPTZ NOT NULL,
			end_time         TIMESTAMPTZ NOT NULL,
			duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
			summary          JSONB NOT NULL DEFAULT '{}',
			recommendations  JSONB NOT NULL DEFAULT '[]',
			status           TEXT NOT NULL DEFAULT 'completed',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		// Columns added in later migrations (safe to re-run on fresh DBs too).
		`ALTER TABLE jobs ADD COLUMN IF NOT EXISTS run_id TEXT`,
		`ALTER TABLE jobs ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'completed'`,
		// Indexes (must come after columns exist).
		`CREATE INDEX IF NOT EXISTS idx_jobs_job_id     ON jobs(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_run_id ON jobs(run_id) WHERE run_id IS NOT NULL`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

// ConfigFromEnv reads server config from environment variables.
func ConfigFromEnv() Config {
	port := 8080
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://runright:runright@localhost:5435/runright?sslmode=disable"
	}
	return Config{
		Port:         port,
		DSN:          dsn,
		APIKey:       os.Getenv("RUNRIGHT_API_KEY"),
		SlackWebhook: os.Getenv("RUNRIGHT_SLACK_WEBHOOK"),
	}
}
