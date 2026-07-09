package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CIPlatform represents detected CI/CD platform.
type CIPlatform string

const (
	CIGitHub    CIPlatform = "github"
	CIGitLab    CIPlatform = "gitlab"
	CIJenkins   CIPlatform = "jenkins"
	CICircleCI  CIPlatform = "circleci"
	CIBitbucket CIPlatform = "bitbucket"
	CIAzure     CIPlatform = "azure"
	CITravis    CIPlatform = "travis"
	CILocal     CIPlatform = "local"
)

// CIDetectionResult contains info about the detected CI environment.
type CIDetectionResult struct {
	Platform   CIPlatform `json:"platform"`
	Confidence string     `json:"confidence"` // high, medium, low
	Repository string     `json:"repository,omitempty"`
	Branch     string     `json:"branch,omitempty"`
	JobID      string     `json:"job_id,omitempty"`
	BuildURL   string     `json:"build_url,omitempty"`
	RunnerType string     `json:"runner_type,omitempty"` // self-hosted, hosted, etc.
}

// DetectCI detects the current CI/CD platform from environment variables.
func DetectCI() CIDetectionResult {
	result := CIDetectionResult{
		Platform:   CILocal,
		Confidence: "high",
	}

	// GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		result.Platform = CIGitHub
		result.Repository = os.Getenv("GITHUB_REPOSITORY")
		result.Branch = os.Getenv("GITHUB_REF_NAME")
		result.JobID = os.Getenv("GITHUB_JOB")
		result.BuildURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s",
			result.Repository, os.Getenv("GITHUB_RUN_ID"))

		// Detect runner type
		if os.Getenv("RUNNER_ENVIRONMENT") == "github-hosted" {
			result.RunnerType = "github-hosted"
		} else {
			result.RunnerType = "self-hosted"
		}
		return result
	}

	// GitLab CI
	if os.Getenv("GITLAB_CI") == "true" {
		result.Platform = CIGitLab
		result.Repository = os.Getenv("CI_PROJECT_PATH")
		result.Branch = os.Getenv("CI_COMMIT_REF_NAME")
		result.JobID = os.Getenv("CI_JOB_ID")
		result.BuildURL = os.Getenv("CI_JOB_URL")
		return result
	}

	// Jenkins
	if os.Getenv("JENKINS_URL") != "" {
		result.Platform = CIJenkins
		result.JobID = os.Getenv("BUILD_ID")
		result.BuildURL = os.Getenv("BUILD_URL")
		result.Branch = os.Getenv("GIT_BRANCH")
		return result
	}

	// CircleCI
	if os.Getenv("CIRCLECI") == "true" {
		result.Platform = CICircleCI
		result.Repository = fmt.Sprintf("%s/%s",
			os.Getenv("CIRCLE_PROJECT_USERNAME"),
			os.Getenv("CIRCLE_PROJECT_REPONAME"))
		result.Branch = os.Getenv("CIRCLE_BRANCH")
		result.JobID = os.Getenv("CIRCLE_BUILD_NUM")
		result.BuildURL = os.Getenv("CIRCLE_BUILD_URL")
		return result
	}

	// Bitbucket Pipelines
	if os.Getenv("BITBUCKET_PIPELINE_UUID") != "" {
		result.Platform = CIBitbucket
		result.Repository = os.Getenv("BITBUCKET_REPO_FULL_NAME")
		result.Branch = os.Getenv("BITBUCKET_BRANCH")
		result.JobID = os.Getenv("BITBUCKET_BUILD_NUMBER")
		return result
	}

	// Azure Pipelines
	if os.Getenv("TF_BUILD") == "True" {
		result.Platform = CIAzure
		result.Repository = os.Getenv("BUILD_REPOSITORY_NAME")
		result.Branch = os.Getenv("BUILD_SOURCEBRANCHNAME")
		result.JobID = os.Getenv("BUILD_BUILDID")
		result.BuildURL = fmt.Sprintf("%s%s/_build/results?buildId=%s",
			os.Getenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI"),
			os.Getenv("SYSTEM_TEAMPROJECT"),
			os.Getenv("BUILD_BUILDID"))
		return result
	}

	// Travis CI
	if os.Getenv("TRAVIS") == "true" {
		result.Platform = CITravis
		result.Repository = os.Getenv("TRAVIS_REPO_SLUG")
		result.Branch = os.Getenv("TRAVIS_BRANCH")
		result.JobID = os.Getenv("TRAVIS_JOB_ID")
		result.BuildURL = os.Getenv("TRAVIS_BUILD_WEB_URL")
		return result
	}

	// Generic CI detection
	if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		result.Confidence = "low"
		// Try to detect from common variables
		if repo := os.Getenv("REPO_NAME"); repo != "" {
			result.Repository = repo
		}
	}

	return result
}

// InitConfig represents the generated configuration.
type InitConfig struct {
	Platform     CIPlatform
	OutputDir    string
	Interval     string
	Exports      []string
	HistoryPath  string
	AutoUpdate   bool
	GitIgnore    bool
}

// DefaultInitConfig returns sensible defaults based on detected platform.
func DefaultInitConfig(detection CIDetectionResult) InitConfig {
	cfg := InitConfig{
		Platform:    detection.Platform,
		OutputDir:   ".runright",
		Interval:    "5s",
		Exports:     []string{"file"},
		HistoryPath: "~/.runright/history.db",
		AutoUpdate:  true,
		GitIgnore:   true,
	}

	// Platform-specific defaults
	switch detection.Platform {
	case CIGitHub:
		cfg.Exports = []string{"file"}
	case CIGitLab:
		cfg.Exports = []string{"file"}
	case CIJenkins:
		cfg.OutputDir = "runright-metrics"
	}

	return cfg
}

// GenerateConfigFile creates a .runright.yaml config file.
func GenerateConfigFile(cfg InitConfig) (string, error) {
	content := fmt.Sprintf(`# RunRight Configuration
# Generated for: %s

# Monitoring settings
interval: %s
output_dir: %s

# Export backends (comma-separated: file, otlp, prometheus, http)
export: %s

# Local history database (for 'runright history' command)
history_path: %s

# Auto-update settings
auto_update:
  enabled: %t
  channel: stable

# Policy enforcement (optional)
# fail_if_oversized: 50  # Exit non-zero if wasting >50%%
# max_cost_per_hour: 0.50

# Machine pool constraints (optional)
# allowed_series: [c7g, m7i, n2, e2]
# allowed_families: [c, m, r]
`, cfg.Platform, cfg.Interval, cfg.OutputDir, 
   strings.Join(cfg.Exports, ","), cfg.HistoryPath, cfg.AutoUpdate)

	return content, nil
}

// GenerateGitHubAction creates a GitHub Actions workflow snippet.
func GenerateGitHubAction(cfg InitConfig) string {
	return `# Add to your workflow:
- uses: gbudjeakp/run-right@v1
  with:
    run: make build  # Your build command
    # Optional settings:
    # export: otlp
    # pr-comment: true
    # upload-artifact: true
  # env:
  #   OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_ENDPOINT }}
`
}

// GenerateGitLabCI creates a GitLab CI snippet.
func GenerateGitLabCI(cfg InitConfig) string {
	return `# Add to your .gitlab-ci.yml:
.runright:
  before_script:
    - curl -fsSL https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64.tar.gz | tar -xz
    - ./runright monitor --duration 0 &
  after_script:
    - pkill runright || true
    - ./runright recommend --metrics .runright/metrics-summary.json

build:
  extends: .runright
  script:
    - make build
`
}

// GenerateJenkinsfile creates a Jenkins pipeline snippet.
func GenerateJenkinsfile(cfg InitConfig) string {
	return `// Add to your Jenkinsfile:
pipeline {
    agent any
    stages {
        stage('Build') {
            steps {
                sh '''
                    curl -fsSL https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64.tar.gz | tar -xz
                    ./runright monitor --duration 0 &
                    RUNRIGHT_PID=$!
                    
                    make build
                    
                    kill $RUNRIGHT_PID || true
                    ./runright recommend --metrics .runright/metrics-summary.json
                '''
            }
        }
    }
    post {
        always {
            archiveArtifacts artifacts: '.runright/**', allowEmptyArchive: true
        }
    }
}
`
}

// GenerateCircleCI creates a CircleCI config snippet.
func GenerateCircleCI(cfg InitConfig) string {
	return `# Add to your .circleci/config.yml:
commands:
  with-runright:
    parameters:
      command:
        type: string
    steps:
      - run:
          name: Install RunRight
          command: |
            curl -fsSL https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64.tar.gz | tar -xz
      - run:
          name: Run with monitoring
          command: |
            ./runright monitor --duration 0 &
            << parameters.command >>
            pkill runright || true
      - run:
          name: Show recommendations
          command: ./runright recommend --metrics .runright/metrics-summary.json

jobs:
  build:
    docker:
      - image: cimg/base:current
    steps:
      - checkout
      - with-runright:
          command: make build
`
}

// GenerateAzurePipelines creates an Azure Pipelines snippet.
func GenerateAzurePipelines(cfg InitConfig) string {
	return `# Add to your azure-pipelines.yml:
steps:
  - script: |
      curl -fsSL https://github.com/gbudjeakp/run-right/releases/latest/download/runright_linux_amd64.tar.gz | tar -xz
      ./runright monitor --duration 0 &
      RUNRIGHT_PID=$!
      
      make build
      
      kill $RUNRIGHT_PID || true
      ./runright recommend --metrics .runright/metrics-summary.json
    displayName: 'Build with RunRight'
    
  - task: PublishBuildArtifacts@1
    inputs:
      pathToPublish: '.runright'
      artifactName: 'runright-metrics'
`
}

// WriteGitIgnore appends runright output to .gitignore.
func WriteGitIgnore(outputDir string) error {
	gitignorePath := ".gitignore"
	entry := fmt.Sprintf("\n# RunRight metrics\n%s/\n", outputDir)

	// Check if already present
	existing, _ := os.ReadFile(gitignorePath)
	if strings.Contains(string(existing), outputDir+"/") {
		return nil // Already present
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

// GenerateCISnippet returns the appropriate CI config snippet for the platform.
func GenerateCISnippet(platform CIPlatform, cfg InitConfig) string {
	switch platform {
	case CIGitHub:
		return GenerateGitHubAction(cfg)
	case CIGitLab:
		return GenerateGitLabCI(cfg)
	case CIJenkins:
		return GenerateJenkinsfile(cfg)
	case CICircleCI:
		return GenerateCircleCI(cfg)
	case CIAzure:
		return GenerateAzurePipelines(cfg)
	default:
		return GenerateGitHubAction(cfg) // Default to GitHub
	}
}

// DetectProjectType tries to detect the project type from files.
type ProjectType struct {
	Language   string
	BuildTool  string
	TestCmd    string
	BuildCmd   string
}

// DetectProjectType scans the current directory for project files.
func DetectProjectType() ProjectType {
	pt := ProjectType{}

	// Go
	if _, err := os.Stat("go.mod"); err == nil {
		pt.Language = "go"
		pt.BuildTool = "go"
		pt.BuildCmd = "go build ./..."
		pt.TestCmd = "go test ./..."
		return pt
	}

	// Node.js
	if _, err := os.Stat("package.json"); err == nil {
		pt.Language = "javascript"
		pt.BuildTool = "npm"
		pt.BuildCmd = "npm run build"
		pt.TestCmd = "npm test"
		if _, err := os.Stat("yarn.lock"); err == nil {
			pt.BuildTool = "yarn"
			pt.BuildCmd = "yarn build"
			pt.TestCmd = "yarn test"
		}
		if _, err := os.Stat("pnpm-lock.yaml"); err == nil {
			pt.BuildTool = "pnpm"
			pt.BuildCmd = "pnpm build"
			pt.TestCmd = "pnpm test"
		}
		return pt
	}

	// Python
	if _, err := os.Stat("pyproject.toml"); err == nil {
		pt.Language = "python"
		pt.BuildTool = "poetry"
		pt.BuildCmd = "poetry build"
		pt.TestCmd = "poetry run pytest"
		return pt
	}
	if _, err := os.Stat("setup.py"); err == nil {
		pt.Language = "python"
		pt.BuildTool = "pip"
		pt.BuildCmd = "pip install -e ."
		pt.TestCmd = "pytest"
		return pt
	}

	// Rust
	if _, err := os.Stat("Cargo.toml"); err == nil {
		pt.Language = "rust"
		pt.BuildTool = "cargo"
		pt.BuildCmd = "cargo build --release"
		pt.TestCmd = "cargo test"
		return pt
	}

	// Java/Maven
	if _, err := os.Stat("pom.xml"); err == nil {
		pt.Language = "java"
		pt.BuildTool = "maven"
		pt.BuildCmd = "mvn package"
		pt.TestCmd = "mvn test"
		return pt
	}

	// Java/Gradle
	if _, err := os.Stat("build.gradle"); err == nil {
		pt.Language = "java"
		pt.BuildTool = "gradle"
		pt.BuildCmd = "./gradlew build"
		pt.TestCmd = "./gradlew test"
		return pt
	}

	// Makefile
	if _, err := os.Stat("Makefile"); err == nil {
		pt.BuildTool = "make"
		pt.BuildCmd = "make build"
		pt.TestCmd = "make test"
		return pt
	}

	// Docker
	if _, err := os.Stat("Dockerfile"); err == nil {
		pt.BuildTool = "docker"
		pt.BuildCmd = "docker build ."
		return pt
	}

	return pt
}

// EnsureOutputDir creates the output directory if needed.
func EnsureOutputDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// WriteConfigFile writes the config to disk.
func WriteConfigFile(path string, content string) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0644)
}
