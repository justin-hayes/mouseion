# Information architecture

Mouseion is organized around a learner moving one owned book through a durable
reading-and-study lifecycle. The interface should foreground books, passages,
vocabulary, and the next learner decision rather than backend jobs or a generic
dashboard.

## Learner goals

The product supports four top-level goals:

1. collect books from a learner-owned OPDS catalog;
2. decide what part of a book to analyze and understand the result;
3. prepare and study one book-and-deck learning campaign at a time;
4. maintain study languages and known vocabulary.

The canonical lifecycle is:

```text
add to library
    -> review and confirm scope
    -> start analysis
    -> monitor analysis
    -> open the exact completed analysis result and insights
    -> prepare and download a deck
    -> queue and activate a learning campaign
    -> finish reading and review the deck
    -> graduate the campaign vocabulary to known
```

Acquisition, scope confirmation, analysis, deck preparation, and campaign
completion are separate explicit transitions. The UI must not imply that one
automatically performs the next.

## Product objects

- **Book** — the learner-facing center of the experience. It owns bibliographic
  identity and links the source, reviewed scopes, analyses, prepared decks, and
  campaigns that came from it.
- **Source snapshot** — immutable acquired EPUB content and extracted units. It
  is provenance, not a primary navigation destination.
- **Reviewed scope** — an immutable learner-confirmed selection of source units.
- **Analysis run** — an asynchronous attempt to analyze one confirmed scope.
  Queue and retry details are operational state.
- **Analysis result** — the immutable completed corpus and provenance used by
  insights and deck preparation.
- **Prepared deck** — an immutable APKG artifact produced asynchronously from
  one completed analysis result.
- **Learning campaign** — one book plus one prepared deck moving through queued,
  active, complete, or abandoned state. At most one campaign is active.
- **Known vocabulary** — owner-scoped vocabulary explicitly imported or
  graduated by completing a learning campaign.
- **Catalog connection** — a learner-owned OPDS endpoint and encrypted
  credentials used to find books.
- **Study language** — an owner-scoped language preference selected from the
  capabilities advertised as ready by the NLP service.

Learner-facing book state is broader than operational analysis-job state. Pages
should answer what the book needs next rather than exposing only the latest
backend status.

## Primary navigation

The authenticated shell currently exposes:

- **My Library** (`/library`) — the canonical home and book collection.
- **Learning** (`/campaigns`) — active campaign, queue, and history.
- **Add books** (`/connections`) — the entry action for catalog setup and
  browsing.
- **Settings** (`/settings`) — study languages and known vocabulary.

`My Library`, `Learning`, and `Settings` are stable destinations. `Add books` is
a global workflow action rather than a peer destination. It enters the
acquisition hub at `/connections` and should be visually distinguishable from
destination navigation without changing its label. The hub shows first-time
connection setup or lets the learner choose a saved catalog; it does not
silently select among multiple connections.

The Mouseion brand link points to `/`, which redirects to `/library`. There is no
active dashboard destination.

## Route and screen hierarchy

```text
/login
    first-account onboarding or sign in

/library
    /books/{id}
        /books/{id}/scope
        /books/{id}/analyze
        /books/{id}/analyses/{analysis-run-id}
        /jobs/{id}
            /jobs/{id}/status
            /jobs/{id}/deck/preparations
                /deck-preparations/{id}/status
                /deck-preparations/{id}/download

/connections
    /catalog
        /opds/language
        /opds/browse
        /opds/search
        /opds/acquire

/campaigns

/settings
    /settings/languages
    /settings#known-vocabulary
    /known-vocab/import
    /known-vocab/imports/{id}/status
```

The route hierarchy shows ownership and transitions; asynchronous status and
mutation endpoints are not separate navigation destinations.

## Secondary and inactive surfaces

- `/jobs` and `/jobs/{id}` are secondary operational/history surfaces. A learner
  reaches an active analysis run after starting it from a book. On completion,
  the primary transition is to the book-centered result at
  `/books/{book-id}/analyses/{analysis-run-id}`. Jobs are not a primary
  navigation destination.
- `/known-vocab` is a secondary direct route retained by the implementation.
  Settings is the canonical navigation entry for study languages and known
  vocabulary. Route consolidation should redirect it to
  `/settings#known-vocabulary` while preserving valid language context.
- The `Dashboard` template is inactive. `/` redirects to the library, and the
  dashboard's older `/languages` link is not part of the current IA. New work
  must not treat this template as an established screen.
- HTMX fragment and JSON status endpoints support a parent screen; they are not
  user-facing pages in the information architecture.

## Learner-facing book lifecycle

Library rows expose one primary next-step label derived from the learner's most
relevant state. The approved order is:

1. **Scope review required**;
2. **Ready to analyze**;
3. **Analysis queued** or **Analysis running**;
4. **Analysis failed — action required**;
5. **Analysis result ready**;
6. **Deck preparing**;
7. **Deck ready**;
8. **Queued for learning**;
9. **Learning in progress**;
10. **Campaign complete** or **Campaign abandoned**.

This is a navigation aid, not a replacement for independent resource states.
Book detail and result screens continue to show analysis, deck, and campaign
state separately when more than one is relevant. A historical failure does not
override a newer successful result, and a ready result remains accessible after
a later campaign transition.

## Analysis continuity decision

The canonical completed-analysis destination is a book-centered, exact result:

```text
/books/{book-id}/analyses/{analysis-run-id}
```

It identifies the book, confirmed scope, immutable analysis, quality state,
insights, and eligible deck action. It is not a mutable “latest analysis” view.

The operational job page remains responsible for queued/running progress,
cancellation, retry, attempts, and failure recovery. When work completes, its
primary action becomes **View analysis result**. Deck preparation moves to the
exact result screen after the insights summary. The book detail screen lists
analysis history: active runs link to operational status and completed scoped
runs link to their exact result.

The current frontend has not yet implemented this route and still exposes deck
preparation on `/jobs/{id}`. Treat that as a known implementation gap, not a
competing pattern. See
[`workflows/book-analysis-and-deck.md`](workflows/book-analysis-and-deck.md).

## Result information hierarchy

An analysis result answers questions in this order:

1. **Identity and trust** — book, scope, analysis identity, and blocking or
   material quality warnings.
2. **Decision summary** — current scoped coverage, the most relevant projection,
   and whether reading now or preparing vocabulary is plausible.
3. **Vocabulary investment** — threshold counts, top unknowns, concentration,
   and learn-next projections.
4. **Structural context** — sentence and extraction signals kept separate from
   lexical coverage.
5. **Provenance and history** — exact scope units, classifier/source details,
   and links to other analyses.
6. **Next action** — prepare a deck, return to the book, or review a different
   scope. Deck preparation appears only after material quality warnings and the
   decision summary have been presented.

The next action may be summarized near the top and repeated after the insights,
but it must not visually bypass a blocking warning or turn the screen into a
deck-generation form with metrics below it.

## Settings ownership

Settings owns study-language preferences and known vocabulary. Removing a study
language removes only the preference; it does not delete books, analyses,
decks, campaigns, or known vocabulary. Known-vocabulary import is additive and
does not provide an implicit correction or campaign-reversal path.

## Consequential campaign transitions

The action that satisfies the second campaign-completion condition must disclose
that assigned vocabulary will graduate to known and that completion cannot
currently be undone in Mouseion. Abandonment must disclose that artifacts and
history remain while reserved vocabulary becomes eligible again. Full rules are
in [`workflows/learning-campaign.md`](workflows/learning-campaign.md).

## Cross-linking rules

Every primary screen should make four things clear:

1. which book, campaign, or setting is in context;
2. the learner-facing current state;
3. the recommended next action and why it is available;
4. how to return to the parent object without reconstructing the route through
   global navigation.

Operational identifiers, attempt counts, provenance, and classifier versions
remain available where useful, but they should not displace the book title,
learner decision, or next step.
