#!/usr/bin/env python3
"""Sanitize the write-probe capture (hetzner_write.py) for the public repo.

Usage: sanitize_write.py <raw dir> <out dir> <denylist>
Probe values are synthetic already (documentation addresses, `_hashi-test` names), so only the real zone leaks:
its name (found as the suffix of `….<probe>.<zone>.` values) → example.com, its id (`rrset.zone`, action resources)
→ 100001. Action ids are kept (opaque, not tied to the account). Fails if a denylist entry is still present.
"""
import json
import pathlib
import re
import sys

raw, out, deny = (pathlib.Path(p) for p in sys.argv[1:4])
out.mkdir(parents=True, exist_ok=True)
docs = {f.name: f.read_text() for f in sorted(raw.glob("*.json"))}

zone_name = zone_id = None
for text in docs.values():
    m = re.search(r"_hashi-test\.([a-z0-9.-]+)\.\"", text)
    zone_name = zone_name or (m and m.group(1))
    for d in [json.loads(text)]:
        rr = (d.get("body") or {}).get("rrset") if isinstance(d.get("body"), dict) else None
        if rr and isinstance(rr.get("zone"), int):
            zone_id = zone_id or rr["zone"]
if not zone_name or not zone_id:
    sys.exit("could not find the zone name and id in the capture")

denylist = [w for w in deny.read_text().split() if w] if deny.exists() else []
for name, text in docs.items():
    text = text.replace(zone_name, "example.com")
    text = re.sub(rf"\b{zone_id}\b", "100001", text)
    for w in denylist:
        if w.lower() in text.lower():
            sys.exit(f"{name}: denylisted value still present after sanitizing")
    (out / name).write_text(text)
print(f"sanitized {len(docs)} files into {out}")
