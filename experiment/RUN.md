# RUN.md — exact commands

> Published copy. Paths rewritten to repo-relative. The scored pilot run used run id `pilot-a-deepseek-v3.2-r2`; `-r1` below was aborted for QA before scoring (see `runs/pilot-a-deepseek-v3.2-r1/ABORTED_SETUP_ONLY.json`). `reference/bin/redis-server` is not in this repository; build it first (see `reference/README.md`) before step (a).

All commands run from the `experiment/` directory of this repository. Python 3 stdlib only; needs `go` and `bwrap` on the host.

> **Do not use Claude (or any LLM or human) as a supervisor of the contestant.** The deterministic script
> `harness/agent_run.py` owns the model loop, tool dispatch, budget caps and logging. Nobody watches the run,
> sends hints, edits the prompt, repairs output, or passes hidden-test results back to a model. Scoring
> (`harness/score.py`) is offline and automatic. If you change the prompt, tests, caps or harness, re-run (a)
> and re-freeze, and log the change. Only `model_id` in `harness/config.json` may differ between models.

## (a) Re-validate tests on the reference redis-server 7.2.17 (local, $0)

```sh
python3 -m harness.validate_reference --repeats 3   # must print all_green=True -> tests/REFERENCE_VALIDATION.json
python3 -m harness.freeze --verify                  # must print "ok": true
```
If you edit tests on purpose: re-run validation, then `python3 -m harness.freeze` to re-freeze.

Optional harness self-test (local fake endpoint, scripted replies, $0, not a model run):
```sh
python3 -m harness.selftest
```

## (b) One Arm A smoke run via the harness (SPENDS MONEY, cap $1.50, pilot ceiling $2)

```sh
export OPENROUTER_API_KEY=...        # set in your shell; never commit, print or paste it
python3 -m harness.agent_run --config harness/config.json --run-id pilot-a-deepseek-v3.2-r1 --confirm-spend
```
The runner refuses to start if FREEZE.json is missing or does not verify, or if `--confirm-spend` is missing.
Output goes to `runs/<run_id>/`: `workspace/` (the agent's view: `upstream/` + `tests/smoke/` only),
`transcript.jsonl`, and `manifest.json` (stop reason, tokens, `cost_usd_usage_sum`, `cost_usd_billed_generation_api`).
Afterwards, check spend on the OpenRouter dashboard as well.

## (c) Score (offline, $0, no model calls)

```sh
python3 -m harness.score --run-id pilot-a-deepseek-v3.2-r1
```
Writes `runs/<run_id>/scores.json`: build_ok, smoke x/6, hidden x/43, per-test results, and whether `tests/smoke`
was modified. Scoring copies the workspace, builds with `go build -o redis-mini .` in a no-network sandbox, starts
`./redis-mini --port 6380`, and runs the **frozen** smoke and hidden suites from `tests/` inside the same sandbox.
