# Running Hashi

For the person who installs, backs up, upgrades and recovers Hashi. Commands assume the compose project from
`deploy/compose/compose.yaml` in a folder on the Docker host (called `<dir>` below) and the container name `hashi`.
`make deploy` does the install and upgrade steps for you when you deploy from a checkout (`deploy/targets.example/`).
Background: `docs/threat-model.md` (what protects what), spec 02 §5 (vault), 11 §8 (operations).

## 1. What you keep outside Hashi

Put both in a password manager before anything else. Without them a lost disk means a fresh setup.

| Item | Where it comes from | Used for |
| --- | --- | --- |
| Master key | `hashi keygen`, once | Opens every stored secret; in service mode it sits on the host as `master.key` |
| Recovery key | Shown once in setup step 9 | Recovers the master key; unlocks the vault in strict mode |

A database copy without one of them can't open any token, password or SSH key.

## 2. Install

1. On the Docker host, in `<dir>`: copy `compose.yaml`, create the data folder and the key:

   ```bash
   mkdir -p data && chown 65532:65532 data && chmod 700 data
   docker run --rm git.juzo.io/juzo/hashi:<version> keygen > master.key
   chown 65532:65532 master.key && chmod 400 master.key
   : > cap.env   # empty unless you run Cap next to Hashi
   printf 'HASHI_TAG=<version>\n' > .env
   docker compose up -d
   ```

2. Save `master.key`'s content in the password manager.
3. Read the bootstrap login from the log: `docker logs hashi` (a block with `username:` and `password:`). It works
   only from a private network and only until setup is finished.
4. Open `http://<docker host>:8080` and follow setup. Step 9 creates your passkey and shows the recovery key once:
   save it before you confirm.

Ports: 8080 admin and API, 8081 public app dashboard, 8082 public status page (`HASHI_ADMIN_PORT` and friends in
`.env`). The container runs as uid 65532 with a read-only root; everything it writes is under `data/`.

## 3. Backups

Hashi writes a consistent copy of its database every night to `data/backups/hashi-<date>.db` and keeps the newest 14.
Activity → Jobs shows the last run (`backup.nightly`) and any failure. Copies made before an upgrade
(`pre-migrate-v<n>-…`) and before a key rotation (`pre-rotate-v<n>-…`) stay until you delete them.

**Your part:** copy `data/backups/` off the host. The host's own backup (for example a Proxmox backup of the LXC) is
enough. Keep the master key and recovery key apart from these copies.

## 4. Restore onto a new host

You need: one copy from `data/backups/`, and either the master key or the recovery key.

1. In `<dir>` with `compose.yaml`: the folders and files from §2 step 1, but **no** `keygen` (a new key would not fit
   the copy), and the copy as the database. Don't start yet.

   ```bash
   mkdir -p data && chown 65532:65532 data && chmod 700 data
   : > master.key && chown 65532:65532 master.key && chmod 600 master.key
   : > cap.env && printf 'HASHI_TAG=<version>\n' > .env
   cp hashi-<date>.db data/hashi.db && chown 65532:65532 data/hashi.db && chmod 600 data/hashi.db
   ```

   A copy from an older Hashi version is fine: the server upgrades it on its first start.

2. The master key:
   - **You have it:** write it into `master.key`.
   - **You only have the recovery key:** `docker compose run --rm --no-deps hashi vault recover`, type the recovery key;
     it prints the master key. Write that into `master.key` (and the password manager).
3. `docker compose up -d`, then `docker logs hashi`: no error about the master key means it matches. Then
   `docker exec hashi hashi vault check` opens every stored secret and prints how many opened (never a value).
4. Sign in with your passkey. Passkeys belong to the root domain, so they keep working on a new host; if the admin
   name now points somewhere else, update its DNS record first.
5. Open Changes. Hashi compares every system with what the copy says; review each plan, high-risk ones above all,
   before you approve it. Anything changed after the copy was taken shows up here.

Pulse agents and Traefik hosts reach Hashi by name; once that name points at the new host they carry on.

## 5. Upgrade

`make deploy ENV=<target> TAG=<version>` or, by hand, set `HASHI_TAG` in `.env` and run
`docker compose pull && docker compose up -d`. Image tags have no `v` (`1.0.0`); release names do (`v1.0.0`).

On start Hashi migrates the database when the new version needs it, after writing a `pre-migrate` copy. A failed
migration puts that copy back and the container exits, so the old version starts again with `make rollback` or the
previous `HASHI_TAG`.

Going back to an older version **after** a successful migration: stop Hashi, put the `pre-migrate` copy back as
`data/hashi.db` (§4 step 1), start the older version. Changes made since the upgrade are lost; review Changes after.

## 6. Recovering access

| Situation | What to do |
| --- | --- |
| Lost every passkey | `docker exec hashi hashi admin recover` prints a one-time link (15 minutes, private networks only) that registers one new passkey |
| Setup not finished and the bootstrap password is lost | `docker exec hashi hashi bootstrap reset` prints new bootstrap credentials |
| Lost the master key, have the recovery key | `docker compose run --rm --no-deps hashi vault recover` (the server need not run) |
| Locked out by a firewall change | Wait: every high-risk firewall apply rolls back by itself unless you confirm it (rescue timer) |

## 7. Changing the master key

Do this if the master key may have leaked. Hashi has to be stopped; Pulse agents, sign-ins and the edge keep their
keys, nothing else needs touching.

```bash
docker compose stop hashi
docker compose run --rm --no-deps hashi vault rotate   # asks for the recovery key, prints the new master key once
```

Write the new key into `master.key` and the password manager, then `docker compose up -d`. The old key no longer opens
the database; the `pre-rotate` copy it wrote still opens with the old one. Passkeys you added as vault unlockers are
removed (they hold the old key); add them again before you switch to strict mode. Not available in strict mode: switch to
service mode first.

## 8. Strict mode

In service mode (the default) Hashi opens its secrets with `master.key` at every start, so everything runs after a
restart without you. In strict mode the master key is not on the host: after every restart or update Hashi starts
locked until you unlock it, and a stolen disk or backup opens no secret.

**While locked** (banner on every page, "Vault locked" in the top bar): DNS, Traefik, firewall, AdGuard, certificate
and script changes wait, and so do notifications and pushing bans to firewall hosts. Monitoring, public pages, sign-in
to Hashi, Pulse agents, CAPTCHA passes and visitors already signed in to protected apps keep working. Everything that
waited runs right after the unlock.

**Switch on:**

1. Settings → Account → Vault → *Add a passkey*. Your browser tells you if that passkey can't (it needs the PRF
   feature; passkeys in recent browsers and password managers usually have it, some security keys need a new passkey).
   Without one, unlock with the recovery key.
2. *Switch to strict mode…* and confirm.
3. Empty the key file so the key is no longer on the host, keeping the file so compose starts:
   `: > master.key`. With `make deploy`, set `DEPLOY_VAULT_MODE=strict` in the target.

**After each restart:** open Hashi, sign in, *Unlock*, then the passkey or the recovery key.

**Switch back:** write the master key into `master.key` again, unlock if Hashi restarted, then Settings → Account →
Vault → *Switch to service mode…*. Hashi checks the file holds the right key first, so the next restart can't fail.

## 9. Where to look when something is wrong

- `docker logs hashi`: JSON lines, secrets redacted. Activity → Live log shows the same.
- Activity → Jobs: every background job with its last run, error and next run.
- Activity → Audit: every admin action, unlock attempt and console command.
- `/readyz` on the admin port answers 200 when the database is fine (also while the vault is locked).
