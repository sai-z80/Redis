# Pilot report — Arm A smoke infrastructure (Redis-mini, C → Go)

> **Post-run note (2026-11-06):** A scored contestant smoke **did** run afterward: `pilot-a-deepseek-v3.2-r2` → build_ok, smoke 6/6, hidden 40/43, billed **$0.169987**. See repository root `PILOT_RESULTS.md`. An earlier `r1` was aborted (~$0.012) and is not scored. Frozen reference validation is **repeats=1** (see `tests/REFERENCE_VALIDATION.json`); earlier authoring used 3 green repeats before the final re-freeze.
> The body below is the infrastructure report from authoring time; it is retained as a historical doc.

## Architecture (fairness boundary)

- **Claude Code authored fixed artefacts only:** `UPSTREAM.md`, the Arm A prompt (`prompts/arm-a/{system,port}.md`), the visible smoke test, the hidden suite, the harness code, the config, and these documents. Claude is not a supervisor, does not direct turns, does not change prompts based on model behaviour, does not repair outputs, does not judge, and does not run the contestant.
- **A deterministic script owns the run:** `harness/agent_run.py` runs the OpenRouter Chat Completions tool loop (`https://openrouter.ai/api/v1`), dispatches tools (`list_dir`, `read_file`, `write_file`, `run_shell`), enforces caps, and logs the transcript (`runs/<id>/transcript.jsonl`) and manifest. Caps live in one file (`harness/config.json`) and are the same for every model. Only `model_id` changes, and it is the only field left out of the frozen config hash.
- **Scoring is offline and deterministic:** `harness/score.py` builds the workspace and runs the frozen smoke and hidden suites. It never calls a model, and hidden results are never sent back to any model.
- **Sandbox:** `run_shell` (and scoring) run under bubblewrap: fresh namespaces with **no network** (loopback only), cleared env (no API key), read-only `/usr` + Go toolchain, the workspace at `/work`, and a Go cache. The pilot directory, `tests/hidden`, `REFERENCE_VALIDATION.json` and `FREEZE.json` are not visible. File tools reject absolute paths and paths that escape the workspace. On return or timeout, the harness kills every process the command started.
- **Cost:** a pre-call guard stops the run if spend so far plus a conservative projection of the next call would exceed `max_cost_usd`. Spend is the sum of OpenRouter `usage.cost`, falling back to estimate pricing if absent. After the run, billed cost is reconciled per generation via `/api/v1/generation?id=`.

## Frozen settings (`harness/config.json`)

| Setting | Value |
|---|---|
| model_id | `deepseek/deepseek-v3.2` (config field only) |
| max_iterations | 60 model turns |
| max_cost_usd | 1.50 (pilot safety ceiling 2.00; runner refuses a cap above the ceiling) |
| max_tokens | 8192 per call |
| wall_timeout_s | 3600 |
| temperature | null (provider default; not sent), so it works the same across providers that reject the parameter |
| shell timeout | 120 s default, 300 s max; tool output clipped to 20,000 chars |

These caps are pilot settings, not pre-registered values. Per the experiment design document (§5.4; not published in this repo), the final caps are set after the pilot.

## Tests

- **Visible smoke** (`tests/smoke/smoke.py`, 6 checks): PING, ECHO, SET, GET, DEL, GET-after-DEL.
- **Hidden** (`tests/hidden/run_hidden.py`, 43 tests): PING/ECHO/SET/GET/DEL, EX/PX lazy expiry, arity, unknown command, bad SET options, binary-safe keys/values (NUL, CRLF, 0xFF), a 512 KiB value, pipelining, split writes, and 20 concurrent clients. RESP2 over TCP to host:port. See `tests/hidden/README.md`.
- **Reference validation:** real `redis-server` 7.2.17 on loopback. During authoring the suite was run for **3 green repeats** (6/6 + 43/43 each). Immediately before the scored contestant smoke, validation was re-run once and `FREEZE.json` / `tests/REFERENCE_VALIDATION.json` were updated to record that final check: **repeats=1, smoke 6/6, hidden 43/43, all_green=true**. Cite the frozen JSON as authoritative; do not invent a frozen repeats=3.
- **Fix made before freeze (test side, not Redis):** (1) a bytes/str bug in the hidden runner's error matcher. (2) A fairness change: error phrases whose text is not in the provided upstream subset (`unknown command`, `syntax error`, `not an integer` live in `server.c` and `object.c`) are now checked only as `-ERR` class. Phrases present in `networking.c` (`wrong number of arguments`, `invalid expire time`) are still required. This was decided before any contestant run.
- **Discrimination check:** the harness self-test's PING-only stub server scores smoke 1/6, hidden 2/43, so the suite is not trivially passable.

## Harness self-test ($0, not a contestant run)

`python3 -m harness.selftest` drives `agent_run` against a **local fake endpoint** with scripted replies (`runs/selftest-20261106T195738/`, marked `test_mode: true`). All 16 checks pass: path escape blocked, API key absent from the sandbox, pilot dir and hidden tests invisible, network blocked, background processes cleaned up, cost summing and billed reconciliation, key redacted from the transcript, scorer build and partial pass counts.

## Freeze

`FREEZE.json` holds SHA-256 of SCOPE.md, both prompt files, smoke and hidden trees (per file and tree digest), config (minus secrets and model_id), upstream tree, harness `.py` files, caps, and the Redis pin (7.2.17, commit `83937c51afb53e1c69661c8e1533e0e19b896fe1`, tarball and reference binary hashes). It sets `tests_validated_on_reference: true`, `scored_runs_allowed: true` (allowed only now that the freeze exists and validation is green), and `contestant_runs_performed_at_freeze: 0`. `agent_run` refuses to start unless `FREEZE.json` verifies.

## Known limitations / caveats

- The commit hash could not be verified offline (release tarball, not git). See `UPSTREAM.md`.
- The reference binary is a manual gcc build (libc malloc) because `make` was unavailable. `--version` reports v=7.2.17.
- `server.c` is not in the upstream subset, so the exact Redis error text is not available to the agent. The tests allow for this (see above).
- The cost guard's fallback pricing is a conservative **estimate**, not a quote. Provider routing is not pinned (`provider_routing: null`). The serving provider appears in each logged response.
- n=1, one model, one arm: this is infrastructure, not a result.

## How to run the smoke later (one command)

```sh
cd experiment && python3 -m harness.agent_run --config harness/config.json --run-id pilot-a-deepseek-v3.2-r1 --confirm-spend && python3 -m harness.score --run-id pilot-a-deepseek-v3.2-r1
```
(Needs `OPENROUTER_API_KEY` in the environment. Spends real money up to the $1.50 cap. See `RUN.md`. Do not use Claude as a supervisor.)
