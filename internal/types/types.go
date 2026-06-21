package types

import "time"

// Provider represents a cloud provider.
type Provider string

const (
	ProviderAWS    Provider = "aws"
	ProviderGCP    Provider = "gcp"
	ProviderGitHub Provider = "github"
)

// MachineType describes a cloud provider instance type with its specs and pricing.
type MachineType struct {
	ID                   string   `json:"id"`
	Provider             Provider `json:"provider"`
	Family               string   `json:"family"`
	Series               string   `json:"series"`
	VCPUs                int      `json:"vcpus"`
	MemoryGiB            float64  `json:"memory_gib"`
	NetworkGbps          float64  `json:"network_gbps"`
	StorageType          string   `json:"storage_type"`
	Architecture         string   `json:"architecture"`
	OnDemandPricePerHour float64  `json:"on_demand_price_per_hour"`
	Tags                 []string `json:"tags"`
}

// MetricSnapshot is a single point-in-time reading from the metrics agent.
type MetricSnapshot struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemUsedGiB     float64   `json:"mem_used_gib"`
	MemTotalGiB    float64   `json:"mem_total_gib"`
	ProcessCount   int       `json:"process_count"`
	ThreadCount    int       `json:"thread_count"`
	DiskReadMBs    float64   `json:"disk_read_mbs"`
	DiskWriteMBs   float64   `json:"disk_write_mbs"`
	NetRxMBs       float64   `json:"net_rx_mbs"`
	NetTxMBs       float64   `json:"net_tx_mbs"`
}

// MetricsSummary aggregates a collection of snapshots into peak/avg/p95 values.
type MetricsSummary struct {
	JobID           string    `json:"job_id"`
	RunID           string    `json:"run_id,omitempty"`           // unique per agent invocation; used for upserts
	Status          string    `json:"status,omitempty"`           // "heartbeat" | "completed"
	CIPlatform      string    `json:"ci_platform,omitempty"`      // "github" | "jenkins" | "gitlab" | "circleci" | "bitbucket" | "local"
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`

	// Detected machine at the time of the run.
	DetectedMachine *MachineType `json:"detected_machine,omitempty"`

	CPUPercentPeak float64 `json:"cpu_percent_peak"`
	CPUPercentAvg  float64 `json:"cpu_percent_avg"`
	CPUPercentP95  float64 `json:"cpu_percent_p95"`

	MemUsedGiBPeak float64 `json:"mem_used_gib_peak"`
	MemUsedGiBAvg  float64 `json:"mem_used_gib_avg"`
	MemUsedGiBP95  float64 `json:"mem_used_gib_p95"`
	MemTotalGiB    float64 `json:"mem_total_gib"`

	ProcessCountPeak int `json:"process_count_peak"`
	ThreadCountPeak  int `json:"thread_count_peak"`

	DiskReadMBsPeak  float64 `json:"disk_read_mbs_peak"`
	DiskWriteMBsPeak float64 `json:"disk_write_mbs_peak"`
	NetRxMBsPeak     float64 `json:"net_rx_mbs_peak"`
	NetTxMBsPeak     float64 `json:"net_tx_mbs_peak"`

	SampleCount int `json:"sample_count"`
}

// RecommendationTier describes how a recommended machine compares to current usage.
type RecommendationTier string

const (
	TierRightSized   RecommendationTier = "right-sized"
	TierCheaper      RecommendationTier = "cheaper-option"
	TierMoreHeadroom RecommendationTier = "more-headroom"
)

// KubernetesResources contains suggested resource requests and limits for
// a Kubernetes runner pod based on observed p95 usage.
type KubernetesResources struct {
	CPURequest    string `json:"cpu_request"`    // e.g. "1200m"
	CPULimit      string `json:"cpu_limit"`      // e.g. "2000m"
	MemoryRequest string `json:"memory_request"` // e.g. "2Gi"
	MemoryLimit   string `json:"memory_limit"`   // e.g. "3Gi"
}

// Recommendation is a single machine type suggestion with cost and reasoning.
type Recommendation struct {
	Machine              MachineType          `json:"machine"`
	Tier                 RecommendationTier   `json:"tier"`
	EstimatedMonthly     float64              `json:"estimated_monthly_usd"`
	CurrentMonthly       float64              `json:"current_monthly_usd"`
	CostDeltaPercent     float64              `json:"cost_delta_percent"`
	RequiredVCPUs        int                  `json:"required_vcpus"`
	RequiredMemoryGiB    float64              `json:"required_memory_gib"`
	Reasoning            string               `json:"reasoning"`
	KubernetesResources  *KubernetesResources `json:"kubernetes_resources,omitempty"`
}
