#!/usr/bin/env python3
"""Write-contract probe for the Hetzner Cloud DNS API (03-dns.md §2 step 3), approved by the user on 2026-10-28.

Every write is confined to probe RRSets: the relative name must be `_hashi-test` or end in `._hashi-test`, and the type
must be one Hashi manages. Paths are built from (name, type) here, never passed in, so no other RRSet can be addressed.
The run refuses to start if probe RRSets already exist (run with --cleanup to remove leftovers) and deletes everything
it created at the end. Values are documentation addresses (RFC 5737/3849) and example names only.

Raw exchanges go to .contracts-raw/hetzner-write/ (gitignored); sanitize with sanitize.py.
Token: $HETZNER_TOKEN, else the macOS keychain item `hashi-hetzner-token`. It is never printed or written.
"""
import datetime
import json
import pathlib
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

from hetzner import API, KEEP_HEADERS, token

DOCS = "https://docs.hetzner.cloud/reference/cloud#zone-rrsets"
OUT = pathlib.Path(".contracts-raw/hetzner-write")
PROBE = "_hashi-test"
MANAGED = {"A", "AAAA", "CNAME", "MX", "TXT", "SRV", "CAA"}
NAME_RE = re.compile(r"^([a-z0-9_-]+\.)*" + re.escape(PROBE) + r"$")


def is_probe(name: str) -> bool:
    return bool(NAME_RE.match(name))


class Client:
    def __init__(self, tok: str, zone_id: int):
        self.tok, self.zone = tok, zone_id

    def _send(self, method: str, path: str, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(API + path, data=data, method=method,
                                     headers={"Authorization": f"Bearer {self.tok}", "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                status, headers, raw = r.status, r.headers, r.read()
        except urllib.error.HTTPError as e:
            status, headers, raw = e.code, e.headers, e.read()
        return {
            "request": f"{method} /v1{path}",
            "request_body": body,
            "status": status,
            "headers": {k.lower(): v for k, v in headers.items() if k.lower() in KEEP_HEADERS},
            "body": json.loads(raw) if raw else None,
        }

    def get(self, path: str):
        return self._send("GET", path)

    def rrset_path(self, name: str, typ: str, suffix: str = "") -> str:
        if not is_probe(name):
            raise SystemExit(f"refusing to touch {name!r}: not a probe name")
        if typ not in MANAGED:
            raise SystemExit(f"refusing type {typ!r}")
        return f"/zones/{self.zone}/rrsets/{urllib.parse.quote(name, safe='')}/{typ}{suffix}"

    def create(self, name: str, typ: str, values, ttl=None, labels=None):
        if not is_probe(name):
            raise SystemExit(f"refusing to create {name!r}: not a probe name")
        if typ not in MANAGED:
            raise SystemExit(f"refusing type {typ!r}")
        body = {"name": name, "type": typ, "ttl": ttl, "records": [{"value": v} for v in values]}
        if labels is not None:
            body["labels"] = labels
        return self._send("POST", f"/zones/{self.zone}/rrsets", body)

    def create_unchecked_type(self, name: str, typ: str, values):
        """Only for the 'unsupported type' error probe: the name is still a probe name."""
        if not is_probe(name):
            raise SystemExit(f"refusing to create {name!r}: not a probe name")
        return self._send("POST", f"/zones/{self.zone}/rrsets",
                          {"name": name, "type": typ, "ttl": None, "records": [{"value": v} for v in values]})

    def action(self, name: str, typ: str, verb: str, body):
        return self._send("POST", self.rrset_path(name, typ, f"/actions/{verb}"), body)

    def put_labels(self, name: str, typ: str, labels):
        return self._send("PUT", self.rrset_path(name, typ), {"labels": labels})

    def delete(self, name: str, typ: str):
        return self._send("DELETE", self.rrset_path(name, typ))

    def get_rrset(self, name: str, typ: str):
        return self.get(self.rrset_path(name, typ))

    def probe_rrsets(self):
        out, page = [], 1
        while page:
            r = self.get(f"/zones/{self.zone}/rrsets?per_page=50&page={page}")
            out += [x for x in r["body"]["rrsets"] if is_probe(x["name"])]
            page = r["body"]["meta"]["pagination"]["next_page"]
        return out


def save(name: str, resp: dict) -> None:
    resp = {"captured_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"), "docs_url": DOCS, **resp}
    (OUT / f"{name}.json").write_text(json.dumps(resp, indent=2, ensure_ascii=False) + "\n")
    err = (resp.get("body") or {}).get("error") if isinstance(resp.get("body"), dict) else None
    print(f"{name}: {resp['status']}" + (f" {err['code']}: {err['message']}" if err else ""))


def wait(c: Client, resp: dict, label: str) -> None:
    """Poll the returned action until it leaves 'running'; save the first poll and the final state."""
    act = (resp.get("body") or {}).get("action")
    if not act:
        return
    deadline = time.time() + 60
    first = True
    while True:
        r = c.get(f"/actions/{act['id']}")
        if first and label:
            save(f"{label}_action_poll", r)
            first = False
        status = r["body"]["action"]["status"]
        if status != "running" or time.time() > deadline:
            if status != "success" and label:
                save(f"{label}_action_final", r)
            return
        time.sleep(1)


def cleanup(c: Client) -> None:
    for r in c.probe_rrsets():
        if r["type"] in MANAGED:
            res = c.delete(r["name"], r["type"])
            print(f"cleanup {r['name']} {r['type']}: {res['status']}")
            wait(c, res, "cleanup")
    left = c.probe_rrsets()
    print("probe RRSets left:", [(r["name"], r["type"]) for r in left])


def challenge(c: Client) -> None:
    """DNS-01 challenge values (03 §5.3): add_records on a missing RRSet, a second writer's value next to Hashi's,
    removing one value, removing the last value. The name is a probe name shaped like a challenge name."""
    name = "_acme-challenge." + PROBE
    if c.probe_rrsets():
        sys.exit("probe RRSets exist already; run with --cleanup first")
    try:
        r = c.action(name, "TXT", "add_records", {"ttl": 60, "records": [{"value": '"hashi challenge probe one"'}]})
        save("challenge_add_missing", r)
        wait(c, r, "challenge_add_missing")
        save("challenge_get_after_add", c.get_rrset(name, "TXT"))
        r = c.action(name, "TXT", "add_records", {"records": [{"value": '"other writer probe two"'}]})
        save("challenge_add_second", r)
        wait(c, r, "challenge_add_second")
        r = c.action(name, "TXT", "add_records", {"records": [{"value": '"other writer probe two"'}]})
        save("challenge_add_duplicate", r)
        wait(c, r, "challenge_add_duplicate")
        save("challenge_get_two", c.get_rrset(name, "TXT"))
        r = c.action(name, "TXT", "remove_records", {"records": [{"value": '"hashi challenge probe one"'}]})
        save("challenge_remove_one", r)
        wait(c, r, "challenge_remove_one")
        save("challenge_get_after_remove", c.get_rrset(name, "TXT"))
        r = c.action(name, "TXT", "remove_records", {"records": [{"value": '"hashi challenge probe one"'}]})
        save("challenge_remove_absent", r)
        wait(c, r, "challenge_remove_absent")
        r = c.action(name, "TXT", "remove_records", {"records": [{"value": '"other writer probe two"'}]})
        save("challenge_remove_last", r)
        wait(c, r, "challenge_remove_last")
        save("challenge_get_after_last", c.get_rrset(name, "TXT"))
        r = c.action(name, "TXT", "remove_records", {"records": [{"value": '"other writer probe two"'}]})
        save("challenge_remove_missing_rrset", r)
        wait(c, r, "challenge_remove_missing_rrset")
    finally:
        cleanup(c)


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    tok = token()
    probe = Client(tok, 0)
    zones = probe.get("/zones?per_page=50")["body"]["zones"]
    zone = max(zones, key=lambda z: z.get("record_count") or 0)
    c = Client(tok, zone["id"])
    zn = zone["name"]

    if "--cleanup" in sys.argv:
        cleanup(c)
        return
    if "--challenge" in sys.argv:
        challenge(c)
        return
    if c.probe_rrsets():
        sys.exit("probe RRSets exist already; run with --cleanup first")

    try:
        # Create one RRSet per managed type (ttl null = zone default; one with an explicit TTL).
        r = c.create(PROBE, "A", ["192.0.2.10", "192.0.2.11"])
        save("create_a", r)
        wait(c, r, "create_a")
        save("get_a", c.get_rrset(PROBE, "A"))
        r = c.create(PROBE, "AAAA", ["2001:db8::10"], ttl=300)
        save("create_aaaa", r)
        wait(c, r, "create_aaaa")
        r = c.create("cname." + PROBE, "CNAME", [f"{PROBE}.{zn}."])
        save("create_cname", r)
        wait(c, r, "create_cname")
        r = c.create(PROBE, "MX", [f"10 mx1.{PROBE}.{zn}.", f"20 mx2.{PROBE}.{zn}."])
        save("create_mx", r)
        wait(c, r, "create_mx")
        r = c.create("_sip._tcp." + PROBE, "SRV", [f"10 60 5060 sip.{PROBE}.{zn}.", f"20 0 5061 sip2.{PROBE}.{zn}."])
        save("create_srv", r)
        wait(c, r, "create_srv")
        r = c.create(PROBE, "CAA", ['0 issue "letsencrypt.org"', '0 issuewild ";"', '128 iodef "mailto:hostmaster@example.com"'])
        save("create_caa", r)
        wait(c, r, "create_caa")

        # TXT: quoted, unquoted, and a 300-byte value in one quoted string (does the API chunk or reject?).
        r = c.create(PROBE, "TXT", ['"hashi contract probe"', '"part one " "part two"', '"with \\"escaped\\" quotes"'])
        save("create_txt", r)
        wait(c, r, "create_txt")
        save("create_txt_unquoted", c.create("unquoted." + PROBE, "TXT", ["hashi unquoted probe"]))
        save("create_txt_long", c.create("long." + PROBE, "TXT", ['"' + "x" * 300 + '"']))
        save("get_after_creates", c.get(f"/zones/{c.zone}/rrsets?name={urllib.parse.quote(PROBE)}&per_page=50"))
        for name, typ in [("unquoted." + PROBE, "TXT"), ("long." + PROBE, "TXT"), ("_sip._tcp." + PROBE, "SRV"), (PROBE, "CAA")]:
            r = c.get_rrset(name, typ)
            save(f"get_{name.split('.')[0].strip('_')}_{typ.lower()}", r)

        # Changes: replace records, change TTL (and back to zone default), add, remove, labels (ownership marker).
        r = c.action(PROBE, "A", "set_records", {"records": [{"value": "192.0.2.12"}]})
        save("set_records_a", r)
        wait(c, r, "set_records_a")
        r = c.action(PROBE, "A", "change_ttl", {"ttl": 600})
        save("change_ttl_a", r)
        wait(c, r, "change_ttl_a")
        r = c.action(PROBE, "A", "change_ttl", {"ttl": None})
        save("change_ttl_a_null", r)
        wait(c, r, "change_ttl_a_null")
        r = c.action(PROBE, "A", "add_records", {"records": [{"value": "192.0.2.13"}]})
        save("add_records_a", r)
        wait(c, r, "add_records_a")
        r = c.action(PROBE, "A", "remove_records", {"records": [{"value": "192.0.2.12"}]})
        save("remove_records_a", r)
        wait(c, r, "remove_records_a")
        save("put_labels_a", c.put_labels(PROBE, "A", {"hashi.io/owner": "instance-0001"}))
        save("get_a_after_changes", c.get_rrset(PROBE, "A"))
        r = c.action(PROBE, "A", "set_records", {"records": [{"value": "192.0.2.12"}]})
        wait(c, r, "")
        r = c.action(PROBE, "A", "set_records", {"records": [{"value": "192.0.2.12"}]})
        save("set_records_a_unchanged", r)
        wait(c, r, "set_records_a_unchanged")

        # Errors: duplicate create, CNAME next to other types, invalid values, unsupported type, missing RRSet.
        save("error_create_exists", c.create(PROBE, "A", ["192.0.2.20"]))
        save("error_cname_conflict", c.create(PROBE, "CNAME", [f"other.{zn}."]))
        save("error_a_invalid", c.create("bad." + PROBE, "A", ["not-an-ip"]))
        save("error_srv_invalid", c.create("_bad._tcp." + PROBE, "SRV", ["10 60 sip.example.com."]))
        save("error_caa_invalid", c.create("badcaa." + PROBE, "CAA", ["issue letsencrypt.org"]))
        save("error_type_unknown", c.create_unchecked_type("bad." + PROBE, "SPF", ['"v=spf1 -all"']))
        save("error_get_missing", c.get_rrset("missing." + PROBE, "A"))
        save("error_delete_missing", c.delete("missing." + PROBE, "A"))

        r = c.delete(PROBE, "AAAA")
        save("delete_aaaa", r)
        wait(c, r, "delete_aaaa")
    finally:
        cleanup(c)


if __name__ == "__main__":
    main()
