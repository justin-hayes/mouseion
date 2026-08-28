# AGENTS.md

## Project overview

Mouseion is a self-hosted Vocabulary Acquisition Tool.

Durable product specifications and architecture decisions live under `doc/`.
Feature documents define substantial units of product work. Significant
architectural decisions are recorded as ADRs. GitHub issues define bounded
implementation work derived from those documents.

The Obsidian vault is for brainstorming and project-management material. It is
not a canonical source for implementation decisions.

## Sources of truth

Mouseion uses the following durable artifacts:

* **Feature documents** define product behavior, motivation, scope, and
  feature-level requirements.
* **ADRs** record significant architectural decisions and their rationale.
* **GitHub issues** define bounded implementation units derived from a feature
  document and applicable ADRs.
* **Pull requests** implement and verify individual issues.

The normal development flow is:

```
feature document
    -> ADR(s), when architectural decisions are required
    -> implementation issues
    -> pull requests
```

GitHub Milestones may be used for organization but are not required and are
not a canonical source of product requirements.

When sources disagree, use this precedence:

1. Explicit human instructions
2. Accepted feature documents and ADRs under `doc/`
3. The current GitHub issue
4. This file and applicable nested `AGENTS.md` files
5. Existing implementation and tests
6. Non-canonical planning material

Feature documents and ADRs normally answer different questions: feature
documents specify what the product should do, while ADRs specify significant
decisions about how it should be built. If they materially conflict, surface
the conflict rather than silently choosing one.

## Feature planning

Substantial product work should normally begin with a feature document.

When asked to plan implementation of a feature:

1. Read the complete feature document.
2. Inspect the existing implementation, relevant specifications, tests, and
   accepted ADRs.
3. Identify architectural decisions required before implementation.
4. Record significant architectural decisions as ADRs before implementation
   issues depend on them.
5. Decompose the feature into a small set of coherent, independently
   actionable GitHub issues.
6. Identify dependencies and an appropriate implementation order.
7. Ensure that the complete set of issues covers the feature requirements
   without unnecessary duplication.

Prefer approximately 3–7 substantive issues for a typical feature, but choose
issue boundaries based on coherent implementation units rather than a target
count.

Prefer issues that can be implemented and reviewed independently. Avoid
splitting work mechanically by file, layer, or implementation step when a
vertical slice would produce a more coherent unit.

Each issue should contain enough context to be executable in a fresh agent
session. It should reference the relevant feature document and ADRs rather
than duplicating their complete contents.

Important architectural decisions must be persisted as ADRs rather than
existing only in an issue or agent conversation.

Do not begin substantial implementation during feature planning unless
explicitly requested.

## Implementation contract

A GitHub issue is the normal unit of implementation work.

Before changing code:

1. Fetch remote state and start from an up-to-date `origin/main`.
2. Never commit directly to `main`; use a feature branch or isolated worktree.
3. Read the complete GitHub issue, including acceptance criteria,
   dependencies, and referenced decisions.
4. Read the referenced feature document.
5. Read referenced ADRs and other directly relevant accepted ADRs.
6. Inspect nearby implementation and tests before designing changes.
7. Verify that required predecessor work is available.
8. Identify whether the task touches a human-review boundary described below.

Treat product requirements in accepted feature documents and architectural
decisions in accepted ADRs as established constraints.

Do not re-litigate those decisions merely because another design appears
preferable.

If implementation reveals concrete evidence that an approved requirement or
architectural decision is incorrect, incomplete, unsafe, or impractical,
document the evidence and escalate the decision rather than silently departing
from the documented design.

## Implementation scope

Make the smallest coherent change that completely satisfies the issue.

Prefer changes that:

* satisfy all acceptance criteria;
* preserve established architecture unless the issue explicitly changes it;
* follow existing repository patterns;
* add or update tests for changed behavior;
* avoid unrelated cleanup and refactoring;
* keep commits and pull requests reviewable.

Do not expand an issue merely because adjacent improvements are convenient.

When useful work is discovered outside the issue scope, record or propose it
as follow-up work rather than incorporating it automatically.

If the issue cannot be completed without substantial out-of-scope work,
surface the dependency before expanding the scope.

Do not implement work assigned to a separate issue merely for convenience,
unless it is strictly necessary to complete the current issue. If issue
boundaries prove incorrect, surface that fact rather than silently combining
multiple issues.

## Coding-agent escalation

Start Codex in `normal` mode for all repository work, including substantial or
cross-cutting issues. Escalate to Sol/deep only when a concrete blocker
requires it, such as architectural ambiguity or a repeated implementation or
test failure. Keep the escalation narrowly scoped to resolving that blocker,
then return to normal mode for the remaining implementation and verification.

## Autonomous implementation

Implementation agents are expected to work autonomously through ordinary
engineering problems, including:

* locating relevant code;
* understanding existing patterns;
* implementing requested behavior;
* writing and updating tests;
* resolving ordinary compilation, lint, and test failures;
* regenerating derived artifacts;
* inspecting their own diffs;
* responding to actionable CI failures.

Do not request human input for routine implementation choices that can be
resolved from the issue, feature document, ADRs, repository, tests, or
established patterns.

Escalation is appropriate when:

* requirements materially conflict;
* a significant architectural decision is missing;
* multiple architectural choices have meaningful long-term consequences;
* implementation evidence contradicts an accepted design;
* a required decision crosses a human-review boundary;
* repeated well-founded implementation attempts fail;
* security, data integrity, or destructive behavior is uncertain;
* completing the issue requires substantial unplanned scope.

When escalating, provide a concise summary of:

1. the decision or blocker;
2. relevant evidence;
3. approaches already attempted;
4. viable options and their trade-offs;
5. the recommended next step.

## Generated artifacts

Regenerate derived artifacts rather than hand-editing them.

* Run `templ generate` for Templ output.
* Run `make gen` for protobuf output.

Generated output must remain committed and reproducible.

After generation, inspect the diff to ensure generated changes correspond to
the intended source changes.

## Verification

Before considering implementation complete, run the applicable formatter,
generator, linter, build, unit tests, and integration tests.

Use:

```bash
export PATH="$HOME/go/bin:$PATH"
export TMPDIR=/root/tmp-go

templ generate
git diff --exit-code -- 'internal/webapp/*_templ.go'

make gen

go build ./...
go vet ./...
go test ./...
make lint

git diff --exit-code
```

Use `TMPDIR=/root/tmp-go` when the execution environment has a `noexec` `/tmp`.

Run integration tests with the repository's PostgreSQL/Testcontainers harness
when the environment supports them.

If a required check cannot run locally:

1. state explicitly which check could not run and why;
2. run every applicable check that can run locally;
3. rely on the corresponding required CI job;
4. do not describe the change as fully verified until that CI job succeeds.

Do not weaken, skip, or modify tests merely to obtain a passing result unless
the test itself is demonstrably incorrect because of an approved behavior
change.

## Completion criteria

An implementation issue is complete only when:

* all acceptance criteria are satisfied;
* relevant tests have been added or updated;
* generated artifacts are current;
* applicable local verification passes;
* the final diff contains no unintended changes;
* relevant documentation is updated;
* the feature branch has been pushed;
* a pull request references the GitHub issue;
* required CI checks pass;
* required human review has been requested.

A passing build alone does not mean an issue is complete.

## Pull-request workflow

After local verification:

1. Inspect the complete diff against `origin/main`.
2. Confirm that the diff is limited to the intended issue.
3. Commit using a Conventional Commit message.
4. Push the feature branch.
5. Open a pull request referencing the issue.
6. Summarize the implementation and verification performed.
7. Explicitly identify anything that could not be verified locally.
8. Monitor required CI checks.
9. Investigate actionable CI failures, fix them, push the fix, and rerun the
   relevant checks.
10. Request required human review when applicable.
11. Request auto-merge only when repository policy permits it and all required
    checks and reviews have passed.

Never bypass branch protection or required review.

## Human-review boundary

The following paths require human review through CODEOWNERS and must not be
auto-merged without that review:

* `.github/` and CI/workflow configuration
* database migrations
* authentication and authorization/security code
* dependency manifests and lockfiles
* deployment, infrastructure, and container configuration
* environment and secret templates
* `CODEOWNERS`
* this `AGENTS.md` policy file

An approved issue may modify these paths when required, but the pull request
must clearly identify the sensitive changes and the human review or decision
required.

Do not block unrelated independent work merely because another change awaits
human review.

Never modify secrets, production environments, GitHub rulesets, branch
protection, or deployment credentials as part of an implementation task.

## Repository conventions

* Use Conventional Commits: `type(scope): imperative summary`.
* Keep ADRs in `doc/adr/`.
* Update the product ADR index when adding an ADR.
* Keep generated output committed and reproducible.
* Prefer owner-scoped queries and explicit transaction boundaries.
* Do not conflate exported vocabulary with mastered/known vocabulary.
* Prefer established repository patterns over introducing new abstractions
  for a single use case.
* Comments should explain non-obvious intent or constraints rather than
  restating code.

## Durable agent knowledge

Agent conversations and session memory are not sources of truth.

Any decision needed by future work must be persisted in the repository or
GitHub.

Use the appropriate durable artifact:

* product behavior, motivation, and feature-level scope -> feature document;
* significant architectural decisions and rationale -> ADR;
* bounded implementation work and acceptance criteria -> GitHub issue;
* implementation and verification -> pull request.

A fresh agent session must be able to reconstruct the intent and constraints
of an implementation task from the issue, referenced feature document, ADRs,
and repository alone.

## Frontend and product design

Before substantial frontend or UX work, read:

* `doc/design/README.md`
* `doc/design/principles.md`
* `doc/design/experience-direction.md`
* `doc/design/design-system.md`
* `doc/design/information-architecture.md`
* any relevant workflow or screen documents under `doc/design/`

Project-specific design decisions belong in `doc/design/`, not in agent
profiles or conversational memory.

When a frontend change introduces a reusable visual or interaction pattern,
update the relevant design documentation when necessary.

Do not introduce arbitrary new typography, spacing, colors, interaction
patterns, or UI primitives when an established project pattern already
exists.
