# Terminology

Status: **Canonical learner-facing design language.** Analysis terms follow the
shipped one-current-analysis contract in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) and the
reading-intent trigger in
[ADR 0049](../adr/0049-reading-intent-triggers-analysis.md). Goal-owned
vocabulary and Journey forecast terms follow
[ADR 0072](../adr/0072-goal-owned-vocabulary-and-journey-forecast.md). Terms that
remain historical or internal are identified explicitly; they must not become
active learner-facing navigation or plan labels.

Use these terms consistently in navigation, headings, actions, status messages,
future feature documents, and tests. Backend names may remain in code, APIs,
logs, and operational detail, but should not become primary learner-facing
language without a product reason.

## Principal learner-facing concepts

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **My Books** | Every book Mouseion knows about for the learner: acquired or metadata-only, assessed or unassessed, desired or not, current or distant. It is a collection, not a task list or readiness ranking. | My Library, Dashboard, Corpus |
| **Inbox / To Read / Set Aside** | My Books workflow buckets: Inbox is untriaged, To Read expresses reading intent through Journey membership, and Set Aside is not currently in the Journey. Every Book occupies one bucket; reading completion remains independent history. | Reading status, Journey position |
| **Reading Journey** | A fluid, provisional order of learner-selected books they currently imagine reading. Membership and later order are reversible; adding a book expresses reading intent and automatically acquires and analyzes it (ensure-once) so its current and on-arrival coverage can be understood. | Learning queue, backlog, curriculum, plan, roadmap |
| **Primary Goal** | The one book the learner currently intends to finish, when one exists. It is embedded in Reading Journey, owns one frozen vocabulary snapshot for its study language, and is a promotion of an analyzed Journey member. | Active campaign, target destination, current project |
| **Choose what to read next** | The completion-receipt action that opens the candidate chooser at `/reading`. It presents To Read candidates without choosing a Goal or advancing automatically. | Where next?, Start next, continue plan, complete Journey |

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
| **Catalogs** | The authenticated catalogue setup and sync-maintenance destination at `/catalogs`. | Import books, ingest books |
| **Add to Reading Journey** | Express reading intent for a My Books Book. It acquires the current EPUB when needed and ensures whole-book analysis once. | Start analysis, import and analyze, add to queue |
| **Catalog connection** | A learner-owned OPDS endpoint and credentials. | Global catalog, admin catalog |
| **Catalog sync** | Periodic, owner-scoped reconciliation that adds or updates bibliographic metadata for offered non-English languages whose NLP pipelines are ready. The resulting chosen-language Books derive study languages. It never implies EPUB content download or destructive mirroring; optional Book cover retrieval is independent metadata work. | Import all books, mirror, admin sync |
| **Metadata-only catalog entry** | A Book and active My Books membership recorded from catalog metadata, with no validated EPUB source snapshot yet. | Imported book, acquired book, placeholder source |
| **Lazy content acquisition** | Download and validate EPUB content only after the learner expresses intent to use a metadata-only Book. Adding a Book to Reading Journey is the intent that triggers acquisition and analysis. | Sync download, automatic analysis |
| **Book** | The learner-facing bibliographic object, led by title and author and qualified by edition when evidence depends on it. | Source, corpus, artifact when referring to the book |
| **Book cover** | The optional catalog-supplied image representing a Book. It may lead visually in My Books and support identity in Reading Journey, but title and author remain visible and authoritative. | Cover art, thumbnail when referring to the metadata |
| **Source snapshot** | Immutable acquired EPUB bytes and extracted units, used when provenance matters. | Book version when no content revision is meant |

Mouseion's current web acquisition path is OPDS. Do not promise direct EPUB
upload unless a shipped route and feature contract support it. **Add to Reading
Journey** acquires and validates content when needed as an ensure-once
consequence of reading intent. Historical compatibility artifacts may retain
**Add to My Books** or **Add to library**.

Connection sync uses complete factual states: **Never synced**, **Syncing**,
**Last synced**, and **Sync failed**. Always identify the connection and, for
last-synced or failed states, the relevant time or recovery. Do not use bare
**Active**, **Ready**, or **Updated** for sync state.

## Scope and analysis

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current analysis input** | The analyzed portion of the current extracted EPUB snapshot: its identified main text when declared EPUB structure provides one, otherwise the complete snapshot. Source revision and extracted-unit provenance remain durable internal facts. | Unscoped text, inferred content |
| **Stale analysis** | Existing evidence belongs to an older EPUB content revision. | Current evidence, failed Journey membership |
| **Analysis trigger** | Adding a Book to Reading Journey submits asynchronous analysis or re-analysis as needed. | Start analysis, continue, process book |
| **Analysis run** | One durable queued/running/completed/failed/cancelled analysis attempt. | Job in primary learner-facing copy |
| **Current analysis evidence** | The Book's current completed evidence, owned by the Book and presented from its Reading Journey anchor. Immutable runs and exact source/revision provenance remain backend and operational audit facts. | Completed analysis #N, latest result, analysis history on the learner surface |
| **Analysis evidence** | Current coverage and warning-only quality information used by Reading Journey; internal thresholds and top-unknown data remain available to analysis services without a learner-facing detail page. | Dashboard metrics, difficulty score, text profile on the learner surface |
| **View in Reading Journey** | Leave operational status and open the Book's canonical Reading Journey anchor, directly or through the run-specific compatibility redirect. | View job, view exact result |

Use **job** only for operational history or implementation-facing detail. A
Book's learner-facing state may be **ready to analyze**, **analysis queued**,
**analysis running**, **analysis result ready**, **stale**, or **unavailable**
even when backend state is expressed differently. **Analysis history** is
operational language for `GET /jobs`, not a learner-facing Book-page section.
Run-specific analysis URLs remain only as compatibility redirects to the Reading
Journey anchor.

## Journey and route evidence

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Your order** | The learner's current, canonical order of books in Reading Journey. | Manual preference, assigned order |
| **On-arrival coverage** | Coverage modeled from current Known vocabulary, the active Goal snapshot, and trustworthy recurring-vocabulary identities contributed by earlier Books in the learner's own order. | Best route, optimal Journey, recommended order |
| **Lower-bound forecast** | An on-arrival value calculated without an unavailable or untrustworthy earlier contribution; it is a useful floor, not a complete prediction. | Zero coverage, exact forecast |
| **Modeled additional vocabulary identities** | Exact lemma-identity preparation counts under a named scope and transition assumption, retained for individual analysis evidence rather than Journey route ranking. | Total coverage mapped, effort score, cost without a unit |
| **Move earlier / Move later** | Visible keyboard-operable controls for reordering. Drag may supplement them. | Fix order, improve route |
| **Choose as Primary Goal** | Promote one analyzed Reading Journey member to the current commitment, from the Reading Journey screen. Requires a successfully completed current analysis, freezes its recurring-vocabulary snapshot, and does not start analysis. | Begin optimal text, promote milestone |
| **Add to Reading Journey** | Express reading intent for a book: include it in the provisional sequence and automatically acquire and analyze it (ensure-once) so it can be weighed against other candidates. | Queue for learning, schedule book |
| **Remove from Reading Journey** | Remove provisional membership without deleting the book from My Books. | Delete book, abandon campaign |

Learner order is the only active order. Recalculation after reordering uses
neutral language: **Moving this book here changes the modeled on-arrival
coverage for later Books.** Never style a preference change as an error or
warning.

## Reading, preparation, and vocabulary

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Reading in progress** | The learner has recorded that they are reading the book. | Learning in progress when only reading is meant |
| **Reading finished** | The learner has recorded finishing the book. This does not imply vocabulary knowledge. | Completed when the completed fact is unclear |
| **Vocabulary work in progress** | Preparation or review activity remains incomplete. | Nearly mastered |
| **Vocabulary work complete** | The learner has accepted Primary Goal completion and its frozen snapshot has entered modeled Known vocabulary. This does not claim verified mastery. | Mastered, deck reviewed |
| **Known vocabulary** | Modeled learner knowledge: lemmas explicitly imported or accepted when a Primary Goal is completed. It does not claim verified mastery. | Generated vocabulary, mastered vocabulary |
| **Reserved vocabulary** | Lemmas in the immutable snapshot owned by the active Primary Goal, excluded from selection in that study language but not counted as Known. | Known, learned, studied vocabulary |
| **Goal vocabulary snapshot** | The exact recurring-vocabulary identity set frozen from the Goal's current analysis, with analysis, source, and selection provenance. | Deck contents, generated vocabulary |
| **Generated vocabulary** | Immutable provenance that an identity was assigned to a prepared deck; it is neither Known nor Reserved and is not a later selection exclusion. | Known vocabulary, reserved vocabulary |
| **Graduated vocabulary** | Historical name for vocabulary accepted into modeled Known vocabulary through Primary Goal completion. | Automatically mastered |
| **Unknown vocabulary** | Eligible analyzed lemmas not currently Known or Reserved by an active Primary Goal in that study language. | Difficult words |
| **Recurring vocabulary** | Unknown lemmas appearing at least N times in the analyzed book; the pool a prepared deck selects, labeled **Deck vocabulary** in preparation. | Rare words, difficult words |

Do not use **mastered** as a synonym for generated, assigned, exported, merely
reviewed, or encountered while reading. Reading history, preparation state,
and vocabulary knowledge remain independent facts.

ADR 0072 supersedes the conflicting learner-facing completion and graduation
semantics of ADRs 0036 and 0053. Completing a Primary Goal records durable
reading completion and accepts its frozen snapshot into modeled Known vocabulary
as one idempotent transition. Deck readiness or review is not required, and the
interface must not imply verified mastery.

## Coverage and projection

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current coverage** | Coverage from current Known vocabulary under the accepted metric contract. | Reading level |
| **After-Goal coverage** | Coverage after adding the active Goal's Reserved snapshot to current Known vocabulary; with no Goal it equals current coverage. | Coverage when the condition is omitted |
| **On-arrival coverage** | Coverage from the vocabulary modeled as available when the learner reaches a Book in their chosen order. | Recommended route, guaranteed outcome |
| **Projected coverage** | A clearly labeled hypothetical result after a named vocabulary transition; on-arrival coverage is the Journey-specific form. | Coverage when the condition is omitted |
| **Coverage threshold** | A planning marker derived from token-weighted coverage, such as 95%, 97%, or 99%. | Difficulty score, readiness rank |
| **Evidence needs review** | Existing evidence is stale, questionable, or no longer safely comparable. | Low confidence as an unexplained score |
| **Not assessed** | Mouseion has no completed comparable analysis for this book. | 0% ready |
| **Cannot currently assess** | Mouseion lacks a supported source, language capability, or other prerequisite and should state which. | Unsupported with no explanation |

Always state whether a number is current, projected, token-weighted, scoped,
conditional, stale, or unavailable. A selected threshold is a planning aid, not
a literary judgment or claim that the learner can or cannot read a book.
Prepared decks and Goal snapshots select recurring vocabulary and make no
coverage claim; coverage thresholds remain internal whole-book planning data,
not Journey ordering inputs or a competing learner-facing destination.

**Language view** and **language corpus view** were the names used for the panel
proposed by [ADR 0042](../adr/0042-derived-language-corpus-view.md). That panel is
retired by [ADR 0057](../adr/0057-retire-language-view-panel.md), so neither term
names a current learner-facing surface. **Corpus** remains an internal analysis
artifact term: `CorpusID` and `normalized_corpus_artifacts` do not refer to the
learner's My Books collection.

## Status and feedback

Use complete, factual labels where space permits:

- Primary Goal;
- in Reading Journey;
- reading in progress;
- reading finished;
- vocabulary work in progress;
- vocabulary work complete;
- analysis stale or unavailable;
- ready to analyze;
- analysis queued, running, failed, cancelled, or result ready;
- catalog never synced, syncing, last synced, or sync failed;
- cover pending, cover unavailable, or no cover available when a visual placeholder must distinguish those states;
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
