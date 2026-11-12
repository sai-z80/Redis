"""Deterministic Arm A runner: OpenRouter Chat Completions tool loop.

The script (not a human, not Claude) owns the loop, tool dispatch, budget enforcement
and transcript logging. Only `model_id` in the config differs between models.

  python -m harness.agent_run --config harness/config.json --run-id <id> --confirm-spend

Refuses to run unless FREEZE.json exists and verifies, and (for the real OpenRouter
endpoint) unless --confirm-spend is given.
"""
import argparse, json, os, platform, shutil, subprocess, sys, time, urllib.error, urllib.request

from . import freeze
from .common import p, load_config, now_iso, sha256_file, sha256_json, tree_digest
from .tools import TOOL_SCHEMAS, Tools

REAL_BASE = "https://openrouter.ai/api/v1"
HARNESS_VERSION = "1"


class Transcript:
    def __init__(self, path, secret):
        self.f = open(path, "a")
        self.secret = secret

    def log(self, kind, **data):
        line = json.dumps({"t": now_iso(), "kind": kind, **data}, ensure_ascii=False)
        if self.secret and self.secret in line:
            line = line.replace(self.secret, "[REDACTED]")
        self.f.write(line + "\n")
        self.f.flush()


def http_json(url, key, body=None, timeout=300):
    req = urllib.request.Request(url, method="POST" if body is not None else "GET")
    req.add_header("Authorization", "Bearer " + key)
    req.add_header("Content-Type", "application/json")
    req.add_header("X-Title", "sf-experiment-pilot-harness")
    data = json.dumps(body).encode() if body is not None else None
    with urllib.request.urlopen(req, data=data, timeout=timeout) as r:
        return json.loads(r.read().decode())


def prepare_workspace(cfg, run_dir):
    ws = os.path.join(run_dir, "workspace")
    if os.path.exists(ws):
        raise SystemExit("run dir already has a workspace; use a new --run-id")
    os.makedirs(ws)
    shutil.copytree(p(cfg["paths"]["upstream_dir"]), os.path.join(ws, "upstream"))
    shutil.copytree(p(cfg["paths"]["smoke_dir"]), os.path.join(ws, "tests", "smoke"),
                    ignore=shutil.ignore_patterns("__pycache__"))
    assert_clean_workspace(ws)
    return ws


def assert_clean_workspace(ws):
    for d, _, files in os.walk(ws):
        rel = os.path.relpath(d, ws)
        if rel.startswith(os.path.join("tests", "hidden")) or "REFERENCE_VALIDATION.json" in files \
                or "FREEZE.json" in files or "run_hidden.py" in files:
            raise SystemExit("FATAL: hidden-test material found in agent workspace: %s" % d)


def render_prompt(cfg):
    with open(p(cfg["paths"]["task_prompt"])) as f:
        t = f.read()
    c = cfg["caps"]
    for k in ("max_iterations", "max_cost_usd", "wall_timeout_s"):
        t = t.replace("{%s}" % k, ("%.2f" % c[k]) if k == "max_cost_usd" else str(c[k]))
    with open(p(cfg["paths"]["system_prompt"])) as f:
        s = f.read().strip()
    return s, t


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--config", default="harness/config.json")
    ap.add_argument("--run-id", required=True)
    ap.add_argument("--confirm-spend", action="store_true",
                    help="required to call the real OpenRouter endpoint (spends money)")
    ap.add_argument("--base-url-override", help=argparse.SUPPRESS)  # harness self-test only (local fake server)
    a = ap.parse_args(argv)

    cfg = load_config(a.config)
    caps, orc, tcfg = cfg["caps"], cfg["openrouter"], cfg["tools"]
    base = a.base_url_override or orc["base_url"]
    test_mode = bool(a.base_url_override)
    if test_mode and "openrouter.ai" in base:
        raise SystemExit("override must be a local fake endpoint")
    if not test_mode and not a.confirm_spend:
        raise SystemExit("Refusing: this calls OpenRouter and spends money. Re-run with --confirm-spend.")
    if caps["max_cost_usd"] > cfg["pilot_safety_ceiling_usd"]:
        raise SystemExit("max_cost_usd exceeds pilot_safety_ceiling_usd")

    frz = freeze.verify(config_path=a.config)
    if not frz["ok"] and not test_mode:
        raise SystemExit("FREEZE verification failed: %s" % frz["problems"])
    if not test_mode and not frz.get("freeze", {}).get("scored_runs_allowed"):
        raise SystemExit("FREEZE.json does not allow runs")

    key = os.environ.get(orc["api_key_env"], "")
    if not key:
        raise SystemExit("%s not set" % orc["api_key_env"])

    run_dir = p(cfg["paths"]["runs_dir"], a.run_id)
    os.makedirs(run_dir, exist_ok=True)
    ws = prepare_workspace(cfg, run_dir)
    tools = Tools(ws, os.path.join(run_dir, "gocache"), tcfg)
    tr = Transcript(os.path.join(run_dir, "transcript.jsonl"), key)
    system, task = render_prompt(cfg)

    manifest = {
        "run_id": a.run_id, "label": cfg["label"], "arm": cfg["arm"], "model_id": cfg["model_id"],
        "test_mode": test_mode, "harness_version": HARNESS_VERSION,
        "config_sha256": sha256_json(freeze.config_sans_secrets(cfg)),
        "freeze_ok": frz["ok"], "freeze_sha256": frz.get("freeze_sha256"),
        "prompt_sha256": sha256_json({"system": system, "task": task}),
        "caps": caps, "started_at": now_iso(), "host": platform.platform(), "python": sys.version.split()[0],
        "go_version": subprocess.run(["go", "version"], capture_output=True, text=True).stdout.strip(),
        "workspace_initial_sha256": tree_digest(ws),
    }
    tr.log("start", manifest=manifest, system=system, task=task, tools=TOOL_SCHEMAS)

    messages = [{"role": "system", "content": system}, {"role": "user", "content": task}]
    pin, pout = cfg["cost_guard_pricing_usd_per_mtok"]["input"], cfg["cost_guard_pricing_usd_per_mtok"]["output"]
    t0 = time.time()
    it = 0
    cost_usage = 0.0
    cost_estimated_part = 0.0
    tok_in = tok_out = 0
    gen_ids = []
    tool_calls_n = 0
    last_prompt_tokens = 0
    pending_chars = len(system) + len(task) + len(json.dumps(TOOL_SCHEMAS))
    stop = None

    while True:
        elapsed = time.time() - t0
        if it >= caps["max_iterations"]:
            stop = "max_iterations"; break
        if elapsed >= caps["wall_timeout_s"]:
            stop = "wall_timeout"; break
        projected_in = last_prompt_tokens + pending_chars / 3.0
        projected = (projected_in * pin + caps["max_tokens"] * pout) / 1e6
        if cost_usage + projected > caps["max_cost_usd"]:
            stop = "max_cost_usd (projected next call would exceed cap)"; break

        body = {"model": cfg["model_id"], "messages": messages, "tools": TOOL_SCHEMAS,
                "tool_choice": "auto", "max_tokens": caps["max_tokens"], "usage": {"include": True}}
        if caps["temperature"] is not None:
            body["temperature"] = caps["temperature"]
        if orc.get("provider_routing"):
            body["provider"] = orc["provider_routing"]

        resp, err = None, None
        for attempt in range(orc["http_retries"] + 1):
            try:
                to = min(orc["request_timeout_s"], max(10, caps["wall_timeout_s"] - (time.time() - t0)))
                resp = http_json(base + "/chat/completions", key, body, timeout=to)
                if "error" in resp and not resp.get("choices"):
                    err = "api_error: %s" % json.dumps(resp["error"])[:500]
                    resp = None
                else:
                    break
            except urllib.error.HTTPError as e:
                err = "HTTP %s: %s" % (e.code, e.read().decode("utf-8", "replace")[:500])
                if e.code not in (408, 429, 500, 502, 503, 504):
                    break
            except Exception as e:  # noqa: BLE001
                err = "%s: %s" % (type(e).__name__, e)
            tr.log("http_retry", iteration=it, attempt=attempt, error=err)
            time.sleep(orc["retry_backoff_s"])
        it += 1
        if resp is None:
            stop = "api_failure: " + (err or "unknown"); break

        usage = resp.get("usage") or {}
        pt, ct = int(usage.get("prompt_tokens") or 0), int(usage.get("completion_tokens") or 0)
        tok_in += pt; tok_out += ct
        last_prompt_tokens = pt + ct
        if usage.get("cost") is not None:
            cost_usage += float(usage["cost"])
        else:
            est = (pt * pin + ct * pout) / 1e6
            cost_usage += est; cost_estimated_part += est
        if resp.get("id"):
            gen_ids.append(resp["id"])
        tr.log("response", iteration=it, response=resp, cost_running_usd=round(cost_usage, 6))

        choice = (resp.get("choices") or [{}])[0]
        msg = choice.get("message") or {}
        amsg = {"role": "assistant", "content": msg.get("content") or ""}
        if msg.get("tool_calls"):
            amsg["tool_calls"] = msg["tool_calls"]
        if msg.get("reasoning_details"):
            amsg["reasoning_details"] = msg["reasoning_details"]
        messages.append(amsg)
        if not msg.get("tool_calls"):
            stop = "model_stopped (finish_reason=%s)" % choice.get("finish_reason"); break

        pending_chars = 0
        for tc in msg["tool_calls"]:
            fn = tc.get("function") or {}
            tool_calls_n += 1
            result = tools.dispatch(fn.get("name"), fn.get("arguments"))
            tr.log("tool", iteration=it, id=tc.get("id"), name=fn.get("name"), arguments=fn.get("arguments"), result=result)
            messages.append({"role": "tool", "tool_call_id": tc.get("id"), "content": result})
            pending_chars += len(result)

    wall = round(time.time() - t0, 3)
    # Reconcile provider-billed cost (best effort; endpoint can lag a few seconds).
    billed, billed_missing = 0.0, []
    for gid in gen_ids:
        got = None
        for _ in range(4):
            try:
                d = http_json(base + "/generation?id=" + gid, key, timeout=30).get("data") or {}
                if d.get("total_cost") is not None:
                    got = float(d["total_cost"]); break
            except Exception:  # noqa: BLE001
                pass
            time.sleep(2)
        if got is None:
            billed_missing.append(gid)
        else:
            billed += got

    manifest.update({
        "finished_at": now_iso(), "stop_reason": stop, "iterations": it, "tool_calls": tool_calls_n,
        "wall_s": wall, "tokens_in": tok_in, "tokens_out": tok_out,
        "cost_usd_usage_sum": round(cost_usage, 6), "cost_usd_estimated_component": round(cost_estimated_part, 6),
        "cost_usd_billed_generation_api": round(billed, 6) if gen_ids and not billed_missing else None,
        "billed_partial_usd": round(billed, 6), "billed_missing_ids": billed_missing,
        "generation_ids": gen_ids, "workspace_final_sha256": tree_digest(ws),
        "smoke_tests_unmodified": freeze.smoke_unmodified(ws),
    })
    tr.log("end", manifest=manifest)
    with open(os.path.join(run_dir, "manifest.json"), "w") as f:
        json.dump(manifest, f, indent=2)
    print(json.dumps({k: manifest[k] for k in ("run_id", "model_id", "stop_reason", "iterations", "tool_calls",
                                                "wall_s", "cost_usd_usage_sum", "cost_usd_billed_generation_api")}, indent=2))


if __name__ == "__main__":
    main()
