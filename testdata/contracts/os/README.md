# OS detection contract (captured 2026-10-28)

Output of one read-only probe over SSH, used by Traefik connection validation (04 §5) and later firewall hosts:

    cat /etc/os-release; echo "--- uname"; uname -m; echo "--- id"; id -u;
    echo "--- pm"; for c in apk apt-get dnf yum; do command -v $c; done;
    echo "--- init"; for c in systemctl rc-service openrc-run; do command -v $c; done;
    echo "--- traefik"; command -v traefik; ls -d /etc/traefik /opt/traefik 2>/dev/null; echo "--- end"

(`traefik.ProbeCommand` is this command on one line.)

- `os_alpine.txt`: Alpine 3.23 LXC (OpenRC; `apk` at `/sbin/apk`), Traefik already installed at `/usr/local/bin/traefik`.
- `os_debian.txt`: Debian 12 LXC (systemd; `apt-get` at `/bin/apt-get`), no Traefik.
- `os_debian13.txt`: Debian 13 LXC (systemd; `apt-get` at `/usr/bin/apt-get`; sshd allows forwarding; `wget`, no `curl`).
- `os_fedora44.txt`: Fedora 44 LXC (systemd; `dnf` and `yum` both present, `yum` is dnf's alias, so prefer `dnf`;
  sshd allows forwarding; `curl`, no `wget`).

`/etc/os-release` quoting differs (Alpine `ID=alpine`, Debian `VERSION_ID="12"`); parse it as shell-style key/value.
`openrc_traefik_alpine.txt`: Alpine's own `/etc/init.d/traefik` (aports package) from a native Alpine Traefik host,
the model for Hashi's `hashi-traefik` OpenRC script.

Docs: https://www.freedesktop.org/software/systemd/man/latest/os-release.html

## logrotate (captured 2026-10-28 on the dev targets)

| Host | Present | Schedule | Install command (exit 0, also when already installed) |
| --- | --- | --- | --- |
| Alpine 3.23 | `/usr/sbin/logrotate` | `/etc/periodic/daily/logrotate`, `crond` running (OpenRC default runlevel) | `apk add --no-cache logrotate` |
| Debian 13 | `/sbin/logrotate` | `logrotate.timer` (`OnCalendar=daily`, `Persistent=true`) and `/etc/cron.daily/logrotate` | `DEBIAN_FRONTEND=noninteractive apt-get install -y logrotate` |
| Fedora 44 (container image) | missing | package ships `logrotate.timer`; the install enables it but does not start it (`is-enabled` enabled, `is-active` inactive) | `dnf install -y logrotate`, then `systemctl enable --now logrotate.timer` (then active) |

All three `/etc/logrotate.conf` include `/etc/logrotate.d`. `pkill` exists on all three (`/usr/bin`, `/bin`, `/sbin`).
