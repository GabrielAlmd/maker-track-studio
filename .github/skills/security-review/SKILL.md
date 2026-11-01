---
name: security-review
description: >
  Security review checklist for the 3D Printing Cost Tracker. Use when adding
  or modifying Go handlers, routes, DTOs, database queries, or React forms.
  Covers OWASP Top 10 mapped to this stack: Go/Gin/GORM + React/Vite +
  SQLite. Invoke automatically when reviewing new handlers, new routes,
  or new frontend forms.
argument-hint: 'Handler, route, or feature to review (e.g. "print_job_handler.go" or "new filament form")'
---

# Security Review — 3D Printing Cost Tracker

## When to Use

- Adding or modifying a Go handler (`apps/api/handlers/`)
- Adding or modifying routes (`apps/api/routes/routes.go`)
- Adding or modifying DTOs or models
- Adding or modifying a React form with user input
- Before any commit touching authentication, file I/O, or external calls
- When updating dependencies (run audit commands from CI section below)

---

## Go / Gin / GORM Checklist

### Input Validation

- [ ] All request bodies are bound with `c.ShouldBindJSON` — never `c.Bind` without error check
- [ ] UUID path parameters are parsed with `uuid.Parse(c.Param("id"))` — reject on error with `400`
- [ ] No raw string concatenation in queries — GORM parameterises automatically, but verify any `db.Raw()` or `db.Exec()` calls use `?` placeholders
- [ ] Numeric fields have explicit range validation if they affect cost calculations (e.g. negative weight, negative price)

### SQL / GORM Safety

- [ ] No `db.Raw()` with user-controlled string interpolation — use `db.Raw("SELECT ... WHERE id = ?", id)` always
- [ ] No `db.Where(userInput)` — always use structured conditions: `db.Where("id = ?", id)`
- [ ] GORM `AutoMigrate` is not called in production entrypoint for destructive schema changes
- [ ] Sensitive fields (if added in future: tokens, passwords) are never returned in JSON — use `json:"-"` tag

### Error Handling and Information Disclosure

- [ ] Internal errors return `500` with a generic message (`"internal server error"`) — never expose raw DB errors or stack traces to the client
- [ ] `404` is returned for missing records — not `500` with GORM's `record not found` error
- [ ] CORS origin in `desktopruntime/server.go` is restricted to known origins — not `"*"` in production

### IDOR (Insecure Direct Object Reference)

- [ ] If multi-tenant or multi-user access is added in future: every query filters by owner/user ID, not just by resource UUID
- [ ] UUIDs provide obscurity but are not a security boundary — document this assumption

### Dependency Safety

Run before merging any dependency update:

```bash
# Go
cd apps/api
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Node
cd apps/web
npm audit --audit-level=high
```

### Documentation Screenshots and Fixtures

- [ ] Playwright fixtures intercept the loopback API and use deterministic synthetic data; never point screenshot generation at a real local database.
- [ ] Tracked screenshots contain no customer data, backup contents, personal filesystem paths, or other secrets.
- [ ] Browser binaries, Playwright reports, and transient test output are ignored; only reviewed documentation images in `docs/screenshots` are tracked.
- [ ] Any new browser-test dependency is covered by `npm audit --audit-level=high` and uses the pinned lockfile version in CI.

---

## React / Frontend Checklist

### Form Input

- [ ] All forms use `react-hook-form` + `yup` schema validation — no unvalidated `useState` forms
- [ ] Numeric fields (`type="number"`) have `min`, `max`, and `step` attributes to prevent invalid submissions
- [ ] No `dangerouslySetInnerHTML` anywhere in the component tree — data is rendered as text nodes only
- [ ] User-supplied strings are never inserted into `eval()`, `new Function()`, or similar

### API Communication

- [ ] Axios uses the local loopback backend; any `VITE_API_URL` override must be intentional
- [ ] Error responses from the API are displayed as user-facing messages via `react-hot-toast` — raw error objects are not rendered to the DOM
- [ ] No sensitive data (tokens, secrets) is stored in `localStorage` or `sessionStorage`

### Dependency Safety

- [ ] `npm audit --audit-level=high` passes before shipping
- [ ] Major version bumps are reviewed manually against their changelogs

---

## Runtime boundaries

- Keep the Desktop backend bound to loopback and CORS restricted to Wails/development origins.
- Self-Hosted browser access must use the exact configured origin through the authenticated HTTPS proxy; never publish the API directly.
- Keep backup recovery routes accessible after an unsafe-state incident.
- Validate backup size, ZIP contents and schema before importing.

## Automated CI Coverage

The quality workflow runs the locked npm audit on every PR. Release-candidate
validation (PRs targeting `main`), version tags, and manual packaging runs also
execute both Go vulnerability scans in the Linux desktop job, reusing its native
library setup:

- `govulncheck ./...` — detects known CVEs in Go transitive dependencies
- `npm audit --audit-level=high` — blocks on high/critical npm vulnerabilities

The standalone [security.yml](../../workflows/security.yml) repeats the full
dependency scan weekly and on manual dispatch. These cover **A06 (Vulnerable
Components)** continuously. The remaining items above require manual review at
code-change time.

The combined frontend quality job additionally regenerates deterministic screenshots and
fails if the tracked documentation images are stale.
