# Vocabulary Browse, Concordance, and Custom decks

Status: **Accepted specification — partially shipped** · Updated: 2026-10-03

This is the cohesive product and acceptance contract for Vocabulary Browse,
Concordance, and Custom decks. Browse now uses Current-reading Book scope,
displays Book-local and across-analyzed-Books occurrence counts, and orders by
those counts; its accepted default exclusions and include-all control are now
shipped. Other accepted Browse behavior remains subject to the
shipped/target distinctions below. The
cross-Book Concordance workbench is shipped. Occurrence
review and correction have a separate [contract](lemma-review-and-correction.md).
Concordance results are one server-rendered native list with grouped sentence
disclosures. The accepted [server-rendering decision](../adr/0083-concordance-server-rendering-and-htmx-4.md)
still governs the later replacement of the temporary handwritten request and
history enhancement.

## Purpose and boundaries

Give a learner working in **one active study language** a quick way to find
high-occurrence vocabulary in the Book they are currently reading that is not
already Known, Reserved, or in a Book deck prepared for that Book. They can
investigate actual occurrences and syntax across Books and save chosen
identities for independent Anki study. Vocabulary remains one of four primary
destinations (My Books, Reading, Vocabulary, Catalogs). Its peer views are
**Browse** (landing), **Concordance**, and **Import known vocabulary**.
Browse is a vocabulary-identity browser, not a second catalog of Books;
My Books remains the sole bibliographic Book browser. A Browse identity opens
Concordance; Concordance also permits
direct lookup. A learner reviews a Browse selection before naming a Custom deck.
Reading keeps its current Book, snapshot, and Book-preparation actions; no
permanent Reading-to-Vocabulary link is required. Browse never selects a Book
independently of the active study language's Current reading. Concordance,
Browse selection, and Custom decks remain language-wide, not Book-bound.

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
- Count only occurrences in an owned Book's **current completed analysis**
  matching its current acquired source, with the normalized corpus and selected
  analysis scope (including selected ancillary units). Any Book disposition is
  eligible. Missing, stale, failed, unavailable, or still incomplete analysis
  contributes nothing; never fall back to old runs, frozen Reading/deck
  snapshots, or aggregate lemma records. Continue using the prior matching
  current completed analysis while a replacement for the same source is
  pending; switch its contribution atomically on promotion. A changed source
  makes the old analysis stale until a matching run completes.
- **Occurrence count** sums eligible effective occurrences in the applied scope.
  Browse's scope is exactly the Current reading's Book; cross-Book selection/deck
  review and Concordance retain their own scope and may report distinct Book
  counts. A correction moves just its occurrence between identities;
  exclusion removes just its occurrence. A singleton counts. An identity with
  no eligible occurrences in the Current reading's Book is absent from Browse
  even if retained in a saved selection; being Known alone does not create a
  Browse row. Known, Reserved, and Generated never alter counts, though Browse
  hides them by default under the rules below. Concordance supplies correction
  and analyzer attribution; the simplified Browse table has no evidence column.
  Never silently mix analyzer and effective counts.
- Browse names the Current reading's Book and states whether it has current
  completed analysis and eligible vocabulary evidence. No Current reading,
  missing/stale analysis, an empty qualifying Book corpus, and zero matches
  within a nonempty corpus are different states. Never fall back to an earlier
  analysis, frozen snapshot, or other Books. Later analysis and occurrence
  decisions do not rewrite an already frozen snapshot or preparation.

For example, three qualifying occurrences in the Current reading's Book A and
one in Book B display `3` **In Book A** and `4` **Across analyzed books** in
Browse; correcting all three in A away removes the identity from Browse even
though B still has one.
`Drachen` corrected from `Drach/NOUN` to
`Drache/NOUN` in one dragon sentence does not change a separate kite use with
the same surface. On reanalysis, earlier decisions never transfer to new
occurrences merely because text or positions match.

## Browse and selection

- Browse only the Book in the active study language's Current reading. Display
  the Book's title and language beside the ranked results; do not offer a Book
  picker or fall back to every analyzed Book. Without a Current reading, show a
  named empty state linking to Reading; if the Book lacks current analysis, show
  a Book-specific recovery state linking to Reading. Keep the language's Browse
  selection and saved-deck access available in both states, without presenting
  them as current-Book results. An analyzed Book with no eligible identities and
  a filtered zero-result page need separate explanations.
- Default to **most eligible effective occurrences in that Book first**, then
  most occurrences across all owned Books with current completed analysis in the
  active language, then canonical lemma and POS for stable ties. The second
  count includes any Book disposition and only current effective eligible
  occurrences; it never adds identities to the Current-reading Book inventory.
  This is a discovery order, not a learning priority, coverage measure, or
  Reading recommendation. Keep only
  case-insensitive canonical-lemma **prefix** search, not surface/substring
  search, and one **Show already accounted-for words** control. Remove Book and
  POS filters, independent Known/Reserved controls, and alternate sorts. Ignore
  former Browse filter/sort URL parameters rather than widening the Book scope.
- By default, hide identities that are Known (including imported lemma-wide
  Known vocabulary), Reserved in the active language, **or** included in any
  successfully prepared Book deck for this Current reading's Book, including
  earlier preparations. An identity prepared first for another Book and later
  for this Book is still hidden; language-wide first-generation provenance is
  insufficient to determine Book inclusion. Omitted cards, frozen snapshot
  membership alone, and Custom deck preparation do not count as a Book-deck
  inclusion. The single include-all control reveals all such identities at
  once, without changing their counts or sort. Explain the exclusions above
  the table.
- Rows show canonical lemma (linked to exact effective-identity Concordance),
  analyzed POS, explicitly labeled occurrence counts **In [Current Book]** and
  **Across analyzed books**, **Learner state**, and Select/Remove. Always show
  both counts, including when equal. Display German `NOUN` lemmas with an
  initial capital using the shared lemma-display convention; canonical identity,
  prefix matching, links, and selection remain unchanged. Learner state uses textual **Unknown**, **Known**, or
  **Reserved** labels; show **Known · Reserved** when both are true rather than
  hiding either fact. Unknown means neither Known nor Reserved, not never
  generated. For identities revealed by include-all that appeared in a
  successfully prepared deck for this Book, show a secondary **In a Book deck**
  note within the Learner state cell, separate from the knowledge label. An
  imported lemma-wide Known identity is Known regardless of analyzed POS.
  Concordance opens initially restricted to this Book, with its existing
  ability to inspect other Books, individual corrections, and analyzer
  provenance. Do not show a Book count (always one in Browse) or correction
  indicator in the simplified table.
- Use stable **25-result** server-rendered pagination, preserving the applied
  prefix and include-all state. A recoverable **Browse selection** is an unnamed
  owner- and language-scoped set of effective identities. Selection is not
  deck creation.
  It survives pages, prefix/toggle changes, visits, switching Current reading
  Books, and switching active languages; languages have separate selections.
  A visible selected count opens review of *all* selected identities, including
  off-page, off-current-Book, and no-longer-evidenced ones. Review supports
  individual removal and confirmed **Clear selection**. The learner explicitly
  names/creates a Custom deck after review; only confirmed creation clears
  that language's unnamed selection.
- Independently page long selection/deck review, showing total selected and
  missing-evidence counts on each page and a missing-evidence filter. Off-Book
  identities can still have current evidence elsewhere; do not call them
  missing-evidence merely because they are absent from Browse. A saved
  deck may be edited while all its identities lack evidence, but cannot be
  prepared in that state. Known/Reserved/Generated state, effective identity,
  and current evidence remain separately legible.

## Concordance and sentence study

- Browse opens an **exact effective lemma + POS** query initially restricted
  to its Current reading's Book; the learner can broaden Concordance to other
  Books. Direct entry starts in **exact observed surface** mode; it can switch
  to effective lemma + POS or an explicitly named **Analyzer lemma + POS
  (evidence)** mode. A surface match
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
  the Browse Book scope never limits candidates. Automatically choose one
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
- KWIC uses one server-rendered list with a native `<details>`/`<summary>`
  disclosure per row. Clicking its summary expands the sentence, while native
  Enter/Space activation remains available. A distinct icon-only study link is
  reachable by Tab whether the row is collapsed or open. The Book title is the
  first column and truncates when needed. Do not add custom Up/Down or Escape
  row-key shortcuts: arrows keep normal scrolling behavior. A shared disclosure
  `name` closes the old row when another opens in supporting browsers, including
  without JavaScript; multiple open rows elsewhere are acceptable. **Do not
  close on blur** and strand the study link. Explicit close retains focus;
  opening another keeps focus on the new control. At narrow widths and 400%
  zoom, reflow into readable, wrapping sentence layouts instead of squeezing
  KWIC columns or scrolling the whole page.
  Visible truncated alignment is not the only way to perceive the full context.
  A diagram, if present, scrolls separately and never traps focus; textual
  syntax is always present when evidence exists. From sentence study, return
  to the applied page and originating result when still present, otherwise
  focus the Current results summary; carry a server-readable return target
  as well as the URL fragment so absent occurrences do not depend on
  client-side focus repair.
- Concordance uses **25-result** server-rendered Previous/Next paging,
  preserving mode, term/POS, Books, grammar, and page. State returned range,
  page and whether more results exist; an exact total can be deferred and
  must never be faked.
  Never call a truncated result set complete. Browse likewise pages stably
  with complete current-Book counts and descending frequency order. A change of
  Current reading, Book evidence/corrections, Known/Reserved state, or Book-deck
  inclusion that alters Browse results invalidates old page links. Announce a
  changed summary once, not every row. The Book and current-evidence scope and
  the include-all control's meaning remain textual, not color-only. Learner
  state stays legible in Browse and review; analyzer syntax in Concordance.
- Distinguish no Current reading, a Current reading without current analysis,
  a genuinely empty current-Book inventory, no matches within the **stated
  current scope**, and failed/over-budget query. Offer Reading recovery, Retry,
  or prefix search; never silently include history or treat a failed search as
  zero matches. For slow enhanced requests retain last
  successful results *under their old applied-query label*, show textual
  pending status, then replace only on success. On failure say the new request
  was not applied, leave the old results and URL unchanged, and offer recovery
  for the attempted query. Distinguish changed evidence (409), server timeout,
  and network failure from a successful empty result; HTMX status-specific
  swaps and a scoped network-error hook may enhance this without replacing
  the results. Native full-page requests use browser loading and an explicit
  error/recovery page. Preserve entered terms and scope for retry.
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
including high-frequency identities in a long Current-reading Book, prefix
search, include-all, and early/deep pages; measure Concordance Book subsets
and grammar filters separately. These are not hardware-independent promises or
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
instead of silently skipping/duplicating. A long Current-reading Book must
remain browsable without scanning unrelated Books, and no omitted Book scope
may masquerade as zero matches. Ordinary
search is interactive, not a hidden background job; preparation remains
durable and asynchronous, with bounded submission acknowledgement.

## Acceptance scenarios

1. Two owners sharing an analyzed source see only their own Books and effective
   correction decisions. Browse shows only the active language's Current
   reading Book; stopping or switching it changes the scope. Stale analysis
   produces Book-specific recovery, not historical or cross-Book counts;
   singletons are eligible. Book deck/snapshot frequency rules are unchanged.
2. Descending current-Book occurrence order, then descending across-analyzed-
   Books total and lemma/POS ties remain stable through prefix search,
   include-all and paging. Both labeled counts remain visible and the cross-Book
   total never creates a row outside the Current-reading Book inventory.
   Default results exclude
   Known (including lemma-wide imports), Reserved, and identities in any
   successfully prepared deck for that Book, even when first generated for
   another Book; omitted cards and Custom deck preparation do not exclude.
   Included rows label Unknown, Known, Reserved, or Known · Reserved truthfully;
   a prepared-for-this-Book word also has a separate Book-deck note, and a
   lemma-wide imported Known word is never labeled Unknown for any POS.
   Off-page, off-Book, and temporarily missing selected identities remain in
   paged review after Current reading and active-language switching. Opening
   a Browse identity starts a Book-restricted Concordance that can be broadened.
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
   p95 for high-frequency queries, grammar, Concordance subsets and later pages;
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
