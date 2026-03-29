#!/usr/bin/env bash
#
# benchmark-models.sh - Run standardized prompts across different Ollama models
# and collect performance metrics (tokens/s, TTFT, duration).
#
# Usage:
#   ./scripts/benchmark-models.sh                    # Run all benchmarks (48GB tier)
#   ./scripts/benchmark-models.sh diagnose           # Run only diagnose benchmarks
#   ./scripts/benchmark-models.sh generate           # Run only generate benchmarks
#   ./scripts/benchmark-models.sh ask                # Run only ask benchmarks
#   ./scripts/benchmark-models.sh all 128gb          # Run all benchmarks (128GB tier)
#   BENCH_TIER=128gb ./scripts/benchmark-models.sh   # Alt: set tier via env var
#
# Prerequisites:
#   - Ollama running locally (http://localhost:11434)
#   - tfai binary built (make build)
#   - Models pulled (ollama pull <model>)
#   - jq installed (brew install jq)
#
# Results are written to benchmark-results/<timestamp>/ as JSON files.

set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────────────

TFAI_BIN="${TFAI_BIN:-./bin/tfai}"
OLLAMA_HOST="${OLLAMA_HOST:-http://localhost:11434}"
RESULTS_DIR="benchmark-results/$(date +%Y%m%d-%H%M%S)"
FILTER="${1:-all}"

# Hardware tier: "48gb" or "128gb" (set via env or second arg)
TIER="${BENCH_TIER:-${2:-48gb}}"

# Backend: "ollama" or "mlx" (set via env or third arg)
BACKEND="${BENCH_BACKEND:-${3:-ollama}}"

# MLX server ports (customize if needed)
MLX_PORT="${MLX_PORT:-8100}"

# ── MLX Model Mapping ───────────────────────────────────────────────────────
# Maps Ollama model tags to MLX Hugging Face model IDs.
# Edit these to match the models you've downloaded.
declare -A MLX_MODEL_MAP=(
    ["deepseek-r1:32b"]="mlx-community/DeepSeek-R1-Distill-Qwen-32B-4bit"
    ["deepseek-r1:70b"]="mlx-community/DeepSeek-R1-Distill-Llama-70B-4bit"
    ["qwen3-coder-next"]="mlx-community/Qwen3-Coder-Next-4bit"
    ["qwen3.5:35b"]="mlx-community/Qwen3.5-35B-A3B-4bit"
    ["qwen3:30b"]="mlx-community/Qwen3-30B-A3B-4bit"
    ["qwen3:235b-a22b"]="mlx-community/Qwen3-235B-A22B-4bit"
)

# Models to benchmark per tier (edit these lists for your hardware)
if [ "$TIER" = "128gb" ]; then
    DIAGNOSE_MODELS=("deepseek-r1:70b" "deepseek-r1:32b" "qwen3:30b")
    GENERATE_MODELS=("qwen3-coder-next" "qwen3:235b-a22b" "qwen3.5:35b")
    ASK_MODELS=("qwen3.5:35b" "qwen3:30b")
    # Ollama-only models (no MLX equivalent or too large)
    if [ "$BACKEND" = "ollama" ]; then
        DIAGNOSE_MODELS+=("llama4:scout")
        GENERATE_MODELS+=("llama4:scout")
        ASK_MODELS+=("llama4:scout" "kimi-k2")
    fi
else
    DIAGNOSE_MODELS=("deepseek-r1:32b" "qwen3:30b" "qwen3.5:35b")
    GENERATE_MODELS=("qwen3-coder-next" "qwen3.5:35b" "qwen3:30b")
    ASK_MODELS=("qwen3.5:35b" "qwen3:30b" "qwen3-coder-next")
fi

# ── Test Prompts ─────────────────────────────────────────────────────────────

DIAGNOSE_PROMPT='Diagnose the following terraform plan error and provide root-cause analysis with remediation steps:

```
Error: creating EC2 Instance (i-0abc123): InvalidParameterValue: Value (ami-0123456789abcdef0) for parameter imageId is invalid.
  status code: 400

  on main.tf line 15, in resource "aws_instance" "web":
  15: resource "aws_instance" "web" {

Error: creating Security Group (sg-web): InvalidGroup.Duplicate: A security group with the same name already exists for this VPC.
  status code: 400

  on main.tf line 30, in resource "aws_security_group" "web":
  30: resource "aws_security_group" "web" {
```'

GENERATE_PROMPT="Generate a production-grade EKS cluster module with IRSA, managed node groups, private API endpoint, KMS encryption for secrets, and cluster autoscaler IAM role."

ASK_PROMPT="What are the security best practices for configuring an S3 bucket with Terraform? Include encryption, access policies, versioning, and logging."

# ── Helpers ──────────────────────────────────────────────────────────────────

mkdir -p "$RESULTS_DIR"

log() { echo "[$(date +%H:%M:%S)] $*"; }

check_model() {
    local model="$1"
    if [ "$BACKEND" = "mlx" ]; then
        if [ -z "${MLX_MODEL_MAP[$model]+x}" ]; then
            log "WARNING: No MLX mapping for '${model}'. Skipping. Add it to MLX_MODEL_MAP."
            return 1
        fi
        return 0
    fi
    if ! curl -sf "${OLLAMA_HOST}/api/tags" | jq -e ".models[] | select(.name == \"${model}\")" > /dev/null 2>&1; then
        # Try with :latest suffix
        if ! curl -sf "${OLLAMA_HOST}/api/tags" | jq -e ".models[] | select(.name | startswith(\"${model%%:*}\"))" > /dev/null 2>&1; then
            log "WARNING: Model '${model}' not found in Ollama. Skipping."
            return 1
        fi
    fi
    return 0
}

unload_models() {
    if [ "$BACKEND" = "mlx" ]; then
        # MLX: stop any running mlx_lm.server
        pkill -f "mlx_lm.server" 2>/dev/null || true
        sleep 2
        return
    fi
    log "Unloading all models from Ollama memory..."
    local models
    models=$(curl -sf "${OLLAMA_HOST}/api/tags" | jq -r '.models[].name' 2>/dev/null || true)
    for m in $models; do
        curl -sf "${OLLAMA_HOST}/api/generate" -d "{\"model\":\"${m}\",\"keep_alive\":0}" > /dev/null 2>&1 || true
    done
    sleep 2
}

start_mlx_server() {
    local mlx_model="$1"
    log "  Starting MLX server for ${mlx_model} on port ${MLX_PORT}..."
    mlx_lm.server --model "$mlx_model" --port "$MLX_PORT" &>/dev/null &
    local mlx_pid=$!

    # Wait for server to be ready (up to 120s for large models)
    local waited=0
    while ! curl -sf "http://localhost:${MLX_PORT}/v1/models" > /dev/null 2>&1; do
        if ! kill -0 "$mlx_pid" 2>/dev/null; then
            log "  ERROR: MLX server died during startup"
            return 1
        fi
        sleep 2
        waited=$((waited + 2))
        if [ "$waited" -ge 120 ]; then
            log "  ERROR: MLX server did not start within 120s"
            kill "$mlx_pid" 2>/dev/null || true
            return 1
        fi
    done
    log "  MLX server ready (${waited}s startup)"
}

run_benchmark() {
    local task="$1"
    local model="$2"
    local prompt="$3"
    local extra_args="${4:-}"
    local outfile="${RESULTS_DIR}/${task}_${BACKEND}_${model//[:\/]/_}.json"

    log "Benchmarking: task=${task} model=${model} backend=${BACKEND}"

    # Unload previous models to get clean memory state
    unload_models

    if [ "$BACKEND" = "mlx" ]; then
        local mlx_model="${MLX_MODEL_MAP[$model]}"
        start_mlx_server "$mlx_model" || return

        # Configure tfai to use MLX via OpenAI-compatible endpoint
        export MODEL_PROVIDER=openai
        export OPENAI_API_KEY=not-needed
        export OPENAI_MODEL="$mlx_model"
        export OPENAI_BASE_URL="http://localhost:${MLX_PORT}/v1"
    else
        # Pre-warm: load model into memory with a tiny prompt
        log "  Warming up ${model}..."
        curl -sf "${OLLAMA_HOST}/api/generate" \
            -d "{\"model\":\"${model}\",\"prompt\":\"hi\",\"stream\":false}" > /dev/null 2>&1 || true
        sleep 1

        export MODEL_PROVIDER=ollama
        export OLLAMA_HOST="${OLLAMA_HOST}"
        export OLLAMA_MODEL="${model}"
    fi

    # Run the actual benchmark
    local start_time
    start_time=$(python3 -c 'import time; print(time.time())')

    local output
    local exit_code=0

    export MODEL_TEMPERATURE=0.2
    # Disable RAG and history for clean benchmarks
    unset QDRANT_HOST 2>/dev/null || true
    export TFAI_HISTORY_DB=disabled

    # Tier-specific tuning
    if [ "$TIER" = "128gb" ]; then
        export MODEL_MAX_TOKENS=16384
        export MAX_CONTEXT_TOKENS=64000
    else
        export MODEL_MAX_TOKENS=8192
        export MAX_CONTEXT_TOKENS=16000
    fi

    case "$task" in
        diagnose)
            output=$(echo "$prompt" | timeout 300 "$TFAI_BIN" diagnose 2>&1) || exit_code=$?
            ;;
        generate)
            local tmpdir
            tmpdir=$(mktemp -d)
            output=$(timeout 300 "$TFAI_BIN" generate --out "$tmpdir" "$prompt" 2>&1) || exit_code=$?
            rm -rf "$tmpdir"
            ;;
        ask)
            output=$(timeout 300 "$TFAI_BIN" ask "$prompt" 2>&1) || exit_code=$?
            ;;
    esac

    local end_time
    end_time=$(python3 -c 'import time; print(time.time())')

    local duration
    duration=$(python3 -c "print(round(${end_time} - ${start_time}, 2))")

    local output_chars=${#output}
    local est_tokens=$((output_chars / 4))
    local tok_per_sec
    if [ "$duration" != "0" ] && [ "$duration" != "0.0" ]; then
        tok_per_sec=$(python3 -c "print(round(${est_tokens} / ${duration}, 1))")
    else
        tok_per_sec="0"
    fi

    # Extract metrics from tfai log output if available (lines with "query metrics")
    local agent_metrics
    agent_metrics=$(echo "$output" | grep -o 'tokens_per_sec=[0-9.]*' | head -1 | cut -d= -f2 || echo "")
    local agent_ttft
    agent_ttft=$(echo "$output" | grep -o 'ttft=[0-9.]*[a-z]*' | head -1 | cut -d= -f2 || echo "")

    # Write results
    cat > "$outfile" <<ENDJSON
{
    "task": "${task}",
    "model": "${model}",
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "exit_code": ${exit_code},
    "duration_sec": ${duration},
    "output_chars": ${output_chars},
    "est_output_tokens": ${est_tokens},
    "est_tokens_per_sec": ${tok_per_sec},
    "agent_tokens_per_sec": "${agent_metrics}",
    "agent_ttft": "${agent_ttft}",
    "backend": "${BACKEND}",
    "tier": "${TIER}",
    "settings": {
        "max_tokens": ${MODEL_MAX_TOKENS},
        "temperature": 0.2,
        "max_context_tokens": ${MAX_CONTEXT_TOKENS}
    }
}
ENDJSON

    if [ "$exit_code" -eq 0 ]; then
        log "  PASS: ${duration}s, ~${est_tokens} tokens, ~${tok_per_sec} tok/s"
    else
        log "  FAIL: exit_code=${exit_code}, duration=${duration}s"
    fi

    # Clean up MLX server after each run
    if [ "$BACKEND" = "mlx" ]; then
        pkill -f "mlx_lm.server" 2>/dev/null || true
        sleep 2
    fi
}

# ── Main ─────────────────────────────────────────────────────────────────────

# Check prerequisites
if [ ! -x "$TFAI_BIN" ]; then
    log "Building tfai..."
    make build
fi

if ! command -v jq &> /dev/null; then
    echo "ERROR: jq is required. Install with: brew install jq"
    exit 1
fi

if [ "$BACKEND" = "ollama" ]; then
    if ! curl -sf "${OLLAMA_HOST}/api/tags" > /dev/null 2>&1; then
        echo "ERROR: Ollama not running at ${OLLAMA_HOST}"
        exit 1
    fi
elif [ "$BACKEND" = "mlx" ]; then
    if ! command -v mlx_lm.server &> /dev/null; then
        echo "ERROR: mlx_lm not installed. Install with: pip install mlx-lm"
        exit 1
    fi
else
    echo "ERROR: Unknown backend '${BACKEND}'. Use 'ollama' or 'mlx'."
    exit 1
fi

log "Benchmark results will be saved to: ${RESULTS_DIR}/"
log "Backend: ${BACKEND}"
log "Hardware tier: ${TIER}"
log "Task filter: ${FILTER}"
echo ""

# Run diagnose benchmarks
if [ "$FILTER" = "all" ] || [ "$FILTER" = "diagnose" ]; then
    log "=== DIAGNOSE BENCHMARKS ==="
    for model in "${DIAGNOSE_MODELS[@]}"; do
        if check_model "$model"; then
            run_benchmark "diagnose" "$model" "$DIAGNOSE_PROMPT"
        fi
    done
    echo ""
fi

# Run generate benchmarks
if [ "$FILTER" = "all" ] || [ "$FILTER" = "generate" ]; then
    log "=== GENERATE BENCHMARKS ==="
    for model in "${GENERATE_MODELS[@]}"; do
        if check_model "$model"; then
            run_benchmark "generate" "$model" "$GENERATE_PROMPT"
        fi
    done
    echo ""
fi

# Run ask benchmarks
if [ "$FILTER" = "all" ] || [ "$FILTER" = "ask" ]; then
    log "=== ASK BENCHMARKS ==="
    for model in "${ASK_MODELS[@]}"; do
        if check_model "$model"; then
            run_benchmark "ask" "$model" "$ASK_PROMPT"
        fi
    done
    echo ""
fi

# ── Summary ──────────────────────────────────────────────────────────────────

log "=== SUMMARY ==="
log ""
printf "%-12s %-25s %-8s %8s %8s %10s\n" "TASK" "MODEL" "BACKEND" "TIME(s)" "TOKENS" "TOK/S"
printf "%-12s %-25s %-8s %8s %8s %10s\n" "----" "-----" "-------" "-------" "------" "-----"

for f in "$RESULTS_DIR"/*.json; do
    [ -f "$f" ] || continue
    task=$(jq -r '.task' "$f")
    model=$(jq -r '.model' "$f")
    backend=$(jq -r '.backend' "$f")
    dur=$(jq -r '.duration_sec' "$f")
    tokens=$(jq -r '.est_output_tokens' "$f")
    tps=$(jq -r '.est_tokens_per_sec' "$f")
    status=$(jq -r 'if .exit_code == 0 then "" else " FAIL" end' "$f")
    printf "%-12s %-25s %-8s %8s %8s %10s%s\n" "$task" "$model" "$backend" "$dur" "$tokens" "$tps" "$status"
done

log ""
log "Full results: ${RESULTS_DIR}/"
