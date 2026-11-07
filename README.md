# Redis-mini C → Go port: Arm A pilot smoke

> **Pilot smoke only. Not publication-grade. No scientific conclusions.**
> n=1 · one model · **Arm A only** · **no Arm B / no 8090** · reduced scope · no fuzzing, benchmarks, quality review or human fix-up.
> Nothing in this repository supports a claim about one way of building software being better than another.

This repository holds the frozen inputs, harness, tests and one scored result of a small pilot. The pilot checks that an
"Arm A" harness works end to end. In Arm A, an LLM agent in a plain tool-calling loop is given a subset of the Redis 7.2.17
C source and asked to port a small part of it (RESP2 + `PING`/`ECHO`/`SET`/`GET`/`DEL` + `SET EX|PX` with lazy expiry) to Go.
A later, larger experiment would compare this with other ways of building the same software. That comparison has **not**
been run, and this repository says nothing about it.


<!-- BATCH_V2_SECTION -->
## Batch v2 multi-model comparison (exploratory, n=1)

See **[`BATCH_V2_RESULTS.md`](BATCH_V2_RESULTS.md)** and **[`experiment/batch-v2/`](experiment/batch-v2/)** for six completed Arm A vibe runs under protocol `batch-v2-completion-oriented`.

**Important methodology notes:**
- The completed slate is **not** the corrected current-flagship set (includes GPT-5.4 / Claude Opus 4.6 / DeepSeek v3.2). Flagship correction finished (partial): Claude Opus 5.5 and DeepSeek V4 Pro completed and were scored; GPT-5.5 Pro was terminated by the user after 24 completed turns ($21.74019) and is **not scored**. See [`FLAGSHIP_COMPARISON.md`](FLAGSHIP_COMPARISON.md) and [`REDIS_FULL_REPORT.md`](REDIS_FULL_REPORT.md) (full final analysis).
- Reused Gemini / Grok / Kimi rows used **default reasoning** and `max_tokens=8192` — do not treat as max-reasoning.
- **Tests:** upstream Redis tests are public; this repo already publishes smoke + a regression suite derived from the earlier pilot hidden suite. The **fresh batch-v2 90-test scoring oracle is withheld** (failure names may appear in tables). Private oracle ≠ contamination-free.
- n=1, Arm A only, no 8090. Cost is an outcome, not a stop.

## Important: this released test suite is now PUBLIC

The files under `experiment/tests/hidden/` were a withheld scoring oracle for the pilot run recorded here.
**Publishing this repository makes that suite public.** It may still be used as a **visible regression / smoke-adjacent** suite, but it must **not** be treated as a hidden scored oracle for any future experiment cell.
Future scored comparisons need a **newly withheld** test suite (new tests and a new freeze), validated again against the pinned Redis reference before any contestant run.


## Layout

| Path | What it is |
|---|---|
| `LICENSE` | MIT licence for this repository's own material (harness, tests, prompts, docs) |
| `NOTICE`, `ATTRIBUTION.md` | Redis BSD-3-Clause provenance, upstream pin, derived-work notes |
| `experiment/SCOPE.md` | frozen scope and explicit omissions |
| `experiment/UPSTREAM.md` | Redis pin, file list, SHA-256 hashes, commit verification note |
| `experiment/upstream/` | the Redis 7.2.17 source subset given to the agent, with `COPYING` (BSD-3-Clause) |
| `experiment/prompts/arm-a/` | frozen Arm A system and task prompts |
| `experiment/tests/smoke/` | visible smoke test (6 checks), copied into the agent workspace |
| `experiment/tests/hidden/` | hidden suite (43 tests). The agent never saw it; it ran only at scoring |
| `experiment/tests/REFERENCE_VALIDATION.json` | smoke and hidden results against real `redis-server` 7.2.17 |
| `experiment/harness/` | deterministic agent runner, sandbox, cost ledger, scorer, freeze, self-test |
| `experiment/FREEZE.json` | SHA-256 freeze of scope, prompts, tests, config (minus `model_id`), upstream, harness, caps |
| `experiment/reference/` | how to fetch and build the reference `redis-server` (binary not committed) |
| `experiment/runs/pilot-a-deepseek-v3.2-r2/` | the one scored run: manifest, scores, transcript, contestant Go sources |
| `experiment/runs/pilot-a-deepseek-v3.2-r1/` | abort marker only (QA pause during setup, not scored) |
| `experiment/RUN.md`, `PILOT_REPORT.md`, `QA_DECISIONS.md` | commands, freeze-time report, QA decisions |

## Fairness boundary

- **A deterministic script owns the contestant loop.** `experiment/harness/agent_run.py` runs the OpenRouter Chat
  Completions tool loop, dispatches four tools (`list_dir`, `read_file`, `write_file`, `run_shell`), enforces caps, and
  logs the transcript and manifest. No human or LLM supervised, hinted, steered, re-prompted or repaired the contestant.
- **Claude Code wrote only fixed artefacts** (prompts, tests, harness, config, docs), and did so before the run. It did not
  run, watch or judge the contestant.
- **Frozen inputs.** `agent_run` refuses to start unless `FREEZE.json` verifies. Only `model_id` may differ between models.
  Caps are the same for every model (`experiment/harness/config.json`).
- **Sandbox.** Tool commands and scoring run under bubblewrap with no network, a cleared environment (no API key), and the
  workspace as the only writable view. Hidden tests are never in the workspace.
- **Offline scoring.** `experiment/harness/score.py` builds and tests the workspace without calling any model. Hidden
  results were never fed back to any model.

## Upstream pin

Redis **7.2.17**, BSD-3-Clause, commit **`83937c51afb53e1c69661c8e1533e0e19b896fe1`**. The files in
`experiment/upstream/` were checked byte-for-byte (`cmp`) against that commit and against the 7.2.17 release tarball
(SHA-256 `c627dc8da3ded4d68279b1832733e45e3d8a8d1b2f6d24c55ffd2251e48a57b3`). See `experiment/UPSTREAM.md` and `ATTRIBUTION.md`.

## Running locally

Requirements: Linux, Python 3 (stdlib only), Go, `bwrap` (bubblewrap). Run everything from `experiment/`.

```sh
cd experiment
python3 -m harness.freeze --verify          # checks the frozen hashes; must print "ok": true ($0, offline)
python3 -m harness.selftest                 # harness self-test against a local fake endpoint ($0, no real model)
```

To re-validate the tests against real Redis, first build `reference/bin/redis-server` (see
`experiment/reference/README.md`), then run:

```sh
python3 -m harness.validate_reference --repeats 3   # expects all_green=True
```

Note: `validate_reference` overwrites `tests/REFERENCE_VALIDATION.json`. The committed file's SHA-256 is part of the
freeze, so a fresh validation file will make `freeze --verify` report a change even when every test passes. Keep the
committed copy (for example with `git checkout`) if you want to check the freeze afterwards.

## How the Arm A smoke works

1. `harness.agent_run` checks the freeze, creates `runs/<run_id>/workspace/` with only `upstream/` and `tests/smoke/`,
   and sends the frozen system and task prompts to the configured model through OpenRouter.
2. The model calls tools until it stops or reaches a cap (60 turns, $1.50, 8192 output tokens per call, 3600 s wall clock).
   Every request and response is written to `transcript.jsonl` with the API key redacted.
3. Cost is summed from OpenRouter `usage.cost`, then checked per generation id against the billing API (`manifest.json`).
4. `harness.score` copies the workspace, runs `go build -o redis-mini .` with no network, starts the server, and runs
   the frozen smoke suite (6) and hidden suite (43). It also checks that `tests/smoke/` was not changed.

A new run needs `OPENROUTER_API_KEY` in your environment and **costs real money**. See `experiment/RUN.md`. No key or
secret is stored in this repository; the config only names the environment variable (`api_key_env`).

## Result: `pilot-a-deepseek-v3.2-r2` (pilot smoke, n=1)

| Field | Value |
|---|---|
| Model id | `deepseek/deepseek-v3.2` (via OpenRouter, provider routing not pinned) |
| Stop reason | model stopped (`finish_reason=stop`) after 57 turns / 56 tool calls |
| Wall clock | ~603 s (602.8 s) |
| Tokens | 1,182,657 in / 25,147 out |
| Billed cost | ~$0.17 ($0.169987, every generation id checked against the billing API) |
| Freeze verified at start | yes |
| `build_ok` | true |
| Smoke tests changed by the agent | no |
| **Visible smoke** | **6/6** |
| **Hidden suite** | **40/43** |

The three hidden tests that failed:

- `del_on_expired_key_returns_zero`: `DEL` on a key whose TTL has passed returned `1`; Redis returns `0`.
- `ex_zero_or_negative_is_error`: `SET ... EX 0` or a negative value returned `ERR value is not an integer or out of range`
  instead of an error containing `invalid expire time`.
- `ex_and_px_together_is_syntax_error`: `SET k v EX .. PX ..` returned `+OK` instead of an `-ERR` reply.

These are the observed results of one run of one model on one prompt. They are not evidence about the model in general,
about other models, or about any other way of building software.

The earlier run `pilot-a-deepseek-v3.2-r1` was stopped during setup for a QA pause on the fairness and source gates
(13 turns, ~$0.012). It was **not scored** and is not an Arm A result. Only its abort marker is published.

Reference check: real `redis-server` 7.2.17 passes smoke 6/6 and hidden 43/43
(`experiment/tests/REFERENCE_VALIDATION.json`). The PING-only stub in the harness self-test scores 1/6 and 2/43, so the
suite is not trivially passable.

## What is not here

The Redis release tarball and reference binary, the full aborted r1 transcript and workspace, the contestant's compiled
binary and Go build cache, self-test run directories, internal working notes and agent logs, and the wider experiment
design document. See `ATTRIBUTION.md` for licensing.
