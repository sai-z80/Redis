# Pilot scope (FROZEN) — Arm A smoke, Redis-mini C → Go

Label: `pilot-smoke-arm-a-port-attempt`. Upstream C source is provided, so this is a **port attempt**, not a behaviour-only reimplementation.

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
