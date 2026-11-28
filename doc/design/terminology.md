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
| **Reading** | Every Book the learner has chosen, on one page per study language: the Current reading hero at the top, then To Read, then Read. It is a collection, not a task list or readiness ranking. A Book that needs action shows a status line. | My Books, My Library, Dashboard |
| **To Read** | The persisted disposition expressing that a chosen Book is a possible future reading, ensuring acquisition and analysis when needed. Current reading and Read are derived visible buckets; each Book appears in exactly one bucket. It is in title order unless the learner chooses the optional **most readable** sort by Mature coverage. | Inbox (retired), Reading status, Journey position, Set Aside (retired), Hidden (retired) |
| **Reading** (earlier page) | A separate Reading page that held the Current reading was retired by ADR 0136; Reading (formerly My Books) holds what it held, and `/reading?reading={bookID}` opens that Book's detail page. "Current reading" remains the role a Book has. | Reading Journey, learning queue, plan, roadmap |
| **Current reading** | The one Book currently being read in a study language, when one exists. It puts its words first among new words in Study until it reaches Target coverage. Finishing records history and changes no vocabulary. | Primary Goal, current project |
| **Read next** | The single action on a To Read Book that makes it the Current reading, replacing any other, with no confirmation. It is refused only when the Book is not analysed or has no Book language. | Start, Switch, Choose what to read next, Where next? |
| **Read / Read again** | Read is the visible bucket for a Book with reading history, not a persisted disposition. Read again returns it to To Read without deleting prior completions. | Read as a disposition; reread replaces history |

Reading intent is expressed through the To Read disposition. Current reading,
disposition, and reading history are independent facts; Reading derives one
visible bucket in precedence order: Currently reading, To Read, then Read when
history exists. The Current reading is the hero at the top and is also listed under To Read with its own label;
Books with history appear in Read, while To Read Books stay in To Read. A learner may have no current Book and no To Read
Books; neither state implies failure or urgency.

**Reading Horizon** may remain an internal design metaphor for changing
possibility. **Campaign**, **milestone**, **destination**, and **Journey
completion** are not primary learner-facing concepts.

## Study reports

Owned by [ADR 0135](../adr/0135-study-reports.md).

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Report** | The control on a revealed Study card that says the card's material is wrong. A reported review is not graded. | Flag, Mark as wrong, Bury |
| **Wrong word** | The report kind for an occurrence that belongs to another word. The word is held and gets no reviews until its occurrence is repaired on the report's remediation screen. | Bad lemma, Error |
| **Unsuitable context** | The report kind for a sentence that does not teach its word, for example inside an Idiom. The sentence is never used for that word again and the word comes back on the next Study day from another one. | Bad sentence, Skip |
| **Reported words** | The page listing the words held by open Wrong-word reports. | Held words (as a destination), Reports |
| **Held** | A word an open Wrong-word report keeps out of Today; counted in plain words ("1 word is held because you reported it"). | Suspended, Paused |
| **Undo** | Takes back an Unsuitable-context report while its card is still shown. **Next** moves on and ends the offer. | Revert, Cancel report |

## Books and acquisition

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Catalog** | The authenticated destination at `/catalogs` where one search and the Language filter (its only language control) cover the built-in, read-only Classical catalog and each OPDS connection's local Catalog index as card sections, with one sync status line and **Sync now** per source. Connection setup lives on Manage sources. | Catalogs (the retired plural label), library, Browse (the vocabulary view), Import books, ingest books |
| **Manage sources** | The connection-management page at `/catalogs/sources` for adding, editing and deleting an OPDS connection, saving credentials, running Sync now, and recovering a failed sync. | Catalog settings, admin catalog |
| **Add** | Choose a Catalog entry for Reading. It creates or restores the Book as To Read, then acquires the current EPUB when needed and ensures whole-book analysis once. The control's copy says adding prepares the text for reading; the confirmation names the Book, keeps the Catalog's Language filter, and links to Reading, the same for every source. | Add to library, import, queue |
| **I've read this** | Record a Catalog entry the learner read outside Mouseion. It creates or restores the Book as Read with a **Previously read** record, downloads nothing and changes no vocabulary. It is offered on every Catalog card, OPDS or Classical, and once per Edition of a classical Work on the Work page; a classical Work is one Book, so there is no per-book control. | Mark as read, import as read |
| **Mark as previously read** | The Reading form of **I've read this**. It appears only on an unstarted To Read Book and moves that Book to Read. | Mark as read, complete book |
| **Move to To Read** | The reading-intent transition into the To Read disposition. Catalog **Add** is its entry point; **Read again** is its Reading form. It acquires the current EPUB when needed and ensures whole-book analysis once. | Start analysis, import and analyze, add to queue |
| **Catalog connection** | A learner-owned OPDS endpoint and credentials. | Global catalog, admin catalog |
| **Catalog sync** | Periodic, owner-scoped reconciliation that maintains each connection's Catalog index for offered non-English languages whose NLP pipelines are ready and follows title, author and cover changes onto already-chosen Books. It never creates a Book or membership, never implies EPUB content download, and never mirrors destructively; optional Book cover retrieval is independent metadata work. | Import all books, mirror, admin sync |
| **Catalog entry** | One book offered by a connection's Catalog index. It is browsable and addable but is not a Book until the learner chooses it. | Imported book, synced Book |
| **Metadata-only Book** | A Book the learner chose from the Catalog with active Reading membership and no validated EPUB source snapshot yet. | Imported book, acquired book, placeholder source |
| **Lazy content acquisition** | Download and validate EPUB content only after the learner expresses intent to use a metadata-only Book. Add or Read again triggers acquisition and analysis. | Sync download, automatic analysis |
| **Book preparation** | The durable work that makes a To Read Book readable. It has two kinds, **Classical preparation** and **EPUB preparation** (acquire, then analyze), and `/jobs` shows every preparation job in one Book preparations table: kind, Book, status, stage, code where one exists, progress where known, and created. An EPUB preparation appears as its acquisition row and its analysis row. | Download job, import, processing |
| **Queued / Running / Completed / Failed / Cancelled** | The one lifecycle vocabulary every `/jobs` row uses, for Book preparations and Catalog sync alike. | Processing, Syncing on `/jobs`, done |
| **Book** | The learner-facing bibliographic object, led by title and author and qualified by edition when evidence depends on it. | Source, corpus, artifact when referring to the book |
| **Book cover** | The optional catalog-supplied image representing a Book. It may lead visually in Reading and support identity on the Book detail page, but title and author remain visible and authoritative. | Cover art, thumbnail when referring to the metadata |
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
| **Current analysis evidence** | The Book's current completed evidence, owned by the Book and presented from Reading and the Book detail page. Immutable runs and exact source/revision provenance remain backend and operational audit facts. | Completed analysis #N, latest result, analysis history on the learner surface |
| **Analysis evidence** | Current coverage and warning-only quality information used by Reading; internal thresholds and top-unknown data remain available to analysis services without a learner-facing detail page. | Dashboard metrics, difficulty score, text profile on the learner surface |
| **View in Reading** | Leave operational status and open the Book in Reading, directly or through the run-specific compatibility redirect. | View job, view exact result |
| **Not analysed yet** | Current acquired content has no completed analysis; a recovery action may accompany the note when available. | Not assessed as 0%, analysis failure |
| **Analysis failed** / **Content unavailable** | A factual current-evidence state. Keep its adjacent recovery action when one is available, and do not expose an action whose route or preconditions are invalid. | Dead retry link, failure framing for absent evidence |
| **Re-analysis queued** / **Re-analysis running** / **Re-analysis failed** / **Re-analysis cancelled** | A newer analysis run of the Book's unchanged revision, shown as secondary information. The **Current analysis evidence** stays in effect while its content still matches; keep **Retry analysis** adjacent to a failed or cancelled run. | Analysis failed or Analysis running for a Book whose published evidence is still current |

Use **job** only for operational history or implementation-facing detail. A
Book's learner-facing state may be **ready to analyze**, **analysis queued**,
**analysis running**, **analysis result ready**, **stale**, or **unavailable**
even when backend state is expressed differently. **Book preparations** and
**Catalog sync** are the two operational tables on `GET /jobs`, not
learner-facing Book-page sections; a failed preparation row states the same
stage and code Reading explains for that Book. Run-specific analysis URLs
remain only as compatibility redirects to Reading.

## Choosing what to read, and evidence

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Coverage band** | Classification of a To Read Book's Mature coverage. Each To Read Book shows its band in its own margin; a band provides no recommendation or predicted outcome. | Difficulty, readiness, best next book |
| **Work position** | A constituent Book's plain bibliographic place, such as "Book 2 of 24", from the reviewed definition rather than the learner's added Books. A whole-work unit has none, and it never becomes a pointer to the next Book. | Next Book, recommended order |
| **Move to To Read** | Express reading intent from Reading: **Read again** returns a Read Book to To Read. The action also ensures current content and analysis when needed. | Add to Journey, queue, schedule |
| **Read next** | Make an eligible To Read Book the current Book in one step, with no confirmation and nothing frozen. When another Book is current, it is replaced and stays To Read. | Start reading, Switch current reading, Choose as Primary Goal |
| **Finish** / **End** | **Finish** records a Reading completion, clears the current role and To Read, and changes no vocabulary. **End** clears the current role and keeps the Book in To Read. Neither takes a confirmation and neither chooses the next Book. | Mark reading finished, Complete Goal, finish Journey |
| **Read again** | Return a previously read Book to To Read while retaining completion history. | Clear history, erase prior completion |

Reading's document order is not a learner-authored order or recommendation.
Never use coverage as a warning, readiness judgment, or literary evaluation.

## Reading, preparation, and vocabulary

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Reading in progress** | The learner has recorded that they are reading the book. | Learning in progress when only reading is meant |
| **Reading finished** | The learner has recorded finishing the book. This does not imply vocabulary knowledge. | Completed when the completed fact is unclear |
| **Vocabulary work in progress** | Study has words in study that are not yet Mature. | Nearly mastered |
| **Mature** | A word in study whose review interval is at least 21 days. The one claim of knowledge; coverage counts Mature words. | Known, mastered |
| **In study** | A word Study has introduced and that is not yet Mature. | Learning, enrolled |
| **Not yet introduced** | A word in a Book that Study has not introduced. | Unknown, new |
| **Already known** | The answer a learner can give when Study introduces a word. It enters study Mature, and from then on is ordinary. An answer, not a status. | Known, skip, mark as known |
| **Corpus** | The analysed Books in Reading in one study language. Study and Concordance draw on all of it. | Library, collection |
| **Target coverage** | The 98% coverage, counting Mature and in-study words, up to which the Current reading's words come first among new words. | Goal, readiness |

Known, Reserved, Unknown and Generated vocabulary, the reading vocabulary
snapshot, Graduated vocabulary, Recurring vocabulary and Book vocabulary
candidates are retired ([ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md));
do not use them in learner-facing copy.

Do not use **mastered** as a synonym for merely reviewed or encountered while
reading. Reading history and vocabulary
knowledge remain independent facts.

### Vocabulary terminology

| Term | Meaning | Avoid |
|---|---|---|
| **Browse** | The read-only **Words** list on a Book's detail page: owner-specific effective lemma + POS identities with current analyzed evidence in that Book, including singletons, frequency-ranked, each with its Study state (**Mature**, **In study** or **Not yet introduced**). | Book prepared-deck candidates, Known-vocabulary list, cross-Book inventory |
| **Concordance** | Occurrence exploration with both sentence/KWIC context and a focused, morphology-first sentence study view backed by analyzer-attributed evidence; effective lookup remains distinct from observed surface and analyzer-evidence lookup. | KWIC as the whole feature, global correction |
| **Sentence study** | The focused **Study this sentence** view reached from a Concordance occurrence: the complete sentence, a source-word-aligned morphology line with a notation key, and a named complete text alternative. | Analyzer-attributed token list, dependency chart, Gloss |

These terms follow the [current Vocabulary specification](../features/vocabulary-browse-and-concordance.md).
Browse selection and Custom decks are retired; do not use them for new
learner-facing copy. Exact surface/effective/analyzer lookup, scoped grammar
exploration, KWIC context, and focused, morphology-first sentence study are
available in Concordance. Browse never shows Known, Reserved or Unknown, and it
edits nothing: a fix starts only from a Wrong-word Study report.

Finishing a Book records durable history and nothing else. It never changes
vocabulary, and the interface must not imply verified mastery. Earlier
Goal-specific behavior in ADR 0072 is historical.

## Coverage and projection

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Current coverage** | The share of a Book's analyzable tokens whose word is Mature, with in-study coverage as a lighter second figure, under the accepted metric contract. | Reading level |
| **Coverage band** | A margin label on one To Read candidate, based on current token-weighted lexical coverage. It compares evidence only and does not rank or recommend candidates. | Difficulty score, readiness rank |
| **Projected coverage** | Historical term for a hypothetical result after a named vocabulary transition; not presented in Reading. | Predicted outcome, recommended route |
| **Evidence needs review** | Existing evidence is stale, questionable, or no longer safely comparable. | Low confidence as an unexplained score |
| **Not assessed** | Mouseion has no completed comparable analysis for this book. | 0% ready |
| **Cannot currently assess** | Mouseion lacks a supported source, language capability, or other prerequisite and should state which. | Unsupported with no explanation |

Always state whether a number is current, token-weighted, scoped, stale, or
unavailable. Coverage bands are evidence labels, not a literary judgment or a
claim that the learner can or cannot read a book. Study orders new words by Target coverage; selection makes no other coverage claim.

Reading may annotate a Book with current analysis state, current token-weighted
Mature coverage and the gap to the next coverage band, or the latest completion
date for a Read Book. Absent coverage is neutral, never `0%`. The Current reading hero also shows its in-study coverage and roughly how many words are left to introduce before Target coverage. These notes report evidence;
they do not imply readiness, recommendation, or vocabulary mastery.

**Language view** and **language corpus view** were the names used for the panel
proposed by [ADR 0042](../adr/0042-derived-language-corpus-view.md). That panel is
retired by [ADR 0057](../adr/0057-retire-language-view-panel.md), so neither term
names a current learner-facing surface. **Corpus** now also names the learner's analysed Books in a study
language; the internal `CorpusID` and `normalized_corpus_artifacts` are analysis
artifacts, not that collection.

## Status and feedback

Use complete, factual labels where space permits:

- current reading;
- To Read and Read as mutually exclusive visible buckets;
- reading history, including prior completions on current or To Read Books;
- reading in progress;
- reading finished;
- vocabulary work in progress;
- analysis stale or unavailable;
- ready to analyze;
- analysis queued, running, failed, cancelled, or result ready;
- book preparation queued, running, completed, failed, or cancelled;
- catalog never synced, syncing, last synced, or sync failed;
- cover pending, cover unavailable, or no cover available when a visual placeholder must distinguish those states;
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
