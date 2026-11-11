#!/usr/bin/env python3
"""Block commits and pushes whose content contains secrets or private infrastructure data (the repo is public).

Two ways in:
- PreToolUse(Bash) hook, before a command that commits or pushes. It runs before the command, so changes staged or
  committed by the same command are not in the index or the log yet: it also checks every uncommitted change and
  new file.
- Git hooks `pre-commit` and `pre-push` (installed by `python3 .claude/hooks/secret_guard.py install`), which see
  exactly what is committed or pushed, however the command was written.

Uses gitleaks when installed, a small built-in pattern set and the local denylist (testdata/contracts/denylist.txt,
gitignored) of real names and addresses, each matched anywhere in a line ("192.0.2." covers the whole range).
"""
import json
import os
import re
import subprocess
import sys

PATTERNS = [
    (r"-----BEGIN [A-Z ]*PRIVATE KEY-----", "private key"),
    (r"\b[0-9a-f]{40}\b", "40-hex token (Gitea/GitHub style)"),
    (r"\b[A-Za-z0-9]{64}\b", "64-char token (Hetzner Cloud style)"),
    (r"(?i)(api[_-]?key|auth[_-]?token|secret|password|passwd)\s*[:=]\s*['\"]?[^\s'\"]{12,}", "credential assignment"),
    (r"\bghp_[A-Za-z0-9]{36}\b", "GitHub token"),
]
ALLOW_MARK = "hashi:allow-secret-pattern"  # put on a line that is a known false positive


def run(cmd, cwd):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=60)


# Lockfiles and checksum files are full of hashes that look like tokens. Generated code (*.gen.go, *.gen.ts, sqlc's
# output) only repeats api/openapi.yaml and the .sql files, which are scanned; its long type names read as tokens.
EXCLUDES = ["--", ".", ":!**/go.sum", ":!**/pnpm-lock.yaml", ":!**/package-lock.json", ":!**/*.sha256",
            ":!**/*.gen.go", ":!**/*.gen.ts", ":!internal/store/db/*.go"]


def pending(project):
    """Every uncommitted change against HEAD plus untracked files, as added lines."""
    out = run(["git", "diff", "HEAD", "--no-color"] + EXCLUDES, project).stdout
    new = run(["git", "ls-files", "--others", "--exclude-standard", "-z"], project).stdout.split("\0")
    for f in new:
        if not f or f.endswith((".sum", ".sha256", "pnpm-lock.yaml", ".gen.go", ".gen.ts")) or f.startswith("internal/store/db/"):
            continue
        try:
            with open(os.path.join(project, f), errors="replace") as fh:
                out += "".join("+" + l for l in fh.read(2 << 20).splitlines(True))
        except OSError:
            pass
    return out


def diff_for(command, project):
    if re.search(r"\bgit\s+push\b", command):
        upstream = run(["git", "rev-parse", "--abbrev-ref", "@{u}"], project)
        base = upstream.stdout.strip() if upstream.returncode == 0 else ""
        if not base and run(["git", "rev-parse", "--verify", "-q", "origin/main"], project).returncode == 0:
            base = "origin/main"
        rng = "%s..HEAD" % base if base else "HEAD~20..HEAD"
        return run(["git", "log", "-p", "--no-color", rng] + EXCLUDES, project).stdout + pending(project)
    return pending(project)


def added_lines(diff):
    for line in diff.splitlines():
        if line.startswith("+") and not line.startswith("+++") and ALLOW_MARK not in line:
            yield line[1:]


def module_path(project):
    """The Go module path from go.mod: public by construction, so it never counts as private infrastructure."""
    try:
        for line in open(os.path.join(project, "go.mod")):
            if line.startswith("module "):
                return line.split()[1]
    except OSError:
        pass
    return ""


def push_ranges(project, stdin):
    """pre-push gets "<local ref> <local sha> <remote ref> <remote sha>" lines."""
    zero = "0" * 40
    out = ""
    for line in stdin.splitlines():
        f = line.split()
        if len(f) != 4 or f[1] == zero:
            continue
        rng = f[1] if f[3] == zero else "%s..%s" % (f[3], f[1])
        if f[3] == zero:
            rng = [f[1], "--not", "--remotes"]
        else:
            rng = [rng]
        out += run(["git", "log", "-p", "--no-color"] + rng + EXCLUDES, project).stdout
    return out


HOOK = """#!/bin/sh
# Installed by .claude/hooks/secret_guard.py install: the same check on exactly what is committed or pushed.
exec python3 "$(git rev-parse --show-toplevel)/.claude/hooks/secret_guard.py" %s
"""


def install(project):
    hooks = run(["git", "rev-parse", "--git-path", "hooks"], project).stdout.strip()
    hooks = os.path.join(project, hooks) if not os.path.isabs(hooks) else hooks
    for name in ("pre-commit", "pre-push"):
        path = os.path.join(hooks, name)
        with open(path, "w") as fh:
            fh.write(HOOK % name)
        os.chmod(path, 0o755)
        print("installed", path)
    return 0


def main():
    project = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if mode == "install":
        return install(project)
    if mode == "pre-commit":
        diff = run(["git", "diff", "--cached", "--no-color"] + EXCLUDES, project).stdout
    elif mode == "pre-push":
        diff = push_ranges(project, sys.stdin.read())
    else:
        data = json.load(sys.stdin)
        command = data.get("tool_input", {}).get("command", "")
        if not re.search(r"\bgit\s+(commit|push)\b", command):
            return 0
        diff = diff_for(command, project)

    findings = []
    if run(["which", "gitleaks"], project).returncode == 0:
        res = run(["gitleaks", "git", "--staged", "--no-banner", "--redact", "-v"], project)
        if res.returncode == 1:
            findings.append("gitleaks:\n" + res.stdout[-2000:])

    patterns = list(PATTERNS)
    deny = os.path.join(project, "testdata", "contracts", "denylist.txt")
    if os.path.exists(deny):
        for item in open(deny).read().split():
            if item.startswith("#"):
                continue
            patterns.append((re.escape(item), "private infrastructure name/address"))

    module = module_path(project)
    for text in added_lines(diff):
        if module:
            text = text.replace(module, "<module>")
        for pat, label in patterns:
            if re.search(pat, text):
                findings.append("%s: %s" % (label, text.strip()[:120]))
                break

    if not findings:
        return 0
    print(
        "Blocked: possible secrets or private infrastructure data in outgoing changes (public repo).\n"
        + "\n".join(findings[:15])
        + "\nRemove them (use placeholders / sanitized fixtures). If a line is a verified false positive, "
        "add the comment '%s' on that line." % ALLOW_MARK,
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as e:
        print("secret_guard failed: %s; blocking to be safe." % e, file=sys.stderr)
        sys.exit(2)
