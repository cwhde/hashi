#!/usr/bin/env python3
"""PreToolUse(Bash) hook: force a human confirmation for state-changing commands on remote hosts.

Read-only inspection over SSH runs freely. Anything matching a mutating pattern inside an ssh/pct/qm
command turns into a permission prompt, regardless of the session's permission mode.
"""
import json
import re
import sys

REMOTE = re.compile(r"\b(ssh|scp|rsync|sftp|pct|qm|pvesh)\b")
MUTATING = [
    r"\biptables(-restore)?\b(?!.*\b(-L|-S|--list|-save)\b)",
    r"\bip6?tables\s+-(F|X|D|I|A|P|R|N|Z)\b",
    r"\bipset\s+(flush|destroy|del|add|swap|restore|create)\b",
    r"\bnft\s+(flush|delete|add|insert|replace|-f)\b",
    r"\bnetfilter-persistent\s+(save|flush|reload)\b",
    r"\bsystemctl\s+(restart|stop|start|disable|enable|reload|mask|daemon-reload)\b",
    r"\brc-service\b.*\b(restart|stop|start)\b",
    r"\b(reboot|shutdown|poweroff|halt)\b",
    r"\bpct\s+(stop|destroy|set|reboot|restore|start|create|exec)\b",
    r"\bqm\s+(stop|destroy|set|reboot|restore|start|create)\b",
    r"\bdocker(\s+compose)?\s+.*\b(up|down|rm|stop|restart|kill|pull|prune)\b",
    r"\bnetbird\s+(down|up|service|routes)\b",
    r"\brm\s+-[a-zA-Z]*r",
    r"\b(apt|apt-get|apk|dnf)\s+(install|remove|purge|upgrade|add|del)\b",
    r"(>|tee\s)\s*/etc/",
    r"\bsed\s+-i\b",
    r"\bcurl\b.*-X\s*(POST|PUT|PATCH|DELETE)",
]


def main():
    data = json.load(sys.stdin)
    command = data.get("tool_input", {}).get("command", "")
    if not REMOTE.search(command):
        return 0
    for pat in MUTATING:
        if re.search(pat, command):
            print(json.dumps({
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "ask",
                    "permissionDecisionReason": "Remote state change (matched %s). Production infrastructure: "
                    "confirm this exact command." % pat,
                }
            }))
            return 0
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "ask",
                                                 "permissionDecisionReason": "infra_guard error; confirm manually."}}))
        sys.exit(0)
