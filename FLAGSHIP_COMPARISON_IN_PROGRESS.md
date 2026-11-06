# Flagship-corrected comparison — **IN PROGRESS** (exploratory n=1)

**Status at publish:** Claude Opus 5.5 and DeepSeek V4 Pro **complete**. GPT-5.5 Pro Responses run **still in progress** — do not treat this table as final.

Protocol: `batch-v2-completion-oriented` + amendment `flagship-correction-1` for new `b2f-*` runs. Cost is an outcome. Overall wall **7200s** unchanged; corrected runs use **1800s** per-call HTTP timeout (reused rows used 300s).

## Corrected / reused provider rows

| Family | Model ID | Hidden | Smoke | Reg | Build | Cost | Wall(s) | Iters | Config note |
|---|---|---|---|---|---|---|---|---|---|
| Gemini | `google/gemini-3.1-pro-preview` | 88/90 | 6/6 | 43/43 | ok | $0.294 | 142 | 15 | reused; max_tokens=8192; default reasoning; chat; 300s HTTP |
| Grok | `x-ai/grok-4.7` | 90/90 | 6/6 | 43/43 | ok | $0.877 | 550 | 24 | reused; max_tokens=8192; default reasoning; chat; 300s HTTP |
| Kimi | `moonshotai/kimi-k3.1-pro` | 90/90 | 6/6 | 43/43 | ok | $0.274 | 413 | 19 | reused; max_tokens=8192; default reasoning; chat; 300s HTTP |
| Claude | `anthropic/claude-opus-5.5` | 90/90 | 6/6 | 43/43 | ok | $1.692 | 170 | 13 | NEW; max_tokens=65536; reasoning_effort=high; api=chat; HTTP timeout 1800s; wall still 7200s |
| DeepSeek | `deepseek/deepseek-v4-pro-0813` | 90/90 | 6/6 | 43/43 | ok | $0.480921 | 455 | 42 | NEW; max_tokens=65536; reasoning_effort=high; api=chat; HTTP timeout 1800s; wall still 7200s |

| GPT | `openai/gpt-5.5-pro` | **pending** | — | — | — | — | — | — | Responses API; reasoning_effort=high; max_output_tokens=65536; run `b2f-gpt-5.5-pro-r1` active |

### Honest caveats
- Reused Gemini/Grok/Kimi are **not** max-reasoning and used `max_tokens=8192`. New flagships use pinned `high` + 65536.
- Previous slate rows (`deepseek-v3.2`, `gpt-5.4`, `claude-opus-4.6`) remain under `experiment/batch-v2/runs/b2-*` for history; they are **not** current flagships.
- Fresh-90 oracle implementation **withheld**. Upstream Redis tests are public; published smoke+regression exist. Private oracle ≠ contamination-free.
- n=1 Arm A only; no 8090.

### Completed new-run billed costs (so far)
- `anthropic/claude-opus-5.5`: **$1.69238** (90/90)
- `deepseek/deepseek-v4-pro-0813`: **$0.480921** (90/90)
- GPT-5.5 Pro: in progress (live Responses verified before full run; no model substitution)
