# ADR 0056: Retire the Campaign learner surface

Status: **Accepted** · Date: 2026-09-10 · Author: Justin + Hermes

This decision supersedes the `/campaigns` compatibility and learner-facing
history portions of [ADR 0034](0034-reading-journey-identity-ordering.md), while
leaving its Reading Journey identity and ordering decision unchanged. The
Campaign queue, campaign history, and campaign operations are not learner-facing
surfaces: the Journey entry provides Goal snapshot and artifact context, and
historical campaign facts remain supporting provenance under
[ADR 0072](0072-goal-owned-vocabulary-and-journey-forecast.md).

Campaign routes and mutation handlers therefore remain unregistered and return
404. This prevents a second plan or history surface from competing with Reading
Journey and keeps vocabulary reservation and graduation attached to the Book's
prepared deck; active reservation and completion semantics follow
[ADR 0072](0072-goal-owned-vocabulary-and-journey-forecast.md).
