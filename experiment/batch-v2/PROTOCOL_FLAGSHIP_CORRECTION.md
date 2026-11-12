# PROTOCOL AMENDMENT — flagship-correction-1 (batch-v2, Arm A)

Status: **pre-registered before any corrected-flagship contestant run.** Base protocol: `PROTOCOL_BATCH_V2.md`
(`batch-v2-completion-oriented`). This amendment is recorded in `harness/config.json`
(`protocol_amendment: "flagship-correction-1"`) and in `FREEZE.json` (`protocol_amendment`, plus a hash of this file).
Authoring made **no contestant calls to OpenRouter** (only the public, unauthenticated `GET /api/v1/models`
catalog and the public docs were read to confirm model ids and API shapes).

## 1. Why

The batch-v2 freeze used `max_tokens=8192` per call and no reasoning-effort pin. That was fine for the six
batch-v2 models (all ended by `model_done`), but it is wrong for the corrected flagship set:

- `openai/gpt-5.5-pro`: max output 128K and heavy default reasoning. 8192 output tokens (reasoning included)
  would starve it. OpenAI lists Chat Completions as unsupported for this model, and OpenRouter documents that
  `phase` on assistant messages (GPT-5.3-codex and later) must be preserved and resent, which needs the
  **Responses API**. It is **not** replaced by `openai/gpt-5.5` (non-pro).
- `anthropic/claude-opus-5.5` and `deepseek/deepseek-v4-pro-0813`: current flagships, run with reasoning
  effort pinned to `high` (DeepSeek documents low/high/max for agentic use; `high` chosen).

## 2. What changes (corrected-flagship runs only: run ids `b2f-*`)

| Setting | batch-v2 (`b2-*`) | flagship-correction-1 (`b2f-*`) |
|---|---|---|
| `caps.max_tokens` | 8192 | **65536** (uniform for all three; chat → `max_tokens`, responses → `max_output_tokens`) |
| Reasoning effort | not sent (provider default) | **`high`**, sent as `reasoning: {"effort": "high"}` |
| API | Chat Completions | Chat Completions (Opus 5.5, DeepSeek V4 Pro); **Responses** (GPT-5.5 Pro) |
| `openrouter.request_timeout_s` (per HTTP call) | 300 | **1800** (see §2.1) |
| Config hash / freeze | `history/FREEZE.batch-v2-max8192.json` | new `FREEZE.json` |

Frozen `api_profiles` in `harness/config.json`:

| Model | api | reasoning_effort |
|---|---|---|
| `anthropic/claude-opus-5.5` | chat | high |
| `deepseek/deepseek-v4-pro-0813` | chat | high |
| `openai/gpt-5.5-pro` | **responses** | high |

Models not in `api_profiles` use chat and `caps.reasoning_effort` (`null` = no `reasoning` field sent, i.e. the
batch-v2 request shape apart from `max_tokens`). The CLI flags `--api` and `--reasoning-effort` may repeat or fill
in a profile value but **may not contradict a profile**: e.g. `--model openai/gpt-5.5-pro --api chat` is refused
before any request is made, so there is no silent downselect or API fallback. Every manifest logs `api`,
`api_source`, `api_endpoint_path`, `reasoning_effort`, `reasoning_effort_source`, `request_settings`
(`max_tokens_field`, `max_tokens`, `reasoning`, `responses_include`) and `protocol_amendment`.

### 2.1 Deviation from the brief: per-call HTTP timeout 300 → 1800 s

Not in the brief; added during authoring, flagged here. Calls are non-streaming. A GPT-5.5 Pro call at high effort
with up to 65 536 output tokens can plausibly run longer than 300 s. With a 300 s client timeout such a call would
be cut off, retried (possibly billed again) and, after 3 retries, end the run as `harness_api_failure`: a
harness artefact, not model behaviour. The timeout is a transport setting, not a stop rule. The 7200 s wall clock
still binds: each call's timeout is `min(request_timeout_s, remaining wall time)`, as before.

## 3. What does not change

Scope (`SCOPE.md`), upstream subset and pin, system prompt and Arm A task prompt (`prompts/arm-a/port.batch-v2.md`),
tool schemas and semantics (`list_dir`, `read_file`, `write_file`, `run_shell`; same limits, same bwrap sandbox),
`prepare_workspace` (upstream + smoke + regression visible, hidden withheld), the three test suites and the scorer,
the stop rules (model_done / `max_iterations=150` / `wall_timeout_s=7200` / harness_api_failure), `temperature=null`,
cost recorded but **never a stop**, and the internal `SF_BATCH_GUARD_USD` supervisor guard (exit 3).

## 4. Reused batch-v2 runs (stated honestly)

`b2-gemini-3.1-pro-r1` (`google/gemini-3.1-pro-preview`), `b2-grok-4.7-r1` (`x-ai/grok-4.7`) and
`b2-kimi-k3.1-pro-r1` (`moonshotai/kimi-k3.1-pro`) are **reused as-is**. They ran under the superseded freeze
(`freeze_sha256 6816d6c4…f868`, archived at `history/FREEZE.batch-v2-max8192.json`) with **`max_tokens=8192`,
default reasoning (no effort pin), Chat Completions and a 300 s per-call timeout**. They are not re-run, re-labelled
or erased. All six `b2-*-r1` results stay valid under those settings. A comparison that mixes `b2-*` and `b2f-*`
rows must show both settings columns (max_tokens, reasoning effort, API). The superseded `b2-*` runs for
`openai/gpt-5.4`, `anthropic/claude-opus-4.6` and `deepseek/deepseek-v3.2` also stay as recorded, and the
corrected flagships do not overwrite them.

## 5. API notes

### Chat Completions (`POST /api/v1/chat/completions`): Opus 5.5, DeepSeek V4 Pro
Same request as batch-v2 (`messages`, `tools`, `tool_choice: "auto"`, `max_tokens`, `usage: {include: true}`),
plus `reasoning: {"effort": "high"}`. Assistant `tool_calls` and `reasoning_details` are replayed unchanged on the
next turn (Anthropic thinking signatures survive), and tool results go back as `role: "tool"` messages.

### Responses (`POST /api/v1/responses`): GPT-5.5 Pro
OpenRouter's Responses API is **stateless**: `store: true` and `previous_response_id` are rejected with a 400, so
the harness never sends them and resends the whole history every turn.

- Request: `model`, `instructions` (= the same system prompt text), `input`, `tools`, `tool_choice: "auto"`,
  `max_output_tokens: 65536`, `reasoning: {"effort": "high"}`, `include: ["reasoning.encrypted_content"]`.
- Initial `input`: one `{"type":"message","role":"user","content":[{"type":"input_text","text": <task prompt>}]}`.
- Tools: the same four schemas, flattened to `{"type":"function","name","description","parameters"}` (identical
  names, descriptions and JSON schemas).
- After every response, **every `output` item is appended to `input` verbatim** (deep copy, nothing dropped or
  rewritten): `reasoning` items (with `encrypted_content`, `summary`), assistant `message` items (with `id`,
  `status`, `phase` = `commentary` / `final_answer`, `output_text` content), and `function_call` items. This
  follows the GPT-5.4+ migration guidance: preserve `phase` and keep assistant `id`/`status`, which OpenRouter
  requires on replayed assistant messages.
- Each `function_call` is run through the same `Tools.dispatch(name, arguments)` as the chat path. Its result is
  appended as `{"type":"function_call_output","call_id": <call_id>, "output": <result string>}`, in call order.
- Stop rule 1 (`model_done`): a response with no `function_call` items. `last_finish_reason` = `status`
  (+ `:incomplete_details.reason`, e.g. `incomplete:max_output_tokens`).
- API failure: HTTP error, or a body with `error`, `status: "failed"`, or no `output` list. These are retried and
  classified exactly like the chat path.
- Accounting: `usage.input_tokens` / `output_tokens` / `output_tokens_details.reasoning_tokens` /
  `input_tokens_details.cached_tokens` / `usage.cost`. Billed-cost reconciliation through `/generation?id=` is
  best effort for both APIs. Response ids that `/generation` cannot resolve are listed in `billed_missing_ids`,
  and `cost_usd_usage_sum` stays the primary cost figure.

## 6. Verification ($0)

- `python3 -m harness.selftest` uses a local fake endpoint and makes no OpenRouter calls. It checks the whole
  batch-v2 test set plus request bodies: `max_tokens=65536`, no `reasoning` field when unpinned, and the effort pin
  when pinned. For the Responses path, resolved from the profile for `openai/gpt-5.5-pro`, it checks the endpoint,
  body shape, identical tools, verbatim replay of output items including `phase` and `encrypted_content`,
  `function_call_output` pairing by `call_id` with the exact `Tools.dispatch` results, `model_done`, token and
  cost accounting, and that a contradicting `--api chat` is refused before any request.
- `python3 -m harness.freeze --verify` must return `"ok": true`.
