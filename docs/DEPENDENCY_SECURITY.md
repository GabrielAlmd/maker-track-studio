# Dependency checks and release gates

Every tagged release must pass the shared dependency-security workflow before
Desktop packaging or final GHCR image publication. The Desktop workflow runs
shared preflight checks once and calls native self-hosted validation for relevant
PRs and version tags only after preflight succeeds. Browser checks and native
Desktop/container jobs then run in parallel. Both publication paths require
successful browser and native checks as well as preflight.
Manual self-hosted validation runs preflight itself once before the heavy jobs. There is no
input to skip security checks. Fork PRs run with read-only permissions; registry
credentials are confined to the trusted upstream tagged publication job.

| Target | Check | Failure policy |
| --- | --- | --- |
| API Linux AMD64/ARM64 and Windows AMD64 | Native `govulncheck` source scan | Any scanner finding that produces a nonzero status, or scanner failure, blocks release |
| Desktop Linux/Windows AMD64 | Native `govulncheck`, with Linux Wails build tags | Same policy |
| All locked npm dependencies, including build dependencies | `npm audit --package-lock-only --audit-level=high` | HIGH/CRITICAL or audit failure blocks release |
| Final Linux AMD64/ARM64 images | Trivy OS and application-library scan | HIGH/CRITICAL, including unfixed findings, or scanner/DB failure blocks publication |

These checks identify known vulnerabilities using their current databases. They
are not a guarantee of absence of vulnerabilities or a substitute for reviewing
application security. Go source analysis accounts for reachable vulnerable
symbols; image package analysis is deliberately stricter and may report packages
whose vulnerable functionality is not used by this application.

## Image identity and evidence

Each native job exports one OCI archive with provenance and SBOM attestations.
Trivy v0.76.1 is pinned by its multi-platform image digest in
`scripts/ci/scan-image.sh`; the scanner receives a read-only OCI layout and no
Docker socket or registry credentials. It inspects Alpine packages and compiled
application binaries, including the API and TLS helper. Its vulnerability database is
refreshed normally; the pipeline does not skip DB updates or ignore unfixed CVEs.

The archive is loaded through Skopeo for Compose tests. After image scans and
runtime tests pass, that archive is uploaded as a short-lived CI artifact.
Publication additionally requires the shared dependency scans to pass. The publisher copies the same OCI archive with `--all
--preserve-digests` and creates the final multi-platform manifest. It does not
rebuild the image. This preserves the scanned contents and attestations.

Go scans use `scripts/ci/scan-go.sh`: the scanner first saves JSON, then the
same report is converted with the text handler to enforce its vulnerability exit
code. JSON mode alone does not fail on findings. A regression test exercises
clean results, a reachable vulnerable symbol and a scanner error using the real
pinned converter.

JSON reports are uploaded even when a scan fails. Image scans also print each
blocking package, CVE and available fixed version directly in the job log. Image evidence includes the
scanner reference and archive SHA256. Each run produces one shared set of Go/npm
reports, so the same scans and artifacts are not duplicated. Successful release evidence
includes the security reports alongside the runtime results and manifest.
The weekly/manual `Dependency Security` workflow also rechecks Go and npm against
newly published advisories; it does not redeploy installed applications. PR
matrices stop siblings after a failure; non-PR matrices collect all targets.
See [CI execution policy](CI.md) for ordering and frontend artifact reuse.

## Current validation and blockers

Local source validation on 2026-10-31 found no reachable Go vulnerabilities.
The scanner reports one advisory in a required module that the analyzed code
does not appear to call; review the verbose report during release qualification.
The npm lockfile was refreshed to `follow-redirects` 1.16.2 to resolve its HIGH
advisory. `npm audit --audit-level=high` now passes, while reporting six
MODERATE paths through ESLint's `minimatch` dependency; the current npm audit
database reports no available fix. These are development-tool dependencies and
remain visible follow-up work. The HIGH/CRITICAL release gate remains enabled.

Native Windows and ARM64 execution still require GitHub Actions. Local checks do
not replace native packaging, image scanning or clean source-tree validation.

An earlier image scan found multiple HIGH/CRITICAL dependencies. The Caddy
rebuild reduced these to one: CVE-2026-116587 in go-jose/v3
v3.0.5, which motivated replacing the proxy. Its
[upstream advisory](https://github.com/go-jose/go-jose/security/advisories/GHSA-mgcc-cww5-fp2q)
lists no patched v3 version; no ignore rule or VEX exception has been added.

Nginx is installed from Alpine packages; Caddy and its go-jose/v3 dependency
are removed. Native final-image scans cover Nginx, OpenSSL, OS packages and
shipped Go binaries. The TLS helper is included in the API module source scan.
No security waiver is used; runtime packages are refreshed on every build.

The replacement AMD64 OCI image was scanned locally with the pinned Trivy gate
on 2026-10-30: zero HIGH/CRITICAL findings. Its report lists only the API and
selfhostctl Go binaries, with no Caddy or go-jose/v3. HTTPS/browser integration,
Compose startup, backup restore and persistence checks passed. Native ARM64 image
validation remains required in CI; there is no published Nginx baseline yet.
