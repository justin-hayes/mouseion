# ADR 0127: Decision carry-over requires the same extracted-unit snapshot

Status: **Accepted; implemented** · Date: 2026-11-24 · Author: Justin + OpenCode

Amends [ADR 0126](0126-analysis-updates-carry-decisions-onto-identical-tokens.md)'s
scope rule. In addition to the same content revision, copying Lemma corrections
and Occurrence exclusions requires the replaced and new runs to share the
same extracted-unit snapshot. A [Source structure update](0103-source-structure-and-poetry-evidence.md)
keeps the content revision but adopts a new snapshot, so it copies neither
kind of decision.
