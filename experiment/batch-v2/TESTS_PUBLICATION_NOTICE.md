# Tests publication notice (batch-v2)

## What is published here
- **Smoke (6)** and the **published regression suite (43)** — the former pilot hidden suite, now public and used as visible regression during batch-v2.
- Upstream Redis 7.2.17 source subset under `experiment/upstream/` (already in this repository from the pilot packaging). Redis itself ships a large public test suite upstream; this experiment does **not** claim that all Redis behaviours are secret.

## What is withheld
- The **fresh batch-v2 scoring oracle (90 tests)** used as the primary metric is **private** and is **not** included in this repository.
- Failure **names** may appear in comparison tables and `scores.json` (as previously shared). The oracle **implementation** is not published.
- Contaminaton note: contestants could in principle consult public Redis tests / docs / this repo’s published suites. Fresh-90 being private does **not** make the task contamination-free.

## Flagship correction status
Corrected flagship runs (`anthropic/claude-opus-5.5`, `openai/gpt-5.5-pro`, `deepseek/deepseek-v4-pro-0813`) are **in progress** and will be added in a follow-up commit when finished. Do not treat the current six-row batch-v2 table as the final flagship comparison.
