#!/usr/bin/env python3
"""PreToolUse(Bash) hook: block `git commit` / `git push` when staged or outgoing changes contain secrets.

The repository is public. Uses gitleaks when installed, plus a small built-in pattern set and the
local denylist (testdata/contracts/denylist.txt, gitignored) of real hostnames/IPs.
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


# Lockfiles and checksum files are full of hashes that look like tokens.
EXCLUDES = ["--", ".", ":!**/go.sum", ":!**/pnpm-lock.yaml", ":!**/package-lock.json", ":!**/*.sha256"]


def diff_for(command, project):
    if re.search(r"\bgit\s+push\b", command):
        upstream = run(["git", "rev-parse", "--abbrev-ref", "@{u}"], project)
        base = upstream.stdout.strip() if upstream.returncode == 0 else ""
        if not base and run(["git", "rev-parse", "--verify", "-q", "origin/main"], project).returncode == 0:
            base = "origin/main"
        rng = "%s..HEAD" % base if base else "HEAD~20..HEAD"
        return run(["git", "log", "-p", "--no-color", rng] + EXCLUDES, project).stdout
    return run(["git", "diff", "--cached", "--no-color"] + EXCLUDES, project).stdout


def added_lines(diff):
    for line in diff.splitlines():
        if line.startswith("+") and not line.startswith("+++") and ALLOW_MARK not in line:
            yield line[1:]


def main():
    data = json.load(sys.stdin)
    command = data.get("tool_input", {}).get("command", "")
    if not re.search(r"\bgit\s+(commit|push)\b", command):
        return 0
    project = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

    findings = []
    if run(["which", "gitleaks"], project).returncode == 0:
        res = run(["gitleaks", "git", "--staged", "--no-banner", "--redact", "-v"], project)
        if res.returncode == 1:
            findings.append("gitleaks:\n" + res.stdout[-2000:])

    patterns = list(PATTERNS)
    deny = os.path.join(project, "testdata", "contracts", "denylist.txt")
    if os.path.exists(deny):
        for item in open(deny).read().split():
            patterns.append((re.escape(item), "private infrastructure name/address"))

    for text in added_lines(diff_for(command, project)):
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
