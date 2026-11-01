# Desktop interface

React/Vite interface shared by the Wails desktop app and standalone browser
runtime. The `web` folder name is historical. There is no user login or session
management. Standalone LAN use requires the authenticated HTTPS proxy in
[LAN security setup](../../docs/LAN_SECURITY.md); credentials never belong in a
Vite environment variable or build bundle.

Use Node 20.19+ (Node 24 in CI). Start the complete desktop application from
`apps/desktop` with Wails so that the local backend also starts. `npm run dev`
starts the browser runtime and expects an API at the same origin under `/api`.

```bash
npm ci
npm run lint
npm test
npm run build
```

Customer-facing screenshots are generated with deterministic fixture data and are
stored in `../../docs/screenshots`. Install Chromium once and regenerate both theme
variants after visible UI changes:

```bash
npm run screenshots:install
npm run screenshots
```

Browser builds use same-origin `/api` requests so a reverse proxy can forward
them to the API. For local browser development, Vite proxies `/api` to
`http://127.0.0.1:8080` by default. To use another API port, set
`VITE_API_URL=http://127.0.0.1:8081 npm run dev` (PowerShell:
`$env:VITE_API_URL="http://127.0.0.1:8081"; npm run dev`). A standalone web build
uses `npm run build` and always calls same-origin `/api`. Wails build and
development scripts explicitly select desktop mode, which keeps the embedded
API at `http://127.0.0.1:8080`.

The browser runtime checks `/api/health/ready` before showing the app, uses
localStorage for locale preferences, and downloads reports and backups through
browser file downloads. Import remains a browser file upload. Desktop version
metadata and local diagnostic file access are desktop-only.

Backup export and restore are available in Settings.
Guided CSV/XLSX migration remains available in **Settings > Data Migration**.
