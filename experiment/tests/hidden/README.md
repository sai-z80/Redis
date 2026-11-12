# Hidden conformance suite (WITHHELD from agents)

Never copied into an agent workspace. Run only by `python -m harness.score` (inside the scoring sandbox)
and by `python -m harness.validate_reference` (against real redis-server 7.2.17).

`run_hidden.py` holds 43 tests in these groups: PING/ECHO (incl. arity, binary echo), SET/GET (overwrite, empty, binary
key/value incl. NUL/CRLF/0xFF, 512 KiB value, arity, bad option), DEL (multi-key counts, duplicates, arity),
EX/PX lazy expiry (expiry, TTL cleared by plain SET, TTL replaced, DEL on expired key = 0, bad/zero/negative/missing
expire values, EX+PX together), unknown command, connection survives errors, pipelining, requests split across writes,
20 concurrent clients sharing a keyspace.

Error policy: replies must be `-ERR...`. A phrase is also required only where it appears verbatim in the provided
upstream subset (`wrong number of arguments`, `invalid expire time`).

Exclusions (out of scope per SCOPE.md, so not tested): inline (non-multibulk) commands, TTL/EXPIRE family,
SET NX/XX/GET/KEEPTTL/EXAT/PXAT, very large EX/PX overflow edge cases, RESP3, every other command.
Expiry tests use sleeps with margin (PX 100–150 ms, then wait ≥ 300 ms; EX 1, then wait 1.3 s).

Validated 100% green on the reference across 3 repeats: see `../REFERENCE_VALIDATION.json`.
