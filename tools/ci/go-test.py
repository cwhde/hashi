"""Run race tests for changed packages and their transitive/test-only importers."""

from collections import defaultdict
import json
import os
from pathlib import Path
import subprocess
import sys


def objects(data):
    decoder = json.JSONDecoder()
    remaining = data.lstrip()
    while remaining:
        package, end = decoder.raw_decode(remaining)
        yield package
        remaining = remaining[end:].lstrip()


def import_path(package):
    # go list -test appends " [pkg.test]" to packages rebuilt for that test.
    return package.split(" ", 1)[0]


def find_packages(paths):
    workspace = Path.cwd()
    data = subprocess.check_output(
        ["go", "list", "-deps", "-test", "-race", "-json=ImportPath,ForTest,Dir,Deps,Module", "./..."],
        text=True,
    )
    owners = {}
    dependencies = defaultdict(set)
    for package in objects(data):
        if not package.get("Module", {}).get("Main"):
            continue
        owner = package.get("ForTest") or import_path(package["ImportPath"])
        if "ForTest" not in package and not owner.endswith(".test"):
            owners[Path(package["Dir"]).relative_to(workspace)] = owner
        dependencies[owner].update(map(import_path, package.get("Deps", ())))

    seeds = {owners[Path(path).parent] for path in paths}
    return sorted(
        owner for owner in owners.values() if owner in seeds or not seeds.isdisjoint(dependencies[owner])
    )


def main():
    paths = json.loads(os.environ.get("CI_GO_PATHS", "[]"))
    packages = ["./..."]
    if paths:
        try:
            selected = find_packages(paths)
            if selected:
                packages = selected
        except (subprocess.CalledProcessError, KeyError, ValueError) as error:
            print(f"Cannot select Go packages ({error}); running all packages.")
    print("Race tests:", *packages, flush=True)
    # internal/app runs ~2 min alone under -race; with every package at once on the shared runner host it reached
    # go test's default 10 minutes (bug 190), so CI allows 20.
    return subprocess.call(["go", "test", "-race", "-count=1", "-timeout=20m", *packages])


if __name__ == "__main__":
    sys.exit(main())
