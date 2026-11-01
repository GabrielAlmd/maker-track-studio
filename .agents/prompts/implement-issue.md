# Implement GitHub Issue

You are working as a senior software engineer in this repository.

Your task is to fully implement the following GitHub issue:

**Issue:** `#<ISSUE_NUMBER>`

Use the GitHub issue as the primary source of requirements.

## Objective

Take the issue from specification to a production-ready implementation, including code, tests, validation, and any required documentation changes.

Do not merely propose a solution. Inspect the repository, implement the change, test it, and leave the working tree in a state suitable for review.

## 1. Understand the issue

First:

1. Read the complete GitHub issue, including:
   - title
   - description
   - acceptance criteria
   - comments
   - linked issues or PRs
   - screenshots or referenced behavior, when available

2. Identify:
   - the user-visible problem or requested feature
   - expected behavior
   - affected components
   - explicit acceptance criteria
   - implicit constraints already established by the codebase

3. Inspect the repository before making changes.

Read relevant project documentation, especially:
- `AGENTS.md`
- `README.md`
- contributing/development documentation
- architecture documentation
- package-specific instructions
- existing tests
- nearby implementations of similar functionality

Repository-specific instructions override generic assumptions in this prompt.

## 2. Investigate the existing implementation

Trace the current behavior end-to-end before modifying it.

Locate the relevant:
- UI components
- domain models
- services
- API/backend code
- persistence/database code
- migrations
- validation
- shared types
- state management
- tests

Search for similar features already implemented elsewhere in the repository and reuse established patterns.

Do not introduce a new architectural pattern when an appropriate pattern already exists.

Determine the root cause before fixing a bug.

For a feature, determine the smallest coherent architecture that satisfies the issue without creating unnecessary coupling.

## 3. Create an implementation plan

Before editing, briefly state:

- what currently happens
- what should happen
- root cause or missing capability
- files/components likely to change
- implementation approach
- tests that need to be added or updated
- any migration or compatibility considerations

Then proceed with the implementation.

Do not stop after the plan.

## 4. Implement the issue

Implement the complete issue using the conventions already present in the repository.

Requirements:

- Keep changes scoped to the issue.
- Prefer small, maintainable changes over broad refactors.
- Do not change unrelated behavior.
- Do not silently alter public APIs or persisted data formats.
- Preserve backward compatibility unless the issue explicitly requires otherwise.
- Avoid duplicated logic.
- Reuse existing abstractions where appropriate.
- Keep domain/business logic separate from presentation logic.
- Handle errors explicitly.
- Validate data at appropriate boundaries.
- Avoid hard-coded values when they represent configurable or domain-specific data.
- Preserve existing logging conventions.
- Do not expose secrets or sensitive data in logs.
- Consider concurrency, race conditions, resource ownership, and cleanup where applicable.
- Consider database migration and existing-user data when persistence changes.
- Consider empty, invalid, missing, and boundary values.

For frontend work:
- follow existing design patterns
- maintain visual consistency with neighboring pages
- preserve responsive behavior
- provide appropriate loading, empty, success, validation, and error states
- maintain keyboard accessibility and semantic HTML where applicable
- avoid introducing unnecessary UI complexity

For backend work:
- validate inputs
- propagate errors correctly
- keep business rules centralized
- preserve transactional consistency where applicable

## 5. Tests

Tests are part of the implementation, not optional follow-up work.

Add or update tests covering:

- the primary success path
- the bug regression, if this fixes a bug
- important edge cases
- validation/error behavior
- affected business rules

Prefer testing observable behavior over implementation details.

Reuse the project's existing testing tools and patterns.

Do not weaken, delete, or skip existing tests simply to make the implementation pass.

If an existing test is incorrect because requirements changed, explain why before updating it.

## 6. Validate the implementation

After implementation, run the relevant repository validation commands.

At minimum, where supported by the project:

- formatting
- linting
- type checking
- unit tests
- integration tests relevant to the change
- build
- targeted end-to-end or UI tests when applicable

For large test suites, run targeted tests first and then the broader applicable suite.

If something cannot be executed in the current environment, clearly state:
- what could not be run
- why
- what command should be run by the developer/CI

Do not claim that a test passed unless you actually ran it successfully.

## 7. Review your own changes

Before finishing, inspect the final diff as if you were reviewing someone else's pull request.

Check for:

- incomplete acceptance criteria
- accidental unrelated changes
- duplicated logic
- dead code
- debugging code
- hard-coded values
- inconsistent naming
- missing error handling
- missing tests
- regression risks
- UI inconsistencies
- backwards compatibility problems
- database migration problems
- security issues
- performance regressions

Fix any problems found.

## 8. Final report

When complete, provide a concise implementation report containing:

### Issue
`#<ISSUE_NUMBER> — <issue title>`

### Implemented
Describe the resulting behavior, not just the files modified.

### Key changes
Summarize the important architectural/code changes.

### Tests
List the relevant tests added or updated.

### Validation
Report commands actually executed and whether they passed.

### Files changed
List the main files or modules modified.

### Remaining concerns
Only include genuine limitations, follow-up work, or decisions that require human review.

If there are none, state that there are no known remaining concerns.

## Important working rules

Do not:
- implement unrelated improvements
- perform opportunistic large refactors
- change dependencies without a concrete need
- suppress compiler/linter warnings instead of fixing their cause
- disable tests
- fake test results
- assume behavior without inspecting existing code
- stop after analysis when implementation is possible
- create compatibility-breaking changes without highlighting them

If the issue is ambiguous, resolve ambiguity from:
1. explicit acceptance criteria
2. issue comments
3. existing repository behavior
4. established architecture and conventions
5. the least surprising behavior for existing users

Only ask for clarification if a genuinely blocking product decision cannot safely be inferred.

Otherwise, make the safest reasonable decision, document it in the final report, and continue.

The definition of done is:

**Issue requirements satisfied + implementation complete + regression coverage added + relevant validation passing + diff reviewed.**
