# Product screenshots

This directory contains the curated screenshots intended for customer-facing
documentation, including the future project README.

The images are generated from deterministic fixture data, not from a developer's
local database. File names follow `<page-or-state>-<theme>.png`, so documentation
can use stable links such as `dashboard-light.png` or `models-add-dark.png`.

The catalogue covers:

- every primary page in light and dark mode;
- every Settings section, including Features, the language selector and Simple and Advanced calculation modes;
- the Portuguese (Portugal) Settings screen and calculation-defaults guidance in both themes;
- create forms, filters (including Print Jobs model search), filament type management and inline cost details;
- physical printer profiles, default/archive states and printer-specific costs;
- the Dashboard's recorded-expense breakdown, first-run migration entry point and the feature-gated Business Dashboard with completed-order financial metrics and recent Orders;
- the Sales list at wide and compact desktop widths, its active-filter menu, the new Order form at wide, compact and narrow widths, active production allocations, completed locked history and inline Sales Channel creation;
- the Dashboard and Print Jobs list at 390px width, plus the narrow Add Print Job dialog;
- the Portuguese (Portugal) new Order workflow, including the optional Sales Channel state;
- backup file selection;
- the Reports & Data Export page with period, category, and PDF format controls;
- the spreadsheet migration wizard with automatic mapping and validated preview;
- keyboard focus and pointer hover in the primary navigation.

The calculation-settings images intentionally capture the complete scrollable
form. Most remaining images use the standard desktop viewport; the Sales compact
scenarios use a 900px-wide viewport to verify laptop reflow. Selected narrow
scenarios use a 390px-wide viewport to verify smaller-window navigation, forms,
cards and dashboard record summaries.

To regenerate them after a visible product change:

```bash
cd apps/web
npm ci
npm run screenshots:install
npm run screenshots
```

Review the affected images in both themes before committing them. Playwright's temporary reports and test
artifacts are ignored; the PNG files in this directory are intentionally tracked.
When adding a page or a meaningful interaction, add its scenario to
`apps/web/visual/catalog.screenshots.spec.js` in the same change.

Run `npm run screenshots:check` to verify the catalogue without updating it (the
same command runs in CI). Playwright waits for consecutive stable captures and
compares pixels with a small colour tolerance for rendering noise and
antialiasing, with no differing pixels allowed beyond that tolerance. Missing
images and visual changes fail the check. CI uploads expected, actual and diff
images in the `desktop-visual-differences` artifact on failure. The separate
`npm run screenshots` command explicitly updates the tracked PNGs for review.
The fixture applies its theme before React mounts and waits for bundled fonts,
avoiding an initial dark-theme paint in light-theme captures.
