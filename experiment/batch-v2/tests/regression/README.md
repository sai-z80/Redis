# Regression suite (VISIBLE in batch-v2)

`run_regression.py` is the pilot-v1 hidden suite (43 tests), published after pilot-v1. In batch-v2 it is
**visible** regression material: `harness.agent_run.prepare_workspace` copies `tests/smoke/` **and**
`tests/regression/` into every agent workspace, byte-identical for every model.

Provenance: pilot-v1 `tests/hidden/run_hidden.py` sha256
`b2aa78448f2a1824203036afb7d33085eb1940a36e09c4625570aa20e494df50`. Test functions are unchanged; only the module
docstring and the printed/JSON suite label (`hidden` -> `regression`) were edited so the file does not describe
itself as withheld.

Run: `python3 tests/regression/run_regression.py --port <port>` (server must be running).

Scoring: reported as a **secondary visible metric** only. The primary metric is the batch-v2 withheld suite in
`tests/hidden/`, which is never copied into a workspace.
