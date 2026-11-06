"""Validate smoke + hidden suites against the real reference redis-server 7.2.17 (local only).

  python -m harness.validate_reference [--repeats 3]

Writes tests/REFERENCE_VALIDATION.json. Requires 100% pass on every repeat.
No model or network calls (loopback only).
"""
import argparse, json, os, socket, subprocess, sys, tempfile, time

from .common import p, now_iso, sha256_file, tree_digest

REDIS = p("reference", "bin", "redis-server")


def free_port():
    s = socket.socket(); s.bind(("127.0.0.1", 0)); port = s.getsockname()[1]; s.close(); return port


def wait_port(port, timeout=10):
    end = time.time() + timeout
    while time.time() < end:
        try:
            socket.create_connection(("127.0.0.1", port), timeout=0.5).close(); return True
        except OSError:
            time.sleep(0.1)
    return False


def run_suite(script, port, tmp):
    out = os.path.join(tmp, os.path.basename(script) + ".json")
    r = subprocess.run([sys.executable, "-I", script, "--host", "127.0.0.1", "--port", str(port), "--json", out],
                       capture_output=True, text=True, timeout=600)
    with open(out) as f:
        return json.load(f), r.stdout


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repeats", type=int, default=3)
    a = ap.parse_args()
    ver = subprocess.run([REDIS, "--version"], capture_output=True, text=True).stdout.strip()
    if "v=7.2.17" not in ver:
        sys.exit("reference binary is not 7.2.17: %s" % ver)
    port = free_port()
    tmp = tempfile.mkdtemp(prefix="refval-")
    srv = subprocess.Popen([REDIS, "--port", str(port), "--bind", "127.0.0.1", "--save", "", "--appendonly", "no",
                            "--dir", tmp], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    runs = []
    try:
        if not wait_port(port):
            sys.exit("reference redis-server did not start")
        for i in range(a.repeats):
            sm, _ = run_suite(p("tests", "smoke", "smoke.py"), port, tmp)
            hd, hout = run_suite(p("tests", "hidden", "run_hidden.py"), port, tmp)
            runs.append({"repeat": i + 1, "smoke": sm, "hidden": hd})
            print("repeat %d: smoke %d/%d hidden %d/%d" % (i + 1, sm["passed"], sm["total"], hd["passed"], hd["total"]))
            if hd["passed"] != hd["total"]:
                print(hout)
    finally:
        srv.terminate(); srv.wait(10)
    green = all(r["smoke"]["passed"] == r["smoke"]["total"] and r["hidden"]["passed"] == r["hidden"]["total"]
                for r in runs)
    last = runs[-1]
    doc = {
        "validated_at": now_iso(), "redis_version": ver, "reference_binary_sha256": sha256_file(REDIS),
        "repeats": a.repeats, "all_green": green,
        "smoke": {"passed": last["smoke"]["passed"], "total": last["smoke"]["total"],
                  "pass_counts_per_repeat": [r["smoke"]["passed"] for r in runs],
                  "tree_sha256": tree_digest(p("tests", "smoke"))},
        "hidden": {"passed": last["hidden"]["passed"], "total": last["hidden"]["total"],
                   "pass_counts_per_repeat": [r["hidden"]["passed"] for r in runs],
                   "tree_sha256": tree_digest(p("tests", "hidden")),
                   "tests": [t["name"] for t in last["hidden"]["results"]]},
        "failures": [{"repeat": r["repeat"], "suite": s, "name": t["name"], "detail": t["detail"]}
                     for r in runs for s in ("smoke", "hidden") for t in r[s]["results"] if not t["pass"]],
    }
    with open(p("tests", "REFERENCE_VALIDATION.json"), "w") as f:
        json.dump(doc, f, indent=2)
    print("all_green=%s -> tests/REFERENCE_VALIDATION.json" % green)
    sys.exit(0 if green else 1)


if __name__ == "__main__":
    main()
