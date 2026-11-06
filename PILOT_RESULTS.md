# Pilot results (factual record)

**Label:** pilot smoke only. n=1, Arm A only, one model. **Not** a publication-grade scientific conclusion about vibe coding vs software factories. No Arm B / no 8090.

## Scored run: `pilot-a-deepseek-v3.2-r2`

| Field | Value |
|---|---|
| Model | `deepseek/deepseek-v3.2` (OpenRouter; provider DeepInfra on this run) |
| Arm | A (autonomous tool-using agent; deterministic harness) |
| Stop reason | `model_stopped (finish_reason=stop)` |
| Iterations / tool calls | 57 / 56 |
| Wall time | 602.797 s |
| Tokens in / out | 1,182,657 / 25,147 |
| Cost (usage sum) | **$0.169987** |
| Cost (OpenRouter generation API billed) | **$0.169987** |
| Build | **ok** (`go build -o redis-mini .`) |
| Visible smoke | **6/6** |
| Hidden (historical pilot oracle) | **40/43** (pass rate 0.9302) |
| Smoke tests unmodified by agent | true |

### Hidden failures (3)

1. `del_on_expired_key_returns_zero` — want int 0, got int 1
2. `ex_zero_or_negative_is_error` — error text lacked required phrase `invalid expire time` (got `ERR value is not an integer or out of range`)
3. `ex_and_px_together_is_syntax_error` — want `-ERR`, got simple `OK`

## Aborted setup run (not scored): `pilot-a-deepseek-v3.2-r1`

Paused during fairness/source QA. Partial usage sum **$0.011617**. See `experiment/runs/pilot-a-deepseek-v3.2-r1/ABORTED_SETUP_ONLY.json`.

## OpenRouter account usage after pilot

Total usage reported by OpenRouter credits API after these runs: **~$0.1848** (aborted r1 + scored r2). Pilot safety ceiling was $2.00; runner cap $1.50.

## Reference validation (frozen)

Authoritative file: `experiment/tests/REFERENCE_VALIDATION.json` and `experiment/FREEZE.json`.
Final freeze records **repeats=1**, smoke **6/6**, hidden **43/43**, `all_green=true` on Redis server v=7.2.17.
(Authoring earlier exercised 3 green repeats before a final re-validate/re-freeze; the frozen JSON is the record that matters.)
