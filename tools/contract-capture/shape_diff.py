#!/usr/bin/env python3
"""Compare the response shapes of a fresh capture with the committed fixtures (03-dns.md §2 point 4).

Usage: shape_diff.py <fresh raw dir> <fixture dir>

A shape is the set of JSON paths with their value types (e.g. `body.rrsets[].ttl: null|number`) plus the status and
the kept header names. Pages of one endpoint (`rrsets_page1`, `rrsets_page2`, …) are merged into one shape, so a zone
that grew a page is not drift. Only paths and types are printed, never values, so the output is safe for a CI log even
though the fresh capture is unsanitized. Objects under `labels` are maps with user-chosen keys: only their value types
count. Exit status 1 on any difference: re-capture and update the adapter when the API changed.
"""
import json
import pathlib
import re
import sys

MAPS = {"labels"}


def walk(v, path: str, out: dict) -> None:
    if isinstance(v, dict):
        out.setdefault(path, set()).add("object")
        leaf = path.rsplit(".", 1)[-1]
        for k, child in v.items():
            walk(child, f"{path}.{'*' if leaf in MAPS else k}", out)
    elif isinstance(v, list):
        out.setdefault(path, set()).add("array")
        for child in v:
            walk(child, path + "[]", out)
    else:
        t = "null" if v is None else "bool" if isinstance(v, bool) else "number" if isinstance(v, (int, float)) else "string"
        out.setdefault(path, set()).add(t)


def shapes(d: pathlib.Path) -> dict:
    groups: dict = {}
    for f in sorted(d.glob("*.json")):
        name = re.sub(r"_page\d+$", "", f.stem)
        resp = json.loads(f.read_text())
        g = groups.setdefault(name, {})
        g.setdefault("status", set()).add(str(resp["status"]))
        g.setdefault("headers", set()).update(resp.get("headers", {}).keys())
        walk(resp.get("body"), "body", g)
    return groups


def main() -> int:
    fresh, fixtures = (shapes(pathlib.Path(p)) for p in sys.argv[1:3])
    drift = []
    for name in sorted(fixtures.keys() | fresh.keys()):
        old, new = fixtures.get(name), fresh.get(name)
        if old is None or new is None:
            drift.append(f"{name}: {'not in the fixtures' if old is None else 'not captured'}")
            continue
        for path in sorted(old.keys() | new.keys()):
            a, b = old.get(path), new.get(path)
            if a != b:
                fmt = lambda s: "absent" if s is None else "|".join(sorted(s))  # noqa: E731
                drift.append(f"{name}: {path}: {fmt(a)} -> {fmt(b)}")
    for line in drift:
        print(line)
    print(f"{len(drift)} difference(s) across {len(fixtures)} endpoint(s)" if drift else f"no drift across {len(fixtures)} endpoint(s)")
    return 1 if drift else 0


if __name__ == "__main__":
    sys.exit(main())
