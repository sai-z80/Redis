# QA decisions resolved (executor, 2026-11-06) — not Sai audit items

Against the experiment design document (not published here) + fairness boundary. No publication claims.

## 1. Error-message loosening — ACCEPT for this pilot
Hidden suite checks `-ERR` class for unknown-command / syntax / not-an-integer because those exact strings live in `server.c`/`object.c`, which are **not** in the supplied upstream subset. Phrases present in supplied `networking.c` (`wrong number of arguments`, `invalid expire time`) remain required. Same oracle for every model; frozen before any scored run. Full Ep1 can add `server.c` later if exact Redis error text becomes in-scope.

## 2. `model_id` excluded from freeze hash — ACCEPT
Only field intended to vary across models. Caps, prompts, tests, harness code remain hashed. Changing a cap fails freeze verify.

## 3. `scored_runs_allowed=true` after freeze — ACCEPT
Means “freeze exists and tests validated”; spend still gated by `--confirm-spend` + freeze verify + `$1.50` cap / `$2` ceiling. Naming is slightly soft but behaviour is correct.

## 4. Upstream commit previously unverified — RESOLVED
Git fetch of `83937c51…` + byte `cmp` of all `upstream/` files: **match**. Documented in `UPSTREAM.md`. Licence BSD-3-Clause via `COPYING` present in agent workspace. Label remains **port attempt** (source supplied), not behaviour-only reimplementation.

## Other gates
- Deterministic `harness/agent_run.py` owns tool loop; Claude does not supervise contestant.
- Isolation: bubblewrap `--unshare-all --clearenv`, no API keys in child, hidden tests not in workspace.
- Budget: `max_cost_usd=1.50`, `pilot_safety_ceiling_usd=2.00`, pre-call projection stop.
- Tests validated on reference redis-server 7.2.17 (6/6 + 43/43) and frozen before scored run.

## Aborted partial
`runs/pilot-a-deepseek-v3.2-r1/` killed mid-run for this QA; marked `ABORTED_SETUP_ONLY`. Not scored.
