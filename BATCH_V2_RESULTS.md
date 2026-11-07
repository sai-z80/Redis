# Batch v2 — Arm A vibe port comparison (exploratory n=1)

**Protocol:** `batch-v2-completion-oriented` (`PROTOCOL_BATCH_V2.md`). Cost is an **outcome**, not a stop.
Uniform stops: `model_done` / `max_iterations(150)` / `wall_timeout(7200s)` / harness API failure.

**Task:** Redis 7.2.17 C subset → Go. Same frozen source, prompt (`prompts/port.batch-v2.md`), tools.
Primary metric: fresh withheld oracle **90** (implementation **not published**). Secondary: smoke 6 + visible regression 43.

## Methodology limitations (read before ranking)
- **n=1 exploratory.** Do not generalize provider rankings or claim software-factory benefits. No 8090 / Arm B.
- The six completed rows below used the **previous (non-canonical for “current flagship”) model slate**: DeepSeek v3.2, Gemini 3.1 Pro preview, Grok 4.7, Kimi K3.1 Pro, GPT-5.4, Claude Opus 4.6 — with **`max_tokens=8192`**, **provider-default reasoning (no effort pin)**, Chat Completions, **300 s** per-call HTTP timeout. Freeze: `FREEZE.batch-v2-max8192.json`.
- A **flagship correction** was run and is **complete but partial** (Claude Opus 5.5, GPT-5.5 Pro via Responses API, DeepSeek V4 Pro) under amendment `flagship-correction-1` (higher output budget, pinned `reasoning_effort=high`, 1800 s per-call timeout; overall wall still 7200 s): Opus 5.5 and DeepSeek V4 Pro scored; GPT-5.5 Pro user-terminated, not scored. Reused Gemini/Grok/Kimi rows will remain under the older settings; do not falsely call those max-reasoning.
- See `TESTS_PUBLICATION_NOTICE.md`. Upstream Redis tests are public; fresh-90 private ≠ contamination-free.

## Results (completed batch-v2 slate)

| Family | Model ID | Hidden | Smoke | Regression | Build | Stop | Cost (billed) | Wall (s) | Iters |
|---|---|---|---|---|---|---|---|---|---|
| DeepSeek | `deepseek/deepseek-v3.2` | 88/90 | 6/6 | 43/43 | ok | model_done | $0.102 | 404 | 37 |
| Gemini | `google/gemini-3.1-pro-preview` | 88/90 | 6/6 | 43/43 | ok | model_done | $0.294 | 142 | 15 |
| Grok | `x-ai/grok-4.7` | **90/90** | 6/6 | 43/43 | ok | model_done | $0.877 | 550 | 24 |
| Kimi | `moonshotai/kimi-k3.1-pro` | **90/90** | 6/6 | 43/43 | ok | model_done | $0.274 | 413 | 19 |
| GPT | `openai/gpt-5.4` | 88/90 | 6/6 | 43/43 | ok | model_done | $0.095 | 48 | 7 |
| Claude | `anthropic/claude-opus-4.6` | 88/90 | 6/6 | 43/43 | ok | model_done | $2.012 | 110 | 24 |

Batch OpenRouter spend (delta from baseline for these six): ~**$3.65**. Common failures on 88/90 models: `ex_seconds_overflow_invalid_expire_time`, `px_absolute_overflow_invalid_expire_time`.

Full machine-readable: `BATCH_V2_RESULTS.json`. Per-run: `runs/<id>/{manifest,scores,transcript,workspace}`.

## Flagship correction

See [`FLAGSHIP_COMPARISON.md`](FLAGSHIP_COMPARISON.md) (Opus 5.5 + DeepSeek V4 Pro complete; GPT-5.5 Pro **user_terminated** incomplete, not scored).
