# Settings architecture

MakerTrack Studio groups preferences under one compact, horizontal Settings navigation:

- **Appearance** changes the theme immediately and persists it locally.
- **Costs & Calculations** loads configuration only when opened and retains its existing Save/Discard workflow.
- **Features** controls optional local workflows.
- **Data Migration** imports CSV/XLSX records through mapping, validation and preview before one transactional write.
- **Data & Backups** provides ZIP export and restore.

Each section is addressable at `/settings?section=<appearance|calculations|features|migration|backups>`. Only the selected section loads its data, so a calculation-settings failure cannot prevent access to migration or backups.

## Compatibility and recovery

Backup export and import remain ordinary local backend routes. Existing ZIP backups remain compatible: unknown legacy manifest fields are ignored, and only application-managed configuration is restored. This keeps user data accessible while preventing obsolete private configuration from returning to the active database.

Spreadsheet migration does not change the SQLite schema or backup manifest. The
API parses bounded in-memory uploads and repeats validation immediately before a
single transaction; validation or write failures leave the live database intact.
