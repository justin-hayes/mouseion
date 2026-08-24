# ADR 0023: NLP service owns language capabilities

Status: **Accepted** · Date: 2026-08-24 · Author: Justin + Hermes

## Context

The web application currently treats study-language availability as a server/admin concern. Language support is actually determined by the NLP microservice's installed models and capabilities.

## Decision

The NLP service is authoritative for supported analysis languages. It exposes a typed capabilities endpoint containing at least:

- BCP-47/ISO language code;
- display name;
- model version;
- supported features;
- readiness.

The Go web application queries and caches this information. Learner study-language preferences remain owner-scoped, but may only select currently available NLP languages. Temporary NLP unavailability must not erase persisted learner preferences; the UI reports degraded capability discovery and blocks only operations that require unavailable analysis.

No admin-managed language allowlist remains.

## Consequences

- Adding a language is an NLP deployment concern.
- The web app no longer needs an admin language-management surface.
- Capability and readiness versions become observable compatibility data.
- A future NLP implementation can advertise different features without changing account policy.
