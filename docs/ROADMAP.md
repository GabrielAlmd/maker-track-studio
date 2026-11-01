# Roadmap

## First public release: v1.0.0

The initial open source release is intended to include both Desktop and
Self-Hosted. Publish the `v1.0.0` tag and artifacts only after the qualification
evidence is complete:

- Validate Windows and Linux Desktop installers/archives on clean systems.
- Complete native Raspberry Pi ARM64 and Linux AMD64 Compose/browser checks.
- Verify backup restore, persistence, and upgrade/rollback with disposable
  data from supported prior versions.
- Review GitHub Actions permissions and complete secret/dependency scans on the
  exact source tree used for the release.
- Record checksums and installation/recovery evidence for both distributions.

## Good first contributions

Documentation fixes, reproducible bug reports and focused tests are useful
starting points. See [CONTRIBUTING.md](../CONTRIBUTING.md). Please check open
issues before starting a larger change and keep both distributions and existing
SQLite backups in view.
