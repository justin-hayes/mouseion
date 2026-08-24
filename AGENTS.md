# AGENTS.md

## Project overview

Mouseion is a self-hosted Vocabulary Acquisition Tool. The canonical product
specification and architecture decisions live under `doc/`; the Obsidian vault
contains planning and project-management documents.

## Required workflow

For implementation tasks:

1. Start from an up-to-date `origin/main`.
2. Never commit directly to `main`; use a feature branch or isolated worktree.
3. Read the relevant product specification, ADRs, issue, and nearby tests
   before changing code.
4. Make the smallest coherent change that satisfies the issue.
5. Add or update tests for behavior changes.
6. Regenerate derived artifacts rather than hand-editing them:
   - run `templ generate` for Templ output;
   - run `make gen` for protobuf output.
7. Run the applicable formatter, linter, build, unit tests, and integration
   tests. Use `TMPDIR=/root/tmp-go` when the execution environment has a
   `noexec` `/tmp`.
8. Inspect the final diff and confirm generated files are clean.
9. Commit with a Conventional Commit message, push the feature branch, and
   open a pull request that references the issue.
10. Monitor required CI checks. If CI fails, investigate, fix, and rerun the
    relevant checks before declaring the PR ready.
11. Request auto-merge only when repository policy permits it and all required
    checks have passed. Never bypass branch protection.

## Human-review boundary

The following paths require Justin's review through CODEOWNERS and must not be
auto-merged without that review:

- `.github/` and CI/workflow configuration
- database migrations
- authentication and authorization/security code
- dependency manifests and lockfiles
- deployment, infrastructure, and container configuration
- environment and secret templates
- `CODEOWNERS` and this policy file

When a task requires one of these paths, explain the human decision needed in
the PR and continue with independent work that does not depend on that review.

Never modify secrets, production environments, GitHub rulesets, branch
protection, or deployment credentials from an implementation task.

## Verification commands

```bash
export PATH="$HOME/go/bin:$PATH"
export TMPDIR=/root/tmp-go

templ generate
git diff --exit-code -- 'internal/webapp/*_templ.go'
go build ./...
go vet ./...
go test ./...
make lint
make gen
git diff --exit-code
```

Run integration tests with the repository's PostgreSQL/Testcontainers harness
when the environment supports them. If they cannot run locally, report that
fact explicitly and rely on the required CI job.

## Repository conventions

- Use Conventional Commits: `type(scope): imperative summary`.
- Keep ADRs in `doc/adr/`; update the product ADR index when adding one.
- Keep generated output committed and reproducible.
- Prefer owner-scoped queries and explicit transaction boundaries.
- Do not conflate exported vocabulary with mastered/known vocabulary.
