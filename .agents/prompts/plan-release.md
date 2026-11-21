# Plan a GitHub Release

This is the repository's reusable process for turning user feedback, code audits and product goals into a reviewable GitHub release roadmap. It complements `AGENTS.md` and `.agents/prompts/implement-issue.md`.

**Invoke for:** planning a new release, redesigning a major feature, preparing GitHub Epics/sub-issues, or organizing a GitHub Project. **Do not use it to silently create issues for speculative improvements.**

## 1. Establish the release boundary

1. Confirm the repository, current release, target version and base branch. Inspect recent releases, existing issues/PRs and current code before deciding what is missing.
2. Apply Semantic Versioning: a compatible feature set normally fits a MINOR release; incompatible public API/behavior may require MAJOR. A large UI redesign does not alone mandate MAJOR.
3. Gather user feedback, reproduction steps, known bugs, current feature behavior, design goals and technical constraints. Separate **confirmed in code**, **requested**, **proposed**, and **not yet verified**.
4. Identify shared foundations and independent feature boundaries. Preserve existing data, identifiers, security boundaries, SQLite migration/backup behavior, desktop and LAN self-hosted operation.
5. Define what **ships together**, what is **optional vs built-in**, and which stretch goals are explicitly deferred.

Do not infer that a function is missing simply because a user asks for it. Search the code and reuse working capabilities. Do not invent source references or dependencies.

## 2. Design the architecture before splitting into tasks

Produce a brief, reviewable architecture plan:

- Current system and verified gaps.
- Proposed domains, ownership boundaries and integration contracts.
- Candidate models, APIs, state machines, migrations, transaction and idempotency rules.
- The most important failure cases and regression risks.
- Which foundational work must happen before cross-feature integration.
- Desktop, self-hosted, import/export and backup compatibility.
- Open questions that genuinely block implementation.

Keep the durable architecture in repository documentation or the parent Epics. Avoid duplicating storage or cost calculations across modules.

## 3. Propose the issue hierarchy **before publishing**

Present a draft to the user with:

1. The release goal and proposed version.
2. Suggested feature names and parent Epics.
3. Ordered child tasks for each Epic, with concise titles, scope and priority.
4. A dependency-aware implementation sequence, including work that may run in parallel.
5. Architecture decisions, risks, exclusions and Definition of Done.
6. Proposed project fields and branch/PR strategy.
7. A clear list of what will actually be created or changed on GitHub.

**Wait for user approval before creating or bulk-editing GitHub issues.** Treat an explicit request such as "create the Epics and sub-issues as planned" as approval for the agreed scope.

## 4. Issue templates

### Parent Epic

- **Title:** `[Epic][vX.Y.Z] Feature / Domain Name`
- **Type:** `Feature`; optionally use a dedicated `epic` label for filtering.
- **Sections:**
  - Goal, expected users and outcomes
  - Current behavior and verified gaps
  - Scope, non-goals and user-facing workflow
  - Domain ownership, architecture and integration contracts
  - Data/API/migration/backup compatibility
  - Target release, development branch policy and cross-Epic dependencies
  - Ordered child issue checklist with actual issue references
  - Explicit acceptance criteria and release Definition of Done
- Link related Epics and preserve their architectural independence.

### Implementation issue

- **Title:** `[vX.Y.Z][AREA-01] Short action-oriented title`
- **Type:** `Task` for implementation; `Bug` for confirmed defects.
- **Sections:**
  - Parent Epic, target release and priority
  - Goal and current behavior (based on inspection, not guesses)
  - Expected behavior and implementation approach
  - Likely components, APIs and persistence impact
  - Technical dependencies / `Blocked by`
  - Testable `- [ ]` acceptance criteria
  - Engineering checklist (tests, regression, migration/backup, docs, security, CI)
- Make tasks independently reviewable. Avoid giant mixed-purpose PRs and duplicate requirements.
- Keep task titles, descriptions, comments and checklists **in English**, unless the release owner requests another language.

### Priorities

- **P0 — Critical:** prerequisite, architecture/integrity risk or blocker for required features.
- **P1 — High:** important functionality required by the approved release scope.
- **P2 — Normal:** supporting work; explicitly decide if it is still part of the release or deferred.

Priority is not the same as execution order.

## 5. Build a valid dependency graph

1. Define shared architecture contracts first, then foundations, domain features, integration and final validation.
2. Represent mandatory prerequisites using GitHub's native **Blocked by** relationships. Mention them in the body for readability, but prose and checklists do **not** create native dependencies.
3. Use native **parent/sub-issue** relationships for hierarchy. A linked `- [ ] #123` checklist alone is **not** a native sub-issue.
4. Verify no cycles, invalid references, unnecessary serial constraints or dependency on an unrelated release.
5. Assign **Execution Order** as a recommendation using a topological ordering, not as a substitute for native blockers.
6. Keep the two architecture tasks eligible together when they must align on contracts; do not create artificial circular blocking relationships.
7. Re-fetch and verify all relations after creating them. State clearly when API/tool permissions prevent a native update.

Do not invent numeric issue identifiers before GitHub returns them.

## 6. Publish and verify GitHub issues

After approval:

1. Check the target repository for duplicate, superseded and already-completed issues.
2. Create parent Epics first.
3. Create child issues in the approved scope and include stable cross-references.
4. Update parent checklists with the real issue numbers.
5. Attach **native sub-issues**, then configure **native Blocked by** relationships.
6. Set labels, type, owners and a release milestone if available. Do not claim a milestone exists when the API cannot create or read it.
7. Confirm all issue titles, parent links, blocker lists and acceptance criteria via read-back.
8. Report canonical issue URLs, counts, failed operations and any duplicates closed or superseded.

Do not silently open speculative tickets beyond the approved release.

## 7. Configure one release-wide GitHub Project

Prefer **one GitHub Project per release**, not separate boards for mutually dependent Epics. Reuse an existing Project when appropriate.

Recommended fields and views:

| Field | Type / meaning |
| --- | --- |
| Status | Backlog → Ready → In Progress → In Review → Done |
| Execution Order | Number; suggested sequence, not a blocker |
| Priority | P0 / P1 / P2 |
| Phase | Release-specific stages (Architecture, Production, domain features, Integration, Finalization) |
| Area | Primary functional domain |
| Parent issue / Sub-issues progress | GitHub-native hierarchy/progress |
| Assignees | Responsible contributors |

Recommended views:

- **Execution Plan:** table ordered by Execution Order, with Status, Priority, Phase and Area.
- **Board:** Kanban grouped by Status for daily work.
- **Epics:** table filtered by the `epic` label, displaying native sub-issue progress.
- **Roadmap:** optional, only when time fields/estimates are meaningful.

`Blocked by` is a native **issue dependency**, not a custom Project field.

Keep previous patch-release work out of the new release view unless explicitly included. The Project shows progress; the issue remains the authoritative record for requirements and acceptance criteria.

## 8. Implement with gated, one-issue PRs

- Work from the updated `develop` branch unless the repository's documented branching policy changes.
- Create **one short-lived branch per issue**: `feat/<number>-topic`, `fix/<number>-topic`, `refactor/<number>-topic` or `docs/<number>-topic`.
- Do not pre-create all feature branches.
- Do not start a blocked task; prefer one `In Progress` coding task at a time for a solo maintainer.
- Use `.agents/prompts/implement-issue.md` when executing a ticket.
- PRs target `develop`; require review and applicable CI, regression and compatibility checks.
- Use focused commit subjects in the existing `type: concise imperative summary` style (for example, `fix: validate filament cost`). Keep commits coherent and avoid filler messages.
- Give each PR a concise title in the same style that describes its overall outcome; use a scope when useful, for example `feat(inventory): track material stock`. Write a proportionate description covering the problem, resulting behavior, relevant compatibility or migration impact, and validation performed or still pending. Link the issue and report checks honestly.
- Squash merge issue PRs into `develop` as required by its squash-only PR rule. Before merging, ensure the squash commit title summarizes the entire PR and its body retains useful context, issue references and any material compatibility notes; do not concatenate unrelated commit messages. Do not merge without authorization.
- Only after the issue is verified and merged into `develop` should it be closed/marked `Done`. Do not rely on automatic closure from a non-default base branch.
- Release to `main` only after all committed release scope has been integrated and end-to-end release validation passes.
- For an approved release PR from `develop` into `main`, select **Squash and merge**. Summarize the release in the squash commit and tag the resulting commit on `main` after its checks pass and publication is authorized. See `docs/development/branching-and-releases.md`.
- Never auto-merge into `main` or publish releases without explicit approval.

## 9. Honest issue/checklist synchronization

When the agent has GitHub write permission:

- Read the latest issue body before editing checkboxes.
- Mark `- [x]` **only when evidence exists** (implemented and verified, passing tests, validated docs/migration).
- Keep partially implemented, skipped, failing and CI-pending items unchecked.
- Preserve wording, user edits, history and unrelated checklists. Verify updates by reading back.
- Transition Project state to In Progress / In Review / Done only when the corresponding conditions actually hold.
- Do not close issues or mark Epics Done solely because an agent generated code or opened a PR.
- If permissions are missing, report exact changes required instead of claiming success.

## 10. Release readiness gate

Before the final `develop` → `main` PR:

- [ ] Approved release scope and every required sub-issue completed.
- [ ] Native blocker relations resolved; no unresolved P0 tasks.
- [ ] All relevant unit, integration, UI, build, security and packaging checks pass.
- [ ] Persistence, migration and desktop/self-hosted backup/restore scenarios verified.
- [ ] Existing user data, public API and commercial/financial history compatibility reviewed.
- [ ] Documentation, screenshots and release notes updated as applicable.
- [ ] Rollback/upgrade risks explained and release owner approved.

If any gate cannot be verified, state the gap; do not declare the release complete.

## Historical example: v1.1.0 (do not hardcode in future releases)

The v1.1.0 planning exercise produced two independent but jointly released feature Epics:

- [#7 — Native Inventory & Stock Management](https://github.com/GabrielAlmd/maker-track-studio/issues/7): 10 child tasks, including shared Production Foundation.
- [#8 — Business Operations (Sales & Orders V2)](https://github.com/GabrielAlmd/maker-track-studio/issues/8): 11 child tasks.
- [Personal GitHub Project #3](https://github.com/users/GabrielAlmd/projects/3): status, priority, phase, area and execution order.
- Architecture first: #9 (Production) and #19 (Business), followed by shared production changes, material stock, commercial workflows, finished-goods integration and release validation.
- Real `Blocked by` relationships and native sub-issues were used to distinguish hierarchy, suggested order and actual blockers.
- The agreed version was `v1.1.0` for both features, following the independent `v1.0.1` patch.

**For future releases:** repeat the discovery, proposal, approval, architecture and publication process; never copy v1.1.0 issue numbers, component assumptions, counts, phases or exact schedule without rechecking the current repository.
