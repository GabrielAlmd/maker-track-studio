# Self-hosted Docker distribution

The image workflow builds AMD64 and ARM64 using Buildx
(native AMD64 and ARM64 build/test runners). The publisher copies the scanned
OCI archives without rebuilding. Publication occurs through the Desktop release workflow on version tags.
Local AMD64 smoke tests have passed; hosted ARM64 execution and publication
have not been run as part of this implementation;
do not treat this document as a release qualification report.

## Installation

Use a compatible Linux Docker engine with Compose v2, including Linux containers
on supported Docker Desktop hosts. ARMv7 is unsupported. Start with 2 GiB RAM on
a 64-bit Pi and reliable SSD storage; allow more memory for imports and builds.
Mini-PC/NAS installations should provision at least the same resources. Avoid
network filesystems for SQLite. No Go/Node installation is needed for image pulls.

Copy `deployments/self-hosted/compose.yml`, `setup-secrets.sh`,
`reset-password.sh` and `.env.example` into a private
installation directory; copy the latter to `.env`. Set an actually published
explicit version, the host LAN IPv4 address and an HTTPS origin on port 443 (or a chosen available HTTPS port).
Optional digest pin: `MAKER_TRACK_VERSION=vX.Y.Z@sha256:<manifest-digest>`.
The placeholder version deliberately cannot install an unqualified release.

Generate `proxy.key` and `auth.htpasswd` with `sh setup-secrets.sh`.
First export `MAKER_TRACK_IMAGE` with the same published image version/digest
selected in Compose, for example
`export MAKER_TRACK_IMAGE=ghcr.io/gabrielalmd/maker-track-studio:vX.Y.Z`.
The image helper hashes the prompted password with bcrypt. Docker must be running.
Setup refuses existing credentials. Use the same variable for password reset.
Never put credentials in `.env`, image layers, command arguments or Git. For
manual setup and credential rotation, see [LAN security](LAN_SECURITY.md).

Configure local DNS (router DNS or client hosts entries) for the chosen name;
using the LAN IP directly also works. mDNS names require host-side mDNS support
and are not provided by the container. The TLS helper issues a local certificate without
public Internet access. After startup, copy only its root certificate:

```sh
docker compose pull
docker compose up -d --no-build
docker compose cp studio:/data/tls/root.crt ./maker-track-root.crt
docker compose ps
```

Verify the certificate fingerprint on the server, then install it in each
client's trusted root store. Never distribute the CA private key. Open the exact
configured origin and authenticate using the generated shared login.

Publish only the configured HTTPS port (443 by default). The API uses loopback inside the container, without a host
port mapping. Bind to a dedicated LAN IP and restrict Docker-forwarded traffic
using the host's Docker-aware firewall (ordinary UFW input rules may not cover
published ports). Allow only trusted subnets, deny guest networks and WAN on IPv4
and IPv6, and disable router port forwarding/UPnP. Authentication alone does not
make Internet exposure supported. Docker Desktop must be configured to start its
engine at login/reboot; Linux hosts must enable the Docker service.

## Storage and lifecycle

Desktop v0.4.0 and the server use the shared SQLite models and ZIP backup/import
implementation. Transfer data with an exported backup and the receiving app's
import workflow; retain the original installation and backup until verification.
There is no automatic Desktop/server synchronization. Never open a live Desktop
database from a server container. Version compatibility still requires the
release's disposable backup/import checks before declaring it qualified.

Named volumes hold SQLite, diagnostics, scheduled ZIP backups, and private TLS CA and certificates.
The image initializes their ownership for UID 10001. Host mounts must be created
with that ownership and private permissions before startup. One installation owns
one database. Never run two containers against the same workspace volume.

The supervisor forwards shutdown to both services; Compose allows 30 seconds
for the API's 10-second drain. Either service exiting restarts the container.
Health probes exercise authenticated API readiness and verify the TLS proxy denies
anonymous access. The local proxy probe skips CA verification solely inside the
container; clients must verify TLS. Docker logs rotate at 3 x 10 MiB; application
diagnostics use their existing rotation. Daily backups default to 03:00 UTC and
14 retained archives. Copy backups off-host regularly; a volume is not protection
against disk failure. See [SQLite recovery](SQLITE_RECOVERY.md).

Before upgrading, export a ZIP backup through the UI, retain it off-host and
record the current version/digest. Stop the app and make a cold copy of all named
volumes, including CA storage, using your volume backup tooling. Change the
version in `.env`, then run `docker compose pull` and `docker compose up -d
--no-build`. Do not delete volumes. Verify readiness, data and a backup export.
For rollback stop the app, restore the pre-upgrade volume copies and previous
version, then start again. Never assume an older binary can open a newer schema.

`docker compose down` uninstalls containers while preserving data. Do **not** use
`down -v` unless intentionally destroying the installation. To relocate, stop the
source, transfer cold volume copies and protected credentials to the new host,
restore ownership and the same version, update LAN IP/DNS and start. SQLite and
ZIP backups are portable between AMD64 and ARM64. Retaining CA storage preserves
client trust; changing hostname still requires a matching configured origin.

For source builds, clone the release repository, enter
`deployments/self-hosted`, configure credentials/environment as above and run
`docker compose build --pull` followed by `docker compose up -d --no-build`.
Build requirements are contained in multi-stage images. BuildKit runs Node on
the builder platform and Go/CGO on the target platform (native or emulated).

## Required release evidence

On both AMD64 and ARM64 hosts, record host/engine versions, image digest and
whether execution is native or QEMU. Use the identical Compose file and temporary
data only. Inspect the published manifest with `docker buildx imagetools inspect`
and confirm both platforms. Then verify:

- Healthy startup, anonymous UI/API denial, trusted TLS and authorized browser CRUD.
- Reports/exports, backup restore and optional Sales & Orders from a second LAN client.
- Host port 8080 inaccessible; foreign/missing Origin writes denied; guest LAN denied.
- Record survival after `down`/`up`, engine reboot and tagged upgrade.
- Clean shutdown, backup relocation, cross-architecture restore and rollback from
  pre-upgrade volume copies.

The workflow builds each platform and runs a Compose smoke test on native
AMD64 and ARM64 runners, then gates publication on these tests and shared quality checks. The smoke
test covers TLS trust, anonymous denial, authenticated record creation, backup export/restore,
shutdown and restart. When a previous image is supplied, it also checks upgrade
and rollback from cold volume copies. Broader browser and independent LAN checks
remain release gates. Deployment to physical devices is performed manually.
Desktop Windows/Linux packaging remains in its existing workflow.

## v1.0.0 release qualification status

Pull requests and manual builds validate both target architectures without package
write permissions or registry login. Only version-tag pushes in the upstream
repository enter the publishing job. Published images request BuildKit provenance
and SBOM attestations; CI checks the combined manifest for AMD64 and ARM64 and
retains the manifest and immutable image reference as release evidence. Buildx
uses GitHub Actions caches for both paths.

The v1.0.0 release is not qualified until the required CI runtime results for
both architectures, compatibility evidence, browser coverage and target-device
checks have been recorded for the exact release source. Confirmed unaddressed
critical/high-risk security findings block publication. A successful image
build alone does not satisfy these gates.

When reporting an installation problem, include the application version and
image digest, Desktop or Self-Hosted target, OS and CPU architecture, Docker and
Compose versions where applicable, native versus emulated execution, reproduction
steps, expected/actual results, and sanitized logs. Never attach credentials,
private CA keys, real databases or backups. State whether persistence, restart,
backup export/import and access from a second LAN device were verified.

## Running execution checks

The `Desktop CI and Release` workflow calls `Self-hosted native validation` for
relevant pull requests into `develop`/`main` and version tags. Validation can also
be started manually with **Run workflow**, which runs quality/security checks
once alongside the native matrix. Its matrix uses native
`ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64) runners; see
[GitHub runner platforms](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
These jobs create only disposable Compose projects, databases and credentials.
They do not deploy to your devices. Each job uploads a `self-hosted-runtime-*`
artifact with phase logs, image identity, architecture and explicit test status,
including on failure.

There is currently no published earlier self-hosted image. Without a baseline,
`result.txt` reports `upgrade_rollback=NOT_EXECUTED`; first-release startup,
backup/restore and restart still run. This is not evidence of compatibility
between releases. Once an earlier image exists, supply `baseline-image` when
starting the workflow manually, or set repository variable
`SELF_HOSTED_BASELINE_IMAGE` to its versioned GHCR tag/digest for automatic runs.
The baseline must be a distinct image and support the same Compose contract.
A missing/unpullable configured baseline fails the job rather than skipping it.

The baseline test creates a fixture, exports/restores its ZIP backup, stops both
services and archives all four named volumes. It starts the candidate image on
the same volumes and verifies the fixture, then adds a post-upgrade record.
Rollback stops the candidate, restores the cold volumes, starts the baseline
and verifies the original records without the post-upgrade change. CA storage
is restored too, and the same trusted certificate is used throughout.

Tagged Desktop releases complete shared preflight first, then run browser,
container and native Desktop checks in parallel. All must pass before image
publication and release attachment. See [CI execution policy](CI.md) for artifact
reuse, cancellation and runner allocation.
The release includes `self-hosted-image.txt` with the pinned image reference,
`self-hosted-execution-evidence.tar.gz` with manifest and per-platform results,
and `SHA256SUMS` covering the archives and installers. No tag or image is
published by a pull request or a manual validation run.

## Dependency security gate

Both publication paths require the [dependency security checks](DEPENDENCY_SECURITY.md).
These cover native Go targets, the npm lockfile and the final AMD64/ARM64 images.
Image reports and immutable OCI archive checksums accompany release evidence;
the publisher transfers the scanned archives without rebuilding. HIGH/CRITICAL
image findings and scanner errors block publication, including unfixed findings.
Consult the final-image report for each build before treating it as release ready.

## Proxy distribution

The runtime uses Nginx from Alpine and the TLS helper built from the API module.
This distribution targets new installations. Existing Caddy credentials and
volumes are not migrated. Final-image HIGH/CRITICAL gates remain enabled.

## Local certificate lifecycle

A private ECDSA P-256 CA lasts ten years; server certificates last 90 days.
The helper checks at startup and every 12 hours, renewing with 30 days remaining
or when the hostname/IP changes, then reloads Nginx. The trusted CA persists in
the tls-data volume. Back up this entire volume securely alongside database
volumes. Only root.crt should be distributed to clients.

Invalid, incomplete or near-expiry CA state fails startup. For manual CA
replacement, stop the service, retain an encrypted copy of the TLS volume,
replace that volume with an empty one and start the service. Verify and install
the new root certificate on every client. Never delete the workspace volume.
Restore an earlier TLS volume only if its CA remains valid.
