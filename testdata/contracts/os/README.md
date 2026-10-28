# OS detection contract (captured 2026-10-28)

Output of one read-only probe over SSH, used by Traefik connection validation (04 §5) and later firewall hosts:

    cat /etc/os-release; uname -m; id -u; command -v apk apt-get dnf yum; command -v systemctl rc-service openrc-run;
    command -v traefik; ls -d /etc/traefik /opt/traefik

- `os_alpine.txt`: Alpine 3.23 LXC (OpenRC; `apk` at `/sbin/apk`), Traefik already installed at `/usr/local/bin/traefik`.
- `os_debian.txt`: Debian 12 LXC (systemd; `apt-get` at `/bin/apt-get`), no Traefik.

`/etc/os-release` quoting differs (Alpine `ID=alpine`, Debian `VERSION_ID="12"`); parse it as shell-style key/value.
Docs: https://www.freedesktop.org/software/systemd/man/latest/os-release.html
