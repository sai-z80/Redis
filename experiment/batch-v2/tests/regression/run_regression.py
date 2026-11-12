#!/usr/bin/env python3
"""VISIBLE regression suite for the Redis-mini experiment (batch-v2).

Provenance: this is the pilot-v1 hidden suite (tests/hidden/run_hidden.py, sha256
b2aa78448f2a1824203036afb7d33085eb1940a36e09c4625570aa20e494df50), published after
pilot-v1. In batch-v2 it is visible regression material, copied into every agent
workspace identically. Test bodies are unchanged; only this docstring and the suite
label in the output were edited. It is NOT the batch-v2 hidden (withheld) suite.

Talks RESP2 over TCP to host:port. Stdlib only. Each test uses fresh connection(s)
and unique keys.

Usage: python3 tests/regression/run_regression.py --host 127.0.0.1 --port 6380 [--json out.json]
Exit code 0 iff all tests pass.

Error-reply checks require an error reply whose message starts with "ERR". A key phrase
is additionally required only for "wrong number of arguments" and "invalid expire time".
"""
import argparse, json, socket, sys, time, traceback, uuid

TIMEOUT = 5.0


class Resp:
    def __init__(self, host, port, timeout=TIMEOUT):
        self.s = socket.create_connection((host, port), timeout=timeout)
        self.buf = b""

    def close(self):
        try:
            self.s.close()
        except OSError:
            pass

    @staticmethod
    def encode(*args):
        out = b"*%d\r\n" % len(args)
        for a in args:
            a = a if isinstance(a, bytes) else str(a).encode()
            out += b"$%d\r\n%s\r\n" % (len(a), a)
        return out

    def send(self, *args):
        self.s.sendall(self.encode(*args))

    def _fill(self):
        d = self.s.recv(65536)
        if not d:
            raise ConnectionError("connection closed by server")
        self.buf += d

    def _line(self):
        while b"\r\n" not in self.buf:
            self._fill()
        line, self.buf = self.buf.split(b"\r\n", 1)
        return line

    def read(self):
        line = self._line()
        t, rest = line[:1], line[1:]
        if t == b"+":
            return ("simple", rest)
        if t == b"-":
            return ("error", rest)
        if t == b":":
            return ("int", int(rest))
        if t == b"$":
            n = int(rest)
            if n == -1:
                return ("nil", None)
            while len(self.buf) < n + 2:
                self._fill()
            data, tail, self.buf = self.buf[:n], self.buf[n:n + 2], self.buf[n + 2:]
            if tail != b"\r\n":
                raise ValueError("bulk not terminated by CRLF")
            return ("bulk", data)
        raise ValueError("unexpected reply type: %r" % line[:80])

    def cmd(self, *args):
        self.send(*args)
        return self.read()


class Fail(Exception):
    pass


def eq(got, want, what=""):
    if got != want:
        g = repr(got)
        raise Fail("%swant %r got %s" % (what + ": " if what else "", want, g[:200]))


def is_err(got, contains=None, what=""):
    if got[0] != "error" or not got[1].startswith(b"ERR"):
        raise Fail("%swant -ERR reply, got %r" % (what + ": " if what else "", got))
    if contains and contains.lower().encode() not in got[1].lower():
        raise Fail("%serror %r lacks %r" % (what + ": " if what else "", got[1], contains))


def K(tag):
    return "h:%s:%s" % (tag, uuid.uuid4().hex)


TESTS = []


def test(fn):
    TESTS.append(fn)
    return fn


# ---------------- PING / ECHO ----------------
@test
def ping_no_arg(c):
    eq(c.cmd("PING"), ("simple", b"PONG"))


@test
def ping_with_arg_returns_bulk(c):
    eq(c.cmd("PING", "hello world"), ("bulk", b"hello world"))


@test
def ping_too_many_args(c):
    is_err(c.cmd("PING", "a", "b"), "wrong number of arguments")


@test
def command_names_case_insensitive(c):
    eq(c.cmd("ping"), ("simple", b"PONG"))
    eq(c.cmd("PiNg"), ("simple", b"PONG"))
    eq(c.cmd("echo", "x"), ("bulk", b"x"))
    k = K("case")
    eq(c.cmd("set", k, "1"), ("simple", b"OK"))
    eq(c.cmd("gEt", k), ("bulk", b"1"))
    eq(c.cmd("del", k), ("int", 1))


@test
def echo_basic(c):
    eq(c.cmd("ECHO", "Hello, Redis"), ("bulk", b"Hello, Redis"))


@test
def echo_empty_string(c):
    eq(c.cmd("ECHO", ""), ("bulk", b""))


@test
def echo_binary(c):
    v = b"a\r\nb\x00c\xff\xfe"
    eq(c.cmd("ECHO", v), ("bulk", v))


@test
def echo_no_args(c):
    is_err(c.cmd("ECHO"), "wrong number of arguments")


@test
def echo_too_many_args(c):
    is_err(c.cmd("ECHO", "a", "b"), "wrong number of arguments")


# ---------------- SET / GET ----------------
@test
def set_get_roundtrip(c):
    k = K("sg")
    eq(c.cmd("SET", k, "bar"), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"bar"))


@test
def get_missing_is_nil(c):
    eq(c.cmd("GET", K("missing")), ("nil", None))


@test
def set_overwrites(c):
    k = K("ow")
    c.cmd("SET", k, "one")
    eq(c.cmd("SET", k, "two"), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"two"))


@test
def set_empty_value(c):
    k = K("empty")
    eq(c.cmd("SET", k, ""), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b""))


@test
def set_get_binary_value(c):
    k = K("bin")
    v = bytes(range(256)) + b"\r\n\r\n\x00"
    eq(c.cmd("SET", k, v), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", v))


@test
def set_get_binary_key(c):
    k = b"h:binkey:\x00\r\n\xff:" + uuid.uuid4().hex.encode()
    eq(c.cmd("SET", k, "val"), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"val"))
    eq(c.cmd("DEL", k), ("int", 1))


@test
def set_get_large_value(c):
    k = K("large")
    v = (b"0123456789abcdef" * 32768)  # 512 KiB, spans many TCP reads
    eq(c.cmd("SET", k, v), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", v))


@test
def numeric_like_values_are_strings(c):
    k = K("num")
    eq(c.cmd("SET", k, "00123"), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"00123"))


@test
def get_wrong_arity(c):
    is_err(c.cmd("GET"), "wrong number of arguments")
    is_err(c.cmd("GET", "a", "b"), "wrong number of arguments")


@test
def set_wrong_arity(c):
    is_err(c.cmd("SET"), "wrong number of arguments")
    is_err(c.cmd("SET", K("ar")), "wrong number of arguments")


@test
def set_unknown_option_is_syntax_error(c):
    k = K("synt")
    is_err(c.cmd("SET", k, "v", "BOGUS"))
    eq(c.cmd("GET", k), ("nil", None), "key must not be set after error")


@test
def keys_shared_across_connections(c):
    k = K("shared")
    c.cmd("SET", k, "x")
    c2 = Resp(c.host, c.port)
    try:
        eq(c2.cmd("GET", k), ("bulk", b"x"))
        eq(c2.cmd("DEL", k), ("int", 1))
    finally:
        c2.close()
    eq(c.cmd("GET", k), ("nil", None))


# ---------------- DEL ----------------
@test
def del_existing_and_missing(c):
    k = K("del")
    c.cmd("SET", k, "v")
    eq(c.cmd("DEL", k), ("int", 1))
    eq(c.cmd("DEL", k), ("int", 0))
    eq(c.cmd("GET", k), ("nil", None))


@test
def del_multiple_keys_counts(c):
    a, b, m = K("d1"), K("d2"), K("dm")
    c.cmd("SET", a, "1")
    c.cmd("SET", b, "2")
    eq(c.cmd("DEL", a, m, b), ("int", 2))
    eq(c.cmd("GET", a), ("nil", None))
    eq(c.cmd("GET", b), ("nil", None))


@test
def del_same_key_twice_in_one_call(c):
    a = K("dd")
    c.cmd("SET", a, "1")
    eq(c.cmd("DEL", a, a), ("int", 1))


@test
def del_no_args(c):
    is_err(c.cmd("DEL"), "wrong number of arguments")


# ---------------- EX / PX lazy expiry ----------------
@test
def set_px_expires(c):
    k = K("px")
    eq(c.cmd("SET", k, "v", "PX", "150"), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"v"), "before expiry")
    time.sleep(0.4)
    eq(c.cmd("GET", k), ("nil", None), "after expiry")


@test
def set_ex_expires(c):
    k = K("ex")
    eq(c.cmd("SET", k, "v", "EX", "1"), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"v"), "before expiry")
    time.sleep(1.3)
    eq(c.cmd("GET", k), ("nil", None), "after expiry")


@test
def set_ex_px_options_case_insensitive(c):
    k = K("exci")
    eq(c.cmd("SET", k, "v", "px", "150"), ("simple", b"OK"))
    k2 = K("exci2")
    eq(c.cmd("SET", k2, "v", "ex", "100"), ("simple", b"OK"))
    eq(c.cmd("GET", k2), ("bulk", b"v"))
    time.sleep(0.4)
    eq(c.cmd("GET", k), ("nil", None))
    c.cmd("DEL", k2)


@test
def ex_long_ttl_not_expired(c):
    k = K("exlong")
    eq(c.cmd("SET", k, "v", "EX", "100"), ("simple", b"OK"))
    time.sleep(0.2)
    eq(c.cmd("GET", k), ("bulk", b"v"))
    c.cmd("DEL", k)


@test
def plain_set_clears_ttl(c):
    k = K("clr")
    c.cmd("SET", k, "v1", "PX", "150")
    eq(c.cmd("SET", k, "v2"), ("simple", b"OK"))
    time.sleep(0.4)
    eq(c.cmd("GET", k), ("bulk", b"v2"))
    c.cmd("DEL", k)


@test
def set_with_ttl_overwrites_old_ttl(c):
    k = K("rettl")
    c.cmd("SET", k, "v1", "PX", "150")
    c.cmd("SET", k, "v2", "EX", "100")
    time.sleep(0.4)
    eq(c.cmd("GET", k), ("bulk", b"v2"))
    c.cmd("DEL", k)


@test
def del_on_expired_key_returns_zero(c):
    k = K("delexp")
    c.cmd("SET", k, "v", "PX", "100")
    time.sleep(0.3)
    eq(c.cmd("DEL", k), ("int", 0))


@test
def set_after_expiry_creates_fresh_key(c):
    k = K("reset")
    c.cmd("SET", k, "v1", "PX", "100")
    time.sleep(0.3)
    eq(c.cmd("SET", k, "v2"), ("simple", b"OK"))
    time.sleep(0.2)
    eq(c.cmd("GET", k), ("bulk", b"v2"))
    c.cmd("DEL", k)


@test
def ex_non_integer_is_error(c):
    k = K("exbad")
    is_err(c.cmd("SET", k, "v", "EX", "abc"))
    eq(c.cmd("GET", k), ("nil", None))


@test
def px_non_integer_is_error(c):
    k = K("pxbad")
    is_err(c.cmd("SET", k, "v", "PX", "1.5"))
    eq(c.cmd("GET", k), ("nil", None))


@test
def ex_zero_or_negative_is_error(c):
    k = K("exzero")
    is_err(c.cmd("SET", k, "v", "EX", "0"), "invalid expire time")
    is_err(c.cmd("SET", k, "v", "PX", "-5"), "invalid expire time")
    eq(c.cmd("GET", k), ("nil", None))


@test
def ex_missing_value_is_syntax_error(c):
    k = K("exmiss")
    is_err(c.cmd("SET", k, "v", "EX"))
    eq(c.cmd("GET", k), ("nil", None))


@test
def ex_and_px_together_is_syntax_error(c):
    k = K("expx")
    is_err(c.cmd("SET", k, "v", "EX", "10", "PX", "100"))
    eq(c.cmd("GET", k), ("nil", None))


# ---------------- unknown commands / protocol ----------------
@test
def unknown_command_error(c):
    is_err(c.cmd("NOSUCHCMD"))
    is_err(c.cmd("NOSUCHCMD", "a", "b"))


@test
def connection_survives_errors(c):
    is_err(c.cmd("NOSUCHCMD"))
    is_err(c.cmd("GET"), "wrong number of arguments")
    eq(c.cmd("PING"), ("simple", b"PONG"))


@test
def pipelined_commands_in_one_write(c):
    k = K("pipe")
    payload = (Resp.encode("SET", k, "p") + Resp.encode("GET", k) + Resp.encode("PING")
               + Resp.encode("DEL", k) + Resp.encode("GET", k))
    c.s.sendall(payload)
    eq(c.read(), ("simple", b"OK"))
    eq(c.read(), ("bulk", b"p"))
    eq(c.read(), ("simple", b"PONG"))
    eq(c.read(), ("int", 1))
    eq(c.read(), ("nil", None))


@test
def request_split_across_writes(c):
    k = K("split")
    raw = Resp.encode("SET", k, "fragmented-value")
    for i in range(0, len(raw), 3):
        c.s.sendall(raw[i:i + 3])
        time.sleep(0.005)
    eq(c.read(), ("simple", b"OK"))
    eq(c.cmd("GET", k), ("bulk", b"fragmented-value"))
    c.cmd("DEL", k)


@test
def many_concurrent_clients(c):
    conns = [Resp(c.host, c.port) for _ in range(20)]
    try:
        keys = [K("cc%d" % i) for i in range(len(conns))]
        for cc, k in zip(conns, keys):
            cc.send("SET", k, k)
        for cc in conns:
            eq(cc.read(), ("simple", b"OK"))
        for cc, k in zip(conns, keys):
            eq(cc.cmd("GET", k), ("bulk", k.encode()))
        eq(c.cmd("DEL", *keys), ("int", len(keys)))
    finally:
        for cc in conns:
            cc.close()


def run(host, port):
    results = []
    for fn in TESTS:
        t0 = time.time()
        c = None
        try:
            c = Resp(host, port)
            c.host, c.port = host, port
            fn(c)
            ok, detail = True, ""
        except Fail as e:
            ok, detail = False, str(e)
        except Exception as e:  # noqa: BLE001 - any crash is a failure of the system under test
            ok, detail = False, "%s: %s" % (type(e).__name__, str(e)[:200])
        finally:
            if c:
                c.close()
        results.append({"name": fn.__name__, "pass": ok, "detail": detail,
                        "seconds": round(time.time() - t0, 3)})
    return results


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=6380)
    ap.add_argument("--json")
    a = ap.parse_args()
    results = run(a.host, a.port)
    for r in results:
        print("%s %s %s" % ("PASS" if r["pass"] else "FAIL", r["name"], r["detail"]))
    passed = sum(r["pass"] for r in results)
    print("regression: %d/%d passed" % (passed, len(results)))
    if a.json:
        with open(a.json, "w") as f:
            json.dump({"suite": "regression", "passed": passed, "total": len(results), "results": results}, f, indent=2)
    sys.exit(0 if passed == len(results) else 1)


if __name__ == "__main__":
    main()
