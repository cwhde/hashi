# Firewall integration image (05 §10): nftables 1.1.3 and iptables-legacy + ipset as on Debian 13 firewall hosts.
# systemd-run and systemctl are a shim (systemd-shim.sh): the rescue timer runs as a background sleep.
FROM debian:13
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends nftables iptables ipset iproute2 procps cron \
    && rm -rf /var/lib/apt/lists/* \
    && update-alternatives --set iptables /usr/sbin/iptables-legacy \
    && update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy
COPY systemd-shim.sh /usr/local/bin/systemd-shim
RUN chmod 755 /usr/local/bin/systemd-shim && ln -s systemd-shim /usr/local/bin/systemd-run \
    && ln -s systemd-shim /usr/local/bin/systemctl
CMD ["sleep", "infinity"]
