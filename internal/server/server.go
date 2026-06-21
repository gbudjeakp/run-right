package server

import (
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
	router *gin.Engine
	db     *sql.DB
}

// Config holds server configuration.
type Config struct {
	Port    int
	DSN     string // Postgres DSN, e.g. postgres://user:pass@localhost/scalecidb
	APIKey  string
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

	s := &Server{router: r, db: db}

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
	}

	// Health check — no auth required.
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

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
	c.JSON(http.StatusCreated, gin.H{"id": id})
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
		Port:   port,
		DSN:    dsn,
		APIKey: os.Getenv("RUNRIGHT_API_KEY"),
	}
}
