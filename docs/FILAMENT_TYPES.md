# Filament types

Use **Manage types** on Filaments or inside a filament form (including a new
filament on Purchases) to add, edit or remove choices. Save applies the entire
list. Names are trimmed, non-empty and unique without regard to letter case.
There are at most 200 choices, each at most 200 characters.

Editing the catalog changes the choices for future entries; it does not rename
or delete types already assigned to filaments. An existing assigned type remains
available when editing that filament.

The local `GET /filament-types` and `PUT /filament-types` endpoints share this
catalog. GET returns an array of strings; PUT accepts `{ "types": ["PLA", "PETG"] }`
and returns the saved array. An empty array is allowed.

Storage uses the `filament_types` JSON entry in the existing `app_config` table.
No schema migration or change to `Filament.Type` is required. Before a catalog is
saved, the standard types plus existing custom filament types are offered.
The catalog is excluded from numeric settings and is included in SQLite backup
export/import. Restoring an older backup without this entry restores the initial
catalog behavior. An older app can continue to read existing filament records;
it will use its own fixed type choices. Back up the database before downgrading;
older releases may display this JSON entry among their settings, where it should
not be edited.
