# Contributing

Thanks for considering a contribution to Maker Track Studio. The project is a
local-first desktop application plus an optional, protected LAN deployment.
Please keep both supported distributions and SQLite/backup compatibility in
view when changing shared behavior.

The official source for development and pull requests is
[GabrielAlmd/maker-track-studio](https://github.com/GabrielAlmd/maker-track-studio).

## Before you start

- Search open issues and discussions for existing work. For a large change,
  open an issue first so the product scope can be agreed.
- Small fixes, documentation improvements and focused tests are good first
  contributions. Issues labeled `good first issue` are suitable starting points.
- Do not include databases, backups, credentials, private keys, customer data,
  generated binaries or local machine paths in commits or screenshots.

## Development setup

Follow the [development guide](docs/DEVELOPMENT.md) for prerequisites, local
commands, architecture links, and common implementation paths. The
[developer wiki](docs/README.md) is the index for project documentation.

## Pull requests

- Keep a change focused and describe the user-visible result and motivation.
- Add tests for behavior changes and run the smallest relevant checks above.
- For visible UI changes, update the deterministic Playwright scenario in
  `apps/web/visual` and regenerate the matching screenshot under
  `docs/screenshots`. Fixtures must remain synthetic and must not read a local
  database or filesystem path.
- For persistence or backup changes, explain SQLite compatibility, recovery
  and rollback behavior.
- Do not add accounts, cloud synchronization, PostgreSQL, standalone Internet
  hosting or tracked build artifacts without an approved product requirement.
- Keep CI permissions minimal. Pull request workflows must not use secrets or
  publish artifacts to registries.

Use the pull request template and link the issue being addressed. A maintainer
will review scope, compatibility, security and validation evidence.
