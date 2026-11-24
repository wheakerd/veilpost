#!/bin/sh
# AlmaLinux 10 provides nf_tables and does not provide the legacy iptables NAT
# module expected by the image default. Keep all changes inside the container.
set -eu
for family in iptables ip6tables; do
    update-alternatives --install "/usr/sbin/$family" "$family" "/usr/sbin/$family-nft" 20 \
        --slave "/usr/sbin/$family-restore" "$family-restore" "/usr/sbin/$family-nft-restore" \
        --slave "/usr/sbin/$family-save" "$family-save" "/usr/sbin/$family-nft-save" >/dev/null
    update-alternatives --set "$family" "/usr/sbin/$family-nft" >/dev/null
done
exec "$@"
