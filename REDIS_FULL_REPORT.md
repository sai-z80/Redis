# Redis-mini C → Go: Arm A Experiment, Full Report

**Prepared for:** Sai, Software Forest
**Date:** 6 November 2026 (all times GMT / Europe/London; on this date London is on GMT, so `…Z` timestamps equal local time)
**Status:** exploratory, n=1 per configuration, Arm A only. No 8090 / Arm B run has been made.
**Published repository:** <https://github.com/sai-z80/Redis>. Pull request: <https://github.com/sai-z80/Redis/pull/1>
**Latest published commit:** [`89304210775864c0214b796692bcefd207eb6752`](https://github.com/sai-z80/Redis/commit/89304210775864c0214b796692bcefd207eb6752).
Its comparison document is [`FLAGSHIP_COMPARISON.md`](https://github.com/sai-z80/Redis/blob/89304210775864c0214b796692bcefd207eb6752/FLAGSHIP_COMPARISON.md).

> **Evidence note.** This report is based on the frozen run manifests, scores, transcripts and marker files, and on the
> published commit `8930421`. The remote commit and PR 1 were verified during publication. PR 1 is open for review; the
> branch has not been merged.

---

## 1. Executive summary

- **What was tested.** A deterministic harness gave one LLM at a time a 7-file subset of the Redis 7.2.17 C source. It
  asked the model, in a plain tool loop ("Arm A", the vibe-coding arm), to port a very small Redis to Go: RESP2 plus
  `PING`/`ECHO`/`SET`/`GET`/`DEL`, with `SET … EX|PX` and lazy expiry.
- **What happened.** 11 contestant runs were started. Nine finished and were scored, one was aborted during setup for QA,
  and one (GPT-5.5 Pro) was stopped by Sai and is **not scored**.
  - In the batch-v2 protocol, 8 runs were scored against a fresh withheld 90-test oracle. **Every one built, passed smoke
    6/6 and passed the visible regression suite 43/43.** Four scored 90/90 and four scored 88/90.
  - The 88/90 runs all failed the same two checks: overflowing `EX`/`PX` values were accepted with `+OK` instead of being
    rejected with `-ERR … invalid expire time`.
- **The task hit a ceiling.** It was too small to separate strong models. Spend per completed run ranged from **$0.095
  to $2.01**. GPT-5.5 Pro spent **$21.74019 in 24 completed turns** and had not finished when Sai stopped it.
- **A model-selection mistake was found and partly fixed.** The first multi-model slate (batch-v2) used older or cheaper
  IDs for three families: `deepseek/deepseek-v3.2`, `openai/gpt-5.4` and `anthropic/claude-opus-4.6`. It also used the
  provider's default reasoning and an 8192-token output limit. A pre-registered "flagship correction" re-ran Claude Opus
  5.5, DeepSeek V4 Pro and GPT-5.5 Pro at `reasoning_effort=high` with 65,536 output tokens. The correction is partial
  only because the GPT-5.5 Pro run was terminated before finishing, so GPT has no corrected scored row. Gemini, Grok and
  Kimi were reused rather than re-run; the correction is not partial because re-running them was forbidden. Their rows
  still use default reasoning and 8192 tokens, so they are not maximum-reasoning results.
- **This says nothing about factories.** Arm B (the 8090 / factory pipeline) was never run, so there is no factory vs.
  direct comparison. Because the task hit a ceiling, the results also **cannot** support "factories are unnecessary".
  A tiny, well-specified port with the source supplied is exactly where a one-shot tool loop should be at its best.
- **Recommended next step.** Design a realistic, enterprise-shaped task with room for models to fail, and run **Kimi, one
  run per arm (Arm A vs Arm B), $30 per arm**, as Sai has budgeted. Sai reports about $70 left on the account. That
  figure is not verified here. **No runs should start until the new project (task, oracle, freeze and factory setup) has
  been built and verified.**

---

## 2. Motivation: factory vs direct

The wider design (`experiment-design.md`, not published) asks whether a **software-factory pipeline** gives better
software than a **direct, one-shot "vibe" port** at the same budget. The pipeline runs requirements → blueprint → work
orders → build → verify, modelled on the 8090 Software Factory. The design also aims to build a dataset of where each
approach fails. The design document states the conflict of interest openly: Sai sells software-factory consulting, so
the project is only credible if Arm B is allowed to lose. That is the reason for pre-registration, automated withheld
tests and publishing every run.

The Redis work in this report is the **first rung**: it builds and debugs the Arm A half of the measuring equipment
(harness, sandbox, oracle, cost accounting, publication hygiene) on a deliberately small task. It was never meant to
answer the main research question, and it does not.

---

## 3. Task scope

Scope was frozen in `experiment/SCOPE.md` and kept the same through every protocol version. It is labelled a **port
attempt**, not a behaviour-only reimplementation, because the upstream C source is supplied.

**In scope**

- RESP2 over TCP: multibulk requests; simple string, error, integer, bulk and null-bulk replies.
- `PING`, `ECHO`, `SET`, `GET`, `DEL`.
- `SET key value [EX seconds | PX milliseconds]`.
- Lazy expiry only: an expired key is treated as missing, and removed, the next time it is accessed.
- Redis-style `-ERR` replies for wrong arity, unknown commands and bad `SET` options.
- A Go module serving TCP, with a `--port` flag defaulting to 6380.

**Explicitly excluded**

- Every other command (e.g. `EXISTS`, `INCR`, `EXPIRE`/`TTL`, `MGET`, `SELECT`, `FLUSHDB`).
- `SET` options `NX`, `XX`, `GET`, `KEEPTTL`, `EXAT`, `PXAT`.
- All non-string types, active expiry, persistence, replication, cluster, pub/sub, Lua, ACL, RESP3/`HELLO`,
  `MULTI`/`EXEC`, blocking commands, `CONFIG`/`CLIENT`/`INFO`, multiple databases, auth, TLS and performance work.

**Error-text policy** (`QA_DECISIONS.md`). The supplied subset leaves out `server.c` and `object.c`. Tests therefore
require only the `-ERR` class for unknown-command, syntax and not-an-integer errors. They require exact phrases only
where the text appears verbatim in the supplied files: `wrong number of arguments` and `invalid expire time`.

---

## 4. Upstream pin and licence

| Field | Value |
|---|---|
| Project / version | Redis **7.2.17** |
| Commit | `83937c51afb53e1c69661c8e1533e0e19b896fe1` |
| Licence | BSD-3-Clause (`experiment/upstream/COPYING`) |
| Tarball SHA-256 | `c627dc8d…48a57b3` (full value in `UPSTREAM.md`) |
| Reference binary | `redis-server v=7.2.17`, built from the tarball with gcc and libc malloc |
| Files given to agent | `COPYING`, `networking.c`, `server.h`, `expire.c`, `t_string.c`, `db.c`, `version.h` |

Each supplied file was checked byte-for-byte against both the tarball and the pinned commit (QA item 4). Contestant Go
code is a port derived from BSD-3 source and must keep the notice. The repository's own material is MIT
(`LICENSE`, `NOTICE`, `ATTRIBUTION.md`).

---

## 5. Timeline

| Time (GMT) | Event |
|---|---|
| 19:59:01 | Pilot-v1 tests validated on reference Redis: smoke 6/6, hidden 43/43 |
| 19:59:13–20:00:39 | `pilot-a-deepseek-v3.2-r1` started, then **aborted after 13 turns** for a QA pause on the fairness and source gates |
| 20:00:43–20:10:58 | `pilot-a-deepseek-v3.2-r2` ran ($1.50 cap protocol); scored 20:11:07 → 6/6, 40/43 |
| 20:01:11 / 20:17:02 | Initial commit `ba1f730`; pilot smoke published as `adabe7a` |
| — | Sai removes the per-run dollar cap: **cost becomes an outcome** (batch-v2 protocol) |
| 20:30:08 | Batch-v2 tests validated on reference Redis: smoke 6/6, regression 43/43, hidden 90/90, ×3 |
| 20:33:18–21:03:30 | Six batch-v2 runs, one after another (older/cheaper slate, 8192 tokens, default reasoning) |
| 21:19:22 | Flagship-correction freeze (`0eaf67ed…`) with 0 contestant runs at freeze |
| 21:21:28–21:24:27 | `b2f-claude-opus-5.5-r1` → 90/90 |
| 21:22:51 | Batch-v2 six published (`ee5daaa`) |
| 21:24:58–21:32:40 | `b2f-deepseek-v4-pro-0813-r1` → 90/90 |
| 21:33:16 | `b2f-gpt-5.5-pro-r1` starts (Responses API) |
| 21:55:53 | Opus 5.5 + DeepSeek V4 Pro published (`b3bd9aa`) |
| 22:04:47 | Last GPT transcript event (a `run_shell` tool result after turn 24) |
| ~22:06 | **Sai stops the GPT run** (`user_terminated`); process confirmed gone |
| 22:08:49 | Termination documented and corrected comparison published (`8930421`) |

### 5.1 The model-selection mistake

The batch-v2 selection file says the rule was "current flagship/capable per family". In practice, three choices did not
follow that rule:

- **DeepSeek:** `deepseek/deepseek-v3.2` was kept deliberately, for comparability with the pilot.
- **GPT:** `openai/gpt-5.4` was chosen although the selection notes record that "gpt-5.5/pro exist at higher price".
- **Claude:** `anthropic/claude-opus-4.6` was chosen although Opus 5.5 was available.

The result was a slate tilted toward older and cheaper models. It also ran without any reasoning-effort setting and with
an output limit (8192) that would starve a heavy reasoning model. The mistake was found after the six runs. It was
corrected by a **pre-registered amendment** rather than by editing or relabelling results. The six original rows stay in
the record under their real settings, and the corrected rows are added beside them. The correction was only partial
because the GPT-5.5 Pro run was terminated before it finished, leaving no corrected GPT row. Gemini, Grok and Kimi were
reused rather than re-run; the correction is not partial because re-running them was forbidden.

---

## 6. Method

### 6.1 Fairness boundary

- **A deterministic script owns the contestant loop.** `harness/agent_run.py` makes the API calls, dispatches the four
  tools (`list_dir`, `read_file`, `write_file`, `run_shell`), enforces stop rules and logs everything.
- **Claude Code only authored fixed artefacts** (scope, prompts, tests, harness, configs, docs), and wrote them before the
  runs. It did not supervise, coach, re-prompt or repair any contestant, and did not judge outputs subjectively. Hidden
  test results were never fed back into any model loop. Scoring is offline and calls no model.
- **Frozen inputs.** `agent_run` refuses to start unless `FREEZE.json` verifies. `model_id` is the only field excluded
  from the config hash. Per-model API and effort settings come from a frozen `api_profiles` table, and a CLI flag that
  contradicts it is refused before any request is sent.
- **Sandbox.** Tools and scoring run under bubblewrap (`--unshare-all --clearenv`): no network, no API key in the child
  process, and the workspace as the only writable view. Hidden tests never enter the workspace. Every scored manifest
  records that the visible smoke and regression suites were not modified.

### 6.2 Test suites and reference validation

| Suite | Size | Visible to agent | Role | Reference 7.2.17 |
|---|---|---|---|---|
| Smoke | 6 | yes | sanity | 6/6 |
| Pilot-v1 hidden | 43 | no (pilot) | pilot primary | 43/43 |
| Regression (= pilot hidden, now public) | 43 | **yes** (batch-v2) | secondary | 43/43 ×3 |
| Batch-v2 hidden oracle | 90 | **never** | primary | 90/90 ×3 |

The harness self-test includes a PING-only stub, which scores 1/6 smoke and 2/43 hidden, so the suites cannot be passed
trivially. The 90-test oracle covers the whole scope. Failing test **names** are published, but the oracle code is
withheld.

**Contamination caveat.** Withholding the 90 tests does **not** make the task contamination-free. Upstream Redis has a
large public test suite, and Redis source, docs and behaviour are almost certainly in every model's training data. This
repository also publishes the smoke suite and the 43-test suite. The results show how well models port familiar,
well-documented software. They are not a clean test of generalisation.

### 6.3 Exact configurations

| Setting | Pilot-v1 (r1, r2) | Batch-v2 `b2-*` | Flagship correction `b2f-*` |
|---|---|---|---|
| API | Chat Completions | Chat Completions | Chat; **Responses** for GPT-5.5 Pro |
| Output limit / call | 8192 | 8192 | **65,536** (`max_tokens` / `max_output_tokens`) |
| Reasoning effort | not sent | not sent (provider default) | **`high`** (`reasoning: {effort: "high"}`) |
| Per-call HTTP timeout | — | 300 s | **1800 s** |
| Max turns | 60 | 150 | 150 |
| Wall clock | 3600 s | 7200 s | 7200 s |
| Dollar cap | **$1.50/run** (+ $2 ceiling) | **none** (cost is an outcome) | none |
| Temperature | provider default | provider default | provider default |
| HTTP retries | — | 3 | 3 |
| Freeze SHA-256 | `c6cd46c8…` | `6816d6c4…f868` | `0eaf67ed…721b` |

Tool limits were the same in every protocol: shell timeout 120 s by default and 300 s maximum, 20,000-character tool
output, 200 kB reads and 1 MB writes. Provider routing was not pinned (`provider_routing: null`).

**What "high" and "default" mean here.** "High" is not the maximum setting: DeepSeek, for example, also documents a
`max` effort level. The reused Gemini, Grok and Kimi rows sent no effort setting, so they ran at **provider-default
reasoning**, which is also not a maximum-reasoning configuration.

**Why 1800 s.** The 1800 s per-call timeout was a flagged deviation from the brief. Calls were not streamed. A
high-effort GPT-5.5 Pro call could plausibly take more than 300 s, and a 300 s timeout would turn that into retries and a
false `harness_api_failure`. The 7200 s wall clock still applies to the whole run.

### 6.4 Cost policy

In pilot-v1, every model had the same $1.50 per-run cap. For batch-v2, **Sai removed that equal dollar cap.** Cost was
recorded on every turn but was never a stop condition. A run could end only by finishing (`model_done`), by reaching
150 turns, by reaching 7200 s, or by a harness/API failure.

A separate supervisor-only guard (`SF_BATCH_GUARD_USD=50`, set as an environment variable, not part of the config and not
a published cap) was in place and was never triggered. Finite safeguards do not guarantee completion, and the GPT-5.5 Pro
run shows this: it was still well inside both 150 turns and 7200 s when a person stopped it.

### 6.5 GPT-5.5 Pro and the Responses API

OpenAI lists Chat Completions as unsupported for GPT-5.5 Pro. Its `phase`-tagged assistant items must be replayed, so the
harness used OpenRouter's **Responses** API for this model. Two features of that API matter for cost:

- **Stateless:** OpenRouter rejects `store`/`previous_response_id`, so the harness **resends the whole history on every
  turn**. Input tokens therefore grow with the transcript. This is not unique to Responses: the Chat Completions loop
  used for every other model also resends the full history each turn.
- **Verbatim replay:** every output item is appended unchanged on the next turn, including reasoning items with
  `encrypted_content`, assistant messages with `phase`, and `function_call` items. Tool results are returned as
  `function_call_output`, paired by `call_id`.

The model, task, tools and stop rules were the same as for the other models. Only the transport differed.

---

## 7. Results

All costs are OpenRouter `usage.cost` sums from the run manifests. For every completed run, each generation ID was also
checked against the `/generation` billing API, and **no IDs were missing**. The two figures match exactly in every case.
Token counts are summed across turns.

### 7.1 Pilot-v1 (historical; old $1.50 cap and old 43-test hidden suite)

| Run | Model | Status | Turns | Wall (s) | In tok | Out tok | Cost (USD) | Score |
|---|---|---|---|---|---|---|---|---|
| `pilot-a-deepseek-v3.2-r1` | `deepseek/deepseek-v3.2` | aborted (QA pause) | 13 | ~86* | 72,377 | 973 | 0.01161656 | not scored |
| `pilot-a-deepseek-v3.2-r2` | `deepseek/deepseek-v3.2` | model stopped | 57 | 602.797 | 1,182,657 | 25,147 | 0.169987 | smoke 6/6, hidden 40/43 |

\*r1 has no manifest. Its transcript runs from 19:59:13 to 19:59:40, and the abort marker is timed 20:00:39. The token
figures above are summed from its transcript. The marker rounds the cost to $0.011617.

The three r2 failures were:

- `DEL` on an expired key returned 1 instead of 0.
- `EX 0` and negative `EX` returned the wrong error phrase.
- `EX` combined with `PX` was accepted.

Pilot-v1 runs are deliberately **not mixed** into the batch-v2 comparisons.

### 7.2 Batch-v2 six (Chat, 8192 tokens, default reasoning, 300 s timeout)

| Run | Model ID | Hidden /90 | Turns | Tools | Wall (s) | Cost (USD) |
|---|---|---|---|---|---|---|
| `b2-deepseek-v3.2-r1` | `deepseek/deepseek-v3.2` | 88 | 37 | 36 | 403.833 | 0.102251 |
| `b2-gemini-3.1-pro-r1` | `google/gemini-3.1-pro-preview` | 88 | 15 | 14 | 142.462 | 0.293585 |
| `b2-grok-4.7-r1` | `x-ai/grok-4.7` | **90** | 24 | 35 | 550.456 | 0.877332 |
| `b2-kimi-k3.1-pro-r1` | `moonshotai/kimi-k3.1-pro` | **90** | 19 | 29 | 413.317 | 0.273925 |
| `b2-gpt-5.4-r1` | `openai/gpt-5.4` | 88 | 7 | 9 | 47.766 | 0.095185 |
| `b2-claude-opus-4.6-r1` | `anthropic/claude-opus-4.6` | 88 | 24 | 33 | 109.622 | 2.01162 |

| Run | In tok | Out tok | Reasoning | Cached in | Provider(s) reported |
|---|---|---|---|---|---|
| deepseek-v3.2 | 708,928 | 20,247 | 0 | 690,496 | DeepInfra |
| gemini-3.1-pro | 185,762 | 13,282 | 9,266 | 131,846 | Google |
| grok-4.7 | 822,021 | 39,639 | 30,545 | 669,696 | xAI |
| kimi-k3.1-pro | 466,144 | 15,359 | 7,761 | 434,176 | InferenceNet, PrimaLabs |
| gpt-5.4 | 43,737 | 3,357 | 0 | 28,672 | OpenAI |
| claude-opus-4.6 | 372,959 | 5,873 | 0 | 0 | Claude Platform on AWS |
| **Total** | **2,599,551** | **97,757** | **47,572** | — | — |

All six runs stopped by `model_done` (`finish_reason=stop`), built successfully, passed smoke 6/6 and regression 43/43,
and left the visible suites unmodified. The manifests sum to **$3.653898**. The account-usage change across the batch was
$3.65389844, which matches to within $0.00000044.

### 7.3 Flagship correction (Chat, 65,536 tokens, effort `high`, 1800 s timeout)

| Run | Model ID | Hidden /90 | Turns | Tools | Wall (s) | Cost (USD) |
|---|---|---|---|---|---|---|
| `b2f-claude-opus-5.5-r1` | `anthropic/claude-opus-5.5` | **90** | 13 | 17 | 170.316 | 1.69238 |
| `b2f-deepseek-v4-pro-0813-r1` | `deepseek/deepseek-v4-pro-0813` | **90** | 42 | 46 | 454.719 | 0.480921 |

| Run | In tok | Out tok | Reasoning | Cached in | Provider(s) reported |
|---|---|---|---|---|---|
| opus-5.5 | 335,145 | 17,590 | 4,879 | 0 | Claude Platform on AWS |
| deepseek-v4-pro-0813 | 2,087,479 | 65,571 | 46,349 | 1,986,048 | CoreWeave, DigitalOcean |

Both runs stopped by `model_done`, built, scored smoke 6/6 and regression 43/43, and had no missing billing IDs. Their
combined cost is **$2.173301**. DeepSeek V4 Pro's output total (65,571) is summed over 42 turns, so it is not a single
call reaching the 65,536 limit.

### 7.4 GPT-5.5 Pro: user-terminated, incomplete, not scored

| Field | Value |
|---|---|
| Run / model | `b2f-gpt-5.5-pro-r1` / `openai/gpt-5.5-pro` |
| API / effort / output limit | Responses (`/api/v1/responses`) / `high` / `max_output_tokens=65536` |
| Started | 21:33:16 GMT |
| Last transcript event | 22:04:47 GMT (tool result after turn 24) |
| Stopped | **~22:06 GMT by Sai** (`user_terminated`), not by any stop rule |
| Completed turns | **24** |
| Cost of completed turns | **$21.74019** (sum of `usage.cost`) |
| Tokens (completed turns) | in **615,551** / out **18,187** / reasoning **11,607** |
| Score | **none.** No `scores.json` was produced and no pass rate should be stated. |

I re-summed the transcript independently and got the same 24 turns, cost and token totals as the termination marker.
The run used about 32 minutes of its 120-minute wall clock and 24 of its 150 turns.

**What the cost data shows**

- Per-turn latency ranged from 6.4 s to **644.3 s**, and per-turn cost from $0.05 to $1.67.
- Per-turn input grew from about 1k to about 44k tokens as the full history was resent each turn.
- Turn 14 included a provider `server_error` at 21:48:53 followed by a successful retry. Whether the failed attempt was
  billed is **unknown**.
- A request was probably in flight when the process was killed. If it is billed later, it is **not** included in
  $21.74019.

**The $21.74019 figure is exact only for the 24 completed generations. It is not the final cost of the run.**

**What the cost data does not show.** The partial workspace (`go.mod`, `main.go`) is published but was never scored. No
statement about GPT-5.5 Pro's ability on this task is supported.

### 7.5 Consolidated scored view (batch-v2 oracle)

| Model | Config | Hidden | Cost (USD) | Wall (s) |
|---|---|---|---|---|
| Claude Opus 5.5 | b2f: high / 65,536 | 90/90 | 1.69238 | 170 |
| DeepSeek V4 Pro 0813 | b2f: high / 65,536 | 90/90 | 0.480921 | 455 |
| Grok 4.7 | b2: default / 8192 | 90/90 | 0.877332 | 550 |
| Kimi K3.1 Pro | b2: default / 8192 | 90/90 | 0.273925 | 413 |
| Gemini 3.1 Pro preview | b2: default / 8192 | 88/90 | 0.293585 | 142 |
| GPT-5.4 | b2: default / 8192 | 88/90 | 0.095185 | 48 |
| Claude Opus 4.6 | b2: default / 8192 | 88/90 | 2.01162 | 110 |
| DeepSeek v3.2 | b2: default / 8192 | 88/90 | 0.102251 | 404 |
| GPT-5.5 Pro | b2f: high / 65,536 (Responses) | **not scored** | ≥21.74019 partial | ~1,890 active |

### 7.6 The two 88/90 failures

All four 88/90 runs failed exactly the same two tests:

- `ex_seconds_overflow_invalid_expire_time`
- `px_absolute_overflow_invalid_expire_time`

In both, the port answered `+OK` to an `EX` or `PX` value so large that converting it to milliseconds, or adding it to
the current time, overflows. Real Redis 7.2.17 rejects such values with `-ERR … invalid expire time`. This guard is in
the supplied `t_string.c`. It is a narrow edge case of the kind that ports miss when they translate the main path and
skip the defensive arithmetic. With n=1 it says nothing general about any model.

---

## 8. Spend accounting

The following are **known, separately sourced figures**. The account total is **not** a balance, and it is **not** the
cost of any single run.

| Item | Source | USD |
|---|---|---|
| Pilot r1 (aborted, 13 turns) | transcript `usage.cost` sum | 0.01161656 |
| Pilot r2 | manifest (billing-API verified) | 0.169987 |
| Batch-v2 six | manifests (billing-API verified) | 3.653898 |
| Flagship Opus 5.5 + DeepSeek V4 Pro | manifests (billing-API verified) | 2.173301 |
| GPT-5.5 Pro, 24 completed turns | transcript `usage.cost` sum | 21.74019 |
| **Sum of the known items** | arithmetic | **27.74899256** |
| OpenRouter key total usage at GPT publication | marker `account_total_usage_usd_at_publish_read` | 27.778810559 |
| **Unattributed difference** | arithmetic | **0.029817999** |

The unattributed $0.0298 breaks down by period as follows:

| Period | Unattributed (USD) |
|---|---|
| Before the batch-v2 baseline ($0.184841017 vs. pilot r1 + r2) | 0.003237457 |
| During batch-v2 (account change vs. manifests) | 0.00000044 |
| After batch-v2, up to GPT publication | 0.026580102 |

That difference may include probe or other calls on the same key, rounding, or partial billing of the GPT failed or
in-flight requests. **This report does not assign it to any of these.** In-flight GPT billing after 22:06 is still
unknown, and none has been added.

Rounded figures in the published docs (~$3.65, ~$2.17, ~$21.74, ~$27.78) agree with these values.

---

## 9. Limitations

- **n=1 per configuration.** There are no repeats, no confidence intervals and no variance estimates. Ranking any row
  above another is not justified.
- **Mixed configurations.** Rows differ in output limit, reasoning effort, API and timeout. Reused rows ran at default
  reasoning, and corrected rows at `high`. Neither is a maximum-reasoning configuration.
- **Ceiling effect.** Every scored batch-v2 run passed all 49 visible tests (6 smoke + 43 regression) and at least 88 of
  the 90 hidden tests, and four of eight passed everything. The task cannot separate strong models, and it cannot show
  where a factory would help.
- **No Arm B.** No 8090 or factory-lite arm was run, so no factory vs. direct claim can be made in either direction.
- **Contamination.** Redis is public and well documented, and it is almost certainly in training data. The 43-test
  suite and the smoke suite are public, and only the 90-test oracle code is private.
- **Cost comparability.**
  - Prompt caching varied widely: DeepSeek, Grok and Kimi report heavy cached input, while both Claude runs report none.
  - The providers behind each model were not pinned.
  - Both the Chat and Responses loops resend the full history every turn, so this is not specific to Responses.
    Caching, per-token prices and reasoning effort dominate the cost differences; the API shape alone does not explain
    GPT-5.5 Pro's cost.
  - Costs are therefore one routing on one evening, not prices per model.
- **Latency and wall time** include provider queueing and retries (for example, the 644 s GPT turn). They are not a
  measure of model speed.
- **Not production proof.** None of the ports was fuzzed, load-tested, reviewed for quality or fixed by a human. Passing
  90/90 on a five-command subset is not evidence of production readiness.
- **Process.** The pilot r1 abort and the slate correction were operator decisions made between runs. Both are recorded
  rather than hidden. Neither changed any scored run's inputs after it started.

---

## 10. Lessons

1. **Pin models and reasoning settings deliberately, and do it before any spend.** The batch-v2 slate drifted toward
   older or cheaper IDs and provider defaults. Next time, the pre-registration should list the exact model ID, effort
   level, output limit and API for every arm, with a written reason for each.
2. **Cost-as-outcome needs a person watching or an agreed guard.** Without a dollar cap, one premium model spent about
   3.7× the combined cost of the other eight scored runs ($21.74 vs. $5.83) without finishing. The $50 internal guard
   never triggered. The next protocol should state up front whether cost is an outcome, an equal budget, or both.
3. **Transport and pricing shape cost.** Both the stateless Chat and Responses loops resend the whole history, so input
   grows every turn for every model. Caching, per-token prices and reasoning effort dominate the cost differences; the
   Responses API alone does not explain GPT-5.5 Pro's cost. The transport, caching and effort settings, and their
   billing behaviour, should be part of the frozen design.
4. **Only a task with room to fail can tell arms apart.** Four of eight runs scoring 90/90, and the other four 88/90,
   measures the task, not the method.
5. **Publish withheld suites only after use, and replace them.** The pilot's 43 hidden tests became public regression
   tests, and a new withheld suite was needed. Plan the same rotation for the next task.
6. **What worked:** the freeze checks, profile-locked API and effort settings, the sandbox, verification of every
   billing ID, honest markers for the aborted and terminated runs, and sanitised publication. All of these carry over.

---

## 11. Proposed next experiment (proposal; nothing run)

**Goal.** Compare Arm A (direct tool loop) with Arm B (factory: requirements → blueprint → work orders → build → verify)
on a task large and ambiguous enough that one or both arms can fail in informative ways.

**Model and budget (Sai's decision).** Kimi, **one run per arm**, **$30 per arm** ($60 total).

- Kimi K3.1 Pro (`moonshotai/kimi-k3.1-pro`) is the Kimi ID in the existing evidence. It scored 90/90 for $0.273925 here.
  The exact ID and effort setting must be pinned and frozen when the new project is set up.
- Sai reports about **$70 remaining**. That balance is **user-reported and not verified** by this report. Any late GPT
  billing would reduce it. Confirm the balance before launch.
- $30 per arm reintroduces an **equal dollar budget**, which batch-v2 deliberately did not have. Pre-register whether
  reaching the budget ends a run and how such a run is scored. For example: score what exists, report it as
  "budget-stopped", and treat it as a result, not a harness failure.

**What a realistic enterprise-shaped task should have** (criteria; a specific task is still to be chosen)

- Several modules and some persistent state, such as a service with storage, authentication or validation, audit or
  idempotency rules, and a migration or integration boundary.
- A written requirements pack with realistic gaps and conflicts, so the requirements and blueprint stages have work to do.
- A fresh withheld oracle validated against a reference implementation before any contestant run. Include hard
  edge-case and failure-mode tests so the scores do not bunch at the ceiling.
- Secondary metrics from the design document: bugs by severity, human fix-up minutes against an acceptance bar, and code
  quality with the blinding protocol. Report cost and wall time for each arm.
- The same sandbox, freeze rules and fairness boundary. Claude authors only. A deterministic runner owns Arm A. Arm B's
  driver (8090 or factory-lite) and its prompts are frozen and published.

**Go/no-go.** **No contestant runs until the new project is built and verified.** That means:

- the task, oracle and reference validation are green;
- the freeze verifies;
- the Arm B pipeline is set up and has been dry-run at $0;
- the balance has been confirmed;
- the pre-registration (model ID, effort, API, caps, budget rule, metrics) has been committed.

Even then, n=1 per arm will give a **case study**, not a statistical result. It should be written up as one.

---

## 12. Links

- Repository: <https://github.com/sai-z80/Redis>
- PR 1: <https://github.com/sai-z80/Redis/pull/1>
- Commit `89304210775864c0214b796692bcefd207eb6752`:
  <https://github.com/sai-z80/Redis/commit/89304210775864c0214b796692bcefd207eb6752>
- Corrected comparison:
  [`FLAGSHIP_COMPARISON.md`](https://github.com/sai-z80/Redis/blob/89304210775864c0214b796692bcefd207eb6752/FLAGSHIP_COMPARISON.md)

---

## Appendix A. Artefact provenance (repository-relative)

Published repository (`sai-z80/Redis` at `8930421`):

| Evidence | Path |
|---|---|
| Scope, pin, QA, run commands | `experiment/SCOPE.md`, `experiment/UPSTREAM.md`, `experiment/QA_DECISIONS.md`, `experiment/RUN.md` |
| Pilot-v1 freeze and reference validation | `experiment/FREEZE.json`, `experiment/tests/REFERENCE_VALIDATION.json` |
| Pilot r2 run | `experiment/runs/pilot-a-deepseek-v3.2-r2/{manifest,scores}.json`, `transcript.jsonl`, `workspace/` |
| Pilot r1 abort marker | `experiment/runs/pilot-a-deepseek-v3.2-r1/ABORTED_SETUP_ONLY.json` |
| Batch-v2 protocol and freeze | `experiment/batch-v2/PROTOCOL_BATCH_V2.md`, `FREEZE.batch-v2-max8192.json`, `config.batch-v2-max8192.json` |
| Flagship amendment | `experiment/batch-v2/PROTOCOL_FLAGSHIP_CORRECTION.md` |
| Batch-v2 results | `BATCH_V2_RESULTS.md`, `experiment/batch-v2/BATCH_V2_RESULTS.json` |
| Each `b2-*` / `b2f-*` run | `experiment/batch-v2/runs/<run-id>/{manifest,scores}.json`, `transcript.jsonl`, `workspace/` |
| GPT termination | `experiment/batch-v2/runs/b2f-gpt-5.5-pro-r1/USER_TERMINATED.json` |
| Comparison | `FLAGSHIP_COMPARISON.md` |
| Test publication status | `PUBLIC_AND_WITHHELD_TESTS.md`, `experiment/batch-v2/TESTS_PUBLICATION_NOTICE.md`, `WITHHELD_ORACLE_NOT_INCLUDED.md` |
| Licensing | `LICENSE`, `NOTICE`, `ATTRIBUTION.md`, `experiment/upstream/COPYING` |

The local experiment tree (not published) is where the figures above were cross-checked. These paths are relative to the
experiment root:

- `pilot/runs/<run-id>/manifest.json`, `scores.json` and `transcript.jsonl`
- `pilot/runs/pilot-a-deepseek-v3.2-r1/transcript.jsonl`, used for the r1 token totals
- `pilot/batch_v2_spend_baseline.json`, `pilot/runs/batch_v2_supervisor.log` and `pilot/batch_v2_models.json`
- `pilot/FREEZE.json`, `pilot/harness/config.json`, `pilot/history/` and `pilot/FLAGSHIP_READY.md`
- `pilot/tests/REFERENCE_VALIDATION_BATCH_V2.json`, read for its counts only

Deliberately not used or reproduced here: credentials and environment files, the withheld 90-test oracle implementation,
raw operator and agent logs, and any client or third-party material.

## Appendix B. Verification notes

- I recomputed every completed run's cost, tokens, turns and wall time from its manifest. The batch-v2 and flagship
  figures match the published tables. For the six-run batch, the manifest sum ($3.653898) matches
  `BATCH_V2_RESULTS.json`.
- The GPT-5.5 Pro totals (24 turns, $21.74019, 615,551 / 18,187 / 11,607 tokens) were re-summed from the transcript and
  match `USER_TERMINATED.json`.
- The pilot r1 totals come from transcript usage records only. No billing-API check is recorded for r1.
- Run start and finish times come from manifests. The GPT stop time (~22:06) comes from the termination marker, which
  records it as approximate.
