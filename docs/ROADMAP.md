# Roadmap

## Next patch: v1.0.1

v1.0.0 has been published. v1.0.1 addresses early-user feedback with bug fixes,
regression tests and clearer guidance. Track the remaining checks in the
[v1.0.1 readiness record](RELEASE_READINESS_1.0.1.md) and changes in the
[draft patch notes](RELEASE_NOTES_1.0.1.md). Before publication:

- Validate Windows and Linux Desktop installers/archives on clean systems.
- Complete native Raspberry Pi ARM64 and Linux AMD64 Compose/browser checks.
- Verify backup restore, persistence, and upgrade/rollback with disposable
  data from published v1.0.0.
- Review GitHub Actions permissions and complete secret/dependency scans on the
  exact source tree used for the release.
- Record checksums and installation/recovery evidence for both distributions.

## Good first contributions

Documentation fixes, reproducible bug reports and focused tests are useful
starting points. See [CONTRIBUTING.md](../CONTRIBUTING.md). Please check open
issues before starting a larger change and keep both distributions and existing
SQLite backups in view.
