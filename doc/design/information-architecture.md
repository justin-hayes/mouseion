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
    -> inspect the completed analysis and insights
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
a primary workflow action that enters connection management; future visual work
may distinguish it from destination navigation without changing its label or
route.

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
    /known-vocab/import
    /known-vocab/imports/{id}/status
```

The route hierarchy shows ownership and transitions; asynchronous status and
mutation endpoints are not separate navigation destinations.

## Secondary and inactive surfaces

- `/jobs` and `/jobs/{id}` are secondary operational/history surfaces. A learner
  reaches an individual analysis after starting it from a book. They are not a
  primary navigation destination.
- `/known-vocab` is a secondary direct route retained by the implementation.
  Settings is the canonical navigation entry for study languages and known
  vocabulary.
- The `Dashboard` template is inactive. `/` redirects to the library, and the
  dashboard's older `/languages` link is not part of the current IA. New work
  must not treat this template as an established screen.
- HTMX fragment and JSON status endpoints support a parent screen; they are not
  user-facing pages in the information architecture.

## Current workflow discontinuity

The accepted lifecycle requires learners to inspect insights for a specific
completed analysis before requesting deck preparation. The current analysis-job
page exposes deck preparation directly and does not provide a prominent path to
the book's analysis insights. Until that frontend gap is resolved, documents
and copy must not describe the job page as the canonical insights experience.
See [`workflows/book-analysis-and-deck.md`](workflows/book-analysis-and-deck.md).

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
