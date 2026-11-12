#!/usr/bin/env python3
"""Visible smoke test for the Redis-mini pilot (RESP2 over TCP). Stdlib only.

Usage: python3 tests/smoke/smoke.py [--host 127.0.0.1] [--port 6380] [--json out.json]
Exit code 0 iff all checks pass.
"""
import argparse, json, socket, sys, uuid


class Resp:
    def __init__(self, host, port, timeout=5.0):
        self.s = socket.create_connection((host, port), timeout=timeout)
        self.buf = b""

    def close(self):
        self.s.close()

    def send(self, *args):
        out = b"*%d\r\n" % len(args)
        for a in args:
            a = a if isinstance(a, bytes) else str(a).encode()
            out += b"$%d\r\n%s\r\n" % (len(a), a)
        self.s.sendall(out)

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
            data, self.buf = self.buf[:n], self.buf[n + 2:]
            return ("bulk", data)
        raise ValueError("unexpected reply type: %r" % line)

    def cmd(self, *args):
        self.send(*args)
        return self.read()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=6380)
    ap.add_argument("--json")
    a = ap.parse_args()
    k = "smoke:" + uuid.uuid4().hex
    checks = [
        ("ping", ("PING",), ("simple", b"PONG")),
        ("echo", ("ECHO", "hello"), ("bulk", b"hello")),
        ("set", ("SET", k, "v1"), ("simple", b"OK")),
        ("get", ("GET", k), ("bulk", b"v1")),
        ("del", ("DEL", k), ("int", 1)),
        ("get_after_del", ("GET", k), ("nil", None)),
    ]
    results = []
    try:
        c = Resp(a.host, a.port)
    except OSError as e:
        print("FAIL: cannot connect to %s:%d: %s" % (a.host, a.port, e))
        results = [{"name": n, "pass": False, "detail": "connect failed"} for n, _, _ in checks]
    else:
        for name, args, want in checks:
            try:
                got = c.cmd(*args)
                ok = got == want
                detail = "" if ok else "want %r got %r" % (want, got)
            except Exception as e:  # noqa: BLE001
                ok, detail = False, "%s: %s" % (type(e).__name__, e)
            results.append({"name": name, "pass": ok, "detail": detail})
            print("%s %s %s" % ("PASS" if ok else "FAIL", name, detail))
        c.close()
    passed = sum(r["pass"] for r in results)
    print("smoke: %d/%d passed" % (passed, len(results)))
    if a.json:
        with open(a.json, "w") as f:
            json.dump({"suite": "smoke", "passed": passed, "total": len(results), "results": results}, f, indent=2)
    sys.exit(0 if passed == len(results) else 1)


if __name__ == "__main__":
    main()
