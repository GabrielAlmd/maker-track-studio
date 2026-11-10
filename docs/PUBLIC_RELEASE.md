# Release qualification

Maker Track Studio is open source under the MIT License. The published
first release is `v1.0.0`. The next patch, `v1.0.1`, covers the Windows/Linux Desktop application and
the optional LAN-only Self-Hosted distribution.

## Release status

The v1.0.1 patch is not published yet. Its version tag, installers,
archives and container images must not be treated as available until they are
published by the project's official GitHub Releases and GHCR locations. A
successful source build or CI run alone does not qualify a release.

Track evidence in the [v1.0.1 readiness record](RELEASE_READINESS_1.0.1.md).
See the [draft release notes](RELEASE_NOTES_1.0.1.md) for the patch scope.

Before publishing `v1.0.1`, maintainers must record evidence for these gates:

- Windows and Linux Desktop packaging and smoke tests on clean systems.
- Native Linux AMD64 and ARM64 Self-Hosted image, browser and runtime checks.
- SQLite persistence, backup/restore, upgrade and rollback checks using
  disposable data from the published v1.0.0.
- LAN access controls, TLS trust, anonymous denial and denial of direct API or
  WAN access on supported target devices.
- Dependency and image security scans, license review, release checksums and
  Software Bill of Materials (SBOM) evidence.
- Physical-device acceptance for Desktop and Self-Hosted installation,
  recovery, and access from a second trusted LAN device.

Any confirmed critical or high-risk security finding blocks publication until
it is fixed and the affected checks pass again. See [dependency security](DEPENDENCY_SECURITY.md),
[LAN security](LAN_SECURITY.md), and [Self-Hosted validation](SELF_HOSTED.md)
for the relevant procedures and current limitations.

## Source distribution

The source archive contains the application, tests, build and deployment
configuration, documentation, license notices, and synthetic screenshot
fixtures. Do not add local databases, backups, credentials, private keys,
customer records, personal machine paths, or generated release binaries.
Preserve the MIT license and the SIL OFL notices for the bundled Inter, Sora,
and JetBrains Mono fonts.

Release checks must be run against the exact source tree used to build the
artifacts. Record scan results and build identities with the release evidence;
scans from an earlier checkout are not evidence for a later release.

## Support boundaries

Desktop keeps its API bound to loopback and stores data locally. Self-Hosted is
for trusted LANs behind its authenticated HTTPS reverse proxy. Public Internet
hosting, router port forwarding, automatic cloud synchronization and multiple
servers sharing one SQLite database are unsupported. See [LAN security](LAN_SECURITY.md)
and [SQLite recovery](SQLITE_RECOVERY.md) before deploying or upgrading.
