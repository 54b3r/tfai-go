# Model Testing Guide

This guide covers how to configure, run, and benchmark different local LLM models
with tfai using Ollama. The goal is to find the best model for each task type
(diagnose, generate, ask) and tune context/token settings for optimal throughput.

Two hardware tiers are covered:
- **48 GB** -- M5 Max entry config (current)
- **128 GB** -- M5 Max upgraded config (planned)

## Hardware Tiers

| | 48 GB (Current) | 128 GB (Upgrade) |
|---|---|---|
| **Chip** | Apple M5 Max | Apple M5 Max |
| **Usable for models** | ~38 GB (leave ~10 GB headroom) | ~110 GB (leave ~18 GB headroom) |
| **Max dense model** | 32B @ Q4 | 70B @ Q8 / Q6 |
| **Max MoE model** | 35B-A3B, 80B-A3B | 235B-A22B @ Q4 |
| **Concurrent models** | Not practical | Yes -- split-model with headroom |
| **Quantization sweet spot** | Q4_K_M | Q6_K / Q8_0 (quality uplift worth it) |
| **Inference runtime** | Ollama | Ollama (or MLX for ~20-30% faster) |

---

## Recommended Models -- 48 GB Tier

| Model | Ollama Tag | Type | Active Params | Mem (Q4) | Best For | Notes |
|---|---|---|---|---|---|---|
| DeepSeek R1 32B | `deepseek-r1:32b` | Dense | 32B | ~20 GB | `diagnose` | Deep reasoning, root-cause analysis |
| Qwen3-Coder-Next | `qwen3-coder-next` | MoE 80B | 3B | ~6 GB | `generate` | #1 SWE-bench (64.6%), fast inference |
| Qwen 3.5 35B | `qwen3.5:35b` | MoE 35B | 3B | ~6 GB | `generate`, `ask` | General-purpose, fast |
| Qwen3 30B | `qwen3:30b` | Dense | 30B | ~18 GB | `ask`, fallback | Proven Ollama support |
| Kimi K2 | `kimi-k2` | MoE 1T | 32B | ~38 GB | `ask` | Agentic, tight fit |

## Recommended Models -- 128 GB Tier

With 128 GB you unlock higher-quality quantizations, 70B dense models, the
frontier-class Qwen3 235B MoE, and the ability to run two models concurrently
for the split-model setup.

| Model | Ollama Tag | Type | Active Params | Mem (rec. quant) | Best For | Notes |
|---|---|---|---|---|---|---|
| DeepSeek R1 70B | `deepseek-r1:70b` | Dense | 70B | ~70 GB (Q8) | `diagnose` | Best local reasoning model; Q8 quality uplift over 32B Q4 is significant |
| DeepSeek R1 70B | `deepseek-r1:70b` | Dense | 70B | ~55 GB (Q6) | `diagnose` | Sweet spot -- leaves room for concurrent MoE generate model |
| Qwen3 235B-A22B | `qwen3:235b-a22b` | MoE 235B | 22B | ~100 GB (Q4) | `generate`, `ask` | Frontier-class; tight fit, disable Qdrant or use minimal RAG |
| Qwen3-Coder-Next | `qwen3-coder-next` | MoE 80B | 3B | ~6 GB (Q4) | `generate` | Still the speed king; pairs perfectly with a 70B chat model |
| Llama 4 Scout | `llama4:scout` | MoE 109B | 17B | ~60 GB (Q4) | `ask` | 10M token context window; strong general knowledge |
| Kimi K2 | `kimi-k2` | MoE 1T | 32B | ~38 GB (Q4) | `ask` | Comfortable fit with headroom; strong agentic behavior |
| Qwen 3.5 35B | `qwen3.5:35b` | MoE 35B | 3B | ~6 GB (Q4) | `generate`, `ask` | Lightweight companion for split-model |
| DeepSeek R1 32B | `deepseek-r1:32b` | Dense | 32B | ~34 GB (Q8) | `diagnose` | Run at Q8 for quality; leaves room for a second model |

## Quick Setup

### 1. Pull Models

```bash
# Pull the models you want to test
ollama pull deepseek-r1:32b
ollama pull qwen3-coder-next
ollama pull qwen3.5:35b
ollama pull qwen3:30b
```

### 2. Environment Configs

Create separate `.env` files per model for easy switching:

**`.env.deepseek-diagnose`** -- Optimized for diagnosis (deep reasoning)
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=deepseek-r1:32b

# Reasoning models benefit from higher token budgets
MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.1

# Larger context window for plan output + history
MAX_CONTEXT_TOKENS=16000

# Disable RAG for pure diagnosis (plan output is the context)
# QDRANT_HOST=
```

**`.env.qwen-generate`** -- Optimized for code generation
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=qwen3-coder-next

# Code generation needs room for multi-file output
MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2

# Moderate context -- RAG docs + workspace files
MAX_CONTEXT_TOKENS=12000

# Enable RAG for provider documentation
QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_COLLECTION=tfai-docs
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=nomic-embed-text
```

**`.env.qwen35-general`** -- General-purpose ask
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=qwen3.5:35b

MODEL_MAX_TOKENS=4096
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=8000

QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_COLLECTION=tfai-docs
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=nomic-embed-text
```

**`.env.split-models`** -- Different models for chat vs generate
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=deepseek-r1:32b

# Override generation to use the code-specialist model
GENERATE_MODEL_PROVIDER=ollama
GENERATE_MODEL=qwen3-coder-next

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=12000
```

### 3. Running with a specific config

```bash
# Method 1: Source the env file
set -a; source .env.deepseek-diagnose; set +a
make run

# Method 2: Inline (single command)
env $(cat .env.qwen-generate | grep -v '^#' | xargs) ./bin/tfai serve

# Method 3: CLI commands directly
env $(cat .env.deepseek-diagnose | grep -v '^#' | xargs) \
  ./bin/tfai diagnose --plan plan.txt
```

## Context Management Optimization

### How the Token Budget Works

tfai uses a character-based heuristic (1 token ~ 4 chars) to manage context:

```
Total Context Budget (MAX_CONTEXT_TOKENS)
  = System Prompt (~fixed, ~2000 tokens)
  + RAG Documents (RAG_TOP_K * ~500 tokens each)
  + Workspace Files (up to 50 .tf files, 1 MiB total)
  + Conversation History (trimmed oldest-first to fit)
  + User Message
```

### Tuning Guidelines

| Model Context Window | Recommended `MAX_CONTEXT_TOKENS` | `MODEL_MAX_TOKENS` | Notes |
|---|---|---|---|
| 8K (older models) | 6000 (default) | 2048 | Conservative |
| 32K | 16000 | 8192 | Good for diagnose with large plans |
| 128K | 64000 | 16384 | Full workspace + deep history |

**Key env vars for tuning:**

| Variable | Default | Description |
|---|---|---|
| `MAX_CONTEXT_TOKENS` | 6000 | Input context budget (tokens). History trimmed to fit. |
| `MODEL_MAX_TOKENS` | 4096 | Max output tokens per response |
| `MODEL_TEMPERATURE` | 0.2 | Lower = more deterministic. Use 0.1 for diagnose. |
| `RAG_TOP_K` | 5 | Number of RAG documents injected per query |

### Tips for 48GB Systems

1. **Run one model at a time** -- Ollama keeps models in memory. Unload before switching:
   ```bash
   ollama stop deepseek-r1:32b
   ollama run qwen3-coder-next
   ```

2. **Monitor memory pressure:**
   ```bash
   # Watch unified memory usage
   sudo powermetrics --samplers gpu_power -i 1000 | grep -i "memory"
   # Or simpler:
   vm_stat 1
   ```

3. **Leave ~8-10 GB headroom** for the OS, Qdrant, and tfai itself. On 48GB, that
   means your model should use < 38GB.

4. **Q4_K_M quantization** is the default in Ollama and the best tradeoff. Don't
   go lower (Q3) unless you're running a 70B+ model.

### Tips for 128GB Systems

The 128GB tier is a fundamentally different operating environment. You move from
"which single model fits?" to "which combination delivers the best results?"

1. **Upgrade quantization first, model size second.** A 70B model at Q8 often
   outperforms the same 70B at Q4 by a noticeable margin on reasoning and code
   quality. With 128GB, you can afford Q6 or Q8 for your primary model.

   ```bash
   # Pull higher-quality quantizations explicitly
   ollama pull deepseek-r1:70b-q8_0     # ~70 GB, best quality
   ollama pull deepseek-r1:70b-q6_K     # ~55 GB, quality/headroom balance
   ```

2. **Run concurrent models for split-model.** With 128GB you have genuine headroom
   for two models resident in memory simultaneously. The recommended pairing:

   | Chat/Diagnose Model | Generate Model | Combined Mem | Headroom |
   |---|---|---|---|
   | deepseek-r1:70b Q6 (~55 GB) | qwen3-coder-next Q4 (~6 GB) | ~61 GB | ~49 GB free |
   | deepseek-r1:70b Q8 (~70 GB) | qwen3.5:35b Q4 (~6 GB) | ~76 GB | ~34 GB free |
   | deepseek-r1:32b Q8 (~34 GB) | llama4:scout Q4 (~60 GB) | ~94 GB | ~16 GB free |
   | kimi-k2 Q4 (~38 GB) | qwen3-coder-next Q4 (~6 GB) | ~44 GB | ~66 GB free |

   Set Ollama to keep both models loaded:
   ```bash
   # Keep models in memory for 30 minutes (default: 5m)
   export OLLAMA_KEEP_ALIVE=30m
   ```

3. **Qwen3 235B-A22B is the ceiling.** At ~100 GB (Q4), this is the most capable
   model you can run locally. It's a tight fit -- disable Qdrant and keep RAG_TOP_K
   low. It delivers frontier-class quality but at 5-10 tok/s due to the sheer number
   of expert weights to shuttle through memory bandwidth.

   ```bash
   # Dedicated .env for the 235B -- single model, no RAG
   MODEL_PROVIDER=ollama
   OLLAMA_MODEL=qwen3:235b-a22b
   MODEL_MAX_TOKENS=8192
   MAX_CONTEXT_TOKENS=32000
   TFAI_HISTORY_DB=disabled
   # Unset QDRANT_HOST to free memory
   ```

4. **Llama 4 Scout's 10M context window** is uniquely valuable for `tfai diagnose`
   on massive plan outputs or monorepo workspaces with hundreds of .tf files. At
   ~60 GB (Q4) it leaves room for a second lightweight model.

5. **Monitor memory with more granularity on 128GB:**
   ```bash
   # Ollama's built-in model listing shows VRAM usage
   ollama ps

   # Detailed per-process memory
   ps aux | grep -E 'ollama|qdrant|tfai' | awk '{printf "%-30s %s MB\n", $11, $6/1024}'

   # macOS Activity Monitor → Memory tab → Memory Pressure graph
   # Green = fine, Yellow = swapping, Red = thrashing
   ```

6. **Consider MLX for the heaviest models.** Apple's MLX framework is consistently
   20-30% faster than llama.cpp (Ollama's backend) on Apple Silicon due to native
   unified memory optimizations. For the 235B model where every tok/s counts, MLX
   can be the difference between usable and painfully slow. tfai works with any
   OpenAI-compatible API, so you can point `MODEL_PROVIDER=openai` at an MLX
   server running on `localhost`.

## Reading Performance Metrics

tfai logs query metrics automatically for every request:

```
INFO query metrics input_tokens=2450 output_tokens=1830 duration=12.4s tokens_per_sec=147.6 ttft=890ms
```

| Metric | What It Tells You |
|---|---|
| `input_tokens` | Total estimated input context size |
| `output_tokens` | Estimated tokens in the generated response |
| `duration` | Wall-clock time from stream start to last token |
| `tokens_per_sec` | Output throughput (higher = better) |
| `ttft` | Time to first token -- measures model load + prefill latency |

### What Good Looks Like -- M5 Max 48 GB

| Model | Quant | Expected tok/s | Expected TTFT |
|---|---|---|---|
| qwen3-coder-next (3B active MoE) | Q4 | 40-80 tok/s | < 500ms |
| qwen3.5:35b (3B active MoE) | Q4 | 40-80 tok/s | < 500ms |
| deepseek-r1:32b (dense) | Q4 | 15-25 tok/s | 1-3s |
| qwen3:30b (dense) | Q4 | 15-25 tok/s | 1-3s |
| kimi-k2 (32B active MoE) | Q4 | 10-18 tok/s | 2-5s |

### What Good Looks Like -- M5 Max 128 GB

| Model | Quant | Expected tok/s | Expected TTFT | Notes |
|---|---|---|---|---|
| qwen3-coder-next (3B active MoE) | Q4 | 40-80 tok/s | < 500ms | Same as 48GB -- model fits easily |
| qwen3.5:35b (3B active MoE) | Q4 | 40-80 tok/s | < 500ms | Same as 48GB |
| deepseek-r1:70b (dense) | Q6 | 12-20 tok/s | 2-5s | Quality leap over 32B; slower due to size |
| deepseek-r1:70b (dense) | Q8 | 8-15 tok/s | 3-6s | Highest quality local reasoning |
| deepseek-r1:32b (dense) | Q8 | 18-28 tok/s | 1-2s | Faster than 70B, Q8 quality uplift |
| llama4:scout (17B active MoE) | Q4 | 20-35 tok/s | 1-3s | 10M context window; good throughput |
| kimi-k2 (32B active MoE) | Q4 | 12-20 tok/s | 2-4s | Comfortable fit, no swapping |
| qwen3:235b-a22b (22B active MoE) | Q4 | 5-10 tok/s | 5-15s | Frontier quality, bandwidth-bound |

**Key insight:** On Apple Silicon, tokens/s is determined by **memory bandwidth**,
not compute. The M5 Max's bandwidth is shared between GPU and CPU. Larger models
don't use more compute -- they shuttle more weights through the same pipe. This is
why a 3B-active MoE runs at 60+ tok/s regardless of total param count, while a
70B dense model runs at ~15 tok/s.

## Benchmark Script

Use the included `scripts/benchmark-models.sh` to run standardized prompts across
models and collect metrics:

```bash
# Run full benchmark suite
./scripts/benchmark-models.sh

# Run only diagnose benchmarks
./scripts/benchmark-models.sh diagnose

# Run only generate benchmarks
./scripts/benchmark-models.sh generate
```

Results are written to `benchmark-results/` as JSON files with timestamps.

See [scripts/benchmark-models.sh](../scripts/benchmark-models.sh) for details.

## Task-Specific Recommendations

### Diagnose (`tfai diagnose`)

**48 GB tier:**
- **Model:** `deepseek-r1:32b` (Q4)
- **Why:** Chain-of-thought reasoning excels at root-cause analysis

**128 GB tier:**
- **Model:** `deepseek-r1:70b` (Q6 or Q8)
- **Why:** Massive quality improvement over 32B for complex multi-resource failures.
  Q8 gives near-lossless reasoning quality.
- **Alternative:** `llama4:scout` (Q4) when diagnosing very large plan outputs --
  its 10M token context window can ingest entire monorepo plan outputs that would
  overflow other models.

**Settings (both tiers):**
- `MODEL_TEMPERATURE=0.1` (deterministic diagnosis)
- `MAX_CONTEXT_TOKENS=16000` (48 GB) / `MAX_CONTEXT_TOKENS=64000` (128 GB)
- `MODEL_MAX_TOKENS=8192` (detailed remediation steps)
- Disable RAG unless diagnosing provider-specific issues

### Generate (`tfai generate`)

**48 GB tier:**
- **Model:** `qwen3-coder-next` (Q4)
- **Why:** Purpose-built for code, #1 SWE-bench (64.6%), very fast (3B active)

**128 GB tier:**
- **Model:** `qwen3-coder-next` (Q4) -- still the top pick for pure speed
- **Alternative:** `qwen3:235b-a22b` (Q4) when quality matters more than speed.
  Frontier-class output but 5-10 tok/s. Use for complex multi-module generation
  where getting the architecture right on the first pass saves iteration time.

**Settings (both tiers):**
- `MODEL_TEMPERATURE=0.2` (creative but consistent)
- `MAX_CONTEXT_TOKENS=12000` (48 GB) / `MAX_CONTEXT_TOKENS=32000` (128 GB)
- `MODEL_MAX_TOKENS=8192` (multi-file output)
- Enable RAG with ingested provider docs

### Ask (`tfai ask`)

**48 GB tier:**
- **Model:** `qwen3.5:35b` or `qwen3:30b`

**128 GB tier:**
- **Model:** `llama4:scout` (Q4) for general Terraform questions -- strong knowledge
  base and the massive context window handles deep follow-up conversations without
  history trimming.
- **Alternative:** `kimi-k2` (Q4) for agentic multi-step reasoning.

**Settings (both tiers):**
- `MODEL_TEMPERATURE=0.2`
- `MAX_CONTEXT_TOKENS=8000` (48 GB) / `MAX_CONTEXT_TOKENS=32000` (128 GB)
- `MODEL_MAX_TOKENS=4096`
- Enable RAG for documentation-backed answers

### Split-Model Setup (Best of Both Worlds)

Use the `GENERATE_MODEL_PROVIDER` / `GENERATE_MODEL` env vars to run different
models for chat vs code generation in the same tfai instance:

**48 GB -- lightweight split (one model at a time, Ollama hot-swaps):**
```bash
MODEL_PROVIDER=ollama
OLLAMA_MODEL=deepseek-r1:32b

GENERATE_MODEL_PROVIDER=ollama
GENERATE_MODEL=qwen3-coder-next

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=12000
```
Note: On 48GB both models cannot be resident simultaneously. Ollama will unload
the chat model when generate is called and vice versa, adding ~5-10s swap latency.

**128 GB -- concurrent split (both models resident in memory):**
```bash
MODEL_PROVIDER=ollama
OLLAMA_MODEL=deepseek-r1:70b       # ~55 GB at Q6 for chat/diagnose

GENERATE_MODEL_PROVIDER=ollama
GENERATE_MODEL=qwen3-coder-next     # ~6 GB at Q4 for code generation

MODEL_MAX_TOKENS=16384
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=32000
```
With ~61 GB combined and ~49 GB headroom, both models stay loaded -- no swap
latency between chat and generate calls.

## 128 GB Environment Configs

Ready-to-use `.env` files for the 128 GB tier:

**`.env.128g-diagnose-70b`** -- Best-quality diagnosis
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=deepseek-r1:70b

# Q8 for maximum reasoning quality; comment out for default Q4
# Pull explicitly: ollama pull deepseek-r1:70b-q8_0

MODEL_MAX_TOKENS=16384
MODEL_TEMPERATURE=0.1

# 70B models support 128K context; use generously
MAX_CONTEXT_TOKENS=64000

# Disable RAG to maximize available memory for the model
# QDRANT_HOST=
TFAI_HISTORY_DB=disabled
```

**`.env.128g-generate-235b`** -- Frontier-class generation
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=qwen3:235b-a22b

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=32000

# Minimal RAG to save memory -- or disable entirely
RAG_TOP_K=3
QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_COLLECTION=tfai-docs
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=nomic-embed-text

TFAI_HISTORY_DB=disabled
```

**`.env.128g-split-optimal`** -- Recommended daily driver for 128 GB
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=deepseek-r1:70b       # Chat + diagnose (Q6, ~55 GB)

GENERATE_MODEL_PROVIDER=ollama
GENERATE_MODEL=qwen3-coder-next     # Generate (Q4, ~6 GB)

MODEL_MAX_TOKENS=16384
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=32000

# Keep models loaded for 30min between requests
# Set via: export OLLAMA_KEEP_ALIVE=30m (before starting Ollama)

# Full RAG enabled -- plenty of headroom
QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_COLLECTION=tfai-docs
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=nomic-embed-text
RAG_TOP_K=5
```

**`.env.128g-scout-longctx`** -- Maximum context for large workspaces
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=llama4:scout           # 10M token context window, 17B active

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2

# Exploit Scout's massive context -- no history will ever be trimmed
MAX_CONTEXT_TOKENS=128000

QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_COLLECTION=tfai-docs
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=nomic-embed-text
RAG_TOP_K=10                         # More docs, we have the room
```

**`.env.128g-kimi-agentic`** -- Agentic multi-step reasoning
```bash
MODEL_PROVIDER=ollama
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=kimi-k2                # 32B active from 1T MoE (~38 GB)

GENERATE_MODEL_PROVIDER=ollama
GENERATE_MODEL=qwen3.5:35b          # Fast lightweight code gen (~6 GB)

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=16000

QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_COLLECTION=tfai-docs
EMBEDDING_PROVIDER=ollama
EMBEDDING_MODEL=nomic-embed-text
```

## MLX vs Ollama: Backend Comparison

MLX is Apple's native ML framework optimized for unified memory. On Apple Silicon,
it consistently outperforms Ollama's llama.cpp backend -- sometimes dramatically.
**This applies to both 48GB and 128GB tiers.**

### Why MLX is Faster

| Factor | Ollama (llama.cpp) | MLX |
|---|---|---|
| Memory access | Generic Metal compute shaders | Native unified memory API |
| Memory usage | Higher (~1.5-2x overhead) | Lower (~half for same model) |
| Token generation | Baseline | 20-50% faster |
| Prompt processing | Baseline | Up to 5x faster |
| KV cache | Standard | Optimized for Apple GPU |

### Real-World Speed Differences

| Model | Ollama tok/s | MLX tok/s | Speedup | Tier |
|---|---|---|---|---|
| qwen3.5:35b-a3b (Q4) | ~35 tok/s | ~60-70 tok/s | ~2x | 48 GB |
| deepseek-r1:32b (Q4) | ~20 tok/s | ~28-35 tok/s | ~1.5x | 48 GB |
| qwen3:30b (Q4) | ~20 tok/s | ~28-32 tok/s | ~1.4x | 48 GB |
| deepseek-r1:70b (Q6) | ~15 tok/s | ~20-25 tok/s | ~1.5x | 128 GB |
| qwen3:235b-a22b (Q4) | ~5 tok/s | ~7-9 tok/s | ~1.5x | 128 GB |

The M5 Max GPU Neural Accelerators provide additional 19-27% uplift over M4,
and up to 4x faster time-to-first-token via MLX.

### When to Use Each

**Use Ollama when:**
- You need an always-on background daemon (API server)
- You want one-command model management (`ollama pull`, `ollama ps`)
- You're running the web UI (`tfai serve`) and want simplicity
- The model is small enough that the speed difference doesn't matter (MoE with
  3B active params is already fast on either backend)

**Use MLX when:**
- You're running dense 30B+ models where every tok/s matters
- You're benchmarking and want peak throughput numbers
- You want to squeeze the most out of 48GB (MLX uses less memory, so larger
  models or higher quantization may fit)
- TTFT (time to first token) is important -- MLX prompt processing is up to 5x faster

### Setup: MLX as a Backend for tfai

tfai talks to any OpenAI-compatible API. MLX exposes one via `mlx_lm.server`.

**Step 1: Install mlx-lm**
```bash
pip install mlx-lm
# Or with uv (faster):
uv pip install mlx-lm
```

**Step 2: Start MLX server with your model**
```bash
# DeepSeek R1 32B for diagnose
mlx_lm.server \
  --model mlx-community/DeepSeek-R1-Distill-Qwen-32B-4bit \
  --port 8100

# Qwen3-Coder-Next for generate (separate port)
mlx_lm.server \
  --model mlx-community/Qwen3-Coder-Next-4bit \
  --port 8101
```

**Step 3: Point tfai at it**

Since tfai's OpenAI provider speaks the standard API, point it at the MLX server:

**`.env.mlx-diagnose`**
```bash
MODEL_PROVIDER=openai
OPENAI_API_KEY=not-needed
OPENAI_MODEL=mlx-community/DeepSeek-R1-Distill-Qwen-32B-4bit
OPENAI_BASE_URL=http://localhost:8100/v1

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.1
MAX_CONTEXT_TOKENS=16000
```

**`.env.mlx-generate`**
```bash
MODEL_PROVIDER=openai
OPENAI_API_KEY=not-needed
OPENAI_MODEL=mlx-community/Qwen3-Coder-Next-4bit
OPENAI_BASE_URL=http://localhost:8101/v1

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=12000
```

**`.env.mlx-split`** -- Different MLX servers for chat vs generate
```bash
MODEL_PROVIDER=openai
OPENAI_API_KEY=not-needed
OPENAI_MODEL=mlx-community/DeepSeek-R1-Distill-Qwen-32B-4bit
OPENAI_BASE_URL=http://localhost:8100/v1

GENERATE_MODEL_PROVIDER=openai
GENERATE_MODEL=mlx-community/Qwen3-Coder-Next-4bit
# Note: GENERATE_MODEL uses the same OPENAI_BASE_URL unless overridden

MODEL_MAX_TOKENS=8192
MODEL_TEMPERATURE=0.2
MAX_CONTEXT_TOKENS=16000
```

### MLX Model Names (Hugging Face)

Common MLX-quantized models on Hugging Face's `mlx-community`:

| Purpose | Ollama equivalent | MLX model ID |
|---|---|---|
| Diagnose (32B) | `deepseek-r1:32b` | `mlx-community/DeepSeek-R1-Distill-Qwen-32B-4bit` |
| Diagnose (70B, 128GB) | `deepseek-r1:70b` | `mlx-community/DeepSeek-R1-Distill-Llama-70B-4bit` |
| Generate | `qwen3-coder-next` | `mlx-community/Qwen3-Coder-Next-4bit` |
| General | `qwen3.5:35b` | `mlx-community/Qwen3.5-35B-A3B-4bit` |
| General | `qwen3:30b` | `mlx-community/Qwen3-30B-A3B-4bit` |

Models are downloaded on first use and cached in `~/.cache/huggingface/`.

### Side-by-Side Benchmark: Ollama vs MLX

Use the benchmark script with the `--backend` flag (or `BENCH_BACKEND` env var)
to compare backends:

```bash
# Benchmark the same models on Ollama
./scripts/benchmark-models.sh diagnose

# Benchmark the same prompts via MLX
BENCH_BACKEND=mlx ./scripts/benchmark-models.sh diagnose

# Compare results
diff <(jq -s 'sort_by(.model)' benchmark-results/*/diagnose_*.json) \
     <(jq -s 'sort_by(.model)' benchmark-results/*/diagnose_*.json)
# Or just eyeball the summary tables
```

### Quality Check: Does MLX Sacrifice Accuracy?

**No.** MLX and Ollama run the same quantized weights through different compute
paths. At the same quantization level (Q4_K_M), the outputs are functionally
identical. Minor floating-point differences may cause slightly different token
sampling, but:

- Deterministic outputs (`temperature=0`) should match closely
- Quality benchmarks (HumanEval, MMLU, etc.) show no measurable difference
- The only real variable is quantization format -- GGUF (Ollama) vs MLX-native.
  Both use 4-bit groupwise quantization with similar information loss.

**Bottom line:** Test both backends with the same prompt at `temperature=0.1`.
If the outputs are qualitatively equivalent (they should be), use whichever
gives you better tok/s.
