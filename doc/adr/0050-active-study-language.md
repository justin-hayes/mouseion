# ADR 0050: The app works in one active study language at a time

Status: **Accepted** · Date: 2026-09-07 · Author: Justin + opencode

## Context

ADR 0043 removed stored language preferences: a learner's study languages are
derived from their chosen-language Books and nothing is configured. But the
derived set only answers *which* languages the learner studies. It does not
answer *which one they are working in right now*, and every surface answered
that question differently: My Books carried per-request `?language=` pills
(including an "All languages" default nobody wanted), Vocabulary carried the
app's only `<select>`, and Reading Journey mixed languages with no language
control at all. Downstream artifacts — known vocabulary, decks, analysis,
campaigns, the ADR 0042 corpus view — were already language-keyed, so the
organizing surfaces were the odd ones out.

The learner's mental model is "I am reading German now." This ADR makes that a
first-class, learner-scoped context — an **active study language** — that scopes
every language-dependent surface. It revises ADR 0043's "no stored selection"
stance narrowly: the stored value points *into* the derived set and never
defines it.

## Decision

- **Active study language** is a stored, learner-scoped selection pointing into
  the derived study-language set. The set itself remains derived (ADR 0043);
  the selection is context, not configuration.
- **Defaulting and reset are lazy and deterministic.** When the set is
  unambiguous the selection is the sole study language. When it is ambiguous and
  nothing is stored, the default is the language of the most recently activated
  chosen-language Book. If the stored selection leaves the set (its last Book is
  removed or re-tagged by resync), it falls back to the most-recently-activated
  remaining language, else none. No eager writes on first run or on removal.
- **The shell carries a native `<select>` for the active language** whenever the
  learner has a language to select. Changing it on a language-scoped screen
  navigates to the same screen in the new language; on other screens it merely
  updates the stored mode. When no active language is resolved, its prompt is
  display-only rather than a choice.
- **Language-scoped surfaces read the mode, not a URL param.** My Books browse
  and search, Reading Journey, and Vocabulary all render the active language.
  The `?language=` parameter is removed from `/library` and `/vocabulary`;
  `/library?needs-language` becomes the distinct out-of-band browse state for
  Books awaiting a language (see ADR 0051's partition).
- **Book detail is not mode-scoped.** A Book renders its own language; a stale
  link to a Book in a non-active language never auto-switches the mode.
- **New-language arrival is passive.** Resync may introduce a new study
  language; it appears in the switcher with a "new" marker, never auto-switches
  the active language, and its Reading Journey is created lazily.
- **Known-vocabulary-only languages** (no current chosen-language Book) remain
  selectable in the switcher, marked "no books", read-only; import stays gated
  on the language being a current study language.

## Alternatives considered

- **Keep per-screen pickers.** Rejected: re-selecting a language on every screen
  is exactly the incoherence this ADR removes, and it leaves Reading Journey
  unscoped.
- **Derived-only with no stored pointer.** Rejected: "German is a study
  language" and "I am working in German now" are different facts; without a
  pointer every request needs a heuristic for the second.
- **Auto-switch the mode on cross-language navigation or new-language arrival.**
  Rejected: background sync and stale links must not hijack the learner's
  context.
- **Keep `?language=` alongside the mode.** Rejected: two competing sources of
  truth that must be reconciled on every request.

## Consequences

- ADR 0043's "no stored selection" is revised for a *context* pointer; the
  derived *set* is untouched.
- `?language=` URLs for `/library` and `/vocabulary` change; the "All languages"
  pill and per-row/card redundant language tags are removed (the switcher and
  section headings carry the context).
- The ADR 0042 corpus panel renders for the active language automatically.
- Requires a learner-scoped storage location (nullable, validated against the
  derived set at read time) — additive, low-risk.
- ADR 0051 makes the Reading Journey and Primary Goal per language; the two
  ADRs are implemented together.

## Related

- [ADR 0043: Study languages are derived from the library and Settings is removed](0043-study-languages-derived-settings-removed.md)
- [ADR 0042: Derive a per-language corpus view without a persisted corpus object](0042-derived-language-corpus-view.md)
- [ADR 0051: Reading journeys and primary goals are one per language](0051-reading-journeys-and-goals-per-language.md)
- [Feature: Language Mode](../features/language-mode.md)
