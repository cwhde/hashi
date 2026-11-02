# ADR 0003 — Pulse agents update themselves to the version their Hashi names

Date: 2026-11-02 · Status: accepted (chosen by the user 2026-11-02)

## Context

Agents did not update at all: each kept the version it was installed with until someone replaced it by hand, and Hashi
only showed the version string. The project releases often, so agents fell behind within days (five agents on three
versions, 2026-11-02).

Pulse's token can only send a heartbeat (ADR 0002, "Host link vs host agent"). Whatever updates an agent decides what code
runs as root on its machine, so auto-update must not turn Hashi, or anyone who controls it, into a way to run code there.

## Decision

1. **Agents follow Hashi.** The heartbeat reply names the agent version this Hashi was released with (its `ReleaseTag`).
   An agent installed directly on a machine replaces its own file when that version is newer than its own; never older.
   Hashi rolled back leaves newer agents as they are.
2. **One switch for all agents** in Hashi's Pulse settings, on by default. No per-agent setting and no staged rollout:
   a broken release is meant to be caught by CI and the dev targets before it is tagged.
3. **Signed releases.** The release job signs each agent binary with an Ed25519 key held as a CI secret; the agent carries
   the public key and installs nothing whose signature, version and architecture do not check out. Hashi only names a
   version and where to fetch it, so a compromised Hashi can at most make agents update to a real, newer release.
   Whoever can run the release job (Gitea's admins) can sign; they can already change what the release contains.
4. **Docker agents do not update themselves**: a container's image is replaced, not its file. The Compose snippet uses
   `:latest`, which the release moves on every version tag, and an image updater (Arcane, Watchtower) pulls it.
5. **Hashi shows the expected version next to each agent's**, older or newer, so an agent that did not follow stands out.

## Consequences

- The agent writes to disk once per update (the new file, next to its own), which the "no disk writes" rule now excepts.
  The systemd unit allows writing `/usr/local/bin`.
- Agents installed before this release (v0.9.2 and older) cannot update themselves; they are upgraded once by hand.
- A Docker agent's version can run ahead of or behind Hashi until its updater runs; Hashi shows it.
