#!/usr/bin/env python3
"""Read-only contract capture for the Hetzner Cloud DNS API (03-dns.md §2).

Writes raw responses to .contracts-raw/hetzner/ (gitignored). Only GET requests are sent.
Token: $HETZNER_TOKEN, else the macOS keychain item `hashi-hetzner-token`. It is never printed or written.
Run via `make contract-capture PROVIDER=hetzner`, then sanitize with sanitize.py.
"""
import datetime
import json
import os
import pathlib
import subprocess
import sys
import urllib.error
import urllib.request

API = "https://api.hetzner.cloud/v1"
DOCS = "https://docs.hetzner.cloud/reference/cloud#zones"
OUT = pathlib.Path(".contracts-raw/hetzner")
KEEP_HEADERS = ("content-type", "ratelimit-limit", "ratelimit-remaining", "ratelimit-reset")


def token() -> str:
    t = os.environ.get("HETZNER_TOKEN", "")
    if not t:
        t = subprocess.run(
            ["security", "find-generic-password", "-s", "hashi-hetzner-token", "-w"],
            capture_output=True, text=True, check=True,
        ).stdout.strip()
    if not t:
        sys.exit("no token: set HETZNER_TOKEN or the keychain item hashi-hetzner-token")
    return t


def get(path: str, tok: str):
    req = urllib.request.Request(API + path, headers={"Authorization": f"Bearer {tok}"}, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            status, headers, body = r.status, r.headers, r.read()
    except urllib.error.HTTPError as e:
        status, headers, body = e.code, e.headers, e.read()
    return {
        "request": f"GET /v1{path}",
        "status": status,
        "headers": {k.lower(): v for k, v in headers.items() if k.lower() in KEEP_HEADERS},
        "body": json.loads(body) if body else None,
    }


def save(name: str, resp: dict) -> None:
    resp = {
        "captured_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        "docs_url": DOCS,
        **resp,
    }
    (OUT / f"{name}.json").write_text(json.dumps(resp, indent=2, ensure_ascii=False) + "\n")
    print(f"{name}: {resp['status']}")


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    tok = token()

    save("zones_page1", get("/zones?per_page=1&page=1", tok))
    save("zones_page2", get("/zones?per_page=1&page=2", tok))
    zones = get("/zones?per_page=50", tok)["body"]["zones"]
    zone = max(zones, key=lambda z: z.get("record_count") or 0)
    save("zone_get", get(f"/zones/{zone['id']}", tok))
    save("zone_get_by_name", get(f"/zones/{zone['name']}", tok))

    page = 1
    while page:
        r = get(f"/zones/{zone['id']}/rrsets?per_page=25&page={page}", tok)
        save(f"rrsets_page{page}", r)
        page = (r["body"] or {}).get("meta", {}).get("pagination", {}).get("next_page")

    save("error_unknown_zone", get("/zones/999999999", tok))
    save("error_bad_token", get("/zones", "hashi-contract-capture-invalid-token"))


if __name__ == "__main__":
    main()
