"""Offline scorer. Never calls any model or network.

  python -m harness.score --run-id <id> [--config harness/config.json]

Copies the agent workspace to runs/<id>/score/build, builds it in the sandbox, starts the
server and runs the FROZEN smoke + hidden suites (from tests/, not from the workspace)
inside the same no-network sandbox. Writes runs/<id>/scores.json.
"""
import argparse, json, os, shutil, sys

from . import freeze
from .common import p, load_config, now_iso, sandbox_shell, tree_digest

SUITE_SH = r'''
set -u
./{binary} --port {port} > /tmp/server.log 2>&1 &
i=0
until python3 -I -c "import socket;socket.create_connection(('127.0.0.1',{port}),0.5).close()" 2>/dev/null; do
  i=$((i+1)); if [ $i -gt {start_ticks} ]; then echo SERVER_DID_NOT_START; tail -c 2000 /tmp/server.log; exit 3; fi
  sleep 0.1
done
python3 -I /suites/smoke/smoke.py --port {port} --json /work/.score_smoke.json > /dev/null 2>&1
python3 -I /suites/hidden/run_hidden.py --port {port} --json /work/.score_hidden.json > /dev/null 2>&1
echo SUITES_DONE
'''


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--config", default="harness/config.json")
    ap.add_argument("--run-id", required=True)
    a = ap.parse_args(argv)
    cfg = load_config(a.config)
    sc = cfg["scoring"]
    run_dir = p(cfg["paths"]["runs_dir"], a.run_id)
    ws = os.path.join(run_dir, "workspace")
    if not os.path.isdir(ws):
        sys.exit("no workspace for run %s" % a.run_id)
    frz = freeze.verify(a.config)

    build = os.path.join(run_dir, "score", "build")
    if os.path.exists(build):
        shutil.rmtree(build)
    shutil.copytree(ws, build, ignore=shutil.ignore_patterns(sc["binary"], ".score_*"))
    cache = os.path.join(run_dir, "score", "gocache")
    b = sandbox_shell(sc["build_cmd"], build, cache, sc["build_timeout_s"], max_output=20000)
    build_ok = b["exit_code"] == 0 and not b["timed_out"] and os.path.isfile(os.path.join(build, sc["binary"]))

    result = {"run_id": a.run_id, "scored_at": now_iso(), "model_id": None, "build_ok": build_ok,
              "build_output_tail": b["output"][-4000:], "freeze_ok": frz["ok"], "freeze_problems": frz["problems"],
              "workspace_sha256": tree_digest(ws), "smoke_tests_unmodified": None,
              "smoke": {"passed": 0, "total": None}, "hidden": {"passed": 0, "total": None}}
    mf = os.path.join(run_dir, "manifest.json")
    if os.path.exists(mf):
        with open(mf) as f:
            m = json.load(f)
        result["model_id"] = m.get("model_id")
        result["test_mode"] = m.get("test_mode")
    try:
        result["smoke_tests_unmodified"] = freeze.smoke_unmodified(ws)
    except (OSError, KeyError):
        pass

    # Denominators come from the frozen reference validation, so a crash mid-suite still scores against the full suite.
    rvp = p("tests", "REFERENCE_VALIDATION.json")
    with open(rvp) as f:
        rv = json.load(f)
    result["smoke"]["total"] = rv["smoke"]["total"]
    result["hidden"]["total"] = rv["hidden"]["total"]

    if build_ok:
        cmd = SUITE_SH.format(binary=sc["binary"], port=sc["port"], start_ticks=int(sc["server_start_timeout_s"] * 10))
        binds = [(p(cfg["paths"]["smoke_dir"]), "/suites/smoke"), (p(cfg["paths"]["hidden_dir"]), "/suites/hidden")]
        r = sandbox_shell(cmd, build, cache, sc["suite_timeout_s"], extra_ro_binds=binds, max_output=20000)
        result["suite_run"] = {"exit_code": r["exit_code"], "timed_out": r["timed_out"], "seconds": r["seconds"],
                               "output_tail": r["output"][-4000:]}
        for s in ("smoke", "hidden"):
            fp = os.path.join(build, ".score_%s.json" % s)
            if os.path.exists(fp):
                with open(fp) as f:
                    d = json.load(f)
                result[s] = {"passed": d["passed"], "total": d["total"], "results": d["results"]}
    for s in ("smoke", "hidden"):
        t = result[s]["total"] or 0
        result[s]["pass_rate"] = round(result[s]["passed"] / t, 4) if t else 0.0
    out = os.path.join(run_dir, "scores.json")
    with open(out, "w") as f:
        json.dump(result, f, indent=2)
    print(json.dumps({"run_id": a.run_id, "build_ok": build_ok, "freeze_ok": frz["ok"],
                      "smoke": "%s/%s" % (result["smoke"]["passed"], result["smoke"]["total"]),
                      "hidden": "%s/%s" % (result["hidden"]["passed"], result["hidden"]["total"]),
                      "scores": out}, indent=2))


if __name__ == "__main__":
    main()
