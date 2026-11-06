# Batch v2 comparison (Arm A vibe port only)

**Protocol:** `batch-v2-completion-oriented` (`PROTOCOL_BATCH_V2.md`). Cost is an **outcome**, not a stop. Uniform stops: model_done / max_iterations(150) / wall_timeout(7200s) / harness_api_failure. Internal `SF_BATCH_GUARD_USD=50` was set for supervision only (not a published score cap); not hit.

**Task:** Redis 7.2.17 C subset → Go; same source, prompt (`port.batch-v2.md`), tools, caps. Fresh withheld oracle **90** tests. Published suite retained as **visible regression** (43). Smoke 6.

**n=1 exploratory.** Do not generalize rankings or claim software-factory benefits. No 8090 / Arm B. **New suite not published yet** (awaiting review).

Pilot-v1 DeepSeek under old $1.50 cost-stop protocol is **not** mixed in.

| Family | Model ID | Hidden | Smoke | Regression | Build | Stop | Cost (billed) | Wall (s) | Iters |
|---|---|---|---|---|---|---|---|---|---|
| DeepSeek | `deepseek/deepseek-v3.2` | 88/90 | 6/6 | 43/43 | ok | model_done | $0.102 | 404 | 37 |
| Gemini | `google/gemini-3.1-pro-preview` | 88/90 | 6/6 | 43/43 | ok | model_done | $0.294 | 142 | 15 |
| Grok | `x-ai/grok-4.7` | **90/90** | 6/6 | 43/43 | ok | model_done | $0.877 | 550 | 24 |
| Kimi | `moonshotai/kimi-k3.1-pro` | **90/90** | 6/6 | 43/43 | ok | model_done | $0.274 | 413 | 19 |
| GPT | `openai/gpt-5.4` | 88/90 | 6/6 | 43/43 | ok | model_done | $0.095 | 48 | 7 |
| Claude | `anthropic/claude-opus-4.6` | 88/90 | 6/6 | 43/43 | ok | model_done | $2.012 | 110 | 24 |

**Batch OpenRouter spend (delta from baseline):** ~**$3.65** (manifest sum $3.654; credits API delta $3.654). Key limit $100 untouched; no top-ups.

**Common failures (88/90 models):** `ex_seconds_overflow_invalid_expire_time`, `px_absolute_overflow_invalid_expire_time` (accepted overflowing EX/PX with OK instead of `-ERR`).

**Artifacts:** `experiment/batch-v2/runs/b2-*-r1/`, `BATCH_V2_RESULTS.json`, `FREEZE.json`, `(withheld scoring oracle — not published)`.
