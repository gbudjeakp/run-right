#!/usr/bin/env bash
# Generate machine sizing recommendations from collected metrics.
# Env vars:
#   RUNRIGHT_OUTPUT_DIR  - directory containing metrics files
#   RUNRIGHT_JOB_ID      - job identifier used in PR comment marker
# GitHub-provided (auto):
#   GITHUB_OUTPUT, GITHUB_STEP_SUMMARY, GITHUB_ENV
#   GITHUB_RUN_NUMBER, GITHUB_SERVER_URL, GITHUB_REPOSITORY, GITHUB_RUN_ID
#   RUNRIGHT_UNEXPECTED_EXIT  (set by monitor-stop.sh when monitor died early)
set -euo pipefail

SUMMARY="${RUNRIGHT_OUTPUT_DIR}/metrics-summary.json"

if [[ ! -f "$SUMMARY" ]]; then
  HEARTBEAT="${RUNRIGHT_OUTPUT_DIR}/metrics-heartbeat.json"
  if [[ -f "$HEARTBEAT" ]] && [[ "${RUNRIGHT_UNEXPECTED_EXIT:-0}" == "1" ]]; then
    echo "::warning::RunRight: Job was interrupted before completion. Using partial metrics from last heartbeat checkpoint for best-effort recommendation."
    SUMMARY="$HEARTBEAT"
  elif [[ "${RUNRIGHT_UNEXPECTED_EXIT:-0}" == "1" ]]; then
    # Killed before the first heartbeat (< 30 s) — report current machine specs.
    VCPUS=$(nproc 2>/dev/null || echo "?")
    MEM_KB=$(grep MemTotal /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "0")
    MEM_GIB=$(python3 -c "print(round(${MEM_KB:-0}/1048576,1))" 2>/dev/null || echo "?")
    echo "::error::RunRight: Monitor was killed before collecting any metrics (likely OOM or force-stop)."
    echo "::notice::Detected runner: ${VCPUS} vCPU / ${MEM_GIB} GiB RAM. The job exceeded this machine's capacity. Consider upgrading to a larger instance type."
    exit 0
  else
    echo "No metrics-summary.json found at $SUMMARY, skipping recommendations."
    exit 0
  fi
fi

# Generate recommendations as JSON.
PROVIDER_FLAG=${RUNRIGHT_PROVIDER:+--provider "$RUNRIGHT_PROVIDER"}
RESULT=$(runright recommend --metrics "$SUMMARY" --format json $PROVIDER_FLAG 2>/dev/null || echo "[]")
echo "result=$(echo "$RESULT" | tr -d '\n')" >> "$GITHUB_OUTPUT"

# Extract top recommendation.
TOP=$(echo "$RESULT" | python3 -c \
  "import sys,json; d=json.load(sys.stdin); print(d[0]['machine']['id'] if d else 'unknown')" \
  2>/dev/null || echo "unknown")
echo "suggested_machine=$TOP" >> "$GITHUB_OUTPUT"

# Extract detected machine.
DETECTED=$(python3 -c \
  "import sys,json; d=json.load(open('$SUMMARY')); m=d.get('detected_machine'); print(m['id'] if m else 'unknown')" \
  2>/dev/null || echo "unknown")
echo "detected_machine=$DETECTED" >> "$GITHUB_OUTPUT"

# Print recommendation table to CI logs.
echo ""
echo "╔══════════════════════════════════════════════════════╗"
echo "║  RunRight — Machine Sizing Recommendation            ║"
echo "╚══════════════════════════════════════════════════════╝"
runright recommend --metrics "$SUMMARY" --format table $PROVIDER_FLAG 2>/dev/null || true
echo ""

# Write markdown to the Actions Step Summary tab.
runright recommend --metrics "$SUMMARY" --format markdown $PROVIDER_FLAG >> "$GITHUB_STEP_SUMMARY" 2>/dev/null || true

# Build PR comment body.
MARKER="<!-- runright:${RUNRIGHT_JOB_ID} -->"
{
  echo "$MARKER"
  echo ""
  echo "### ⚡ RunRight — \`${RUNRIGHT_JOB_ID}\`"
  echo ""
  runright recommend --metrics "$SUMMARY" --format markdown $PROVIDER_FLAG 2>/dev/null || true
  echo ""
  echo "<sub>Powered by [RunRight](https://github.com/sgbudje/runright) · [Run #${GITHUB_RUN_NUMBER}](${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID})</sub>"
} > /tmp/runright-pr-comment.md

# Save evaluated output-dir so the upload-artifact step resolves the same path.
echo "RUNRIGHT_OUTPUT_DIR=${RUNRIGHT_OUTPUT_DIR}" >> "$GITHUB_ENV"
