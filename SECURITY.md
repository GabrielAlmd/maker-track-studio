# Security policy

## Supported versions

Security fixes target the latest supported release. `v1.0.0` is the first public
release target; consult [release qualification](docs/PUBLIC_RELEASE.md) for its
publication status and security gates.

## Reporting a vulnerability

Please do not report exploitable details in a public issue. Use GitHub's
**Report a vulnerability** action on the repository's Security tab to contact
the maintainers privately. Include the affected version, operating system,
distribution (Desktop or Self-Hosted), impact and reproducible steps. Do not
attach real databases, backups, credentials or private keys.
Local databases in `apps/api/.local-dev/` are ignored by Git and must remain
development-only.

If private vulnerability reporting is unavailable, contact the repository
owner through the email address listed on their GitHub profile and request a
private channel. Allow time for triage and a fix before public disclosure.

## Security boundaries

Desktop API access is loopback-only. Self-Hosted is intended for a trusted home
LAN behind its authenticated HTTPS proxy; direct API exposure, Internet
port-forwarding and automatic cloud synchronization are unsupported. Read the
[LAN security guide](docs/LAN_SECURITY.md) before deploying.
