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
