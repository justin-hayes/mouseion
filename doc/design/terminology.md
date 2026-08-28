# Terminology

Use these terms consistently in navigation, headings, actions, status messages,
feature documents, and tests. Backend names may remain in code, APIs, logs, and
operational detail, but should not become the primary learner-facing language
without a product reason.

## Books and acquisition

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Add books** | Navigation/action that enters catalog setup and browsing. | Import books, ingest books |
| **Add to library** | Acquire and validate an EPUB from OPDS without starting analysis. | Import and analyze, analyze now |
| **My Library** | The learner's owned books and their current lifecycle state. | Dashboard, corpora |
| **Catalog connection** | A learner-owned OPDS endpoint and credentials. | Global catalog, admin catalog |
| **Book** | The learner-facing bibliographic object. | Source, corpus, artifact when referring to the book |
| **Source snapshot** | Immutable acquired EPUB bytes and extracted units, used when provenance matters. | Book version when no content revision is meant |

Mouseion's current web acquisition path is OPDS. Do not promise direct EPUB
upload unless a shipped route and feature contract support it.

## Scope and analysis

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Scope review** | Review extracted units and decide what should be analyzed. | Preprocessing, import review |
| **Recommended scope** | The classifier's explainable starting selection. It is not automatic approval. | Smart scope, correct scope |
| **Confirmed scope** | An immutable learner-confirmed scope revision. | Current selection when historical identity matters |
| **Start analysis** | Explicitly submit one confirmed scope for asynchronous analysis. | Continue, process book |
| **Analysis run** | One durable queued/running/completed/failed/cancelled analysis attempt. | Job in primary learner-facing copy |
| **Analysis result** | One immutable completed analysis and its exact source/scope provenance. | Latest data when identity matters |
| **Analysis insights** | Coverage, threshold, structural, quality, and unknown-vocabulary information for one completed analysis. | Dashboard metrics, difficulty score |
| **View analysis result** | Leave operational status and open the exact completed, book-centered result. | View job, latest analysis |

Use **job** only for operational history or implementation-facing detail. A
book's learner-facing state may be “scope review required,” “ready to analyze,”
or “analysis result ready” even when the backend state is expressed differently.

## Decks and learning

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Prepare deck** | Start asynchronous creation of an APKG from a completed analysis. | Generate cards when referring to the whole workflow |
| **Prepared deck** | The immutable ready APKG artifact and its preparation record. | Export job |
| **Download deck** | Retrieve an already prepared APKG without changing product state. | Generate deck |
| **Learning campaign** | One prepared deck and its source book moving through the reading-and-study workflow. | Project, session |
| **Learning queue** | Campaigns waiting behind the single active campaign. | Backlog |
| **Active campaign** | The one campaign currently reserved for reading and study. | Current deck when the book is also part of the state |
| **Complete campaign** | Both the book is finished and the deck has been reviewed; assigned vocabulary graduates to known. | Finish deck when describing the combined transition |
| **Abandon campaign** | Stop the campaign and make its assigned vocabulary eligible again unless independently known. | Delete campaign |

Do not use **mastered** as a synonym for generated, assigned, exported, or merely
reviewed. Mouseion records explicit known vocabulary and campaign graduation; it
does not implement a mastery model.

When the second progress action completes a campaign, prefer an outcome-based
label such as **Complete campaign and add 132 lemmas to known vocabulary**. A
generic **Mark complete** label hides the consequential vocabulary transition.

Use **Remove study language** only for deleting a learner preference. Copy must
state that this does not delete books or known vocabulary for that language.

## Vocabulary and coverage

| Canonical term | Meaning and usage | Avoid |
|---|---|---|
| **Known vocabulary** | Lemmas explicitly imported/marked known or graduated by a completed campaign. | Generated vocabulary, mastered vocabulary |
| **Active-campaign vocabulary** | Lemmas reserved by the active campaign but not counted as known. | Known, learned |
| **Generated vocabulary** | Immutable provenance that a lemma was assigned to a deck. | Known vocabulary |
| **Graduated vocabulary** | Vocabulary promoted to known when its campaign completes. | Automatically mastered |
| **Unknown vocabulary** | Eligible analyzed lemmas not currently known or reserved by the active campaign. | Difficult words |
| **Current coverage** | Coverage from current known vocabulary under the accepted metric contract. | Reading level |
| **Projected coverage** | A clearly labeled hypothetical result after learning additional vocabulary. | Coverage when the condition is omitted |
| **Coverage threshold** | A target derived from token-weighted coverage, such as 95%, 97%, or 99%. | Difficulty score |

Always state whether a number is current, projected, token-weighted, scoped, or
conditional. Do not imply CEFR proficiency or general book difficulty.

## Status and feedback

Use complete, actionable labels where space permits:

- scope review required;
- ready to analyze;
- analysis queued;
- analysis running;
- analysis failed;
- analysis cancelled;
- analysis result ready;
- deck preparing;
- deck ready;
- queued for learning;
- learning in progress;
- campaign complete;
- campaign abandoned.

Raw values such as `analyzed`, `ready`, or `complete` are ambiguous without the
resource they describe. Status text must identify the resource or appear within
an unambiguous labeled context.

Use **error** for a failed request or operation, **warning** for risk or degraded
quality that does not block progress, **notice** for neutral contextual
information, and **success** for a completed user action. Messages should say
what happened and, when applicable, what the learner can do next.
