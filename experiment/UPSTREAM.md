# Upstream pin

> Published copy: `reference/redis-7.2.17.tar.gz`, `reference/src/` and `reference/bin/redis-server` are **not** committed to this repository. Fetch and rebuild them with `reference/README.md`; the SHA-256 values below let you check what you get.

| Field | Value |
|---|---|
| Project | Redis |
| Version | 7.2.17 (`src/version.h`: `REDIS_VERSION "7.2.17"`) |
| Commit (pin) | `83937c51afb53e1c69661c8e1533e0e19b896fe1` |
| Licence | BSD-3-Clause (`upstream/COPYING`, copyright Salvatore Sanfilippo et al.) |
| Source tarball | `reference/redis-7.2.17.tar.gz` |
| Tarball SHA-256 | `c627dc8da3ded4d68279b1832733e45e3d8a8d1b2f6d24c55ffd2251e48a57b3` |
| Reference binary | `reference/bin/redis-server` — `Redis server v=7.2.17 sha=00000000:0 malloc=libc bits=64 build=a995b17c51e5f94c` |
| Reference binary SHA-256 | `559705dce766662ad4b0f2aa89fcd970ca1ef7675be2941f1bea8f2bcf46ca92` |

Verification note: the tarball is a release archive, not a git checkout, so the commit hash could not be checked offline during authoring. It is recorded as the pin given in the brief. The files under `upstream/src/` are byte-identical to the same files in the extracted tarball (`reference/src/src/`), and checked with `cmp` at authoring time. The reference binary was built from that tarball with `reference/build_redis.sh` (a manual gcc build, because `make` was not available; libc malloc).

Redis 7.2.x is BSD-3-Clause. Redistribution of `upstream/` keeps `COPYING`. Contestant output is a port derived from BSD-3 source and must keep the notice.

## Files given to the agent (`upstream/`)

| File | SHA-256 | Why included |
|---|---|---|
| `upstream/COPYING` | `97f0a15b7bbae580d2609dad2e11f1956ae167be296ab60f4691ab9c30ee9828` | licence |
| `upstream/src/networking.c` | `e7b410f85ceea68aebad04a43617886599adb97e1b15c65445e22c042508819c` | RESP parsing and reply building |
| `upstream/src/server.h` | `7f6c4a8b9698f7af351dfb0310d17ca3e77239de9d219db738d16d9c0577e8c8` | core structs, command table types |
| `upstream/src/expire.c` | `34a2ec3fa54de2b29a51206639c01170991f63a8e6a4df519b739a0344631d3a` | expiry checks |
| `upstream/src/t_string.c` | `9fae89d4f0c7355612fe2e5daa8fa859d4df2d8f419fffa78719b07daac60b9f` | SET/GET and SET option parsing |
| `upstream/src/db.c` | `a4410edfb7c538abc37f8cfb78e689d8bc111add25a40d61b0c8df7f0aed7793` | keyspace lookup, DEL, lazy expiry (expireIfNeeded) |
| `upstream/src/version.h` | `4c2398da395c8b40930494827902192e76df1e3d64c732ab50c96924403272c1` | version string |

Tree digest (`harness.common.tree_digest(upstream)`), also in `FREEZE.json`: `9d4ca19b951ea9d27f7c4e68009f9c4620d9fae2a4348143782f48fa11a3b04e`

Note: `server.c` (which holds the command table and the exact `unknown command` / `wrong number of arguments` error text) is **not** in the subset. That is a fixed authoring choice made before any run. The hidden suite checks error *classes* and key phrases, not the full Redis error text.

## Commit verification (post-authoring QA, 2026-11-06)

Verified offline against git: `git fetch --depth 1 origin 83937c51afb53e1c69661c8e1533e0e19b896fe1` then `cmp` of every file under `upstream/` against that commit. **All matched** (`COPYING`, `src/networking.c`, `src/server.h`, `src/expire.c`, `src/t_string.c`, `src/db.c`, `src/version.h`). Tarball SHA-256 `c627dc8da3ded4d68279b1832733e45e3d8a8d1b2f6d24c55ffd2251e48a57b3` unchanged. This note does not alter frozen upstream bytes.
