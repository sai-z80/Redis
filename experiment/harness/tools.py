"""Agent tools. All paths are confined to the run workspace; run_shell executes in a
bubblewrap sandbox (no network, clean env, no access to the pilot dir / API keys)."""
import json, os

from .common import sandbox_shell

TOOL_SCHEMAS = [
    {"type": "function", "function": {
        "name": "list_dir",
        "description": "List entries of a directory in the workspace. Path is relative to the workspace root.",
        "parameters": {"type": "object", "properties": {
            "path": {"type": "string", "description": "Directory path relative to workspace root; default '.'"}},
            "required": []}}},
    {"type": "function", "function": {
        "name": "read_file",
        "description": "Read a text file from the workspace. Optional 1-based line range.",
        "parameters": {"type": "object", "properties": {
            "path": {"type": "string"},
            "start_line": {"type": "integer"},
            "end_line": {"type": "integer"}},
            "required": ["path"]}}},
    {"type": "function", "function": {
        "name": "write_file",
        "description": "Create or overwrite a file in the workspace with the given content. Parent directories are created.",
        "parameters": {"type": "object", "properties": {
            "path": {"type": "string"},
            "content": {"type": "string"}},
            "required": ["path", "content"]}}},
    {"type": "function", "function": {
        "name": "run_shell",
        "description": ("Run a shell command (sh -c) with the workspace root as working directory. "
                        "No network access. All processes started by the command, including background "
                        "ones, are killed when the call returns. Returns exit code and combined stdout/stderr."),
        "parameters": {"type": "object", "properties": {
            "command": {"type": "string"},
            "timeout_s": {"type": "integer", "description": "Timeout in seconds (default and max are fixed by the harness)."}},
            "required": ["command"]}}},
]


class ToolError(Exception):
    pass


class Tools:
    def __init__(self, workspace, cache_dir, cfg):
        self.ws = os.path.realpath(workspace)
        self.cache = cache_dir
        self.cfg = cfg

    def _resolve(self, rel):
        if rel is None or rel == "":
            rel = "."
        if not isinstance(rel, str):
            raise ToolError("path must be a string")
        if rel == "/work" or rel.startswith("/work/"):  # sandbox view of the workspace
            rel = rel[len("/work"):].lstrip("/") or "."
        if os.path.isabs(rel):
            raise ToolError("absolute paths are not allowed; use paths relative to the workspace root")
        full = os.path.realpath(os.path.join(self.ws, rel))
        if full != self.ws and not full.startswith(self.ws + os.sep):
            raise ToolError("path escapes the workspace")
        return full

    def _clip(self, s):
        m = self.cfg["max_tool_output_chars"]
        if len(s) > m:
            half = m // 2
            return s[:half] + "\n...[%d chars truncated]...\n" % (len(s) - m) + s[-half:]
        return s

    def dispatch(self, name, raw_args):
        try:
            args = json.loads(raw_args) if isinstance(raw_args, str) and raw_args.strip() else (raw_args or {})
            if not isinstance(args, dict):
                raise ToolError("arguments must be a JSON object")
        except json.JSONDecodeError as e:
            return "ERROR: invalid JSON arguments: %s" % e
        fn = getattr(self, "t_" + name, None) if name in {t["function"]["name"] for t in TOOL_SCHEMAS} else None
        if fn is None:
            return "ERROR: unknown tool %r" % name
        try:
            return self._clip(fn(**args))
        except ToolError as e:
            return "ERROR: %s" % e
        except TypeError as e:
            return "ERROR: bad arguments: %s" % e
        except OSError as e:
            return "ERROR: %s" % e

    def t_list_dir(self, path="."):
        full = self._resolve(path)
        if not os.path.isdir(full):
            raise ToolError("not a directory: %s" % path)
        rows = []
        for n in sorted(os.listdir(full)):
            fp = os.path.join(full, n)
            if os.path.isdir(fp):
                rows.append(n + "/")
            else:
                rows.append("%s\t%d" % (n, os.path.getsize(fp)))
        return "\n".join(rows) if rows else "(empty)"

    def t_read_file(self, path, start_line=None, end_line=None):
        full = self._resolve(path)
        if not os.path.isfile(full):
            raise ToolError("not a file: %s" % path)
        with open(full, "rb") as f:
            data = f.read(self.cfg["max_read_bytes"] + 1)
        truncated = len(data) > self.cfg["max_read_bytes"]
        text = data[: self.cfg["max_read_bytes"]].decode("utf-8", "replace")
        if start_line or end_line:
            lines = text.splitlines(keepends=True)
            s = max(1, int(start_line or 1))
            e = int(end_line or len(lines))
            text = "".join(lines[s - 1:e])
        elif truncated:
            text += "\n...[file truncated at %d bytes; use start_line/end_line]" % self.cfg["max_read_bytes"]
        return text

    def t_write_file(self, path, content):
        if not isinstance(content, str):
            raise ToolError("content must be a string")
        if len(content.encode()) > self.cfg["max_write_bytes"]:
            raise ToolError("content exceeds max_write_bytes")
        full = self._resolve(path)
        if os.path.isdir(full):
            raise ToolError("path is a directory")
        os.makedirs(os.path.dirname(full), exist_ok=True)
        with open(full, "w") as f:
            f.write(content)
        return "wrote %d bytes to %s" % (len(content.encode()), os.path.relpath(full, self.ws))

    def t_run_shell(self, command, timeout_s=None):
        if not isinstance(command, str) or not command.strip():
            raise ToolError("command must be a non-empty string")
        t = int(timeout_s or self.cfg["shell_timeout_default_s"])
        t = max(1, min(t, self.cfg["shell_timeout_max_s"]))
        r = sandbox_shell(command, self.ws, self.cache, t)
        head = "exit_code=%s%s seconds=%s\n" % (r["exit_code"], " TIMED_OUT" if r["timed_out"] else "", r["seconds"])
        return head + r["output"]
