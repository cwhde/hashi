"""Choose PR feedback from the exact trees checked out by CI."""

import json
import os
import re
import subprocess

JOBS = ("go", "web", "integration", "e2e")


def checks(paths):
    jobs = set()
    go_paths = []
    for path in paths:
        if path in {"README.md", "CLAUDE.md"} or (
            path.startswith("docs/") and path.endswith(".md")
        ):
            continue
        if path.startswith(("internal/", "cmd/", "agents/")) and path.endswith(".go"):
            if path.startswith(("internal/store/", "internal/httpapi/")):
                jobs.add("web")
            jobs.update(("go", "integration"))
            if not path.endswith("_test.go"):
                jobs.add("e2e")
            go_paths.append(path)
        elif path.startswith(("web/src/", "web/static/", "web/e2e/")):
            jobs.update(("web", "e2e"))
        else:
            # Shared inputs, fixtures, build/config changes and unclassified files.
            return set(JOBS), []
    return jobs, go_paths


def changed_paths(base):
    if not re.fullmatch(r"[0-9a-f]{40}", base):
        raise ValueError("missing exact PR base SHA")
    exists = subprocess.run(
        ["git", "cat-file", "-e", base + "^{commit}"], capture_output=True
    ).returncode == 0
    if not exists:
        subprocess.run(
            ["git", "fetch", "--no-tags", "--depth=1", "--filter=blob:none", "origin", base],
            check=True,
        )
    result = subprocess.check_output(["git", "diff", "--no-renames", "--name-only", "-z", base, "HEAD"])
    if len(result) > 16_000:
        raise ValueError("diff too large to pass through job outputs")
    return os.fsdecode(result).split("\0")[:-1]


def main():
    jobs, go_paths = set(JOBS), []
    if os.environ["EVENT"] == "pull_request":
        try:
            jobs, go_paths = checks(changed_paths(os.environ.get("BASE_SHA", "")))
        except (subprocess.CalledProcessError, ValueError) as error:
            print(f"Cannot classify the PR ({error}); running all checks.")

    outputs = {job: str(job in jobs).lower() for job in JOBS}
    outputs["go_paths"] = json.dumps(go_paths)
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as file:
        for key, value in outputs.items():
            print(f"{key}={value}", file=file)
            print(f"{key}={value}")


if __name__ == "__main__":
    main()
