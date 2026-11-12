# Public vs withheld tests

- **Upstream Redis** ships a large **public** test suite; this experiment’s `experiment/upstream/` is a source subset pin, not a claim of secret Redis behaviour.
- This repository already publishes **smoke** and a **regression** suite (the former pilot hidden suite).
- The **fresh batch-v2 primary scoring oracle (90 tests)** used for the multi-model comparison is **withheld** and is not in this tree. Tables may list failing test **names**; they do not include the oracle implementation.
- Contaminaton / leakage: models can consult public Redis tests, docs, and this repo’s published suites. Withholding fresh-90 does **not** make the task contamination-free.
