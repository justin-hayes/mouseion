# Terminology

Status: **Canonical learner-facing design language.** Analysis terms follow the
shipped one-current-analysis contract in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) and the
reading-intent trigger in
[ADR 0049](../adr/0049-reading-intent-triggers-analysis.md). Current-reading,
disposition, and history language follows the shipped
[Reading workflow](../features/reading-workflow.md). Journey and Primary Goal
terms in older ADRs are historical; they must not become active learner-facing
navigation or plan labels.

Use these terms consistently in navigation, headings, actions, status messages,
future feature documents, and tests. Backend names may remain in code, APIs,
logs, and operational detail, but should not become primary learner-facing
language without a product reason.

## Principal learner-facing concepts

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **My Books** | Every Book the learner has chosen: acquired or metadata-only, assessed or unassessed, current, To Read or Read. It is a collection, not a task list or readiness ranking. | My Library, Dashboard, Corpus |
| **To Read** | The persisted disposition expressing that a chosen Book is a possible future reading, ensuring acquisition and analysis when needed. Current reading and Read are derived visible buckets; each Book appears in exactly one bucket. | Inbox (retired), Reading status, Journey position, Set Aside (retired), Hidden (retired) |
| **Reading** | The current Book with its vocabulary Browse, or, between Books, the neutral title-ordered chooser of To Read candidates, one ordinary entry per Book. Coverage bands describe current evidence; they are not recommendations. | Reading Journey, learning queue, plan, roadmap |
| **Current reading** | The one Book currently being read in the active study language, when one exists. Starting freezes a vocabulary snapshot; finishing records history and accepts eligible identities into modeled Known vocabulary. | Primary Goal, current project |
| **Choose what to read next** | The completion-receipt action that opens `/reading`. It presents To Read candidates without ranking or choosing automatically. | Where next?, Start next, continue plan, complete Journey |
| **Read / Read again** | Read is the visible bucket for a Book with reading history, not a persisted disposition. Read again returns it to To Read without deleting prior completions. | Read as a disposition; reread replaces history |

Reading intent is expressed through the To Read disposition. Current reading,
disposition, and reading history are independent facts; My Books derives one
visible bucket in precedence order: Currently reading, To Read, then Read when
history exists. Currently reading appears in the To Read tab with its own label;
Books with history appear in Read, while To Read Books stay in To Read. A learner may have no current Book and no To Read
Books; neither state implies failure or urgency.

**Reading Horizon** may remain an internal design metaphor for changing
possibility. **Campaign**, **milestone**, **destination**, and **Journey
completion** are not primary learner-facing concepts.

## Books and acquisition

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Catalog** | The authenticated destination at `/catalogs` where one search and the Language filter (its only language control) cover the built-in, read-only Classical catalog and each OPDS connection's local Catalog index as card sections, with one sync status line and **Sync now** per source. Connection setup lives on Manage sources. | Catalogs (the retired plural label), library, Browse (the vocabulary view), Import books, ingest books |
| **Manage sources** | The connection-management page at `/catalogs/sources` for adding, editing and deleting an OPDS connection, saving credentials, running Sync now, and recovering a failed sync. | Catalog settings, admin catalog |
| **Add** | Choose a Catalog entry for My Books. It creates or restores the Book as To Read, then acquires the current EPUB when needed and ensures whole-book analysis once. The control's copy says adding prepares the text for reading; the confirmation names the Book, keeps the Catalog's Language filter, and links to My Books, the same for every source. | Add to library, import, queue |
| **I've read this** | Record a Catalog entry the learner read outside Mouseion. It creates or restores the Book as Read with a **Previously read** record, downloads nothing and changes no Known vocabulary. It is offered on every Catalog card, OPDS or Classical, and once per Edition of a classical Work on the Work page; a classical Work is one Book, so there is no per-book control. | Mark as read, import as read |
| **Mark as previously read** | The My Books form of **I've read this**. It appears only on an unstarted To Read Book and moves that Book to Read. | Mark as read, complete book |
| **Move to To Read** | The reading-intent transition into the To Read disposition. Catalog **Add** is its entry point; **Read again** is its My Books form. It acquires the current EPUB when needed and ensures whole-book analysis once. | Start analysis, import and analyze, add to queue |
| **Catalog connection** | A learner-owned OPDS endpoint and credentials. | Global catalog, admin catalog |
| **Catalog sync** | Periodic, owner-scoped reconciliation that maintains each connection's Catalog index for offered non-English languages whose NLP pipelines are ready and follows title, author and cover changes onto already-chosen Books. It never creates a Book or membership, never implies EPUB content download, and never mirrors destructively; optional Book cover retrieval is independent metadata work. | Import all books, mirror, admin sync |
| **Catalog entry** | One book offered by a connection's Catalog index. It is browsable and addable but is not a Book until the learner chooses it. | Imported book, synced Book |
| **Metadata-only Book** | A Book the learner chose from the Catalog with active My Books membership and no validated EPUB source snapshot yet. | Imported book, acquired book, placeholder source |
| **Lazy content acquisition** | Download and validate EPUB content only after the learner expresses intent to use a metadata-only Book. Add or Read again triggers acquisition and analysis. | Sync download, automatic analysis |
| **Book preparation** | The durable work that makes a To Read Book readable. It has two kinds, **Classical preparation** and **EPUB preparation** (acquire, then analyze), and `/jobs` shows every preparation job in one Book preparations table: kind, Book, status, stage, code where one exists, progress where known, and created. An EPUB preparation appears as its acquisition row and its analysis row. | Download job, import, processing |
| **Queued / Running / Completed / Failed / Cancelled** | The one lifecycle vocabulary every `/jobs` row uses, for Book preparations and Catalog sync alike. | Processing, Syncing on `/jobs`, done |
| **Book** | The learner-facing bibliographic object, led by title and author and qualified by edition when evidence depends on it. | Source, corpus, artifact when referring to the book |
| **Book cover** | The optional catalog-supplied image representing a Book. It may lead visually in My Books and support identity in Reading, but title and author remain visible and authoritative. | Cover art, thumbnail when referring to the metadata |
| **Title plate** | The image Mouseion composes from a text's own title, Greek title and author where no Book cover exists, such as for every Classical Reading unit. Presentation only: never retained, never supplied by a catalog. | Generated cover, placeholder cover, title initial |
| **Source snapshot** | Immutable acquired EPUB bytes and extracted units, used when provenance matters. | Book version when no content revision is meant |

Mouseion's current web acquisition path is OPDS. Do not promise direct EPUB
upload unless a shipped route and feature contract support it. Reading intent —
expressed by Catalog **Add** or **Read again** — acquires
and validates content when needed as an ensure-once consequence. Historical
compatibility artifacts may retain **Add to My Books** or **Add to library**.

Connection sync uses complete factual states: **Never synced**, **Syncing**,
**Last synced**, and **Sync failed**. Always identify the connection and, for
last-synced or failed states, the relevant time or recovery. Do not use bare
**Active**, **Ready**, or **Updated** for sync state.

## Scope and analysis

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current analysis input** | The analyzed portion of the current extracted EPUB snapshot: its identified main text when declared EPUB structure provides one, otherwise the complete snapshot. Source revision and extracted-unit provenance remain durable internal facts. | Unscoped text, inferred content |
| **Stale analysis** | Existing evidence belongs to an older EPUB content revision. | Current evidence, failed Journey membership |
| **Analysis trigger** | Moving a Book to To Read submits asynchronous analysis or re-analysis as needed. | Start analysis, continue, process book |
| **Analysis run** | One durable queued/running/completed/failed/cancelled analysis run. | Job in primary learner-facing copy |
| **Current analysis evidence** | The Book's current completed evidence, owned by the Book and presented from Reading. Immutable runs and exact source/revision provenance remain backend and operational audit facts. | Completed analysis #N, latest result, analysis history on the learner surface |
| **Analysis evidence** | Current coverage and warning-only quality information used by Reading; internal thresholds and top-unknown data remain available to analysis services without a learner-facing detail page. | Dashboard metrics, difficulty score, text profile on the learner surface |
| **View in Reading** | Leave operational status and open the Book's canonical Reading anchor, directly or through the run-specific compatibility redirect. | View job, view exact result |
| **Not analysed yet** | Current acquired content has no completed analysis; a recovery action may accompany the note when available. | Not assessed as 0%, analysis failure |
| **Analysis failed** / **Content unavailable** | A factual current-evidence state. Keep its adjacent recovery action when one is available, and do not expose an action whose route or preconditions are invalid. | Dead retry link, failure framing for absent evidence |
| **Re-analysis queued** / **Re-analysis running** / **Re-analysis failed** / **Re-analysis cancelled** | A newer analysis run of the Book's unchanged revision, shown as secondary information. The **Current analysis evidence** stays in effect while its content still matches; keep **Retry analysis** adjacent to a failed or cancelled run. | Analysis failed or Analysis running for a Book whose published evidence is still current |

Use **job** only for operational history or implementation-facing detail. A
Book's learner-facing state may be **ready to analyze**, **analysis queued**,
**analysis running**, **analysis result ready**, **stale**, or **unavailable**
even when backend state is expressed differently. **Book preparations** and
**Catalog sync** are the two operational tables on `GET /jobs`, not
learner-facing Book-page sections; a failed preparation row states the same
stage and code My Books explains for that Book. Run-specific analysis URLs
remain only as compatibility redirects to the Reading anchor.

## Reading choice and evidence

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Coverage band** | Classification of a To Read Book's current lexical coverage. Each To Read Book shows its band in its own margin; a band provides no recommendation or predicted outcome. | Difficulty, readiness, best next book |
| **Work position** | A constituent Book's plain bibliographic place, such as "Book 2 of 24", from the reviewed definition rather than the learner's added Books. A whole-work unit has none, and it never becomes a pointer to the next Book. | Next Book, recommended order |
| **Move to To Read** | Express reading intent from My Books: **Read again** returns a Read Book to To Read. The action also ensures current content and analysis when needed. | Add to Journey, queue, schedule |
| **Start reading** | Make an eligible To Read Book the current Book and freeze its vocabulary snapshot. Requires explicit confirmation. | Choose as Primary Goal |
| **Switch current reading** | Replace the current Book with an eligible To Read Book after explicit confirmation. The former current Book remains To Read. | Reorder Journey |
| **Mark reading finished** | Record the completion and exact modeled vocabulary transition. It does not choose the next Book. | Complete Goal, finish Journey |
| **Read again** | Return a previously read Book to To Read while retaining completion history. | Clear history, erase prior completion |

The chooser's document order is not a learner-authored order or recommendation.
Never use coverage as a warning, readiness judgment, or literary evaluation.

## Reading, preparation, and vocabulary

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Reading in progress** | The learner has recorded that they are reading the book. | Learning in progress when only reading is meant |
| **Reading finished** | The learner has recorded finishing the book. This does not imply vocabulary knowledge. | Completed when the completed fact is unclear |
| **Vocabulary work in progress** | Preparation or review activity remains incomplete. | Nearly mastered |
| **Vocabulary work complete** | Eligible identities from the current-reading snapshot have entered modeled Known vocabulary. This does not claim verified mastery. | Mastered, deck reviewed |
| **Known vocabulary** | Modeled learner knowledge: identities accepted when a current reading is finished. It does not claim verified mastery. | Generated vocabulary, mastered vocabulary |
| **Reserved vocabulary** | Lemmas in the current Book's frozen snapshot, excluded from selection in that study language but not counted as Known. | Known, learned, studied vocabulary |
| **Reading vocabulary snapshot** | The exact Book vocabulary candidate set frozen when Current reading starts, with analysis, source, cross-Book selection, and learner-state provenance. | Deck contents, generated vocabulary |
| **Generated vocabulary** | Immutable provenance that an identity was assigned to a prepared deck; it is neither Known nor Reserved and is not a later selection exclusion. | Known vocabulary, reserved vocabulary |
| **Graduated vocabulary** | Historical provenance label for vocabulary accepted into modeled Known vocabulary when current reading is finished. | Automatically mastered |
| **Unknown vocabulary** | Eligible analyzed lemmas not currently Known or Reserved by current reading in that study language. | Difficult words |
| **Recurring vocabulary** | Unknown lemmas appearing at least N times in the analyzed Book (default three); one route into Book vocabulary candidates. | Rare words, difficult words |
| **Book vocabulary candidates** | Unknown identities selected for a Current reading's frozen snapshot and any Book deck prepared from it: recurring in this Book, or appearing twice here and frequently across the learner's currently analyzed same-language Books. | Recurring vocabulary for the entire pool, corpus words |

Do not use **mastered** as a synonym for generated, assigned, exported, merely
reviewed, or encountered while reading. Reading history, preparation state,
and vocabulary knowledge remain independent facts.

### Accepted Vocabulary terminology (partially shipped)

| Term | Meaning | Avoid |
|---|---|---|
| **Browse** | Working-desk view on Reading of owner-specific effective lemma + POS identities with current analyzed evidence in the active language's Current reading Book, including singletons; frequency-ranked and excluding Known, Reserved, and already prepared-for-this-Book identities by default. | Book prepared-deck candidates, Known-vocabulary list, cross-Book inventory |
| **Concordance** | Occurrence exploration with both sentence/KWIC context and a focused, morphology-first sentence study view backed by analyzer-attributed evidence; effective lookup remains distinct from observed surface and analyzer-evidence lookup. | KWIC as the whole feature, global correction |
| **Sentence study** | The focused **Study this sentence** view reached from a Concordance occurrence: the complete sentence, a source-word-aligned morphology line with a notation key, and a named complete text alternative. | Analyzer-attributed token list, dependency chart, Gloss |

These terms follow the [current Vocabulary specification](../features/vocabulary-browse-and-concordance.md)
and [Reading-owned Book vocabulary](../features/reading-owned-book-vocabulary.md).
Browse selection and Custom decks exist in the shipped implementation but are
retired in the accepted target; do not use them for new learner-facing copy.
Exact surface/effective/analyzer lookup, scoped grammar exploration, KWIC
context, and focused, morphology-first sentence study are available in
Concordance. Browse's table labels each identity Unknown, Known, or Reserved
(both if applicable);
when include-all reveals an identity prepared for this Book, **In a Book deck**
appears as a separate note, not a learner-knowledge state. Generation never
makes vocabulary Known.

Current-reading completion records durable history and accepts eligible frozen
snapshot identities into modeled Known vocabulary as one idempotent transition.
Deck readiness or review is not required, and the interface must not imply
verified mastery. Earlier Goal-specific behavior in ADR 0072 is historical.

## Coverage and projection

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current coverage** | Coverage from current Known vocabulary under the accepted metric contract. | Reading level |
| **Coverage band** | A margin label on one To Read candidate, based on current token-weighted lexical coverage. It compares evidence only and does not rank or recommend candidates. | Difficulty score, readiness rank |
| **Projected coverage** | Historical term for a hypothetical result after a named vocabulary transition; not presented in current Reading. | Predicted outcome, recommended route |
| **Evidence needs review** | Existing evidence is stale, questionable, or no longer safely comparable. | Low confidence as an unexplained score |
| **Not assessed** | Mouseion has no completed comparable analysis for this book. | 0% ready |
| **Cannot currently assess** | Mouseion lacks a supported source, language capability, or other prerequisite and should state which. | Unsupported with no explanation |

Always state whether a number is current, token-weighted, scoped, stale, or
unavailable. Coverage bands are evidence labels, not a literary judgment or a
claim that the learner can or cannot read a book. Prepared decks and reading
snapshots select recurring vocabulary and make no coverage claim.

My Books may annotate a Book with current analysis state, current token-weighted
Known coverage and the gap to the next coverage band, or the latest completion
date for a Read Book. Absent coverage is neutral, never `0%`. Deck state belongs
in this margin only for the current-reading Book. These notes report evidence;
they do not imply readiness, recommendation, or vocabulary mastery.

**Language view** and **language corpus view** were the names used for the panel
proposed by [ADR 0042](../adr/0042-derived-language-corpus-view.md). That panel is
retired by [ADR 0057](../adr/0057-retire-language-view-panel.md), so neither term
names a current learner-facing surface. **Corpus** remains an internal analysis
artifact term: `CorpusID` and `normalized_corpus_artifacts` do not refer to the
learner's My Books collection.

## Status and feedback

Use complete, factual labels where space permits:

- current reading;
- To Read and Read as mutually exclusive visible buckets;
- reading history, including prior completions on current or To Read Books;
- reading in progress;
- reading finished;
- vocabulary work in progress;
- vocabulary work complete;
- analysis stale or unavailable;
- ready to analyze;
- analysis queued, running, failed, cancelled, or result ready;
- book preparation queued, running, completed, failed, or cancelled;
- catalog never synced, syncing, last synced, or sync failed;
- cover pending, cover unavailable, or no cover available when a visual placeholder must distinguish those states;
- deck preparing or deck ready;
- evidence needs review;
- not assessed;
- cannot currently assess.

Avoid ambiguous pills such as **Reading**, **Completed**, **Ready**, or **Active**
without naming the resource or fact. Current reading and a Book's disposition
are separate facts.

Use **error** for a failed request or operation, **warning** for risk or degraded
quality that does not block all progress, **notice** for neutral contextual
information, and **success** for a completed learner action. Messages say what
happened, what remained unchanged when material, and what the learner can do
next.
