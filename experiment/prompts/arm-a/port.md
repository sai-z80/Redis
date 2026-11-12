# Task: port a subset of Redis from C to Go

Your workspace (the working directory of every tool) contains:

- `upstream/` — a subset of the Redis 7.2.17 C source code (`upstream/src/`) and its licence (`upstream/COPYING`, BSD-3-Clause).
- `tests/smoke/smoke.py` — a small visible smoke test (Python 3, standard library only).

Port the scope below from the C source in `upstream/` to Go. The scope is fixed:

## In scope
- RESP2 protocol over TCP (multibulk requests from clients; simple string, error, integer, bulk string and null bulk replies).
- Commands: `PING`, `ECHO`, `SET`, `GET`, `DEL`.
- `SET key value [EX seconds | PX milliseconds]`.
- Lazy expiry only: an expired key is treated as missing (and removed) when it is next accessed.
- Error replies for wrong arity, unknown commands and bad `SET` options, in Redis style (`-ERR ...`).
- A Go module whose server listens on TCP; port configurable with a `--port` flag, default `6380`.

## Explicitly out of scope
- Every other command, including EXISTS, INCR/DECR/INCRBY, APPEND, STRLEN, MGET/MSET, EXPIRE/PEXPIRE/TTL/PTTL/PERSIST, SELECT, FLUSHDB, DBSIZE, TYPE.
- SET options NX, XX, GET, KEEPTTL, EXAT, PXAT.
- Lists, hashes, sets, sorted sets, streams.
- Active expiry sweep (lazy expiry only).
- Persistence (RDB/AOF), replication, cluster, pub/sub, Lua, ACL, RESP3/HELLO, MULTI/EXEC, blocking operations, CONFIG, CLIENT, INFO, DEBUG, OBJECT.
- Multiple databases, authentication, TLS, performance tuning.

## Deliverable

- A Go module at the workspace root. Running `go build -o redis-mini .` in the workspace root must produce the server binary.
- The server speaks RESP2 over TCP and accepts a `--port` flag (default `6380`). It must accept connections on `127.0.0.1`.
- Use the Go standard library only. There is no network access in the sandbox.
- Do not modify `upstream/` or `tests/smoke/`.

## How to check your work

The visible smoke test is: `python3 tests/smoke/smoke.py --port <port>`. Background processes are killed when each `run_shell` call returns, so start the server and run the smoke test in the same command, for example:

    go build -o redis-mini . && (./redis-mini --port 6390 & sleep 1; python3 tests/smoke/smoke.py --port 6390)

Passing the smoke test shows that the server runs at all. It does not cover the whole scope; your port should implement the whole scope above.

## Budget

This run has fixed limits: at most {max_iterations} model turns, a cost cap of ${max_cost_usd} USD, and a wall-clock limit of {wall_timeout_s} seconds. The run is stopped automatically when any limit is reached.

## When to stop

When the build succeeds and the smoke test passes, and you believe the scope is implemented, stop: reply with a short summary and make no further tool calls.
