# Backup and Restore

Hashi V2 stores authoritative state in PostgreSQL and operational artifacts under `/data`.

## Mandatory backups

1. **PostgreSQL** — full logical backup (`pg_dump`) at least daily. Schedule via cron or systemd timer:
   ```bash
   pg_dump -U hashi -d hashi -Fc -f /data/backups/hashi_$(date +%Y%m%d_%H%M%S).dump
   ```
2. **`/data` volume** — GeoIP database cache (`/data/geoip`), uploaded secrets, and locally generated artifacts (Traefik configs, firewall scripts). Archive with:
   ```bash
   tar czf /data/backups/hashi-data_$(date +%Y%m%d).tar.gz /data/geoip /data/secrets /data/traefik
   ```
3. **Recovery key** — the vault recovery key generated during initial setup **must** be stored outside Hashi (e.g., a password manager or offline secure storage). Without it, encrypted secrets cannot be restored after a disaster.

## Restore procedure

1. Restore PostgreSQL from backup:
   ```bash
   pg_restore -U hashi -d hashi -Fc /data/backups/hashi_<timestamp>.dump
   ```
2. Restore `/data` to the application host (extract the archive to the original paths).
3. Start Hashi (`docker compose up -d` or the systemd service).
4. Unlock the vault with a passkey or the stored recovery key.
5. Run global reconcile from the admin UI or `POST /api/sync/reconcile`.

## Locked vault behavior

- Public app dashboard and status page continue serving last known state on their configured public ports.
- Monitoring checks that require no secrets may continue.
- Provider syncs continue when service-sync vault mode is enabled.
- Last applied firewall state remains active on managed hosts.

## Disaster recovery

1. Provision a new host or clean the existing host.
2. Install and configure PostgreSQL, then restore the Hashi database backup.
3. Restore the `/data` volume archive to the original location.
4. Start Hashi and unlock the vault using the recovery key.
5. Trigger global reconcile to resync all subsystems.
6. Verify: check the admin dashboard, monitoring endpoints, and firewall state.
