# Language Mode

Status: Proposed · Date: 2026-09-07

## Motivation

The learner thinks "I am reading German now," but the app does not: My Books
defaults to an "All languages" browse nobody wants, Vocabulary re-selects a
language per visit, and Reading Journey mixes languages silently. Downstream
artifacts are already per-language (known vocabulary, decks, analysis, and
campaigns), so the organizing surfaces are the odd ones out.
Treating language as the app's organizing mode — one **active study language**
scoping every surface — aligns the surfaces with the artifacts.

## Goal

Introduce the active study language as a global mode: a shell-level switcher;
per-language My Books browse and search; per-language Reading Journeys and
Primary Goals; per-language Vocabulary. Remove the "All languages" default and
per-screen language pickers. Legacy no-language Books get an explicit,
out-of-band "needs language" surface.

## Scope

This feature defines the learner-facing behaviour of language-as-mode across the
shell, My Books, Reading Journey, Vocabulary, and Journey entry. It implements
[ADR 0050](../adr/0050-active-study-language.md) and
[ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md). Learner-facing
labels use **Language** and the language's own name, never **Mode** or
**Active study language**.

## Requirements

### The switcher

- When the learner has a language, the authenticated shell carries a native
  `<select>` for the active language on every screen, labelled for accessibility,
  adjacent to the three destinations.
- Options are the learner's study languages, plus any known-vocabulary-only
  language marked "no books". A newly arrived study language is marked "new".
- The switcher is hidden until at least one such language exists. When no active
  language is resolved, "Choose a study language" is a display-only prompt, not
  a selectable language.
- Changing the selection on a language-scoped screen navigates to the same
  screen in the new language; on other screens it only updates the mode.
- Defaulting is deterministic: the sole study language when unambiguous;
  otherwise the language of the most recently activated chosen-language Book.
  The selection resets lazily when it leaves the derived set.

### My Books (`/library`)

- Browse, paging, and search are scoped to the active language. The "All
  languages" pill and per-row language tags are removed; a section heading names
  the language.
- Per-Book evidence remains visible in My Books; current analysis insights remain
  on the Journey entry. The retired Language view panel does not render.
- When any Book lacks a language, an out-of-band "N books need a language" strip
  appears (display-only: fix the language in the catalogue, then re-sync; no
  per-book actions). Its browse state is `/library?needs-language`.
- Adding a Book to the Reading Journey targets that Book's language Journey —
  which equals the active language by construction.

### Reading Journey (`/journey`)

- Shows only the active language's Journey: its order, its Primary Goal, its
  route comparison. A heading names the language.
- One Goal per language; completing or clearing a Goal does not affect other
  languages' Goals.
- An empty Journey in the active language offers a path to browse that
  language's Books.

### Vocabulary (`/vocabulary`)

- Scoped to the active language; the page's language `<select>` is removed.
  Import targets the active language and is always eligible there.
- Known-vocabulary-only languages are reachable through the switcher
  (read-only, import disabled).

### Journey entry (`/journey/{bookID}`)

- Not mode-scoped: a Book renders its own language; a stale cross-language link
  never auto-switches the mode. Language remains on the page header.

## States

| State | Required presentation | Primary exit |
|---|---|---|
| No study languages yet | Empty My Books with catalogue CTA; switcher absent or empty | Connect a catalogue |
| One study language | Sole language active; no switcher ambiguity | Browse |
| Multiple study languages | Switcher lists them; the active one scopes every surface | Switch language |
| Active language leaves the set | Lazy reset to most-recently-activated remaining, else none | Switch |
| New language arrives via resync | Appears in switcher marked "new"; mode unchanged | Switch |
| Known-vocabulary-only language | Switcher entry marked "no books", read-only vocab | Switch back |
| Legacy no-language Books exist | "Needs language" strip on My Books | Fix catalogue, re-sync |

## Non-goals

- Configuring which languages are studied (still derived, ADR 0043).
- Cross-language browse, search, or a global "All languages" default.
- A remediation flow for no-language Books (fixing happens in the catalogue).
- Auto-switching the mode on navigation or sync.
- Any persisted per-language inventory beyond the Journey/Goal and the active
  selection.

## Acceptance criteria

- The active language persists across requests and restores after sign-in.
- Every language-scoped surface reflects the active language without a per-page
  picker.
- Browse and search never return Books outside the active language.
- Journeys and Goals are isolated per language in storage and UI; reordering
  German does not change Italian's revision.
- A no-language Book is reachable only through the "needs language" strip and
  becomes a language Book after fix + re-sync.
- The switcher is keyboard-accessible and server-rendered before progressive
  enhancement.
