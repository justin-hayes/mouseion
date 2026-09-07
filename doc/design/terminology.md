# Terminology

Status: **Canonical learner-facing design language.** Analysis terms include
the target contract proposed in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md), which remains
unshipped until its implementation issues land. Terms that remain historical or
internal are identified explicitly; they must not become active learner-facing
navigation or plan labels.

Use these terms consistently in navigation, headings, actions, status messages,
future feature documents, and tests. Backend names may remain in code, APIs,
logs, and operational detail, but should not become primary learner-facing
language without a product reason.

## Principal learner-facing concepts

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **My Books** | Every book Mouseion knows about for the learner: acquired or metadata-only, assessed or unassessed, desired or not, current or distant. It is a collection, not a task list or readiness ranking. | My Library, Dashboard, Corpus |
| **Reading Journey** | A fluid, provisional order of learner-selected books they currently imagine reading. Membership and later order are reversible; adding a book expresses reading intent and automatically acquires and analyzes it (ensure-once) so it can be weighed against other candidates. | Learning queue, backlog, curriculum, plan, roadmap |
| **Primary Goal** | The one book the learner currently intends to finish, when one exists. It is embedded in Reading Journey, not a separate destination, and is a promotion of an analyzed Journey member. | Active campaign, target destination, current project |
| **Where next?** | The choice after a Primary Goal is finished or when no Goal exists. It invites selection or reconsideration without urgency or automatic advancement. | Start next, continue plan, complete Journey |

Only the Primary Goal carries commitment. Reading Journey membership carries
the consequence of automatic acquisition and analysis, but no commitment. A
learner may have no Primary Goal, an empty Reading Journey, or books in My
Books that never enter the Journey. The Journey has no destination, schedule,
overdue state, completion state, or progress percentage.

**Reading Horizon** may remain an internal design metaphor for changing
possibility. **Campaign**, **milestone**, **destination**, and **Journey
completion** are not primary learner-facing concepts.

## Books and acquisition

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Add books** | The catalogue setup and sync-maintenance experience on `/connections`; it is not a navigation destination. | Import books, ingest books |
| **Acquire EPUB content** | Download and validate content for one metadata-only My Books Book from Book detail, without implying Journey membership or commitment. It does not itself start analysis; adding the book to Reading Journey acquires and analyzes it automatically. | Import and analyze, add to queue |
| **Catalog connection** | A learner-owned OPDS endpoint and credentials. | Global catalog, admin catalog |
| **Catalogue sync** | Periodic, owner-scoped reconciliation that adds or updates bibliographic metadata for offered non-English languages whose NLP pipelines are ready. The resulting chosen-language Books derive study languages. It never implies content download or destructive mirroring. | Import all books, mirror, admin sync |
| **Metadata-only catalogue entry** | A Book and active My Books membership recorded from catalogue metadata, with no validated EPUB source snapshot yet. | Imported book, acquired book, placeholder source |
| **Lazy content acquisition** | Download and validate EPUB content only after the learner expresses intent to use a metadata-only Book. Adding a Book to Reading Journey is the intent that triggers acquisition and analysis. | Sync download, automatic analysis |
| **Book** | The learner-facing bibliographic object, led by title and author and qualified by edition when evidence depends on it. | Source, corpus, artifact when referring to the book |
| **Source snapshot** | Immutable acquired EPUB bytes and extracted units, used when provenance matters. | Book version when no content revision is meant |

Mouseion's current web acquisition path is OPDS. Do not promise direct EPUB
upload unless a shipped route and feature contract support it. **Acquire EPUB
content** creates or restores membership after the validated EPUB snapshot is
persisted. Historical compatibility artifacts may retain **Add to My Books** or
**Add to library**.

Connection sync uses complete factual states: **Never synced**, **Syncing**,
**Last synced**, and **Sync failed**. Always identify the connection and, for
last-synced or failed states, the relevant time or recovery. Do not use bare
**Active**, **Ready**, or **Updated** for sync state.

## Scope and analysis

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Scope review** | Review an all-on top-level EPUB 3 TOC checklist, or a flat readable-unit checklist when the TOC cannot be projected reliably, and decide what should be analyzed. | Preprocessing, import review, evidence dashboard |
| **TOC scope choice** | One top-level EPUB 3 navigation entry whose nested targets expand to existing persisted unit IDs in spine order. It is a view grouping, not a new durable unit. | Nested TOC control, hierarchy group |
| **Readable-unit fallback** | One checkbox per readable persisted unit in flat spine order when a complete TOC-to-unit partition is unavailable. | Whole-book recommendation, degraded classifier mode |
| **Confirmed scope** | An immutable learner-confirmed scope revision. | Current selection when historical identity matters |
| **Start analysis** | Explicitly submit one confirmed scope for asynchronous analysis. | Continue, process book |
| **Analysis run** | One durable queued/running/completed/failed/cancelled analysis attempt. | Job in primary learner-facing copy |
| **Analysis result** | The book's single current learner-facing analysis, shown on the book page. Immutable runs and exact source/scope provenance remain backend and operational audit facts. | Completed analysis #N, latest result, analysis history on the book page |
| **Analysis insights** | Current known coverage, vocabulary investment, highest-impact unknown vocabulary, and warning-only quality information for the current analysis. | Dashboard metrics, difficulty score, text profile on the learner surface |
| **View analysis result** | Leave operational status and open the book page for its current analysis, directly or through the run-specific compatibility redirect. | View job, view exact result |

Use **job** only for operational history or implementation-facing detail. A
book's learner-facing state may be **scope review required**, **ready to
analyze**, or **analysis result ready** even when backend state is expressed
differently. **Analysis history** is operational language for `GET /jobs`, not
a learner-facing book-page section. Run-specific analysis URLs remain only as
compatibility redirects to the book page.

## Journey and route evidence

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Your order** | The learner's current, canonical order of books in Reading Journey. | Manual preference, assigned order |
| **Vocabulary-efficient alternative** | An optional order of the same learner-selected books, optimized only for an explicitly stated lexical property and assumptions. | Best route, optimal Journey, recommended order |
| **Modeled additional vocabulary identities** | Exact lemma-identity preparation counts under named threshold, scope, sequence, and transition assumptions. | Total coverage mapped, effort score, cost without a unit |
| **Move earlier / Move later** | Visible keyboard-operable controls for reordering. Drag may supplement them. | Fix order, improve route |
| **Choose as Primary Goal** | Promote one analyzed Reading Journey member to the current commitment, from the Reading Journey screen. Requires a successfully completed current analysis and does not start analysis. | Begin optimal text, promote milestone |
| **Add to Reading Journey** | Express reading intent for a book: include it in the provisional sequence and automatically acquire and analyze it (ensure-once) so it can be weighed against other candidates. | Queue for learning, schedule book |
| **Remove from Reading Journey** | Remove provisional membership without deleting the book from My Books. | Delete book, abandon campaign |

Learner order always remains the active order unless the learner explicitly
adopts an alternative. Recalculation after reordering uses neutral language:
**Moving this book here changes the modeled preparation across the remaining
Journey by …** Never style a preference change as an error or warning.

## Reading, preparation, and vocabulary

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Reading in progress** | The learner has recorded that they are reading the book. | Learning in progress when only reading is meant |
| **Reading finished** | The learner has recorded finishing the book. This does not imply vocabulary knowledge. | Completed when the completed fact is unclear |
| **Vocabulary work in progress** | Preparation or review activity remains incomplete. | Nearly mastered |
| **Vocabulary work complete** | The product's accepted review condition has been recorded; any resulting knowledge transition must still be stated explicitly. | Mastered |
| **Known vocabulary** | Lemmas explicitly imported/marked known or graduated through an accepted transition. | Generated vocabulary, mastered vocabulary |
| **Active-campaign vocabulary** | Internal term for lemmas reserved by the accepted active-campaign contract but not counted as known. | Known, learned |
| **Generated vocabulary** | Immutable provenance that a lemma was assigned to a deck. | Known vocabulary |
| **Graduated vocabulary** | Vocabulary promoted to known through the accepted consequential transition. | Automatically mastered |
| **Unknown vocabulary** | Eligible analyzed lemmas not currently known or reserved by the accepted active-campaign contract. | Difficult words |
| **Recurring vocabulary** | Unknown lemmas appearing at least N times in the analyzed book; the pool a prepared deck selects, labeled **Deck vocabulary** in preparation. | Rare words, difficult words |

Do not use **mastered** as a synonym for generated, assigned, exported, merely
reviewed, or encountered while reading. Reading history, preparation state,
and vocabulary knowledge remain independent facts.

ADR 0036 supersedes the learner-facing completion and graduation semantics of
ADR 0027. The canonical experience presents reading-finished and deck-reviewed
as independent facts, and only the justified transition graduates vocabulary.
Do not imply that **Reading finished** alone changes known vocabulary.

## Coverage and projection

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current coverage** | Coverage from current known vocabulary under the accepted metric contract. | Reading level |
| **Projected coverage** | A clearly labeled hypothetical result after a named vocabulary transition. | Coverage when the condition is omitted |
| **Coverage threshold** | A planning marker derived from token-weighted coverage, such as 95%, 97%, or 99%. | Difficulty score, readiness rank |
| **Evidence needs review** | Existing evidence is stale, questionable, or no longer safely comparable. | Low confidence as an unexplained score |
| **Not assessed** | Mouseion has no completed comparable analysis for this book. | 0% ready |
| **Cannot currently assess** | Mouseion lacks a supported source, language capability, or other prerequisite and should state which. | Unsupported with no explanation |
| **Language view** | A derived, evidence-only same-language panel over current analyses and known vocabulary. Suitable specific headings include **Analyzed books** and **Coverage across German**. | Corpus, aggregate analysis, language dashboard |

Always state whether a number is current, projected, token-weighted, scoped,
conditional, stale, or unavailable. A selected threshold is a planning aid, not
a literary judgment or claim that the learner can or cannot read a book.
Prepared decks select recurring vocabulary and make no coverage claim; coverage
thresholds remain whole-book planning markers.

The internal feature name **language corpus view** is acceptable in technical
documents, but the learner-facing surface is never called **Corpus**. `CorpusID`
and `normalized_corpus_artifacts` already refer to an internal analysis artifact,
not the learner's My Books collection or language lens.

## Status and feedback

Use complete, factual labels where space permits:

- Primary Goal;
- in Reading Journey;
- reading in progress;
- reading finished;
- vocabulary work in progress;
- vocabulary work complete;
- scope review required;
- ready to analyze;
- analysis queued, running, failed, cancelled, or result ready;
- catalogue never synced, syncing, last synced, or sync failed;
- deck preparing or deck ready;
- evidence needs review;
- not assessed;
- cannot currently assess.

Avoid ambiguous pills such as **Reading**, **Completed**, **Ready**, or **Active**
without naming the resource or fact. Primary Goal is a role, not a generic
success status.

Use **error** for a failed request or operation, **warning** for risk or degraded
quality that does not block all progress, **notice** for neutral contextual
information, and **success** for a completed learner action. Messages say what
happened, what remained unchanged when material, and what the learner can do
next.
