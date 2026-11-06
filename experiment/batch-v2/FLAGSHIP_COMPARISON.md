# Corrected flagship comparison (exploratory n=1) — truthful status

**Do not treat as a publication-grade ranking or a software-factory conclusion.** Arm A vibe only. No 8090 / Arm B.

## What this is
- **Older batch-v2 slate** (`b2-*`): exploratory six-model batch under `max_tokens=8192`, **provider-default reasoning** (not pinned), Chat Completions, 300s HTTP timeout. Includes non-current-flagship IDs (`deepseek-v3.2`, `gpt-5.4`, `claude-opus-4.6`).
- **Flagship correction** (`b2f-*`, amendment `flagship-correction-1`): intended current strong models with pinned `reasoning_effort=high`, `max_tokens`/`max_output_tokens=65536`, per-call HTTP timeout **1800s**, overall wall still **7200s**.
- Reused Gemini / Grok / Kimi rows keep their original default-reasoning configs — **not** re-run at max reasoning.
- Fresh-90 scoring oracle implementation is **withheld**. Upstream Redis tests are public; this repo publishes smoke + regression. Private oracle ≠ contamination-free.

## Provider table (corrected intent)

| Family | Model ID | Run | Hidden | Smoke | Reg | Build | Stop | Cost (USD) | Wall(s) | Iters | Config |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Claude | `anthropic/claude-opus-5.5` | `b2f-claude-opus-5.5-r1` | **90/90** | 6/6 | 43/43 | ok | model_done | **1.69238** | 170 | 13 | chat; reasoning_effort=**high**; max_tokens=65536; HTTP 1800s |
| DeepSeek | `deepseek/deepseek-v4-pro-0813` | `b2f-deepseek-v4-pro-0813-r1` | **90/90** | 6/6 | 43/43 | ok | model_done | **0.480921** | 455 | 42 | chat; reasoning_effort=**high**; max_tokens=65536; HTTP 1800s |
| GPT | `openai/gpt-5.5-pro` | `b2f-gpt-5.5-pro-r1` | **incomplete** | — | — | — | **user_terminated** (~22:06Z) | **partial 21.74019** (completed Responses turns only; may rise if in-flight billed later) | ~31+ min active | 24 model iters then kill | Responses API; reasoning_effort=**high**; max_output_tokens=65536; HTTP 1800s; **not scored** |
| Gemini | `google/gemini-3.1-pro-preview` | `b2-gemini-3.1-pro-r1` | 88/90 | 6/6 | 43/43 | ok | model_done | 0.293585 | 142 | 15 | **reused**; max_tokens=8192; **default reasoning**; chat; HTTP 300s |
| Grok | `x-ai/grok-4.7` | `b2-grok-4.7-r1` | 90/90 | 6/6 | 43/43 | ok | model_done | 0.877332 | 550 | 24 | **reused**; max_tokens=8192; **default reasoning**; chat; HTTP 300s |
| Kimi | `moonshotai/kimi-k3.1-pro` | `b2-kimi-k3.1-pro-r1` | 90/90 | 6/6 | 43/43 | ok | model_done | 0.273925 | 413 | 19 | **reused**; max_tokens=8192; **default reasoning**; chat; HTTP 300s |

### Historical non-flagship batch-v2 rows (same task; not current flagships)
See `BATCH_V2_RESULTS.md` / `experiment/batch-v2/BATCH_V2_RESULTS.json` for `deepseek/deepseek-v3.2`, `openai/gpt-5.4`, `anthropic/claude-opus-4.6` (all model_done; 88/90 on the two expire-overflow tests except where noted in that file).

## GPT termination (important)
- Status file: `experiment/batch-v2/runs/b2f-gpt-5.5-pro-r1/USER_TERMINATED.json`
- **Not** `model_done`. **No** `scores.json`. Do not invent a hidden pass rate.
- Partial cost **$21.74019** = sum of `usage.cost` on **24 completed** Responses turns in the transcript. OpenRouter **account** usage at publish (~$27.78) is a different number (whole key). An in-flight request at kill may bill later.

## Aggregate spend notes (approximate)
- Completed flagship billed (Opus+DeepSeek): ~$2.17
- GPT partial (completed turns): ~$21.74
- Earlier batch-v2 six: ~$3.65
- Probes / overhead / possible late GPT billing: see account usage vs sums; do not equate account total to one run.

## Links in-repo
- Batch-v2 six: `BATCH_V2_RESULTS.md`, `experiment/batch-v2/`
- Tests notice: `PUBLIC_AND_WITHHELD_TESTS.md`
- Protocol: `experiment/batch-v2/PROTOCOL_BATCH_V2.md`, `experiment/batch-v2/PROTOCOL_FLAGSHIP_CORRECTION.md`
