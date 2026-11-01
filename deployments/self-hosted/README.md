# Manual LAN installation

Follow the [installation and recovery guide](../../docs/SELF_HOSTED.md).
Copy `compose.yml`, `.env.example`, `setup-secrets.sh` and `reset-password.sh`
into a private directory on your server. Rename `.env.example` to `.env`, select
an actually published version, configure the LAN address and exact HTTPS origin,
and run `sh setup-secrets.sh` before starting Compose.

Linux AMD64 servers/NAS and Raspberry Pi OS 64-bit use this same file without
architecture flags or source builds. Docker selects the matching image from the
published manifest. Raspberry Pi requires ARM64; 32-bit OS installations are
unsupported. Prefer SSD storage and at least 2 GiB RAM. Configure router DNS or
host mDNS for the chosen name; the image does not provide mDNS.

Deployment to your devices is always manual. CI uses disposable containers and
volumes only. Follow the guide for certificate trust, firewall rules, backups,
pinned upgrades, rollback and the release verification checklist.
