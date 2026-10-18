# SEC-003: Legacy artifact dependencies retain unresolved advisories

Priority: P2

Category: Security / separately deployed legacy artifact

Status: open; fresh production dependency audit; not attributed to the active V2 process.

## Finding and evidence

`npm audit --omit=dev --json` in `hashi.old` reports14 affected package entries:2 critical,9 high and3 moderate across177 dependencies. Reports include proxy-addr/tar critical entries, Fastify/static/router/ws and transitive native-build/path/parser dependencies. Severity/counts describe the dependency graph, not14 independently demonstrated runtime exploits. The legacy Docker publication workflow remains present, so the artifact still needs an explicit supported/deprecated/security-maintenance decision.

The complete raw report is retained as `legacy-audit-restarted.json` in the task workspace; it includes advisory URLs, vulnerable paths and patched-version metadata. No upgrade or blanket suppression was applied.

## Expected outcome and acceptance criteria

Either maintain the published legacy artifact with compatible dependency upgrades and advisory reachability checks, or clearly retire its publication/deployment path through an explicit product decision. Keep its independent credentials/state and LEG-series defects separate from V2. Verify the resulting legacy build and actual enabled server/native-install paths when repairing, without reintroducing public bootstrap or destructive DNS failures.
