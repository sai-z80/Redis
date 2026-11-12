# PROTOCOL — batch-v2 (completion-oriented), pre-registered

Status: **pre-registered before any batch-v2 contestant run.** Frozen in `FREEZE.json`
(`protocol: batch-v2-completion-oriented`, `contestant_runs_performed_at_freeze: 0`).
Change source: Sai, t195u/t196u. Applies to Arm A, Redis-mini C → Go, scope `SCOPE.md` (unchanged from pilot-v1).

## 1. Cost is an outcome, not a stop condition

- No contestant is stopped because it spent money. There is **no per-run cost cap and no per-batch cost cap** in the
  scored path. The pilot-v1 `$1.50` per-run stop, the `$2.00` `pilot_safety_ceiling_usd` check and the pre-call cost
  projection stop are **removed** from `harness/agent_run.py` and from `harness/config.json`.
  `harness.freeze` refuses to freeze a config that still carries `max_cost_usd` or `pilot_safety_ceiling_usd`.
- Cost is still **recorded every turn** and reported per run (see §4).
- The task prompt no longer mentions a cost cap (new template `prompts/arm-a/port.batch-v2.md`; pilot-v1 prompt kept
  at `prompts/arm-a/port.md` and `history/port.pilot-v1.md`).

## 2. Uniform stop rules (identical for every model)

| # | `stop_reason` | `stop_class` | Meaning |
|---|---|---|---|
| 1 | `model_done` | `model_done` | Assistant message with **no tool calls** (the model finished). `last_finish_reason` is recorded. |
| 2 | `max_iterations` | `safeguard` | `caps.max_iterations` model turns reached (runaway tool-loop safeguard). |
| 3 | `wall_timeout` | `safeguard` | `caps.wall_timeout_s` reached. |
| 4 | `harness_api_failure` | `harness_failure` | Exhausted HTTP retries, non-retryable API error, sandbox crash or harness exception. Logged as a **harness/API failure, distinct from a model failure** (`is_harness_or_api_failure: true`; process exit 2). Per experiment-design §5.5 such runs are restarted and the failure is logged. |

A freeze that does not verify stops the runner **before** any model call (no run directory is created).

Caps (frozen, identical for all models): `max_iterations=150`, `wall_timeout_s=7200`, `max_tokens=8192` per call,
`temperature=null` (provider default). Tool limits unchanged from pilot-v1 (`shell_timeout` 120 s default / 300 s max,
20 000-char tool output, 200 kB read, 1 MB write).

**Finite safeguards do not guarantee completion.** A run can end by rule 2 or 3 with the model still working; that is
reported as such, not converted into a model failure or success.

## 3. Internal protective guard (not part of the published protocol)

`SF_BATCH_GUARD_USD` (environment variable, **default unset**, not in `config.json`, not hashed). If a supervisor sets
it and this process's cumulative spend plus the last call's cost would exceed it, the runner stops with
`stop_reason=batch_guard_notify` (`stop_class=internal_batch_guard`) and exits **3** so the supervisor can notify the
parent. It is an operational safeguard against runaway spend, **not** the experiment's budget and **not** a score cap.
A run ended this way is incomplete and is reported as guard-stopped, never scored as if it were a normal completion.

## 4. What every run logs (`runs/<id>/manifest.json`, `transcript.jsonl`)

Exact requested `model_id` (+ source: config or `--model`), provider-reported model ids and providers, tool settings,
request settings (max_tokens, temperature, provider routing, retries), caps, `stop_reason` / `stop_class` /
`stop_detail` / `last_finish_reason`, iterations, tool calls, wall time, per-call latency, tokens in / out / reasoning /
cached, OpenRouter `usage.cost` sum, estimated-cost component (only if `usage.cost` was absent), billed cost from the
`/generation` API (with missing ids), generation ids, workspace digests, and whether the visible suites were modified.

## 5. Test suites

| Suite | Path | Visible to agent? | Role |
|---|---|---|---|
| Smoke | `tests/smoke/` (6) | yes | secondary visible metric |
| Regression | `tests/regression/` (43) | **yes** (new in batch-v2) | secondary visible metric. This is the pilot-v1 hidden suite, published after pilot-v1. |
| Hidden | `tests/hidden/` (90, `oracle_batch_v2.py`) | **never** | **primary metric**: hidden pass rate. New suite covering the full `SCOPE.md`. |

- `prepare_workspace` copies `upstream/`, `tests/smoke/` and `tests/regression/` (identical bytes for every model) and
  aborts if any hidden/freeze material appears in the workspace.
- `harness.score` runs all three **frozen** suites from `tests/` (never from the workspace) offline in the no-network
  sandbox. Each suite gets a fresh server process on its own port; hidden runs first. The hidden oracle writes results
  incrementally, so a suite timeout still yields partial credit against the full denominator. Builds that fail score 0.
- Error policy unchanged: `-ERR` class always; phrase required only where verbatim in the supplied upstream subset
  (`wrong number of arguments`, `invalid expire time`).
- Validation on reference redis-server 7.2.17: smoke 6/6, regression 43/43, hidden 90/90, 3 repeats, all green
  (`tests/REFERENCE_VALIDATION_BATCH_V2.json`).
- The hidden suite was written for the full scope. The three pilot-v1 DeepSeek failure behaviours are covered within a
  broader oracle; no contestant output was run against the new suite during authoring.

## 6. Separation from pilot-v1

`pilot-a-deepseek-v3.2-r1` (aborted) and `pilot-a-deepseek-v3.2-r2` (scored 40/43 on the old hidden suite under a
$1.50 cap) are **pilot-v1, not batch-v2**. They are marked `PILOT_V1_NOT_BATCH_V2.txt`, are not re-scored
(`harness.score` refuses non-batch-v2 manifests), and must not be mixed into batch comparisons. The pilot-v1 freeze
hashes are historical (git/PR1; local copy `history/FREEZE.pilot-v1.json`).

## 7. Unchanged

Scope, upstream subset and pin, system prompt, tools and sandbox, and the rule that only `model_id` may vary between
models. No human or LLM supervises a contestant; the deterministic runner owns the loop; scoring is offline.
