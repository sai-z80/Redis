# Attribution

## Redis (BSD-3-Clause)

| Field | Value |
|---|---|
| Project | Redis |
| Version | 7.2.17 |
| Upstream commit | `83937c51afb53e1c69661c8e1533e0e19b896fe1` |
| Licence | BSD-3-Clause, `experiment/upstream/COPYING` (Copyright (c) 2006-2020, Salvatore Sanfilippo) |
| Release tarball SHA-256 (not committed) | `c627dc8da3ded4d68279b1832733e45e3d8a8d1b2f6d24c55ffd2251e48a57b3` |

Files redistributed unmodified under `experiment/upstream/`:

| File | SHA-256 |
|---|---|
| `COPYING` | `97f0a15b7bbae580d2609dad2e11f1956ae167be296ab60f4691ab9c30ee9828` |
| `src/networking.c` | `e7b410f85ceea68aebad04a43617886599adb97e1b15c65445e22c042508819c` |
| `src/server.h` | `7f6c4a8b9698f7af351dfb0310d17ca3e77239de9d219db738d16d9c0577e8c8` |
| `src/expire.c` | `34a2ec3fa54de2b29a51206639c01170991f63a8e6a4df519b739a0344631d3a` |
| `src/t_string.c` | `9fae89d4f0c7355612fe2e5daa8fa859d4df2d8f419fffa78719b07daac60b9f` |
| `src/db.c` | `a4410edfb7c538abc37f8cfb78e689d8bc111add25a40d61b0c8df7f0aed7793` |
| `src/version.h` | `4c2398da395c8b40930494827902192e76df1e3d64c732ab50c96924403272c1` |

Verification: each file was compared with `cmp` against the same path at commit `83937c51…` (shallow `git fetch` of that
commit) and against the extracted 7.2.17 release tarball. All matched. See `experiment/UPSTREAM.md`.

## Derived work: contestant Go port

`experiment/runs/pilot-a-deepseek-v3.2-r2/workspace/main.go`, `resp2.go` and `go.mod` were written by an LLM agent
(`deepseek/deepseek-v3.2`) inside the sandboxed harness. The agent had the Redis source subset above and was asked to port
it. The code is therefore a **derived port** of BSD-3-Clause Redis source and keeps that licence's notice requirement
(see `NOTICE`). It is published byte-for-byte as the agent left it. No human or other model edited it.

## Everything else

The harness, prompts, tests and documentation are original to this repository and are under the MIT `LICENSE`.
