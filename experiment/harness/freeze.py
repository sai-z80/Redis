"""Write / verify FREEZE.json (hashes of scope, prompts, tests, config, upstream pin, caps).

  python -m harness.freeze            # write FREEZE.json (requires green REFERENCE_VALIDATION.json)
  python -m harness.freeze --verify   # check current files against FREEZE.json
"""
import argparse, json, os, sys

from .common import p, load_config, now_iso, sha256_file, sha256_json, tree_digest, tree_hashes

REDIS_PIN = {"version": "7.2.17", "commit": "83937c51afb53e1c69661c8e1533e0e19b896fe1",
             "licence": "BSD-3-Clause"}
FREEZE_PATH = p("FREEZE.json")
SECRET_HINTS = ("secret", "token", "password", "api_key")


def config_sans_secrets(cfg, drop_model=True):
    """Config minus secrets. model_id is excluded from the frozen hash because it is the ONE
    field allowed to change between runs; it is recorded separately in every manifest."""
    if drop_model:
        cfg = {k: v for k, v in cfg.items() if k != "model_id"}

    def clean(o):
        if isinstance(o, dict):
            return {k: clean(v) for k, v in o.items()
                    if not (any(h in k.lower() for h in SECRET_HINTS) and k != "api_key_env")}
        return o
    return clean(cfg)


def scope_body():
    with open(p("SCOPE.md")) as f:
        s = f.read()
    return s[s.index("## In scope"):].strip()


def compute(config_path="harness/config.json"):
    cfg = load_config(config_path)
    with open(p(cfg["paths"]["task_prompt"])) as f:
        prompt = f.read()
    probs = []
    if scope_body() not in prompt:
        probs.append("SCOPE.md scope text is not word-for-word in the Arm A prompt")
    harness_files = {k: v for k, v in tree_hashes(p("harness")).items() if k.endswith(".py")}
    return {
        "scope_sha256": sha256_file(p("SCOPE.md")),
        "prompt_system_sha256": sha256_file(p(cfg["paths"]["system_prompt"])),
        "prompt_arm_a_sha256": sha256_file(p(cfg["paths"]["task_prompt"])),
        "smoke_tree_sha256": tree_digest(p(cfg["paths"]["smoke_dir"])),
        "smoke_files": tree_hashes(p(cfg["paths"]["smoke_dir"])),
        "hidden_tree_sha256": tree_digest(p(cfg["paths"]["hidden_dir"])),
        "hidden_files": tree_hashes(p(cfg["paths"]["hidden_dir"])),
        "config_sha256_sans_secrets": sha256_json(config_sans_secrets(cfg)),
        "upstream_tree_sha256": tree_digest(p(cfg["paths"]["upstream_dir"])),
        "harness_files": harness_files,
        "caps": cfg["caps"],
        "pilot_safety_ceiling_usd": cfg["pilot_safety_ceiling_usd"],
    }, probs, cfg


def smoke_unmodified(ws):
    with open(FREEZE_PATH) as f:
        fr = json.load(f)
    return tree_hashes(os.path.join(ws, "tests", "smoke")) == fr["hashed"]["smoke_files"]


def verify(config_path="harness/config.json"):
    if not os.path.exists(FREEZE_PATH):
        return {"ok": False, "problems": ["FREEZE.json missing"]}
    with open(FREEZE_PATH) as f:
        fr = json.load(f)
    cur, probs, _ = compute(config_path)
    for k, v in fr["hashed"].items():
        if cur.get(k) != v:
            probs.append("mismatch: %s" % k)
    rv = p("tests", "REFERENCE_VALIDATION.json")
    if not os.path.exists(rv) or sha256_file(rv) != fr["reference_validation"]["sha256"]:
        probs.append("REFERENCE_VALIDATION.json missing or changed")
    return {"ok": not probs, "problems": probs, "freeze": fr, "freeze_sha256": sha256_file(FREEZE_PATH)}


def write():
    rv_path = p("tests", "REFERENCE_VALIDATION.json")
    if not os.path.exists(rv_path):
        sys.exit("REFERENCE_VALIDATION.json missing: run python -m harness.validate_reference first")
    with open(rv_path) as f:
        rv = json.load(f)
    if not rv.get("all_green"):
        sys.exit("reference validation is not 100%% green; refusing to freeze")
    hashed, probs, cfg = compute()
    if probs:
        sys.exit("cannot freeze: %s" % probs)
    up = {}
    if os.path.exists(p("reference", "redis-7.2.17.tar.gz")):
        up["tarball_sha256"] = sha256_file(p("reference", "redis-7.2.17.tar.gz"))
    if os.path.exists(p("reference", "bin", "redis-server")):
        up["reference_binary_sha256"] = sha256_file(p("reference", "bin", "redis-server"))
    fr = {
        "label": cfg["label"] + " (setup/smoke infrastructure; not published results)",
        "frozen_at": now_iso(),
        "redis_pin": {**REDIS_PIN, **up},
        "model_id_at_freeze": cfg["model_id"],
        "model_id_note": "model_id is the only config field excluded from config_sha256_sans_secrets; changing it does not break the freeze.",
        "hashed": hashed,
        "reference_validation": {"sha256": sha256_file(rv_path), "redis_version": rv["redis_version"],
                                 "smoke": "%d/%d" % (rv["smoke"]["passed"], rv["smoke"]["total"]),
                                 "hidden": "%d/%d" % (rv["hidden"]["passed"], rv["hidden"]["total"]),
                                 "repeats": rv["repeats"]},
        "tests_validated_on_reference": True,
        "scored_runs_allowed": True,
        "contestant_runs_performed_at_freeze": 0,
        "openrouter_spend_usd_at_freeze": 0.0,
        "authored_by": "Claude Code (authoring only; fixed artefacts). Loop/scoring owned by deterministic scripts.",
    }
    with open(FREEZE_PATH, "w") as f:
        json.dump(fr, f, indent=2)
    print("wrote", FREEZE_PATH)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--verify", action="store_true")
    a = ap.parse_args()
    if a.verify:
        r = verify()
        print(json.dumps({"ok": r["ok"], "problems": r["problems"]}, indent=2))
        sys.exit(0 if r["ok"] else 1)
    write()


if __name__ == "__main__":
    main()
