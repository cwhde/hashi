# Firewall integration image (05 §10): nftables 1.1.3 and iptables-legacy + ipset as on Debian 13 firewall hosts.
FROM debian:13
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends nftables iptables ipset iproute2 \
    && rm -rf /var/lib/apt/lists/* \
    && update-alternatives --set iptables /usr/sbin/iptables-legacy \
    && update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy
CMD ["sleep", "infinity"]
