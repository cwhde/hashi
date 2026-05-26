# Backup and Restore

Hashi V2 stores authoritative state in PostgreSQL and operational artifacts under `/data`.

## Mandatory backups

1. **PostgreSQL** — full logical backup (`pg_dump`) at least daily.
2. **`/data` volume** — GeoIP cache, uploads, generated artifacts.
3. **Recovery key** — stored outside Hashi; required to unwrap the vault after disaster recovery.

## Restore procedure

1. Restore PostgreSQL from backup.
2. Restore `/data` to the application host.
3. Start Hashi (`docker compose up -d` or systemd service).
4. Unlock the vault with a passkey or recovery key.
5. Run global reconcile from the admin UI or `POST /api/sync/reconcile`.

## Locked vault behavior

- Public status (port 8081) and app dashboard (port 8082) continue serving last known state.
- Monitoring checks that require no secrets may continue.
- Provider syncs continue when service-sync vault mode is enabled.
- Last applied firewall state remains active on managed hosts.
