#!/usr/bin/env python3
"""Stop hook: Claude may only finish a turn that changed code once `make check` passes.

Loop guard: after MAX_BLOCKS consecutive failed checks in one session, let Claude stop and
tell it to record the failure under "Blocked" in docs/progress.md instead of trying again.
"""
import json
import os
import signal
import subprocess
import sys

PROJECT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
MAX_BLOCKS = 3
# A full `make check` on the laptop takes several minutes (the Go race tests alone need ~20 CPU-minutes). Keep the
# hook's own timeout in .claude/settings.json above this one.
TIMEOUT_S = 720


def changed_code(project):
    try:
        out = subprocess.run(
            ["git", "status", "--porcelain"], cwd=project, capture_output=True, text=True, timeout=20
        ).stdout
    except Exception:
        return False
    for line in out.splitlines():
        path = line[3:]
        if not path.endswith(".md"):
            return True
    return False


def has_check_target(project):
    makefile = os.path.join(project, "Makefile")
    if not os.path.exists(makefile):
        return False
    with open(makefile) as f:
        return any(line.startswith("check:") for line in f)


def run_check(project):
    # In its own process group, so a timeout stops the whole run: killing only `make` left its `go test` running,
    # and that slowed down the next check.
    proc = subprocess.Popen(
        ["make", "check"], cwd=project, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        start_new_session=True,
    )
    try:
        out, _ = proc.communicate(timeout=TIMEOUT_S)
        return proc.returncode == 0, out or ""
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        proc.communicate()
        return False, "make check timed out after %ss" % TIMEOUT_S


def main():
    data = json.load(sys.stdin)
    project = PROJECT
    if not has_check_target(project) or not changed_code(project):
        return 0

    state_dir = os.path.join(project, ".claude", ".state")
    os.makedirs(state_dir, exist_ok=True)
    state = os.path.join(state_dir, "verify-blocks-%s" % data.get("session_id", "default"))
    blocks = int(open(state).read().strip() or 0) if os.path.exists(state) else 0

    ok, output = run_check(project)

    if ok:
        if os.path.exists(state):
            os.remove(state)
        return 0

    if blocks >= MAX_BLOCKS:
        os.remove(state)
        print(
            "make check still fails after %d attempts. Stop fixing. Write the failing output, your "
            "hypothesis and what you tried under 'Blocked' in docs/progress.md, then tell the user." % MAX_BLOCKS,
            file=sys.stderr,
        )
        return 0

    with open(state, "w") as f:
        f.write(str(blocks + 1))
    tail = "\n".join(output.strip().splitlines()[-40:])
    print(
        "make check failed (attempt %d/%d):\n%s\n\nFix the cause. Do not weaken, skip or delete tests, "
        "and do not edit TestInvariant_* tests." % (blocks + 1, MAX_BLOCKS, tail),
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.exit(0)  # a broken hook must never wedge the session
