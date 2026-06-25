#!/usr/bin/env bash
# Generate machine sizing recommendations from collected metrics.
# Env vars:
#   RUNRIGHT_OUTPUT_DIR       - directory containing metrics files
#   RUNRIGHT_JOB_ID           - job identifier used in PR comment marker
#   RUNRIGHT_MAX_COST_PER_HOUR - budget guard; non-empty = fail if detected machine exceeds this price
#   RUNRIGHT_DRY_RUN          - "true" = skip writing PR comment body
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

# ── Budget guard ──────────────────────────────────────────────────────────────
if [[ -n "${RUNRIGHT_HTTP_URL:-}" ]]; then
  POLICY_PAYLOAD=$(python3 -c "import json; d=json.load(open('$SUMMARY')); print(json.dumps({'repository': d.get('repository', ''), 'job_id': d.get('job_id', ''), 'detected_price_per_hour': (d.get('detected_machine') or {}).get('on_demand_price_per_hour', 0)}))" 2>/dev/null || echo "")
  if [[ -n "$POLICY_PAYLOAD" ]]; then
    AUTH_HEADER=()
    if [[ -n "${RUNRIGHT_API_KEY:-}" ]]; then
      AUTH_HEADER=(-H "Authorization: Bearer ${RUNRIGHT_API_KEY}")
    fi
    POLICY_RESULT=$(curl -fsS -X POST "${RUNRIGHT_HTTP_URL}/api/v1/policies/evaluate" \
      -H 'Content-Type: application/json' "${AUTH_HEADER[@]}" \
      -d "$POLICY_PAYLOAD" 2>/dev/null || echo "")
    if [[ -n "$POLICY_RESULT" ]]; then
      VIOLATED=$(echo "$POLICY_RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print('yes' if d.get('violated') else 'no')" 2>/dev/null || echo "no")
      if [[ "$VIOLATED" == "yes" ]]; then
        SCHEMA=$(echo "$POLICY_RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); p=d.get('matched_policy') or {}; scope='global' if p.get('repository','')=='' and p.get('job_id','')=='' else ('repository' if p.get('job_id','')=='' else 'job'); print(scope)" 2>/dev/null || echo "policy")
        THRESHOLD=$(echo "$POLICY_RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('effective_max_cost_per_hour', 0))" 2>/dev/null || echo "0")
        DETECTED_PRICE=$(echo "$POLICY_RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('detected_price_per_hour', 0))" 2>/dev/null || echo "0")
        echo "::error::RunRight policy violation (${SCHEMA} scope): detected machine costs ${DETECTED_PRICE}/hr which exceeds the allowed maximum of ${THRESHOLD}/hr."
        exit 1
      fi
    fi
  fi
fi

if [[ -n "${RUNRIGHT_MAX_COST_PER_HOUR:-}" ]]; then
  DETECTED_PRICE=$(python3 -c \
    "import sys,json; d=json.load(open('$SUMMARY')); m=d.get('detected_machine'); print(m['on_demand_price_per_hour'] if m else 0)" \
    2>/dev/null || echo "0")
  OVER_BUDGET=$(python3 -c \
    "print('yes' if ${DETECTED_PRICE} > ${RUNRIGHT_MAX_COST_PER_HOUR} else 'no')" \
    2>/dev/null || echo "no")
  if [[ "$OVER_BUDGET" == "yes" ]]; then
    DETECTED_ID=$(python3 -c \
      "import sys,json; d=json.load(open('$SUMMARY')); m=d.get('detected_machine'); print(m['id'] if m else 'unknown')" \
      2>/dev/null || echo "unknown")
    echo "::error::RunRight budget guard: detected machine '$DETECTED_ID' costs \$${DETECTED_PRICE}/hr which exceeds the allowed maximum of \$${RUNRIGHT_MAX_COST_PER_HOUR}/hr. Update your runner configuration to use a smaller machine type."
    exit 1
  fi
fi

# ── Generate recommendations as JSON ─────────────────────────────────────────
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

# ── Annual savings projection ─────────────────────────────────────────────────
ANNUAL_SAVINGS=$(echo "$RESULT" | python3 -c "
import sys, json
recs = json.load(sys.stdin)
if not recs:
    print('')
else:
    best = next((r for r in recs if r.get('cost_delta_percent', 0) < -0.5), None)
    if best:
        monthly = best.get('current_monthly_usd', 0) - best.get('estimated_monthly_usd', 0)
        annual  = monthly * 12
        print(f'~\${annual:.0f}/yr (~\${monthly:.2f}/mo) by switching to {best[\"machine\"][\"id\"]}')
    else:
        print('')
" 2>/dev/null || echo "")

# ── Print recommendation table to CI logs ────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════╗"
echo "║  RunRight — Machine Sizing Recommendation            ║"
echo "╚══════════════════════════════════════════════════════╝"
runright recommend --metrics "$SUMMARY" --format table $PROVIDER_FLAG 2>/dev/null || true
if [[ -n "$ANNUAL_SAVINGS" ]]; then
  echo ""
  echo "  💡 Projected annual savings: $ANNUAL_SAVINGS"
fi
echo ""

# ── Write markdown to the Actions Step Summary tab ───────────────────────────
runright recommend --metrics "$SUMMARY" --format markdown $PROVIDER_FLAG >> "$GITHUB_STEP_SUMMARY" 2>/dev/null || true
if [[ -n "$ANNUAL_SAVINGS" ]]; then
  echo "" >> "$GITHUB_STEP_SUMMARY"
  echo "> 💡 **Projected annual savings:** $ANNUAL_SAVINGS" >> "$GITHUB_STEP_SUMMARY"
fi

# ── Build PR comment body (skipped in dry-run mode) ──────────────────────────
if [[ "${RUNRIGHT_DRY_RUN:-false}" != "true" ]]; then
  MARKER="<!-- runright:${RUNRIGHT_JOB_ID} -->"
  {
    echo "$MARKER"
    echo ""
    echo "### ⚡ RunRight — \`${RUNRIGHT_JOB_ID}\`"
    echo ""
    runright recommend --metrics "$SUMMARY" --format markdown $PROVIDER_FLAG 2>/dev/null || true
    if [[ -n "$ANNUAL_SAVINGS" ]]; then
      echo ""
      echo "> 💡 **Projected annual savings:** $ANNUAL_SAVINGS"
    fi
    echo ""
    echo "<sub>Powered by [RunRight](https://github.com/sgbudje/runright) · [Run #${GITHUB_RUN_NUMBER}](${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID})</sub>"
  } > /tmp/runright-pr-comment.md
fi

# ── Save evaluated output-dir so the upload-artifact step resolves the same path ──
echo "RUNRIGHT_OUTPUT_DIR=${RUNRIGHT_OUTPUT_DIR}" >> "$GITHUB_ENV"


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
