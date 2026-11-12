# Pilot: Arm A smoke (Redis-mini, C → Go, one model)

**What this is:** a tiny smoke pilot of a larger experiment design (the design document is not published in this repo). It checks that the Arm A harness works end to end: a real tool-calling agent loop through OpenRouter, sandboxed tool execution, cost capture (estimated and provider-billed), frozen inputs, visible smoke tests, and an offline hidden test suite.

**What this is not:** a result. One model, one run (n=1), reduced scope, no Arm B, no 8090, no fuzzing, no benchmarks, no quality review, no human fix-up phase. Nothing here supports a claim about vibe coding vs software factories.

| Path | Purpose |
|---|---|
| `SCOPE.md` | frozen scope and explicit omissions |
| `UPSTREAM.md` | Redis pin, licence, file list, SHA-256 hashes |
| `upstream/` | Redis 7.2.17 source subset + `COPYING` (BSD-3-Clause) |
| `prompts/arm-a/port.md` | frozen Arm A prompt |
| `tests/smoke/` | visible smoke test (copied into the agent workspace) |
| `tests/hidden/` | hidden suite; never copied into the agent workspace; run only at scoring |
| `harness/` | agent runner, sandbox, cost ledger, scorer |
| `reference/` | build script for a real redis-server 7.2.17 from the pinned release, used only to validate the hidden suite (binary and tarball not committed; see `reference/README.md`) |
| `runs/<run_id>/` | contestant Go sources, transcript, manifest, scores (published: `pilot-a-deepseek-v3.2-r2` only, plus the r1 abort marker) |
| `FREEZE.json` | hashes of scope/prompt/tests + model + caps |
| `PILOT_REPORT.md` | human summary |
| `RUN.md` | exact commands: re-validate, run, score |
| `tests/REFERENCE_VALIDATION.json` | hidden + smoke results on reference redis-server 7.2.17 |
