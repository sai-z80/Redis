"""Harness self-test with a LOCAL FAKE OpenRouter endpoint (scripted replies). Spends $0.

  python -m harness.selftest

Exercises: tool loop, tool dispatch, path confinement, sandbox (no pilot dir, no API key,
no hidden tests visible), cost accounting, transcript, manifest, and the offline scorer.
The scripted 'model' writes a deliberately minimal PING-only Go server, so the expected
score is build_ok with partial pass counts. This is NOT a contestant run.
"""
import json, os, shutil, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from . import agent_run, score
from .common import p

GO_MAIN = r'''package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func main() {
	port := flag.Int("port", 6380, "port")
	flag.Parse()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		panic(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			r := bufio.NewReader(c)
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return
				}
				n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
				args := []string{}
				for i := 0; i < n; i++ {
					l, _ := r.ReadString('\n')
					sz, _ := strconv.Atoi(strings.TrimSpace(l[1:]))
					b := make([]byte, sz+2)
					if _, err := ioReadFull(r, b); err != nil {
						return
					}
					args = append(args, string(b[:sz]))
				}
				if len(args) > 0 && strings.ToUpper(args[0]) == "PING" {
					c.Write([]byte("+PONG\r\n"))
				} else {
					c.Write([]byte("-ERR selftest stub\r\n"))
				}
			}
		}(c)
	}
}

func ioReadFull(r *bufio.Reader, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := r.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
'''

SCRIPT = [
    [("list_dir", {"path": "."})],
    [("read_file", {"path": "../../../FREEZE.json"}),
     ("read_file", {"path": "/etc/passwd"}),
     ("run_shell", {"command": "echo KEY=${OPENROUTER_API_KEY:-unset}; ls /workspace 2>&1 | head -1; "
                               "ls tests; test -e /work/tests/hidden && echo HIDDEN_VISIBLE || echo hidden_absent; "
                               "python3 -I -c \"import urllib.request;urllib.request.urlopen('http://example.com',timeout=3)\" "
                               ">/dev/null 2>&1 && echo NET_OK || echo net_blocked"})],
    [("write_file", {"path": "go.mod", "content": "module redismini\n\ngo 1.22\n"}),
     ("write_file", {"path": "main.go", "content": GO_MAIN})],
    [("run_shell", {"command": "go build -o redis-mini . && (./redis-mini --port 6390 & sleep 1; "
                               "python3 tests/smoke/smoke.py --port 6390; sleep 30 & echo bg_started)",
                    "timeout_s": 120})],
    [],  # stop
]


class Fake(BaseHTTPRequestHandler):
    turn = 0
    seen_auth = []

    def log_message(self, *a):
        pass

    def do_POST(self):
        Fake.seen_auth.append(self.headers.get("Authorization"))
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        calls = SCRIPT[min(Fake.turn, len(SCRIPT) - 1)]
        Fake.turn += 1
        msg = {"role": "assistant", "content": "" if calls else "done"}
        if calls:
            msg["tool_calls"] = [{"id": "call_%d_%d" % (Fake.turn, i), "type": "function",
                                  "function": {"name": n, "arguments": json.dumps(a)}} for i, (n, a) in enumerate(calls)]
        body = {"id": "gen-selftest-%d" % Fake.turn, "choices": [{"message": msg,
                "finish_reason": "tool_calls" if calls else "stop"}],
                "usage": {"prompt_tokens": 1000, "completion_tokens": 100, "cost": 0.0001}}
        self._send(body)

    def do_GET(self):
        self._send({"data": {"total_cost": 0.0001}})

    def _send(self, body):
        b = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)


def main():
    run_id = "selftest-" + time.strftime("%Y%m%dT%H%M%S")
    srv = ThreadingHTTPServer(("127.0.0.1", 0), Fake)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    os.environ["OPENROUTER_API_KEY"] = "selftest-dummy-not-a-real-key"
    try:
        agent_run.main(["--run-id", run_id, "--base-url-override", "http://127.0.0.1:%d" % srv.server_port])
    finally:
        srv.shutdown()
    rd = p("runs", run_id)
    tr = [json.loads(l) for l in open(os.path.join(rd, "transcript.jsonl"))]
    tool_out = "\n".join(e["result"] for e in tr if e["kind"] == "tool")
    m = json.load(open(os.path.join(rd, "manifest.json")))
    checks = {
        "path_escape_blocked": "path escapes the workspace" in tool_out,
        "abs_path_blocked": "absolute paths are not allowed" in tool_out,
        "key_not_in_sandbox_env": "KEY=unset" in tool_out,
        "pilot_dir_not_visible": "/workspace" in tool_out and "No such file" in tool_out,
        "hidden_tests_absent": "hidden_absent" in tool_out and not os.path.exists(os.path.join(rd, "workspace", "tests", "hidden")),
        "network_blocked": "net_blocked" in tool_out,
        "smoke_ran_in_sandbox": "smoke: 1/6 passed" in tool_out,
        "background_proc_did_not_hang": all(e.get("result", "").find("TIMED_OUT") < 0 for e in tr if e["kind"] == "tool"),
        "stopped_by_model": m["stop_reason"].startswith("model_stopped"),
        "cost_summed": abs(m["cost_usd_usage_sum"] - 0.0005) < 1e-9,
        "billed_reconciled": m["cost_usd_billed_generation_api"] is not None,
        "key_redacted_in_transcript": "selftest-dummy-not-a-real-key" not in open(os.path.join(rd, "transcript.jsonl")).read(),
        "test_mode_flagged": m["test_mode"] is True,
    }
    score.main(["--run-id", run_id])
    s = json.load(open(os.path.join(rd, "scores.json")))
    checks["scorer_build_ok"] = s["build_ok"]
    checks["scorer_smoke_partial"] = s["smoke"]["passed"] == 1 and s["smoke"]["total"] == 6
    checks["scorer_hidden_partial"] = 0 < s["hidden"]["passed"] < s["hidden"]["total"]
    ok = all(checks.values())
    print(json.dumps({"selftest_run_id": run_id, "ok": ok, "checks": checks,
                      "hidden_score_of_stub": "%d/%d" % (s["hidden"]["passed"], s["hidden"]["total"])}, indent=2))
    with open(os.path.join(rd, "SELFTEST_NOT_A_CONTESTANT_RUN.txt"), "w") as f:
        f.write("Harness self-test against a local fake endpoint. Scripted replies. Not a model run.\n")
    raise SystemExit(0 if ok else 1)


if __name__ == "__main__":
    main()
