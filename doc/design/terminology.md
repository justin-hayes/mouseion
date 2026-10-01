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
| **My Books** | Every book Mouseion knows about for the learner: acquired or metadata-only, assessed or unassessed, desired or not, current or distant. It is a collection, not a task list or readiness ranking. | My Library, Dashboard, Corpus |
| **Inbox / To Read / Set Aside** | Persisted Book dispositions: Inbox is untriaged, To Read expresses reading intent and ensures acquisition/analysis when needed, and Set Aside is not currently intended. Current reading and Read are derived visible buckets; each Book appears in exactly one bucket. | Reading status, Journey position |
| **Reading** | The current Book or, between Books, the unordered chooser of To Read candidates. Coverage bands describe current evidence; they are not recommendations. | Reading Journey, learning queue, plan, roadmap |
| **Current reading** | The one Book currently being read in the active study language, when one exists. Starting freezes a vocabulary snapshot; finishing records history and accepts eligible identities into modeled Known vocabulary. | Primary Goal, current project |
| **Choose what to read next** | The completion-receipt action that opens `/reading`. It presents To Read candidates without ranking or choosing automatically. | Where next?, Start next, continue plan, complete Journey |
| **Read / Read again** | Read is the visible bucket for an Inbox or Set Aside Book with reading history, not a persisted disposition. Read again returns it to To Read without deleting prior completions. | Read as a disposition; reread replaces history |

Reading intent is expressed through the To Read disposition. Current reading,
disposition, and reading history are independent facts; My Books derives one
visible bucket in precedence order: Currently reading, To Read, Read when
history exists, Inbox, then Set Aside. Currently reading appears in the To Read
tab with its own label; historical Inbox Books appear in Read, while historical
To Read Books stay in To Read. A learner may have no current Book and no To Read
Books; neither state implies failure or urgency.

**Reading Horizon** may remain an internal design metaphor for changing
possibility. **Campaign**, **milestone**, **destination**, and **Journey
completion** are not primary learner-facing concepts.

## Books and acquisition

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Catalogs** | The authenticated catalogue setup and sync-maintenance destination at `/catalogs`. | Import books, ingest books |
| **Move to To Read** | Express reading intent for a My Books Book. It acquires the current EPUB when needed and ensures whole-book analysis once. | Start analysis, import and analyze, add to queue |
| **Catalog connection** | A learner-owned OPDS endpoint and credentials. | Global catalog, admin catalog |
| **Catalog sync** | Periodic, owner-scoped reconciliation that adds or updates bibliographic metadata for offered non-English languages whose NLP pipelines are ready. The resulting chosen-language Books derive study languages. It never implies EPUB content download or destructive mirroring; optional Book cover retrieval is independent metadata work. | Import all books, mirror, admin sync |
| **Metadata-only catalog entry** | A Book and active My Books membership recorded from catalog metadata, with no validated EPUB source snapshot yet. | Imported book, acquired book, placeholder source |
| **Lazy content acquisition** | Download and validate EPUB content only after the learner expresses intent to use a metadata-only Book. Moving a Book to To Read triggers acquisition and analysis. | Sync download, automatic analysis |
| **Book** | The learner-facing bibliographic object, led by title and author and qualified by edition when evidence depends on it. | Source, corpus, artifact when referring to the book |
| **Book cover** | The optional catalog-supplied image representing a Book. It may lead visually in My Books and support identity in Reading, but title and author remain visible and authoritative. | Cover art, thumbnail when referring to the metadata |
| **Source snapshot** | Immutable acquired EPUB bytes and extracted units, used when provenance matters. | Book version when no content revision is meant |

Mouseion's current web acquisition path is OPDS. Do not promise direct EPUB
upload unless a shipped route and feature contract support it. **Move to To Read**
acquires and validates content when needed as an ensure-once consequence of
reading intent. Historical compatibility artifacts may retain
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
| **Analysis trigger** | Moving a Book to To Read submits asynchronous analysis or re-analysis as needed. | Start analysis, continue, process book |
| **Analysis run** | One durable queued/running/completed/failed/cancelled analysis attempt. | Job in primary learner-facing copy |
| **Current analysis evidence** | The Book's current completed evidence, owned by the Book and presented from Reading. Immutable runs and exact source/revision provenance remain backend and operational audit facts. | Completed analysis #N, latest result, analysis history on the learner surface |
| **Analysis evidence** | Current coverage and warning-only quality information used by Reading; internal thresholds and top-unknown data remain available to analysis services without a learner-facing detail page. | Dashboard metrics, difficulty score, text profile on the learner surface |
| **View in Reading** | Leave operational status and open the Book's canonical Reading anchor, directly or through the run-specific compatibility redirect. | View job, view exact result |

Use **job** only for operational history or implementation-facing detail. A
Book's learner-facing state may be **ready to analyze**, **analysis queued**,
**analysis running**, **analysis result ready**, **stale**, or **unavailable**
even when backend state is expressed differently. **Analysis history** is
operational language for `GET /jobs`, not a learner-facing Book-page section.
Run-specific analysis URLs remain only as compatibility redirects to the Reading
anchor.

## Reading choice and evidence

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Coverage band** | Grouping of To Read candidates by current lexical coverage; it provides no recommendation or predicted outcome. | Difficulty, readiness, best next book |
| **Move to To Read** | Express reading intent from My Books. The action also ensures current content and analysis when needed. | Add to Journey, queue, schedule |
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
| **Known vocabulary** | Modeled learner knowledge: lemmas explicitly imported or accepted when current reading is finished. It does not claim verified mastery. | Generated vocabulary, mastered vocabulary |
| **Reserved vocabulary** | Lemmas in the current Book's frozen snapshot, excluded from selection in that study language but not counted as Known. | Known, learned, studied vocabulary |
| **Reading vocabulary snapshot** | The exact recurring-vocabulary identity set frozen from the current Book's analysis, with analysis, source, and selection provenance. | Deck contents, generated vocabulary |
| **Generated vocabulary** | Immutable provenance that an identity was assigned to a prepared deck; it is neither Known nor Reserved and is not a later selection exclusion. | Known vocabulary, reserved vocabulary |
| **Graduated vocabulary** | Historical provenance label for vocabulary accepted into modeled Known vocabulary when current reading is finished. | Automatically mastered |
| **Unknown vocabulary** | Eligible analyzed lemmas not currently Known or Reserved by current reading in that study language. | Difficult words |
| **Recurring vocabulary** | Unknown lemmas appearing at least N times in the analyzed book; the pool a prepared deck selects, labeled **Deck vocabulary** in preparation. | Rare words, difficult words |

Do not use **mastered** as a synonym for generated, assigned, exported, merely
reviewed, or encountered while reading. Reading history, preparation state,
and vocabulary knowledge remain independent facts.

### Accepted Vocabulary terminology (partially shipped)

| Term | Meaning | Avoid |
|---|---|---|
| **Browse** | Vocabulary view of owner-specific effective lemma + POS identities with current analyzed evidence across active-language Books, including singletons. | Book prepared-deck candidates, Known-vocabulary list |
| **Browse selection** | Recoverable unnamed set of effective identities awaiting review and explicit naming; scoped to one learner and study language. | Custom deck before creation, filtered results |
| **Concordance** | Occurrence exploration with both sentence/KWIC context and analyzer-attributed syntax; effective lookup remains distinct from observed surface and analyzer-evidence lookup. | KWIC as the whole feature, global correction |
| **Custom deck** | Learner-named editable cross-Book identity selection with independent frozen preparations, not a Reading snapshot or Book Prepared deck. | Reading deck, Custom Prepared deck |

These are accepted terms from the [Vocabulary feature
specification](../features/vocabulary-browse-concordance-and-custom-decks.md).
Browse is available; exact surface/effective/analyzer lookup and KWIC context are
available in the initial Concordance slice. Browse selection, focused sentence
study, and Custom decks remain target terms for unshipped workflows. Known,
Reserved, and Generated remain separate facts in Browse and on Custom decks.

Current-reading completion records durable history and accepts eligible frozen
snapshot identities into modeled Known vocabulary as one idempotent transition.
Deck readiness or review is not required, and the interface must not imply
verified mastery. Earlier Goal-specific behavior in ADR 0072 is historical.

## Coverage and projection

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current coverage** | Coverage from current Known vocabulary under the accepted metric contract. | Reading level |
| **Coverage band** | A group in the between-Books chooser based on current token-weighted lexical coverage. It compares evidence only and does not rank or recommend candidates. | Difficulty score, readiness rank |
| **Projected coverage** | Historical term for a hypothetical result after a named vocabulary transition; not presented in current Reading. | Predicted outcome, recommended route |
| **Evidence needs review** | Existing evidence is stale, questionable, or no longer safely comparable. | Low confidence as an unexplained score |
| **Not assessed** | Mouseion has no completed comparable analysis for this book. | 0% ready |
| **Cannot currently assess** | Mouseion lacks a supported source, language capability, or other prerequisite and should state which. | Unsupported with no explanation |

Always state whether a number is current, token-weighted, scoped, stale, or
unavailable. Coverage bands are evidence labels, not a literary judgment or a
claim that the learner can or cannot read a book. Prepared decks and reading
snapshots select recurring vocabulary and make no coverage claim.

**Language view** and **language corpus view** were the names used for the panel
proposed by [ADR 0042](../adr/0042-derived-language-corpus-view.md). That panel is
retired by [ADR 0057](../adr/0057-retire-language-view-panel.md), so neither term
names a current learner-facing surface. **Corpus** remains an internal analysis
artifact term: `CorpusID` and `normalized_corpus_artifacts` do not refer to the
learner's My Books collection.

## Status and feedback

Use complete, factual labels where space permits:

- current reading;
- Inbox, To Read, Read, and Set Aside as mutually exclusive visible buckets;
- reading history, including prior completions on current or To Read Books;
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
without naming the resource or fact. Current reading and a Book's disposition
are separate facts.

Use **error** for a failed request or operation, **warning** for risk or degraded
quality that does not block all progress, **notice** for neutral contextual
information, and **success** for a completed learner action. Messages say what
happened, what remained unchanged when material, and what the learner can do
next.
