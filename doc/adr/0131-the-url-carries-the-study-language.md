# ADR 0131: The URL carries the study language

Status: **Accepted — implemented** · Date: 2026-11-24 · Author: Justin + OpenCode

Decided by Justin in a grilling session on 2026-11-24. Supersedes the stored
mode of [ADR 0050](0050-active-study-language.md) and the language-change
landing of
[ADR 0086](0086-reading-working-desk-hidden-visibility-and-concordance.md) §8.

## Context

ADR 0050 made the Active study language a stored, learner-scoped selection and
rejected URL language as "a second persistent source of truth". The rule was
never applied everywhere. My Books and Reading read the stored value, but
`/reading?language=` is a one-request exception, and Concordance and Sentence
study carry their language in the URL. The shell picker can therefore show
Greek over a German Concordance page. Changing the picker always lands on My
Books, even from Reading or Vocabulary (ADR 0086 §8). Two tabs cannot work in
two languages, and a bookmarked page renders in whichever language happens to
be stored.

## Decision

**The URL is the only source of truth for a page's study language.** Every
per-language surface (My Books, Reading and its Switch page, Vocabulary,
Concordance and Sentence study) carries its language as a path prefix, such as
`/de/library`, `/grc/reading` or `/grc/vocabulary/concordance`. The Active study
language is the language of the per-language page in view. Book-addressed
routes and their actions keep their Book's own language and take no prefix, and
the Catalog's Language filter remains a filter, not a page language
([ADR 0129](0129-one-catalog-presentation-for-every-source.md)).

**The shell picker is navigation.** It appears only on per-language pages and
lists the learner's study languages, including known-vocabulary-only languages
as ADR 0050 allows. Choosing a language opens the same kind of page in that
language and resets everything specific to the old language: search, filters,
page number, and a particular lemma or sentence. A deep page opens its
surface's landing page in the new language; for example, Concordance for one
lemma opens `/grc/vocabulary/concordance`.

**The stored selection becomes the Last study language.** It is updated whenever
the learner views a per-language page and is used only to resolve an unprefixed
URL or a navigation link. ADR 0050's lazy, deterministic defaulting and reset
rules apply to it unchanged. Unprefixed URLs, including every pre-existing
`/library`, `/reading` and `/vocabulary` URL and `?language=` link, redirect to
the prefixed form. A prefix that is not one of the learner's study languages
redirects to the same page under the Last study language. A learner with no
study language sees the unprefixed My Books empty state pointing to the
Catalog, with no picker.

Nothing switches the language except opening a page in it. Adding a Book, a
catalog sync, or a new language arriving never navigates the learner.

## Considered options

- **The database is the only source of truth; strip language from every URL.**
  Rejected: two tabs fight over one value, a shared or bookmarked link renders
  in whatever language is stored, and Back can show a page in a language other
  than the one the learner left.
- **A query parameter instead of a path prefix.** Rejected: the language is the
  page's scope, not a filter on it, and a parameter keeps "what if it is
  absent?" as a case on every request.
- **Keep ADR 0050 and make opening a language-scoped link switch the stored
  mode.** Rejected: it is the automatic switching ADR 0050 rejected, and it
  keeps two sources that can disagree.

## Consequences

- ADR 0050's objection to URL language as a *second* source of truth is met: the
  URL is the only one, and a stale link names its own language rather than
  overriding a stored one. Its "Changing it on a language-scoped screen navigates
  to the same screen in the new language" survives; its "on other screens it
  merely updates the stored mode" is retired with the picker on those screens.
- ADR 0086 §8's landing on My Books after a language change, and its redirect of
  old-language requests with a "Your study language changed" notice, are
  superseded. Its stale-context safety stays: a Browse origin or return pointer
  is still validated against its language, now the URL's, before anything is
  restored or mutated.
- Every per-language route, link builder and redirect changes, as do the
  browser smoke tests that address these pages.
