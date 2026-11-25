# Language Mode

Status: Implemented · Date: 2026-09-07 · Updated: 2026-11-25

Current Reading behavior is defined by the shipped
[Reading workflow](reading-workflow.md). Earlier Journey and Primary Goal
wording in this feature is historical and has been superseded.

## Motivation

The learner thinks "I am reading German now," but the app does not: My Books
defaults to an "All languages" browse nobody wants, Vocabulary re-selects a
language per visit, and Reading Journey mixes languages silently. Downstream
artifacts are already per-language (known vocabulary, decks, and
analysis), so the organizing surfaces are the odd ones out.
Treating language as the app's organizing context — the address of each page
naming its study language — aligns the surfaces with the artifacts.

## Goal

Make the study language part of the address of every per-language page
(`/de/library`, `/grc/reading`): a shell-level switcher that navigates between
languages; per-language My Books browse and search; per-language current reading
and Vocabulary. Remove the "All languages" default and
per-screen language pickers. Books without a chosen language are not listed in
My Books; their Catalog entries remain available for a later Add after the
language is fixed.

## Scope

This feature defines the learner-facing behaviour of the URL-carried study
language across the shell, My Books, Reading, Vocabulary, and compatibility
bookmarks. It implements
[ADR 0131](../adr/0131-the-url-carries-the-study-language.md), which supersedes
the stored mode of [ADR 0050](../adr/0050-active-study-language.md), and
[ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md).
Learner-facing labels use **Language** and the language's own name, never
**Mode** or **Active study language**.

## Requirements

### The switcher

- On every per-language page the authenticated shell carries a native
  `<select>` for the page's language, labelled for accessibility, adjacent to the
  four destinations. Job status, the Catalog, and Book-addressed pages have no
  language and no switcher.
- Options are the learner's study languages, plus any known-vocabulary-only
  language marked "no books". A newly arrived study language is marked "new".
- The switcher is hidden until at least one such language exists.
- Changing the selection is a plain GET navigation to the landing page of the
  same kind of page in the new language, with language-specific state (query,
  filters, lemma, sentence, return state) dropped.
- Viewing a per-language page records its language as the **Last study
  language**, which only chooses where an address without a language leads
  (`/library` redirects to `/de/library`, and the navigation links use it). It
  is recorded only once the page renders: a refused request does not change it.
  It defaults deterministically: the sole study language when unambiguous;
  otherwise the language of the most recently activated chosen-language Book.
  It resets lazily when it leaves the derived set.
- With no study language, an address without a language other than My Books
  redirects to the unprefixed My Books empty state.
- An unknown prefix redirects to the same page under the Last study language; a
  non-canonical prefix (`DE`, `de-DE`, `de_DE`) redirects to the canonical code,
  keeping path and query.

### My Books (`/{language}/library`)

- Browse, paging, and search are scoped to the active language. The "All
  languages" pill and per-row language tags are removed; a section heading names
  the language.
- The shipped My Books cover grid communicates disposition but not per-Book
  evidence or current-reading state. Book evidence remains on the Reading
  surface. The retired Language view panel does not render.
- A Book without a chosen language is not listed in My Books, but one with
  reading history keeps its membership and is reported by the backfill (see
  [Collection Browsing](collection-browsing.md#upgrading-existing-accounts)).
  Its Catalog entry stays browsable, so fixing the language and adding it later
  gives it a study language. The retired needs-language strip and
  `/library?needs-language` are gone: the route returns 404.
- Moving a Book to To Read targets that Book's language —
  which equals the active language by construction.

### Reading (`/{language}/reading`)

- Shows the active language's current Book, with its vocabulary Browse, or the unordered To Read chooser between Books.
- Current reading is independent per language; completion in one language does
  not affect current reading or history in another.
- An empty chooser offers a path to the active language's My Books.

### Vocabulary (`/{language}/vocabulary`)

- Opens Concordance; the current Book's Browse lives on Reading.
- Scoped to the active language; the page's language `<select>` is removed.
  Import targets the active language and is always eligible there.
- Known-vocabulary-only languages are reachable through the switcher
  (read-only, import disabled).

### Compatibility bookmark (`/journey/{bookID}`)

- The route validates the Book's own language, owner, membership, and current
  evidence before deciding where to send the learner.
- A same-language bookmark redirects to its anchor in Reading.
- A cross-language bookmark is a supported old-language request: it recovers to
  Reading in the Book's own language (see "Language precedence" below). The
  former Reading language-selection handoff chooser is retired.
- The route never depends on a stored language. Unauthorized, non-member,
  and unavailable Books remain unavailable.

## Language precedence ([ADR 0086](../adr/0086-reading-working-desk-hidden-visibility-and-concordance.md))

- A switcher change navigates to the landing page of the same kind of page in the
  chosen language, with screen, query, filter, and return state reset. Opening a
  page never starts a reading or submits an import.
- Per-language pages take their language from the address; Book-addressed
  actions take it from the Book and return to the Book's language. Two tabs may
  work in two languages at once, and no form is refused for naming a language
  other than the Last study language.
- A Browse origin carried to Concordance or Study is validated against the
  address's language and the live Browse evidence **before** anything is
  restored; a stale origin is refused with an explanation and mutates nothing.
- Language wins over stale commitment, evidence, or return restoration. References
  stay owner/language validated and grant no authorization.
- A catalog correction that makes a Current reading's Book language incompatible
  ends that reading with an explanation: reservations are released without
  completion or Known acceptance; visibility, history, snapshots, and artifacts
  are preserved; a fresh commitment in the corrected language is required.
  Missing or stale analysis alone never ends a commitment.
- Native cached Back may show stale content without a request; safety is enforced
  on every subsequent request. No polling or Back interception is required.

## States

| State | Required presentation | Primary exit |
|---|---|---|
| No study languages yet | Empty My Books with catalog CTA; switcher absent or empty | Connect a catalog |
| One study language | Sole language active; no switcher ambiguity | Browse |
| Multiple study languages | Switcher lists them; the page's language scopes the page | Switch language |
| Last study language leaves the set | Lazy reset to most-recently-activated remaining, else none | Switch |
| New language arrives via resync | Appears in switcher marked "new"; open pages unchanged | Switch |
| Known-vocabulary-only language | Switcher entry marked "no books", read-only vocab | Switch back |
| Legacy no-language Books exist | Never listed in My Books; one with reading history keeps its membership and is reported by the backfill | Fix catalog, re-sync |

## Non-goals

- Configuring which languages are studied (still derived, ADR 0043).
- Cross-language browse, search, or a global "All languages" default.
- A remediation flow for no-language Books (fixing happens in the catalog).
- Auto-switching the mode on navigation or sync.
- Persisted per-language inventory beyond the active selection and current
  reading state.

## Acceptance criteria

- The active language persists across requests and restores after sign-in.
- Every language-scoped surface reflects the active language without a per-page
  picker.
- Browse and search never return Books outside the active language.
- Current reading and My Books scope follow the active language; switching
  languages does not change another language's current Book or Read history.
- A Book without a chosen language is never listed in My Books; it becomes a
  study-language Book only after the learner adds it with a language.
- The switcher is keyboard-accessible and server-rendered before progressive
  enhancement.
