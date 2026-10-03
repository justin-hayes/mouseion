# Vocabulary Browse, Concordance, and Custom decks

Status: **Accepted specification — partially shipped** · Updated: 2026-10-03

This is the cohesive product and acceptance contract for Vocabulary Browse,
Concordance, and Custom decks. Browse and the first Concordance workbench are
shipped; remaining target behavior is called out below. Occurrence review and
correction have a separate [contract](lemma-review-and-correction.md).

## Purpose and boundaries

Give a learner working in **one active study language** a way to discover
vocabulary identities across their currently analyzed Books, investigate the
actual occurrences and syntax, and save chosen identities for independent Anki
study. Vocabulary remains one of four primary destinations (My Books, Reading,
Vocabulary, Catalogs). Its peer views are **Browse** (landing),
**Concordance**, and **Import known vocabulary**. Browse is a vocabulary-identity
browser, not a second catalog of Books; My Books remains the sole bibliographic
Book browser. A Browse identity opens Concordance; Concordance also permits
direct lookup. A learner reviews a Browse selection before naming a Custom deck.
Reading keeps its current Book, snapshot, and Book-preparation actions; no
permanent Reading-to-Vocabulary link is required.

The [Custom deck ADR](../adr/0082-independent-custom-vocabulary-decks.md)
owns the durable independence/freeze decision. This feature does **not** change
the existing recurring-vocabulary threshold or selection policy for Book
Prepared decks and current-reading snapshots. Generating or downloading any
deck never makes its identities Known, and Custom deck preparation does not
change Known, Reserved, or Generated vocabulary. No fifth destination,
cross-language inventory, in-app spaced repetition, individual
Concordance-sentence-to-deck action, collocations, or fuzzy/general full-text
search is included.

## Shared identity and evidence contract

- A Browse or saved-selection identity is `(study language, canonical lemma,
  analyzed POS)`, **effective** for this owner after confirmed occurrence
  decisions. Distinct senses with the same lemma and POS are not split. An
  occurrence must have a valid canonical lemma, analyzed `NOUN`, `VERB`, `ADJ`,
  or `ADV` POS, and pass the existing vocabulary-eligibility token predicate;
  detached `compound:prt` particles do not become items. Corrections may
  replace the canonical lemma but never POS; exclusions remove that occurrence
  from vocabulary counts. Neither surface spelling nor correction implies a
  language-wide rule. Preserve the raw lemma, analyzer-derived canonical
  lemma/POS, syntax, and source text as evidence.
- Count only occurrences in each owned Book's **current completed analysis**
  matching its current acquired source, with the normalized corpus and selected
  analysis scope (including selected ancillary units). Any Book disposition is
  eligible. Missing, stale, failed, unavailable, or still incomplete analysis
  contributes nothing; never fall back to old runs, frozen Reading/deck
  snapshots, or aggregate lemma records. Continue using the prior matching
  current completed analysis while a replacement for the same source is
  pending; switch its contribution atomically on promotion. A changed source
  makes the old analysis stale until a matching run completes.
- **Occurrence count** sums eligible effective occurrences; **Book count** is
  distinct owned Book IDs with at least one of them, even when two Books contain
  identical text. A correction moves just its occurrence between identities;
  exclusion removes just its occurrence. A singleton counts. An identity with
  no current eligible occurrences is absent from Browse unless it remains in a
  saved selection; being Known alone does not create a Browse row. Known,
  Reserved, and Generated are independent of counts and do not hide identities.
  Indicate when learner corrections contribute to a row's count; Concordance
  supplies individual attribution. Never silently mix analyzer and effective
  counts.
- Disclose how many active-language Books contributed current evidence and how
  many could not, with the applied Book subset identified where relevant. An
  empty qualifying corpus, zero matches within a nonempty corpus, and a
  partially analyzed library are different facts. Later analysis and occurrence
  decisions do not rewrite an already frozen snapshot or preparation.

For example, three qualifying occurrences in Book A and one in Book B display
`4 occurrences · 2 Books`; if all three in A are corrected away, display
`1 occurrence · 1 Book`. `Drachen` corrected from `Drach/NOUN` to
`Drache/NOUN` in one dragon sentence does not change a separate kite use with
the same surface. On reanalysis, earlier decisions never transfer to new
occurrences merely because text or positions match.

## Browse and selection

- Default to canonical-lemma alphabetical order with POS tie-break. Optional
  sorts by **most effective occurrences** and **most contributing Books** break
  ties by lemma then POS. These are discovery orders, not learning priority,
  coverage, or a Reading recommendation. Search is case-insensitive
  canonical-lemma **prefix** search, not surface search.
- Multi-select Book and POS filters; separate Known and Reserved state filters,
  including **Not Known or Reserved**. Show Generated provenance secondarily,
  without treating it as a filter or exclusion state in this first version.
  State filters narrow identities but do not alter their counts. Book filtering
  scopes the displayed occurrence and distinct-Book counts and both count
  sorts to those Books; explicitly label the scope. It is only a discovery
  filter, **not** a constraint on Custom deck sentence choice. Rows show lemma,
  POS, both scoped effective counts, Known/Reserved state, concise Generated
  provenance, and a correction indicator. Link to the identity's Concordance
  for exact examples, source attribution, and analyzer decisions.
- Use stable **25-result** server-rendered pagination, preserving applied
  query, filters, and sort. A recoverable **Browse selection** is an unnamed
  owner- and language-scoped set of effective identities. Selection is not
  deck creation.
  It survives pages, query/filter/sort changes, visits, and switching active
  languages; languages have separate selections. A visible selected count opens
  review of *all* selected identities, including off-page and no-longer-
  evidenced ones. Review supports individual removal and confirmed **Clear
  selection**. The learner explicitly names/creates a Custom deck after review;
  only confirmed creation clears that language's unnamed selection.
- Independently page long selection/deck review, showing total selected and
  missing-evidence counts on each page and a missing-evidence filter. A saved
  deck may be edited while all its identities lack evidence, but cannot be
  prepared in that state. Known/Reserved/Generated state, effective identity,
  and current evidence remain separately legible.

## Concordance and sentence study

- Browse opens an **exact effective lemma + POS** query. Direct entry starts in
  **exact observed surface** mode; it can switch to effective lemma + POS or an
  explicitly named **Analyzer lemma + POS (evidence)** mode. A surface match
  returns every observed match, even when analyzer/effective assignments
  differ and even for corrected or excluded occurrences. Effective lookup
  follows corrections and excludes omitted occurrences; analyzer-attribution
  lookup can find retained corrected/excluded evidence without claiming it is
  effective vocabulary. An excluded dependent can still be shown as syntactic
  evidence when a governor's dependents are inspected, clearly labeled as
  excluded. The governor is matched by effective identity in effective mode;
  dependency relations and heads remain analyzer-attributed.
- Query the active language's **current analyzed** owned Books by default; a
  learner can restrict to any selected subset, including multiple Books. No
  stale/historical corpus is included. Grammar exploration supports (a)
  restricting queried occurrences by their own dependency relation and (b)
  listing a queried governor's dependents in a selected relation. Each collapsed
  result shows its Book title, KWIC context centered on the observed target, and
  a link to study that exact sentence. Structural location, source order,
  analyzer lemma/POS, effective correction/exclusion state, relation, and head
  surface are available on the dedicated sentence-study page, not repeated in
  the flowing sentence. Sort deterministically by source order; offsets support
  highlighting but are not required as user-facing metadata. Source and analyzer
  attribution must not be rewritten to look like the learner's correction.
- Keep a compact always-visible lookup strip with explicit mode, term, POS
  when applicable, and **Find**. Book and grammar controls have separate
  disclosures and **Apply Books** / **Apply grammar** actions. The **Current
  results** summary always states the *applied* mode, term/POS, Book subset,
  grammar direction/relation, and result range; switching a mode or editing
  draft controls retains text for editing but does not relabel or rerun old
  results. Returning from sentence study restores applied query, page, and
  subset rather than beginning a new search.
- Results are a continuous KWIC list centered on the **observed surface**, with
  compact, truncated left/right windows and the Book title as the first column.
  The Book title stays compact and truncates when needed, including at narrow
  widths. A separate, icon-only study link is visible at the end of every
  collapsed row; its accessible name and tooltip identify the action. Expanding
  a row reveals only the complete flowing sentence with the observed target
  emphasized. It does not repeat Book/location or technical evidence. The study
  link opens a focused view with the complete sentence, Book and structural
  location, analyzer/effective token evidence, correction/exclusion state, and
  analyzer-attributed dependency relations and heads. A diagram may complement,
  never replace, structured textual tokens, relations/heads, and relevant
  dependents. If syntax evidence is missing, keep the valid sentence, explain
  the gap, and omit the diagram.

## Custom decks, preparation, and Anki

- A **Custom deck** is a named, saved, editable, owner- and study-language-
  scoped effective-identity shortlist. Multiple decks are allowed. Selected
  identities persist if evidence disappears, shrinks, or returns; do not
  silently change them to a newly corrected identity or transfer corrections
  to a replacement analysis. Review displays current counts, missing evidence,
  and available Book/analysis or exact-occurrence correction routes. Where the
  reason cannot be established, say current evidence is absent, not that a
  particular Book caused it. If the language ceases to be a derived study
  language, retain deck and artifacts read-only; disable edits and preparation
  until it returns. Preserve the unnamed selection without exposing editing in
  an unavailable language context.
- Permit naming a deck with missing identities. A mixed selection can proceed
  after review with expected omissions made explicit; if **every** identity
  lacks current eligible evidence, disable Prepare and queue nothing. Refresh
  evidence at review and submission. A gain or loss of *all* evidence for any
  identity between review and Prepare returns to updated review before freeze;
  count or contributing-Book changes alone do not require reconfirmation.
  **Prepare again** previews material evidence differences where appropriate.
- Preparation is an explicit, durable asynchronous generation from the current
  saved selection and currently eligible corpus, across all Book dispositions;
  the Browse Book filter never limits candidates. Automatically choose one
  occurrence-specific target and **complete** representative sentence per
  identity using the existing language-appropriate sentence-quality gate and
  ranking across all Books; prefer quality, not Reading state or Book frequency.
  Break score ties by stable Book identity then source location/order, with an
  occurrence-level tie-break for multiple targets in one sentence. Only an
  occurrence whose *effective* identity matches qualifies, including a
  corrected one; an excluded occurrence or identical surface under another
  identity does not. No per-card sentence picker is required.
- At submission freeze ordered identities, currently decidable selection and
  omission outcomes, exact
  Book/source/analysis/corpus identity, sentence, target occurrence, effective
  identity, analyzer attribution, and render inputs in the independent deck
  specification. Persist later quality/meaning omissions as durable outcomes
  against those frozen inputs. Persist enrichment under its exact frozen
  identity. Later
  shortlist edits, source/analysis changes, or occurrence decisions do not
  change that generation. Show chosen sentence and Book in results. A missing
  current occurrence or unacceptable sentence omits a card with its reason;
  an indefensible contextual meaning after the configured LLM response also
  omits it, without silently trying another sentence or sending extra text.
  Never publish a zero-card APKG. Distinguish evidence, quality, and meaning
  omissions. Provider/configuration/transport failures fail closed with safe
  retry; they must not publish a local-only deck. Automatic retries reuse the
  same frozen inputs; learner-requested Prepare again creates a new generation
  from current saved/effective evidence. Presentation-only rebuilds are
  revisions of one frozen specification, not new generations.
- Keep prior generations as history. During a replacement the most recent
  Ready artifact stays downloadable as **previous ready preparation**; a
  successful replacement becomes the only Mouseion-downloadable revision.
  Failed or cancelled replacement does not withdraw it. Deleting a deck
  withdraws its artifacts from *future Mouseion downloads*, but cannot revoke
  already downloaded APKGs. Download is a pure owner-scoped read. Existing
  recognition-card presentation, dictionary provenance, configured-LLM
  contextual Gloss and translation, and APKG-only export apply. Only target
  lemma, tested form, **one** chosen sentence, and bounded public lexical
  evidence may leave the installation; not Book title, surrounding Book text,
  owner, Reading history, or source location. Keep Book/source provenance in
  local results and suitable Anki metadata. No additional consent toggle or
  local-only fallback is introduced.
- A Custom deck has an Anki deck identity distinct from Book decks, stable
  across renames, with learner-chosen display name and collision-safe naming.
  Across Book and Custom decks, one owner/language/lemma/POS identity remains
  **one Anki note**; an import may update or move a note rather than make an
  isolated copy. Explain this before download. Verify actual Anki re-import
  behavior, including the current lemma-level GUID versus the
  source-occurrence-bearing `Identity` field; do not promise in-place update
  or deletion when the identity or imported deck changes. Anki-side cleanup
  may be needed. Do not silently change the existing note-identity ADRs.

  The optional real-Anki backend acceptance test currently observes that
  Book-then-Custom imports with the shared GUID leave one note under Anki's
  default import behavior, retaining its original fields and Book deck placement.
  A changed source sentence and renamed Prepare-again package still leave that
  note unchanged. Keeping the Custom deck's Anki name stable lets Anki reuse the
  same deck and update its description with the renamed learner-facing title.
  The Anki deck-list name is a stable identity, while the learner-chosen title is
  shown in Mouseion, the download filename, and the Anki deck description.
  Treat these as observed behavior of the tested importer, not a guarantee across
  Anki versions or import choices; the learner warning must not promise note
  updates, moves, or cleanup.

## Interaction, accessibility, and failure contract

- All Browse, review, Concordance, sentence-study, confirmation, preparation
  status, Retry, and download paths work as semantic server-rendered pages,
  lists, native labeled forms/controls, ordinary links, and pagination without
  JavaScript. HTMX may enhance but never become the only way to act. Browse
  select/remove keeps focus on the control if present, else the results
  summary; applied query/page changes focus and announce the results summary
  once. Long reviews have their own paging and summaries, not a hidden
  current-page-only selection. Confirm Clear selection and deck deletion
  natively with their consequences stated.
- KWIC uses a named native disclosure per row (Enter/Space), with a distinct
  icon-only study link reachable by Tab whether the row is collapsed or open.
  The Book title is the first column and truncates when needed. When JavaScript
  runs, Up/Down moves focus between row disclosures in both the native list and
  Lit enhancement; elsewhere, including at the first/last row, arrows keep
  normal scrolling behavior. With JavaScript disabled, native disclosures and
  page scrolling remain available. Enhancement closes the old row upon opening
  another; without JS multiple open disclosures are acceptable. **Do not close
  on blur** and strand the study link. Escape may close an open row and return
  focus to its control; explicit close retains focus; opening another keeps
  focus on the new control. At narrow widths and 400% zoom, reflow into readable,
  wrapping sentence layouts instead of squeezing KWIC columns or scrolling the
  whole page.
  Visible truncated alignment is not the only way to perceive the full context.
  A diagram, if present, scrolls separately and never traps focus; textual
  syntax is always present when evidence exists. From sentence study, return
  to the applied page and originating result when still present, otherwise
  focus the summary; do not rely solely on URL fragments.
- Concordance uses **25-result** server-rendered Previous/Next paging,
  preserving mode, term/POS, Books, grammar, and page. State returned range,
  page and whether more results exist; an exact total can be deferred and
  must never be faked.
  Never call a truncated result set complete. Browse likewise pages stably
  with full-corpus scoped counts and count sorting. Announce a changed summary
  once, not every row. Book/identity, current evidence, Known/Reserved/Generated,
  and analyzer syntax remain textual, not color-only.
- Distinguish no Books with current analysis, partial corpus, genuinely empty
  inventory, no matches within the **stated current scope**, stale/unavailable
  Book evidence, and failed/over-budget query. Offer relevant current Book
  recovery, Retry, or narrower scope; never silently include history or treat
  a failed search as zero matches. For slow enhanced requests retain last
  successful results *under their old applied-query label*, show textual
  pending status, then replace only on success. On failure say the new request
  was not applied; native full-page requests use browser loading and an
  explicit error/recovery page. Preserve entered terms and scope for retry.
- Show durable queued/running/failed/cancelled/ready and complete-with-omission
  states; allow leaving and returning. If a submission times out or its outcome
  is uncertain, inspect durable saved-deck/preparation state before offering
  another submission. Repeating the **same action** cannot create duplicate
  decks or generations; when outcome is unknown, say so. Keep the previous
  Ready download available during failed replacement. Focus and announce
  status transitions once without stealing focus repeatedly. No enhancement
  failure implies an unacknowledged edit or preparation succeeded.

## Corpus-scale acceptance

The reference test corpus is about **500 currently analyzed Books in one study
language, 50 million analyzed tokens**, a realistic short/long-Book mix, and
1–2 users; review and preparation submission cover **1,000 selected
identities**. On a documented warm local acceptance setup, target **p95
completed page-request time ≤2 seconds** for Browse and ordinary Concordance
lookup/filter/paging, **≤5 seconds** for grammar-filtered Concordance. Record
hardware, fixture composition, setup, measurement boundary, workload and p95,
including common/high-frequency terms, Book subsets, filters, scoped count
sorts, and early/deep pages. These are not hardware-independent promises or
grounds for silently sampling counts. Browse ordering, page membership, and
counts use complete eligible current evidence; a successful zero-result claim
must search the complete stated eligible scope.

Enhanced requests expose pending text by approximately **1 second** and end
an unsuccessful interactive request by **10 seconds** with recovery rather
than indefinite loading. An expired deadline is not permission to report a
partial result as complete. The next successful request after analysis
promotion or an occurrence decision reflects new current evidence; a
lagging derived view marks itself updating and withholds affected results
instead of presenting stale numbers as current. Across requests paging uses
current evidence; if changes disrupt stable paging, offer restart-from-results
instead of silently skipping/duplicating. At thousands of Books, a broad
query may request narrower Book/query scope, but Book-scoped discovery must
remain usable and no omitted scope may masquerade as zero matches. Ordinary
search is interactive, not a hidden background job; preparation remains
durable and asynchronous, with bounded submission acknowledgement.

## Acceptance scenarios

1. Two owners sharing an analyzed source see only their own Books and effective
   correction decisions; one Book reanalysis or source staleness changes only
   its current contribution. Browse includes singletons, Known and Reserved
   identities, correct scoped counts, empty/partial distinctions, and every
   disposition; Book deck/snapshot frequency rules remain unchanged.
2. Browse prefix search, Book/POS/state filtering and both count sorts retain
   correct counts/order through paging. Off-page and temporarily missing
   selected identities remain in paged review after active-language switching.
3. Effective, surface and analyzer-evidence searches distinguish corrected,
   excluded, and unchanged occurrences; multi-Book, both syntax directions,
   complete sentence, source order, and direct lookup work without conflating
   analyzer evidence with the effective identity. No-JS disclosure and focused
   study preserve applied search and back navigation.
4. A saved deck can outlive missing evidence or a temporarily unavailable
   study language. Mixed evidence is reviewed; all-zero eligibility queues
   nothing. A gain/loss of all evidence between review/submission returns to
   review, while smaller count changes do not rewrite selection. Concurrent
   edits do not mutate a queued/ready generation.
5. Cross-Book sentence scoring and ties, corrected exact targets, quality or
   meaning omissions, provider failure, safe retries, re-preparation and
   presentation-only rebuilds preserve provenance and never publish a zero-card
   artifact. Re-import interactions with Book and renamed Custom decks are
   checked in real Anki; historical download succession is truthful.
6. Test 400% zoom, keyboard/screen-reader traversal, no JS, slow/failed HTMX,
   stale results and uncertain repeated submissions. Measure reference-scale
   p95 for common/high-frequency queries, grammar, subsets and later pages;
   verify fresh evidence and truthful bounded failure on an extreme corpus.

## Related contracts

- [Information architecture](../design/information-architecture.md) and
  [screen inventory](../design/screen-inventory.md) (target versus shipped views).
- [Reading workflow](reading-workflow.md), [lemma review](lemma-review-and-correction.md),
  [Concordance foundation](concordance-foundation.md),
  [dependency foundation](dependency-parse-foundation.md), and
  [sentence-quality scoring](sentence-quality-scoring.md).
- [ADR 0006](../adr/0006-anki-export-import-contracts.md),
  [ADR 0020](../adr/0020-anki-package-output.md),
  [ADR 0050](../adr/0050-active-study-language.md),
  [ADR 0059](../adr/0059-persisted-normalized-corpus-for-concordance.md),
  [ADR 0060](../adr/0060-persist-dependency-parses.md),
  [ADR 0071](../adr/0071-decouple-deck-data-from-presentation.md),
  [ADR 0078](../adr/0078-book-dispositions-and-current-reading.md),
  [ADR 0079](../adr/0079-contextual-glosses-require-llm.md), and
  [ADR 0081](../adr/0081-learner-owned-occurrence-lemma-corrections.md).
