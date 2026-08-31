# Corpus, Campaign, and Horizon: experience-architecture discovery

Status: **Historical discovery evidence — learner-facing Corpus / Campaign /
Reading Horizon architecture superseded by the canonical
[`My Books / Reading Journey / Primary Goal`](information-architecture.md)
architecture.**

Date: 2026-08-31

This document preserves the investigation that led to the accepted direction.
Its relationship-graph reasoning, desire-first principle, and distinction
between evidence and decision remain useful. Its proposed primary nouns,
containment questions, and destination structure are not current learner-facing
guidance. Do not implement them from this document.

## Executive proposal

Mouseion should stop treating a book's processing lifecycle as the shape of the
whole product. It should instead distinguish three centers of gravity:

- the **work** is the center of textual evidence and the reading encounter;
- the **campaign** is the center of an intentional undertaking;
- the learner's **reading field**—working domain term: **corpus**—is the center of
  cumulative progress and choosing what becomes possible next.

These are not a `Corpus -> Campaign -> Book` containment hierarchy. They form a
relationship graph. A work can be considered without a campaign, can support
more than one campaign over time, and may appear in more than one future
collection or corpus lens. A campaign references a work and an exact body of
analysis evidence; it does not own the work. Corpus membership makes works
comparable but does not combine their scopes, analyses, or decks.

The recommended product architecture is therefore:

1. Let learners collect and **assess prospective works before committing** to a
   campaign.
2. Create a campaign at an **explicit decision to undertake a work**, before any
   particular preparation mechanism is required.
3. Make Anki a campaign preparation mechanism, not the prerequisite that gives a
   campaign existence.
4. Derive a learner's reading horizon from comparable, provenance-linked analyses
   and current language-scoped vocabulary.
5. After campaign completion, reveal which assessed works changed position and
   why, using exact current-versus-prior readiness evidence rather than rewards or
   generic progress scores.
6. Make the active campaign the default home context when one exists; make
   exploration of the reading horizon the primary way to choose what comes next.

The strongest version of the hypothesis is not “replace Library with Corpus” or
“put Horizon in the copy.” It is: **preparation is in service of an undertaking,
and the visible consequence of learning is a larger set of approachable
reading choices.**

## Investigation basis

The current model is explicit and internally coherent. The following documents
were reviewed as the principal sources of truth:

- [`../product.md`](../product.md)
- [`principles.md`](principles.md)
- [`experience-direction.md`](experience-direction.md)
- [`information-architecture.md`](information-architecture.md)
- [`terminology.md`](terminology.md)
- [`screen-inventory.md`](screen-inventory.md)
- [`workflows/acquisition-to-library.md`](workflows/acquisition-to-library.md)
- [`workflows/book-analysis-and-deck.md`](workflows/book-analysis-and-deck.md)
- [`workflows/learning-campaign.md`](workflows/learning-campaign.md)
- [`workflows/study-languages-and-known-vocabulary.md`](workflows/study-languages-and-known-vocabulary.md)
- [`../features/explicit-scoped-analysis-workflow.md`](../features/explicit-scoped-analysis-workflow.md)
- [`../features/analysis-insights.md`](../features/analysis-insights.md)
- [`../features/epub-analysis-scope-review.md`](../features/epub-analysis-scope-review.md)
- [`../adr/0009-home-lab-auth-corpus-isolation.md`](../adr/0009-home-lab-auth-corpus-isolation.md)
- [`../adr/0020-anki-package-output.md`](../adr/0020-anki-package-output.md)
- [`../adr/0022-prepared-decks.md`](../adr/0022-prepared-decks.md)
- [`../adr/0024-learner-owned-catalogs-no-admin.md`](../adr/0024-learner-owned-catalogs-no-admin.md)
- [`../adr/0025-analysis-coverage-threshold-metrics.md`](../adr/0025-analysis-coverage-threshold-metrics.md)
- [`../adr/0027-learning-campaigns.md`](../adr/0027-learning-campaigns.md)
- [`../adr/0028-explicit-scoped-analysis-lifecycle.md`](../adr/0028-explicit-scoped-analysis-lifecycle.md)

The implementation was also spot-checked. `/` redirects to `/library`; library
rows project one canonical next action; `learning_campaigns` requires both a
source material and a prepared deck; and the existing Go `Corpus` type and
`corpora` table represent one completed analysis artifact for one source/scope.
That last usage is materially different from the proposed long-term learner
corpus and creates a terminology and migration hazard.

## 1. Problem with the current experience architecture

### The lifecycle has become the information architecture

The current IA begins with the statement that Mouseion is organized around one
owned book moving through a durable reading-and-study lifecycle. The canonical
path is effectively:

```text
acquire book
  -> review scope
  -> analyze
  -> inspect analysis
  -> prepare deck
  -> create campaign
  -> read and study
  -> graduate vocabulary
```

This was a valuable sequencing model. It separated consequential transitions,
created durable identities, and made provenance understandable. The problem is
not the lifecycle itself; it is that the lifecycle has become the learner-facing
map of the product.

Consequences of that architecture include:

- **My Library is both inventory and home.** Its primary question is what each
  book needs next, so processing state competes with reading intention.
- **Analysis machinery becomes peer experience.** Scope, jobs, insights, deck
  preparation, and campaign state all appear as successive book stages even
  though learners care about them only in relation to a reading decision.
- **Campaign begins too late.** It is created only after an immutable prepared
  deck exists, so it cannot represent the learner's earlier commitment,
  preparation choices, or a decision to read without Anki.
- **The deck remains the gateway to learning.** Although insights correctly ask
  whether to read now, prepare, or choose another book, the modeled campaign is
  still one book plus one prepared deck.
- **Cross-book effects are numerically present but experientially absent.** Known
  vocabulary and active-campaign projections already change other books'
  coverage, but those consequences are shown one analyzed book at a time.
- **Completion points backward.** It updates known vocabulary and history but
  does not make the resulting expansion of future reading choice the primary
  outcome.
- **Acquisition implies a workflow starting point.** “Add to library” is safely
  decoupled from analysis, but the resulting book still enters a queue of
  lifecycle obligations rather than a field of possible reading.

### What should remain

The current architecture established unusually strong product guarantees. They
are not incidental to the old IA and should not be weakened to make a new model
feel smoother:

| Guarantee | Why it remains essential |
|---|---|
| Acquisition does not automatically confirm scope or start analysis. | Collecting interest is not consent to spend resources or accept an interpretation. |
| Scope recommendations are explainable aids; confirmation is explicit and immutable. | The learner retains authority over what counts as the text. |
| Analysis is explicit, owner-scoped, asynchronous, idempotent, retryable, and bound to one exact scope. | Readiness claims need reproducible evidence. |
| Completed analyses and prepared artifacts are immutable and historically addressable. | Corpus comparisons must not erase their basis when the source or learner state changes. |
| Source content, example sentences, and learner state remain owner-scoped; normalized shared corpus artifacts contain only lemma/statistical data, and external providers do not receive owner or book metadata. | A corpus view is a private learner lens, not a reason to pool books or expose cross-learner evidence. |
| Current, projected, token-weighted, scoped, and conditional quantities are labeled. | “Distance” must never become an opaque score. |
| Lexical, structural, and quality dimensions remain separate. | A single difficulty number would be falsely authoritative. |
| Generated, active, graduated, imported, and unknown vocabulary remain distinct. | Preparation is not knowledge, and knowledge claims need provenance. |
| Completion and abandonment are explicit consequential transitions. | Corpus-wide effects must not be caused silently. |
| The server-rendered baseline remains usable without JavaScript. | Horizon and campaign experiences must progressively enhance, not depend on a visual map. |
| Native semantics, keyboard use, assistive technology, narrow viewports, and reduced motion are first-order constraints. | Exploration must not become a vision- or pointer-only metaphor. |

## 2. Alternatives considered

### A. Rename Library to Corpus and keep the lifecycle

This is the least disruptive option: retain the existing book rows and
next-action pipeline, add cross-book coverage summaries, and replace library
language with corpus/horizon language.

**Rejected as the target model.** It decorates the current architecture but does
not change the late creation of campaigns, the deck prerequisite, or the fact
that processing stages define the experience.

### B. Make Campaign the sole top-level object

Every acquired book would immediately create a campaign, and all preparation
would occur inside it.

**Rejected.** Acquisition, curiosity, assessment, and commitment are different
learner decisions. Immediate campaign creation would turn every interesting book
into an obligation, inflate queues, and recreate automatic coupling at a higher
level. It would also prevent the learner from using analysis to decide whether a
work deserves a campaign.

### C. Treat Corpus as a strict parent of campaigns and books

A learner would select a corpus, find its campaigns, and find books within each
campaign.

**Rejected.** This assumes containment where the product has overlapping
relationships. A book can remain relevant after a campaign, a campaign may be a
rereading of a book, and future corpus lenses may overlap. It also prematurely
assumes that a corpus is permanently equivalent to one language, one library, or
one collection.

### D. Relationship graph with activity-first and horizon-first views

Works, campaigns, analyses, preparation mechanisms, and learner vocabulary keep
separate identities. The product presents two dominant views over that graph:
current undertaking and future reading possibility.

**Recommended.** It preserves evidence and explicit transitions while aligning
navigation with the learner's two recurring questions: “What am I undertaking
now?” and “What can I read next?”

## 3. Proposed conceptual model: roles and lifecycles

```text
Catalog entry
    | discover / acquire
    v
Work <---- Source snapshot <---- Confirmed scope <---- Analysis result
 |              immutable             immutable             immutable
 |                                                           |
 |                           readiness profile --------------+
 |                              (derived, current)
 |                                                           |
 +---- 0..n Campaigns ---- pinned evidence ------------------+
             |
             +---- 0..n preparation mechanisms
             |          +---- Anki prepared deck
             |          +---- future mechanisms
             |
             +---- reading progress and completion
                            |
                            v
                  vocabulary knowledge transition
                            |
                            v
Known vocabulary by learner + language
                            |
                            +---- recompute readiness across assessed works

Reading field / Corpus = a learner-relevant set and lens over Works;
it does not own the immutable evidence chain or merge analyses.
```

### Work / Book

Use **work** in the conceptual model to separate bibliographic identity from a
particular acquired EPUB. Keep **book** in ordinary learner-facing language while
the product is one-EPUB-per-work.

A work can be:

- discovered as catalog metadata;
- acquired with an immutable source snapshot;
- unassessed, being assessed, assessed, or in need of reassessment;
- prospective, attached to a planned or active campaign, completed, abandoned,
  or returned to later;
- represented by multiple historical scopes and analyses without any of them
  becoming a mutable “latest” artifact.

The work remains the center when showing text, passages, scope evidence,
analysis provenance, and a reading encounter. It stops being the parent of one
universal lifecycle state.

### Reading field / Corpus

The corpus is the learner's body of relevant literature and the context in which
progress has cumulative meaning. It is better understood as a **set plus a
comparison lens** than as a folder or a workflow container.

Initial product behavior can use one implicit reading field per learner and
language without introducing a first-class persisted `Corpus` object. Language
is the natural first partition because known vocabulary and NLP capability are
language-scoped; it should be a facet of the corpus view, not a permanent
ontological definition of corpus.

The conceptual reading field is defined by learner concern, not current pipeline
capability. It may therefore include a desired work known only through catalog
metadata, an unsupported language, or a source Mouseion cannot yet analyze. The
initial **quantitative horizon** is narrower: only an acquired, validated,
owner-scoped source snapshot with trustworthy analysis can receive a readiness
placement. Other desired works remain visible as **Not currently assessable**
and must not be assigned invented distance.

For the existing acquisition path, the current source material acts as the work
proxy, and idempotent acquisition of the same owner, catalog entry, and source
content resolves to the existing work. Different editions remain separate
acquired works until Mouseion has a real bibliographic work/edition identity
model. A future content replacement creates a new source revision and requires
new scope review; it does not rewrite historical analysis. Supporting
metadata-only desire records requires an explicit identity, deduplication, and
removal contract; supporting source deletion requires a separate retention
contract specifying what remains historically addressable.

A corpus view answers:

1. Which works do I care about?
2. Which have trustworthy readiness evidence?
3. Which meet my selected planning threshold now?
4. What vocabulary investment would bring another work to that threshold?
5. Which evidence is stale, incomplete, unsupported, or not yet produced?
6. How would completing the active campaign change these answers?
7. What changed after a completed campaign or vocabulary import?

It does **not** combine book scopes, build aggregate decks, or claim a shared
analysis result. Existing “no cross-book scopes or aggregate decks” boundaries
can remain intact.

### Campaign

A campaign is an intentional undertaking to prepare for and read a work. It
should be created by an explicit learner commitment, not by acquisition,
analysis, or deck preparation.

Recommended boundary:

- a work in the reading field is **prospective**;
- choosing **Plan to read** or **Begin reading** creates a campaign;
- choosing preparation creates one only when the learner also confirms the intent
  to undertake the work; asking for readiness evidence alone does not;
- the campaign pins the work and, once chosen, the exact scope/analysis evidence
  used for its preparation decisions;
- **Begin reading** may create and activate a campaign with no Anki preparation
  when the learner intentionally declines that mechanism.

The primary path should let the learner assess readiness first. It may also allow
an explicit early commitment to an unassessed work; in that case assessment
becomes the campaign's first preparation step, not an automatic side effect.

A campaign can have zero or more preparation records conceptually, but the
initial generalized product should allow **zero or one selected preparation
mechanism**. That keeps completion and vocabulary effects understandable while
leaving the model open to a future composition contract. The first mechanism is
Anki. Adding several simultaneous mechanisms later requires an explicit decision
about precedence, completion, failure, and duplicate vocabulary evidence.

Campaign orientation is derived from independent facts rather than one linear
pipeline:

```text
prospective work (not a campaign)
        |
        | explicit commitment
        v
planned campaign
   |         |                    |
   |         | assess first       | use existing exact analysis
   |         v                    v
   |     assessment --------> optional preparation
   |                                  |
   | choose no preparation            |
   +-------------------------+--------+
                             v
                       ready to begin
                             |
                             | explicit activation; at most one active
                             v
                          active
                         /      \
                 completed      abandoned
```

A valid acquired work and explicit learner intent are sufficient to activate a
no-preparation campaign; analysis is not secretly required for reading. Such a
campaign reserves no vocabulary. An Anki-backed campaign cannot reserve assigned
vocabulary until its immutable preparation exists. Completion of a no-preparation
campaign changes campaign history but adds no vocabulary unless a separate
explicit knowledge action is defined.

Assessment state, preparation state, reading state, and knowledge transitions
remain independent. The campaign phase is only an orientation projection and
must not flatten their histories into one ambiguous percentage.

Evidence is bound deliberately. A planned campaign may adopt a different exact
analysis before a preparation artifact is finalized. Once a preparation is
ready, that mechanism permanently records the exact analysis it used. Activation
pins the campaign's chosen reading/preparation basis for history. A later source
content revision creates a new snapshot, scope, and analysis under ADR 0028; it
does not silently update the planned or active campaign. Before activation, the
learner may explicitly adopt new evidence or continue with the historical basis.
After activation, reassessment informs a future campaign rather than rewriting
the current one.

#### Initial reading boundary

In the initial model, reading occurs outside Mouseion. **Begin reading** is an
explicit manual transition into the active undertaking, not a command that opens
an integrated reader or infers page progress. Mouseion records the learner's
manual reading state, keeps preparation and provenance available, and later asks
the learner to confirm that the work is finished. The **Reading** navigation
label names planning and conduct of that undertaking; it does not promise EPUB
rendering, automatic position tracking, or time estimates. A future reader
integration would be a separate product decision and evidence source.

The one-active-campaign rule remains a useful initial constraint: it preserves
focus and deterministic active-vocabulary reservation. It does not require only
one prospective work or planned campaign. Whether pausing, overlapping reading,
or preparing several future campaigns should be supported is an unresolved
product decision, not something to infer from the metaphor.

### Analysis and readiness profile

An analysis result remains immutable evidence for one source and confirmed
scope. A **readiness profile** is a derived interpretation of that evidence
against current learner vocabulary and a named planning threshold.

```text
readiness profile =
  exact analysis result
  + current known vocabulary for its language
  + optional, separately labeled active-campaign projection
  + selected coverage threshold
  + structural context
  + analysis-quality state
```

The readiness profile is not another mastery artifact. Values should be computed
from current state, as Analysis Insights already requires. A campaign pins the
exact analysis used for preparation; the corpus view may use a clearly disclosed
comparison basis. If several valid analyses exist, the product must not silently
use an unexplained “latest” result. It needs a documented default and a visible
path to inspect or change the basis.

### Preparation mechanism

A preparation mechanism is something the learner chooses in service of a
campaign. Anki is the first implementation:

```text
Campaign
  Preparation
    Anki deck
      exact analysis -> candidate selection -> enrichment -> immutable APKG
```

Future mechanisms might include a vocabulary list, guided preview, contextual
reading aid, or another SRS integration. Each mechanism needs its own evidence,
status, privacy, cancellation, and vocabulary-effect contract. The campaign
must not assume every mechanism produces a deck or that finishing any mechanism
proves the same knowledge transition.

### Known vocabulary

Known vocabulary remains learner- and language-scoped state, not a child of a
corpus or campaign. It receives explicit imports and justified campaign-linked
graduations, and it influences every assessed work in the same language.

The current Anki contract can remain: generated and active-assigned vocabulary
is not known; the informed completion transition can graduate eligible assigned
vocabulary. Generalizing campaigns exposes an important boundary: **campaign
completion and vocabulary graduation cannot permanently be synonyms.** A
campaign with no vocabulary-producing preparation may complete without adding
lemmas. A future mechanism must define what evidence, if any, makes vocabulary
eligible for an explicit knowledge update.

## 4. Learner's end-to-end journey under the proposed model

```text
Explore catalogs and existing works
    -> add a desirable work to the reading field
    -> assess it when useful
        -> review and confirm scope
        -> explicitly analyze
        -> inspect readiness evidence
    -> compare it with other prospective works
    -> explicitly commit to a campaign
    -> choose no preparation or one initial mechanism
    -> explicitly activate the campaign
    -> read and conduct preparation
    -> explicitly complete or abandon
    -> apply any disclosed vocabulary transition
    -> inspect changes across the reading horizon
    -> choose the next campaign
```

This journey changes the purpose of the existing machinery without hiding it:

| Current mechanism | New experience role |
|---|---|
| Catalog connection | A source for discovering and acquiring works; connection maintenance remains secondary. |
| Acquisition | Adds evidence and a possible future work to the reading field; creates no campaign and starts no analysis. |
| Scope review | Defines which text the learner intends to assess or read; an explicit supporting decision. |
| Analysis | Produces trustworthy readiness evidence; useful before or during a campaign. |
| Analysis insights | The exact-work evidence behind horizon placement and campaign preparation decisions. |
| Vocabulary selection | A named preparation calculation against one analysis and current learner state. |
| Prepared deck | One immutable preparation artifact, not the endpoint of Mouseion. |
| Known vocabulary | The learner-state input that changes readiness across all same-language works. |
| Learning queue | Planned campaigns ordered by intention; not a list that can exist only after deck creation. |
| Campaign completion | Completion of the undertaking plus any separately disclosed mechanism-specific knowledge transition. |

## 5. Information architecture and navigation

### Recommended destination model

Use learner goals rather than domain implementation nouns as primary navigation:

```text
Reading
  Current campaign
  Planned campaigns
  Completed / abandoned history

Explore
  Reading horizon (default view for one explicit language)
  Owned books (source-inventory filter)
  Works needing assessment (evidence-state filter)
  Work detail and exact evidence

Add books                 [global workflow action]
  Catalog choice / setup
  Browse / search / acquire

Settings
  Study languages
  Known vocabulary maintenance
```

Recommended labels:

- **Reading** instead of **Learning** for the campaign destination. It names the
  undertaking rather than one preparation mechanism.
- **Explore** as the discoverable navigation label for the corpus-level
  destination, with **Your reading horizon** as its explanatory page heading.
  “Horizon” alone may be too metaphorical for primary navigation until tested.
- **Add books** can remain the global action. **Add to library** can remain the
  precise acquisition action while “library” still describes owned source
  material. Not every occurrence of library language needs to be replaced.
- **Settings** remains maintenance, not the main place where vocabulary's
  consequences are experienced.

### Home and route behavior

`My Library` should no longer be the unconditional home. The proposed canonical
model is:

- `/reading` is the campaign destination. When a campaign is active, `/` resolves
  here and the current undertaking is primary.
- `/explore?language=<code>` is the corpus/horizon destination. When no campaign
  is active, `/` resolves here if there is exactly one valid language context.
  With several study languages and no explicit context, `/` presents a language
  choice rather than silently combining incomparable works.
- `/explore?language=<code>&view=owned` is the acquired-source inventory. **Owned
  books** is a filter inside Explore, not another primary destination.
- `/library` remains a compatibility route to the owned view, preserving a valid
  language query when available. Without a resolvable language it leads to the
  same explicit language choice.
- Existing `/campaigns` routes may remain compatibility URLs during transition,
  while **Reading** becomes the learner-facing destination label.

The URL carries language and view state so server-rendered links, bookmarks, and
history remain coherent without JavaScript. Session memory may offer a
convenience default, but it does not replace explicit URL state on a comparison
surface.

This preserves library semantics where the learner is genuinely managing
acquired source material while establishing one canonical corpus destination.
The home remains a purposeful start surface rather than a generic dashboard.

### Example interface hierarchy, not a mockup

A horizon view should answer questions in this order:

1. language and comparison basis;
2. works the learner has explicitly marked as desired or planned;
3. active-campaign effect, if a projection is being shown;
4. readiness evidence for desired and other assessed works;
5. works newly changed by learning, without promoting them above stronger desire;
6. works not yet assessable, not yet assessed, or whose evidence needs review;
7. exact metric definitions and analysis provenance.

A campaign view should answer:

1. what the learner has undertaken;
2. whether it is planned, preparing, ready, active, complete, or abandoned;
3. what must happen before reading can begin or completion can be recorded;
4. which preparation mechanisms are chosen and what each claims;
5. which exact evidence supports the plan;
6. what completion or abandonment will change;
7. how to return to the work and the reading horizon.

## 6. Horizon as actual UX

“Horizon” should change what Mouseion organizes, compares, and reveals. It
should not be a layer of adventurous copy.

### Readiness bands with explicit bases

A learner selects a planning threshold—initially one of the accepted 95%, 97%,
or 99% token-coverage targets. The horizon may then annotate assessed works with
these evidence states without making them the only grouping or priority model:

- **Within threshold** — no additional eligible lemma identities are required to
  reach the selected target under current known vocabulary;
- **Preparation required** — the target is reachable and the exact additional
  lemma count is shown, ordered from lower to higher investment;
- **Target not currently reachable** — exclusions or evidence conditions prevent
  the calculation from reaching the target;
- **Not assessed** or **Needs review** — no trustworthy comparable result exists.

“Within reach” may be used as a plain-language interpretation only when it is
immediately paired with its basis, for example: “Within reach at your 97%
planning threshold; no additional whole-scope vocabulary is required.” It must
not mean comfortable reading, general proficiency, or a CEFR level.

Every placement keeps the exact values visible or one disclosure away:

- current token-weighted coverage for the selected scope;
- whole-scope threshold investment under ADR 0025: target, full analyzable-token
  denominator, exact additional lemma-identity count, and whether the target is
  reachable;
- active/reserved and legacy-generated categories that explain the calculation;
- lexical concentration;
- structural signals and analysis-quality warnings;
- exact analysis and scope provenance.

The whole-scope threshold investment used for horizon placement is **not** the
Anki deck candidate count. Deck generation applies its own eligible-unknown-pool
contract and can produce a different number at the same nominal 97% label. Deck
count appears only inside the chosen Anki preparation mechanism and is labeled
**Deck vocabulary**, never as horizon distance. A target that is unreachable
under the metric contract displays the reason and no invented lemma count.

Whole-scope investment supports an explicit **Preparation required** sort, with
ties resolved by normalized title and stable work identity. It must not become
the default priority ranking. The default horizon preserves learner-expressed
desire and campaign intent; analysis annotates what preparation that direction
may require. Popularity, recency, catalog order, and vocabulary efficiency do
not silently decide what the learner should want.

Avoid hard-coded “near” and “far” bands based on arbitrary lemma counts. If the
product later lets a learner state a preparation budget, it can show works
within that budget. Until then, exact investment and ordering are more honest
than qualitative distance labels.

### Current and projected horizons

The default view uses current known vocabulary. A separate control may show
**After the active campaign**. This is a proposed corpus-level extension of the
existing single-book active-campaign projection, not a capability that already
exists.

For each assessed work in the active campaign's language, Mouseion recomputes the
same whole-scope readiness metric using current known vocabulary plus only the
active campaign's vocabulary that is eligible to graduate on completion. The
projection does not treat reserved vocabulary as currently known, does not alter
deck eligibility, and uses each work's exact comparison analysis. Other
languages are unchanged; when the selected horizon language differs from the
owner-wide active campaign, the UI says that the campaign has no projected effect
in this language. If the campaign has no eligible graduation set, a work lacks
trusted comparison evidence, or recalculation fails, Mouseion preserves the
current placement and explains why no projection is available.

Current and projected states must not be blended. Projected works remain
explicitly conditional and do not visually become known/current before
completion.

### Completion reveal

Campaign completion should have deliberate emotional pacing without reward
mechanics:

1. confirm the exact completion and vocabulary consequence;
2. show a quiet completion acknowledgment with the work and dates;
3. show the corpus effect: works that newly meet the selected threshold and
   works whose required vocabulary investment materially decreased;
4. offer **Explore your updated horizon** and **Choose the next campaign**.

No confetti, points, levels, badges, streaks, celebratory scarcity, or locked
territory is needed. The change in available literature is the reward.

A “newly within reach” claim requires a reproducible before-and-after basis: the
same work analysis/scope, threshold, and vocabulary rules on both sides of the
completion transition. If Mouseion cannot preserve or compute that comparison,
it should say that readiness was recalculated rather than make a historical
claim.

### Visualization

The canonical horizon must be a semantic list or table with headings, exact
numbers, status text, and links to evidence. A visual horizon map may
progressively enhance it, but must:

- encode distance with the documented additional-lemma measure, not an opaque
  score;
- show quality or structural cautions separately;
- expose the same information in the accessible list;
- support keyboard focus and meaningful screen-reader labels;
- not depend on drag, hover, color, animation, or spatial memory;
- respect reduced motion and remain coherent on narrow screens;
- avoid landscape, map, constellation, or game-board decoration.

A restrained transition—such as reordering works after an explicit filter
change—can support comprehension, but it must not be the only indication of
change.

## 7. Concrete experience examples

All values below are illustrative and retain the existing metric language.

### Discovering a desirable book

A learner browsing an Italian OPDS catalog finds *Il sentiero dei nidi di ragno*.
They choose **Add to library**. Mouseion validates and stores the EPUB, marks the
work **Not assessed**, and preserves the catalog position so browsing can
continue. No campaign, scope confirmation, or analysis is created.

In Explore, the book now appears among prospective Italian works. The main
question is “Would you like to assess how approachable this is?” rather than
“What pipeline step is overdue?”

### Deciding whether it is within reach

The learner chooses **Assess this work**. They review the recommended reading
scope, explicitly confirm it, and explicitly start analysis. When it completes,
the work shows:

- current scoped token coverage;
- additional lemma identities to 95%, 97%, and 99%;
- top unknown vocabulary and concentration;
- structural context and quality warnings;
- the exact analysis and scope used.

At the selected 97% planning threshold the result requires additional
whole-scope vocabulary, so the book is ordered among other preparation
candidates. A later Anki preparation may select a different **Deck vocabulary**
count under its own eligible-pool contract. The UI does not call the book
globally difficult or claim the learner cannot read it.

### Preparing for it

The learner compares this work with two others and chooses **Plan to read**. That
explicit action creates a planned campaign. The campaign pins the selected
analysis as its preparation evidence.

The learner selects **Prepare an Anki deck** and reviews the privacy consequence
of optional translation. Preparation remains asynchronous and retryable; the
resulting APKG is immutable and downloadable. A future preparation mechanism
would appear as another choice with its own contract, not as another kind of
deck.

### Beginning and conducting a campaign

When no other campaign is active, the learner chooses **Begin reading**. The
campaign becomes active; its assigned vocabulary is reserved but not counted as
known. The reading undertaking is now primary, while reading itself remains
external and progress is manually confirmed. Mouseion shows the independent
reading and Anki-review facts without inventing a completion percentage.

The exact analysis, preparation history, and APKG remain available as supporting
material. They do not compete with the work and the next meaningful campaign
action.

### Completing it

After the book is finished and the deck is reviewed, the final action names the
number of eligible lemma identities that will be added to known vocabulary and
states that the change cannot currently be undone. Completion and graduation
remain atomic for this Anki-backed campaign.

The success state first acknowledges the completed work, then explains that
same-language readiness has been recalculated. It does not claim general
mastery.

### Discovering what became newly accessible

The completion result shows that an already assessed Italian work now meets the
learner's selected 97% planning threshold and that several others require fewer
additional lemmas. Each item shows its exact current coverage or investment and
links to the immutable analysis behind the calculation.

Works whose current source revision still needs scope review, whose analysis
failed, or whose completed evidence has quality warnings appear separately;
Mouseion does not place them optimistically merely to create a stronger reward.

### Choosing the next campaign

The learner opens the updated horizon. Their explicitly desired works remain
primary and carry readiness annotations; a separate preparation sort can order
assessed works by exact investment. They can compare a newly within-threshold
work, one requiring a small explicit lemma set, and a distant personal favorite
without being pushed toward the mathematically cheapest choice.

The product may surface newly approachable works as discoveries, but it does not
promote them above the learner's stated interests. Choosing a work opens its
evidence first. **Plan to read** remains an explicit commitment. Desire determines
direction; ranking informs preparation rather than replacing intention.

## 8. Terminology recommendations

| Concept | Recommended learner-facing language | Internal/domain guidance | Avoid |
|---|---|---|---|
| Long-term body of relevant literature | **Reading horizon** for the comparative experience; **works** or **all works** for inventory | Use **reading field** as a neutral working term until the `Corpus` collision is resolved | Making **Corpus** primary navigation before comprehension testing; equating corpus with library or language |
| Intentional undertaking | **Campaign** in explanatory/detail language; **Reading** as the destination | Generalize campaign beyond a required deck only after a new accepted contract | Quest, mission, project, level |
| Bibliographic object | **Book** | Distinguish work identity from source/edition in the model | Corpus or artifact when the learner means a book |
| Evidence intake | **Add books**, **Add to library** | Acquisition creates source evidence and corpus membership, not a campaign | Import and analyze, start campaign |
| Readiness action | **Assess this work** | Explicit scope plus analysis; keep exact resource terminology in status/history | Scan, unlock, discover difficulty |
| Current readiness | **Current scoped coverage** and **within reach at [threshold]** | Derived from exact analysis plus current known vocabulary | Reading level, difficulty score, comfortable without a stated basis |
| Conditional readiness | **After the active campaign** or **projected coverage** | Keep separate from current state | Soon unlocked, guaranteed next |
| Study mechanism | **Prepare an Anki deck** | Anki is one preparation mechanism | Prepare campaign when only the deck is being prepared |
| Long-term change | **Newly within reach at [threshold]**, **updated reading horizon** | Require reproducible before/after evidence | XP, level up, achievement, territory unlocked |

There is a critical namespace decision: the backend currently calls each
immutable analyzed result a `Corpus`. The proposed learner corpus means something
else. Do not expose both under one word. Before introducing a first-class
long-term object, either rename the existing internal analysis artifact through a
planned migration or choose a distinct domain name such as `ReadingField` or
`Collection`. Learner-facing use can remain “reading horizon” regardless.

## 9. Required states, accessibility, and responsive behavior

The corpus/horizon experience introduces aggregate states that must be designed
before a visual representation:

| State | Required interpretation |
|---|---|
| No works | Explain how adding a book creates future reading possibilities; offer Add books. |
| Works but none assessed | Distinguish collecting from assessment; offer an explicit assessment action. |
| Mixed languages | Preserve language context and never compare language-scoped vocabulary across languages. |
| Analysis queued/running | Keep the work in place with durable status; do not show a speculative readiness band. |
| Analysis failed/cancelled | Show an actionable evidence problem, not a distant-work judgment. |
| Stale source or changed scope | Remove or qualify comparative placement until the basis is confirmed. |
| Quality warning | Keep the exact metrics available but separate the work into Needs review when comparison would mislead. |
| Unsupported/degraded language capability | Preserve acquired works and prior evidence; disable only unsupported new operations. |
| Active campaign projection unavailable | Keep the current horizon usable and omit the conditional lens with an explanation. |
| Recalculation pending/failed | Do not show old values as new; identify which values remain current and offer recovery. |
| Campaign completion with no changed work | Acknowledge the completed undertaking without manufacturing an expansion claim. |
| Very large corpus | Use searchable, filterable, paginated server-rendered results; do not require a spatial canvas. |

Interaction contract:

- A semantic list/table is the source presentation; graphics are supplementary.
- Sort and filter controls use native labels and expose the current ordering.
- Every status has text and does not rely on color, distance, or iconography.
- Dynamic assessment and recalculation use scoped live regions and do not
  repeatedly move focus.
- A completion POST has a normal server-rendered success destination. Animation
  or client-side reordering is optional enhancement.
- Narrow layouts preserve the order: work identity, readiness basis, exact
  investment, cautions, action, provenance.
- Long numeric tables use focusable contained regions rather than page-level
  horizontal scrolling.
- The design respects `prefers-reduced-motion`; there is no essential animated
  “horizon expansion.”

## 10. Conflicts and implications for existing documentation

### Decisions that can remain intact

- ADR 0028's explicit acquisition, scope, analysis, and immutable artifact chain
  remains the trust substrate. The chain becomes supporting evidence rather than
  global IA.
- ADRs 0025 and 0026 remain the numerical basis for readiness. Corpus comparison
  must not introduce a composite difficulty score.
- ADR 0022's immutable prepared artifact and pure download remain appropriate.
- ADR 0024's learner-owned catalogs remain appropriate; acquisition is reframed,
  not centralized.
- Current privacy, ownership, idempotency, retry, historical readability, and
  progressive-enhancement guarantees remain applicable.
- Cross-book comparison does not require cross-book scopes or aggregate decks.

### Decisions that materially conflict

- **ADR 0027:** a campaign currently ties exactly one source book to exactly one
  prepared deck and begins when a ready deck is added to the queue. Earlier
  campaign creation and preparation mechanisms other than Anki require a new ADR
  that supersedes this boundary while preserving one-active focus and explicit
  vocabulary semantics where still desired.
- **Campaign completion:** current completion is exactly `book finished AND deck
  reviewed`, atomically graduating assigned vocabulary. A deck-optional campaign
  needs a general completion contract and mechanism-specific knowledge effects.
- **Persistence:** `learning_campaigns.deck_preparation_id` is required, and
  campaign progress uses book/deck columns. The proposed model cannot be achieved
  by navigation changes alone.
- **Corpus terminology:** `Corpus` currently means one immutable analysis result
  in domain code, persistence, and documentation. The long-term corpus hypothesis
  conflicts semantically even before a new table or route exists.
- **Information architecture:** `information-architecture.md`,
  `screen-inventory.md`, and workflow documents explicitly make My Library the
  home, the book the learner-facing center, and the status sequence the canonical
  lifecycle. Acceptance of this proposal would require coordinated replacement,
  not isolated copy edits.
- **Design principles:** “text- and book-centered rather than
  dashboard-centered” remains right during textual encounters, but needs a
  companion principle that campaign centers activity and reading possibility
  centers long-term progress. The change must not license generic dashboard
  cards.
- **Terminology:** current guidance says My Library is canonical and avoids
  `corpora` for the learner-facing book collection. That remains correct until a
  distinct learner model and tested language are accepted.

### Existing foundations that already point toward the proposal

- Analysis Insights explicitly exists to decide whether to read now, prepare, or
  choose another book.
- Readiness values are calculated on demand from current vocabulary rather than
  frozen as mastery snapshots.
- Queued books already have current and after-active-campaign projections.
- Acquisition is already separated from analysis and supports collecting several
  books.
- Learning already foregrounds one active campaign over queue and history.
- Completion already promises future coverage recalculation.

The proposal is therefore a reorganization and generalization of existing
strengths, not a repudiation of them.

## 11. Unresolved product questions and tradeoffs

1. **How should desired but currently unanalyzable works be identified and stored?**
   They belong conceptually, but metadata-only identity, deduplication, source
   attachment, and removal semantics remain to be defined.
2. **Is corpus ever first-class?** An implicit language-filtered lens may be
   sufficient initially. Multiple named or overlapping corpora should not be
   built without evidence.
3. **What is the comparison basis when a work has several scopes or analyses?**
   The policy must be explicit, inspectable, and safe under source changes.
4. **What does “within reach” mean?** The recommended initial meaning is meeting a
   selected coverage threshold, not comfort. Is a default threshold acceptable,
   or must the learner choose one before banding appears?
5. **Should the learner set a preparation budget?** It would make “near horizon”
   honest, but it adds configuration and may overemphasize lemma counts.
6. **How should structural context affect ordering?** It should warn and help
   comparison without becoming a hidden composite score.
7. **Can a campaign be created before assessment?** This supports strong intent
   but may make campaigns feel operational. The recommended answer is yes by
   explicit exception, with assessment as the first planned step.
8. **Can a campaign begin with no preparation mechanism?** The proposed model
   says yes; the completion and knowledge contract must say what then changes.
9. **Can several planned campaigns prepare concurrently?** This affects
   vocabulary overlap, reservations, cost, and queue determinism.
10. **Does one-active remain permanent and owner-wide?** It is useful now but may
    not represent a learner who alternates a novel and a difficult reference work.
    The current constraint also spans languages; a future language-partitioned
    horizon must not silently imply that German and Italian can each have an
    active campaign.
11. **What counts as campaign completion?** Reading finished, all chosen
    preparation commitments satisfied, or an explicit learner declaration?
12. **How do future mechanisms justify vocabulary graduation?** Review, exposure,
    explicit recognition, or no automatic knowledge transition?
13. **Should historical horizon changes be durable?** A one-time completion
    comparison can be computed transactionally; a later replay requires a
    persisted, provenance-linked impact snapshot.
14. **Can a work be removed from the reading field without deleting immutable
    source and analysis history?** Corpus membership, source retention, and
    deletion need separate semantics.
15. **Work versus edition:** when multiple EPUB editions appear, does one campaign
    attach to a work, an edition, or an exact source snapshot plus scope?
16. **What is the source of reading progress?** Manual facts remain honest today;
    future reader integration should not be presumed.
17. **Does “Campaign” feel appropriately serious to learners?** It is already
    established, but the navigation label **Reading** should be tested against
    **Campaigns**.
18. **What happens when no other work changes threshold band?** Completion must
    still feel meaningful without fabricated “new territory.”

## 12. Review response addendum: desire, reach, and progress

These responses refine the proposal without accepting its unresolved product
choices. They add a second governing principle:

> **Desire determines direction; analysis informs preparation.**

Together with “The reward is not a score. It is more world,” this prevents two
opposite failures: turning Mouseion into a gamified language-learning product or
turning it into an elegant vocabulary-optimization dashboard.

### Is a threshold too binary or vocabulary-centric?

It is if it defines the horizon. A 95%, 97%, or 99% threshold is one conditional
planning marker, not the boundary of literary possibility and not a statement of
comfort. The horizon should retain continuous preparation evidence, structural
context, evidence quality, and works that cannot be measured. **Within reach**
means only that no additional whole-scope vocabulary is required for the selected
threshold. It must not become the default ranking or the definition of what the
learner ought to read.

### Can assessment be further subordinated?

Yes. **Assessment** should remain a domain grouping for scope and analysis, not a
primary learner destination or an obligation attached to every acquired work. The
learner-facing action should answer the immediate question, for example **See what
preparation might help** or **Understand what this work would require**. Scope
review, explicit analysis submission, status, and provenance remain available as
the trustworthy machinery behind that answer.

### Does the conceptual corpus include currently unanalyzable literature?

Yes. The reading field represents literature the learner cares about; it must not
be bounded by Mouseion's current providers, formats, language support, or source
access. Analyzability determines whether Mouseion can place a work quantitatively,
not whether the work belongs. Desired but unsupported works remain visible as
**Not currently assessable**, with the blocking reason and no invented distance.
The quantitative horizon is therefore a subset of the conceptual reading field.

### Is Reading the right navigation label when reading is external?

It remains a hypothesis, not an accepted label. External activity does not by
itself disqualify **Reading**—calendar and fitness products also organize activity
that occurs elsewhere—but the destination must not imply an integrated reader.
Test **Reading**, **My reading**, and **Campaigns** against the questions learners
expect each destination to answer. The initial page description must state that
Mouseion plans preparation and records manually confirmed reading undertaken
outside the product.

### What commitment creates a campaign?

A campaign exists when the learner explicitly says, “I intend to read this work,”
and chooses to place it among planned or current undertakings. **Plan to read** and
**Begin reading** satisfy that boundary. Saving a work, expressing interest,
acquiring a source, requesting readiness evidence, or preparing a comparison does
not. Choosing preparation creates a campaign only when the learner also confirms
that undertaking; analysis alone is not commitment. No deck, threshold, date, or
completed assessment is required.

### What if completion moves no other work across a threshold?

The completed textual encounter is the primary accomplishment. The completion
state first records the work, reading facts, and any learner reflection; corpus
effects are secondary evidence. It may accurately show reduced preparation
without a threshold crossing. If no comparable work changed, Mouseion should say
so quietly and offer the updated reading field without treating the result as a
failed reward. The product must never manufacture “new territory” to make a
completion feel consequential.

### How is reading progress represented without claiming vocabulary knowledge?

Keep four ledgers distinct:

1. undertaking state—planned, active, complete, or abandoned;
2. reading history—manual start/finish facts and, if later supported, optional
   location or section progress;
3. preparation state—what the learner chose and completed;
4. knowledge state—only vocabulary explicitly imported, marked, or graduated
   under an accepted mechanism contract.

Exposure, pages read, elapsed time, and campaign completion are meaningful
progress, but none automatically proves vocabulary recognition. A reading-only
campaign can complete with no knowledge transition and still become part of the
learner's durable reading history.

### Should the backend `Corpus` be renamed?

Probably, if `corpus` becomes important product language. The existing domain
object is a completed owner-scoped analysis artifact, not the learner's body of
literature. Historical implementation terminology should not veto the clearer
product concept. A later ADR should evaluate a code-level name such as
`AnalysisArtifact` or `AnalyzedText`, while allowing the PostgreSQL table name to
remain temporarily for compatibility. The rename is a clarification and migration
concern, not a reason to distort learner-facing language now.

### What visual language can express horizon and movement?

Use a restrained visual field rather than a game map or a table alone. Candidate
system decisions to prototype are:

- near/far position based on one explicitly selected preparation measure, with
  exact values always available;
- learner-desired works given stable visual emphasis rather than automatically
  moved behind cheaper works;
- a quiet horizon line or depth layers that distinguish currently within the
  selected marker, preparation required, and not assessable without implying
  territory or levels;
- before/after positions shown through a restrained trace or paired position after
  completion, with textual deltas and reduced-motion behavior;
- typography, spacing, and bibliographic identity carrying more weight than
  icons, illustration, or decorative geography.

The semantic list remains the accessible source presentation, but the visual
field can make relative distance and movement perceptible at a glance. It must
never collapse lexical, structural, and evidence-quality dimensions into an
unexplained spatial score.

### How does desire become first-class without becoming another score?

For existing acquired work records, a learner can mark **Want to read** without
creating a campaign and may optionally record why or indicate current priority.
Discovery prototypes may represent the same state on metadata-only prospects, but
persisting those prospects is not accepted until identity, deduplication, and
removal semantics are defined. Desire is categorical and learner-owned, not
inferred from clicks or converted into points. The default horizon foregrounds
wanted and planned works, annotating each with preparation evidence. Newly
approachable or low-investment works appear as discoveries and optional sorts,
not as an optimized next-work prescription. Mouseion explains the likely cost of
a chosen direction; it does not choose the direction.

## 13. Proposed evolution sequence

This is a sequence of product/design decisions and validation, not an
implementation backlog.

### Stage 1 — settle the model and measurement language

- Review this proposal against real learner goals and representative German and
  Italian libraries.
- Decide whether `Corpus` is a learner concept, an internal reading-field concept,
  or only the inspiration for the horizon view.
- Define the comparison-basis policy and the exact threshold meaning of “within
  reach.”
- Define campaign creation, activation, completion, and no-deck semantics.
- Record accepted changes in a feature document and new/superseding ADRs before
  implementation work is decomposed.

### Stage 2 — validate the horizon with existing data, read-only

- Use existing completed analyses and on-demand vocabulary calculations to test
  a comparative, language-filtered corpus view.
- Include desired but unsupported or source-unavailable works as explicit
  non-quantitative states in the prototype; do not invent readiness for them.
- Preserve exact result links, quality states, and current versus after-active
  projections.
- Test semantic-list-first prototypes with realistic empty, sparse, large,
  mixed-state, and narrow-viewport content.
- Do not change campaign persistence or completion semantics in this stage.

### Stage 3 — reframe navigation and acquisition

- Validate Reading and Explore as primary goals and remove My Library as the
  unconditional home only after the comparative view is useful.
- Retain library/source inventory as a secondary function and preserve existing
  acquisition guarantees.
- Reframe assessment as an optional action in service of comparison or a planned
  undertaking, while keeping every scope and analysis decision explicit.

### Stage 4 — make campaign independent of Anki

- Introduce the accepted earlier campaign boundary and a general preparation
  model.
- Adapt the current prepared deck workflow as the first mechanism without
  weakening immutable artifacts, privacy consent, retry, or pure download.
- Preserve the current Anki graduation contract for compatible campaigns while
  defining honest completion for no-deck and future-mechanism campaigns.
- Migrate existing deck-backed campaigns conservatively and retain history.

### Stage 5 — make corpus change visible at completion

- Compute a reproducible before-and-after readiness comparison around an explicit
  vocabulary transition.
- Present newly within-threshold and reduced-investment works with exact evidence,
  while treating the completed reading encounter as the primary accomplishment.
- Define the honest no-crossing and no-comparable-change completion states; do not
  manufacture movement.
- Add a durable impact snapshot only if learners need to revisit historical
  horizon changes; do not persist derived state without that requirement.

### Stage 6 — extend only from evidence

- Consider durable metadata-only desire records, named/overlapping corpora,
  multiple editions, concurrent campaigns, additional preparation mechanisms, or
  reading integrations only after the core campaign/horizon model proves useful;
  their conceptual states should still be represented during validation.

## 14. Evaluation criteria

The direction should be accepted only if research and prototype evaluation show
that learners can answer these questions more easily than in the current model:

- What am I preparing for or reading now?
- Which works do I care about but have not committed to?
- What evidence says a work is or is not within my selected threshold?
- What exact preparation would change that?
- Why is Mouseion asking me to review scope, run analysis, or prepare a deck?
- What changed elsewhere because I completed this undertaking?
- What should I read next, and why might I choose it?

It should be rejected or revised if it merely adds new nouns, obscures the exact
analysis chain, turns literature into a ranked optimization problem, makes
coverage look like proficiency, or requires decorative exploration metaphors to
feel coherent.

The intended emotional result is restrained but real: the learner completes a
serious undertaking, returns to a body of literature they care about, and can
see—accurately—which reading possibilities have changed. **The reward is not a
score. It is more world. Desire determines direction; analysis informs
preparation.**
