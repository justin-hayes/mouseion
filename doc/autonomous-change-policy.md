# Autonomous change policy

Mouseion uses a path-based human-review boundary so routine work can proceed
without making the implementation agent a security authority.

## Routine changes

Ordinary application code, tests, templates, and documentation may proceed
through the normal workflow:

1. issue → feature branch;
2. implementation and tests;
3. pull request;
4. required CI checks;
5. merge when repository review policy is satisfied.

The required CI checks currently are:

- **Build and test** — Go and Python build, unit tests, Python lint, and
  generated protobuf, sqlc, and stylesheet freshness
- **Integration tests** — PostgreSQL-backed Go integration tests
- **Browser smoke** — the Playwright suite, gated over four parallel shards

**Go lint** also runs on every pull request.

## Human-review paths

`.github/CODEOWNERS` assigns Justin as the owner for changes that can alter
security, data integrity, build trust, or deployment behavior. These include
workflows, migrations, auth/security code, dependency manifests, container and
infrastructure files, environment templates, and the ownership policy itself.

A pull request touching one of those paths must remain blocked until the
CODEOWNER review is complete. Stale approvals should be dismissed when new
commits are pushed so approval applies to the actual reviewed diff.

## Agent boundaries

The implementation agent may create branches, push branches, open and update
pull requests, comment on issues, inspect CI, and repair ordinary CI failures.
It must not bypass branch protection, alter repository rulesets, modify secrets,
or deploy production systems.

Auto-merge is intentionally a repository-settings decision, not an agent
judgment. It should only be enabled after the CODEOWNERS and required-check
policy has been reviewed and accepted by the repository owner.
