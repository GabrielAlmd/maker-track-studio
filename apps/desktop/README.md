# Maker Track Studio (Wails)

Last updated: 2026-09-25

Desktop target for Linux and Windows. The app starts an embedded local API runtime automatically on port 8080.

## Prerequisites

- Go 1.26.10+
- Node.js 20.19+ (Node.js 24 in CI)
- Linux builds: GTK/WebKit development packages

Ubuntu 24.04+ dependencies:

```bash
sudo apt-get update
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config
```

## Development

```bash
cd apps/desktop
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 dev -tags webkit2_41
```

## Build

Linux:

```bash
cd apps/desktop
node scripts/build.mjs --version 1.2.3 -clean -platform linux/amd64 -tags webkit2_41
```

Windows:

```bash
cd apps/desktop
node scripts/build.mjs --version 1.2.3 -clean -platform windows/amd64 -nsis
```

Windows cross-compilation from Linux (requires MinGW-w64 and NSIS):

```bash
cd apps/desktop
CGO_ENABLED=1 \
CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
node scripts/build.mjs --version 1.2.3 -clean -platform windows/amd64 -nsis
```

The build script invokes `go run` with the explicit Wails `v2.12.0` module
version. Go's module and build caches can therefore reuse the CLI without a
temporary or global installation and without mixing CLI-only dependencies into
the application module graph. The script restores all temporarily rendered
metadata templates even if the build fails.

Use `scripts/build.mjs` for distributable builds. Set `info.productVersion` in
`wails.json` to your default version, or pass `--version` with a SemVer such as
`1.2.3`, `v1.2.3-rc.1`, or `1.2.3+build.5`. The optional leading `v` is removed.
Version precedence is explicit: command/environment request, Git tag, configured
fallback. Core numeric components must be 0–65535 for Windows compatibility;
prerelease and build identifiers remain part of the application version.

The build embeds the full `git rev-parse HEAD` hash. The window title displays
`Maker Track Studio — <semanticVersion>`, and the sidebar displays that same
semantic version using the native desktop bridge. Running the executable with
`--version` prints JSON with the product, semantic version and full commit without
starting the application or touching the database. The executable is named
`Maker Track Studio` on Linux and `Maker Track Studio.exe` on Windows.
Windows fixed file/manifest/NSIS versions use the three numeric core components
(for example, `0.1.0`), while human-readable ProductVersion and installer
DisplayVersion fields retain the full SemVer (for example, `1.0.0`). The
Comments property includes both the application version and commit.
Direct `wails dev` builds display `dev`. Build from a clean, committed
checkout for releases: the hash identifies HEAD, not uncommitted changes.


## Packaging Outputs

CI packaging creates:

- Linux archive: `maker-track-studio-linux-amd64-<version>.tar.gz`
- Windows archive: `maker-track-studio-windows-amd64-<version>.zip`
- Windows installer: `maker-track-studio-windows-amd64-<version>-installer.exe`

Each archive includes:

- Desktop binary
- `LICENSE`
- `README.md` — installation and backup guide for end users

## Smoke Test Scripts

- Linux archive: `scripts/smoke-test-linux-bundle.sh`
- Windows archive: `scripts/smoke-test-windows-bundle.ps1`
- Windows installer: `scripts/smoke-test-windows-installer.ps1`

## Runtime Notes

- Desktop runtime always uses SQLite; no mode or driver selection is needed.
- Default SQLite path:
  - Linux: `$HOME/.local/share/3d-printing-cost-tracker/app.db`
  - Windows: `%AppData%/3d-printing-cost-tracker/app.db`
- You can override DB location with `SQLITE_PATH` during local troubleshooting.

## CI and releases

`desktop-packaging.yml` is the CI entry point for pull requests into `develop` or
`main`, version tags (`v*`), and manual runs. It deliberately has no branch-push
trigger, so merging an already validated PR does not repeat the same pipeline.

Feature PRs run backend vet/tests plus one combined frontend job: deterministic
`npm ci`, dependency audit, lint, tests, production build, metadata tests, visual
regression, and curated screenshot consistency checks.

PRs targeting `main`, tags, and manual runs add the complete native validation.
The Linux job reuses its GTK/WebKit setup for both Go vulnerability scans, then
builds, packages, and smoke-tests Linux. The Windows job builds the archive and
pinned-NSIS installer and smoke-tests both. Artifacts are available for 14 days.
Go setup caches both module dependency sets where desktop builds require them.

CI version precedence is manual input, Git tag, then `info.productVersion` in
`wails.json`. The optional leading `v` is removed from application and artifact
versions, so tag `v1.0.0` creates names containing `1.0.0`. Embedded
diagnostic metadata retains the full checkout commit. Smoke tests verify the
semantic application version, numeric Windows version, and commit before launch.

The Playwright browser cache is keyed by OS and the frontend lockfile, so a
Playwright dependency change naturally selects a new cache. Linux system packages
are still installed each run; system directories and `node_modules` are not cached.
The npm cache continues to use `package-lock.json`, and installation uses `npm ci`.

`security.yml` runs the full Go and npm dependency scan weekly and on manual
dispatch, preserving periodic coverage without adding another runner to every PR.

Only `v*` tags publish to this repository's existing GitHub Releases; tags with a
prerelease suffix are marked as prereleases. Manual runs build artifacts without
publishing. Optional Windows signing runs only for tags or manual runs when the
existing signing secrets are available. Archives are created after signing, and
only the release job receives `contents: write`. The existing SQLite directory
remains unchanged so upgrades retain local data.
