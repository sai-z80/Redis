"""Shared helpers: paths, hashing, config, sandbox. Stdlib only. No network."""
import hashlib, json, os, shutil, signal, subprocess, time

PILOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GO_ROOT = os.environ.get("SF_GO_ROOT", "/usr/local/go")


def p(*parts):
    return os.path.join(PILOT, *parts)


def load_config(path):
    with open(path if os.path.isabs(path) else p(path)) as f:
        return json.load(f)


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def tree_hashes(root):
    """{relpath: sha256} for every regular file under root (sorted, skips __pycache__)."""
    out = {}
    for d, dirs, files in os.walk(root):
        dirs[:] = sorted(x for x in dirs if x != "__pycache__")
        for fn in sorted(files):
            fp = os.path.join(d, fn)
            out[os.path.relpath(fp, root)] = sha256_file(fp)
    return dict(sorted(out.items()))


def tree_digest(root):
    h = hashlib.sha256()
    for rel, digest in tree_hashes(root).items():
        h.update(("%s  %s\n" % (digest, rel)).encode())
    return h.hexdigest()


def sha256_json(obj):
    return hashlib.sha256(json.dumps(obj, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def now_iso():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


# ---------------------------------------------------------------- sandbox
SAFE_ENV = {
    "PATH": "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
    "HOME": "/tmp",
    "LANG": "C.UTF-8",
    "GOCACHE": "/gocache/build",
    "GOPATH": "/gocache/gopath",
    "GOMODCACHE": "/gocache/gopath/pkg/mod",
    "GOTOOLCHAIN": "local",
    "GOPROXY": "off",
    "GOFLAGS": "-mod=mod",
    "GOTELEMETRY": "off",
    "CGO_ENABLED": "0",
}


def bwrap_argv(workdir, cache_dir, extra_ro_binds=(), cwd="/work"):
    """bubblewrap: fresh namespaces incl. network (loopback only), clean env,
    only /usr (+Go root) read-only, workspace rw at /work, go cache rw at /gocache."""
    if not shutil.which("bwrap"):
        raise RuntimeError("bwrap not found: refusing to run untrusted commands unsandboxed")
    argv = ["bwrap", "--unshare-all", "--die-with-parent", "--new-session", "--clearenv",
            "--ro-bind", "/usr", "/usr"]
    for link in ("lib", "lib64", "bin", "sbin"):
        tgt = os.path.join("/", link)
        if os.path.islink(tgt):
            argv += ["--symlink", os.readlink(tgt), tgt]
        elif os.path.isdir(tgt):
            argv += ["--ro-bind", tgt, tgt]
    if not GO_ROOT.startswith("/usr/"):
        argv += ["--ro-bind", GO_ROOT, GO_ROOT]
    for etc in ("/etc/ld.so.cache", "/etc/localtime", "/etc/nsswitch.conf"):
        if os.path.exists(etc):
            argv += ["--ro-bind", etc, etc]
    argv += ["--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
             "--bind", workdir, "/work", "--bind", cache_dir, "/gocache"]
    for src, dst in extra_ro_binds:
        argv += ["--ro-bind", src, dst]
    for k, v in SAFE_ENV.items():
        argv += ["--setenv", k, v]
    argv += ["--chdir", cwd]
    return argv


def sandbox_shell(command, workdir, cache_dir, timeout, extra_ro_binds=(), max_output=None):
    """Run `sh -c command` in the sandbox. Kills the whole process group on return or
    timeout (so background servers never outlive the call). Returns dict."""
    os.makedirs(cache_dir, exist_ok=True)
    argv = bwrap_argv(workdir, cache_dir, extra_ro_binds) + ["/bin/sh", "-c", command]
    t0 = time.time()
    proc = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, start_new_session=True,
                            env={"PATH": SAFE_ENV["PATH"]})
    timed_out = False
    try:
        out, _ = proc.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        _killpg(proc)
        out, _ = proc.communicate()
    finally:
        _killpg(proc)
    text = out.decode("utf-8", "replace")
    if max_output and len(text) > max_output:
        half = max_output // 2
        text = text[:half] + "\n...[%d chars truncated]...\n" % (len(text) - max_output) + text[-half:]
    return {"exit_code": proc.returncode, "timed_out": timed_out,
            "seconds": round(time.time() - t0, 3), "output": text}


def _killpg(proc):
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass
