# ADR 0024: Learner-owned catalogs and removal of the admin role

Status: **Accepted** · Date: 2026-08-24 · Author: Justin + Hermes

## Context

The admin role was introduced for two capabilities: managing available languages and configuring shared OPDS connections. Language capability belongs to the NLP service, and OPDS catalogs/credentials belong to the learner's reading workflow.

## Decision

Mouseion removes the active admin role and administrator user-management workflow.

- OPDS connections become owner-scoped learner resources.
- Credentials remain encrypted at rest and are never shared between owners.
- Existing admin accounts migrate non-destructively to ordinary learner behavior.
- Admin dashboards, admin user creation/reset routes, and admin-only catalog configuration are removed from active application behavior.
- A fresh installation with no users allows creation of its first account through onboarding; existing users continue to use normal login.
- This milestone does not add an open/closed registration setting. The current deployment is Tailscale-only; account exposure policy can be revisited if deployment boundaries change.

The database may retain compatibility columns or historical records temporarily, but runtime authorization no longer depends on an admin role.

## Consequences

- Each learner configures their own OPDS catalogs.
- The first-account path must be safe, CSRF-protected, and transactional.
- Existing role and catalog data require migration tests.
- Server administration moves to deployment/configuration tooling rather than an in-application account role.
