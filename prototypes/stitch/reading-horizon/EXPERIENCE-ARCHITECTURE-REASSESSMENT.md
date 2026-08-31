# Experience architecture reassessment

Status: **Conceptual direction frozen for visual exploration — not yet accepted product behavior**

This note reassesses the prior Corpus / Campaign / Horizon proposal after the Reading Journey exploration. Its purpose is subtraction.

## Recommendation

Adopt **My Books / Reading Journey / Primary Goal** as the principal learner-facing conceptual model.

| Concept | Learner intent | Boundary |
|---|---|---|
| **My Books** | Books Mouseion knows about for me. | Broad collection: desired or not, assessed or not, current or distant. It is not a readiness ranking. |
| **Reading Journey** | Where I currently imagine I may go. | A fluid ordering of learner-selected books. It has no destination, completion state, schedule, or implied promise. |
| **Primary Goal** | This is the book I intend to finish. | The one meaningful commitment when present; there is at most one, and Mouseion may also be between Goals. Preparation, reading, and evidence orient around it; no next commitment is automatic. |

This model is sufficient as the principal architecture. Analysis, preparation, evidence, progress, and vocabulary remain important states and capabilities, but they do not need to become peer organizing nouns.

## What is demoted or removed

- **Reading Horizon:** demote from learner-facing concept to an internal interaction metaphor. Preserve its strongest behavior—showing changing possibility and current-versus-projected readiness—inside My Books and the Journey.
- **Campaign:** remove from primary learner-facing language if Primary Goal can present its essential facts in ordinary reading language. Preserve Campaign internally for undertaking state, reading/deck progress, reservation, abandonment, provenance, and atomic vocabulary graduation until those contracts are deliberately revised.
- **Corpus:** keep as an internal analysis/evidence term. The learner sees books, exact editions, confirmed scopes, and analysis provenance.
- **Milestone:** retain at most as an internal view role or occasional explanatory label. A numbered book in the Journey does not need another product noun.
- **Destination:** remove. There is no final Journey work.
- **Journey completion:** remove completely. A Journey is reconsidered, not completed.

The important semantic loss from hiding Campaign is its explicit distinction between **book finished**, **deck reviewed**, and **vocabulary graduated**. Primary Goal can replace Campaign as the learner's expression of commitment only if those independent facts and the consequential vocabulary transition remain visible without requiring the learner to understand the Campaign object.

## Journey semantics

Reading Journey should remain permanently fluid:

- membership is explicit and reversible;
- later order is provisional;
- no dates, overdue states, completion percentage, or queue pressure;
- vocabulary-efficient ordering applies only to learner-selected books;
- the learner may accept, partially adopt, ignore, or freely alter an alternative;
- every change produces a neutral recalculation, not a warning or correction;
- the Primary Goal is anchored as the present undertaking; everything after it remains reorderable;
- once the Goal is finished, the first remaining book is a natural candidate, never an automatic commitment.

Use language such as **Vocabulary-efficient alternative** and **94 fewer modeled additional identities across these books**. Do not use **best route**, **optimal Journey**, or **recommended next book**.

> **Mouseion can optimize a lexical property of the route. It cannot optimize the learner's reading life.**

## Immediately before and after Primary Goal completion

Before completion, show the exact consequences of the action:

- what reading fact will be recorded;
- whether any preparation condition remains;
- which vocabulary identities, if any, are eligible to become known;
- that later Journey projections will be recalculated from actual state.

After completion:

1. record what actually happened;
2. apply only justified vocabulary transitions;
3. replace the old forecast with recalculation from actual vocabulary;
4. show which later books changed, which did not, and why;
5. return attention to the reordered Journey;
6. present the first remaining book as the next candidate;
7. offer **Choose as Primary Goal**, reorder, remove, add from My Books, or choose another book.

Do not auto-create or activate the next undertaking. Do not show Journey progress.

## Experience principle

Adopt **The road continues beyond the book** as an internal experience principle, with the more actionable interpretation:

> **You do not plan the whole road. You choose where to go next.**

This improves the architecture because it gives completion a rhythm without inventing a finish line: undertake one book, reveal the changed conditions ahead, then choose again. It should guide information flow and transitions, not appear as themed copy or illustration.

Reject fantasy maps, quests, Tolkien references, medieval styling, route decoration, achievements, XP, levels, and artificial celebration. The intended quality comes from continuation, uncertainty, possibility, and learner choice.

## Contract conflicts to resolve later

The simplified conceptual direction can be frozen for visual exploration, but implementation planning must eventually reconcile:

1. **Primary Goal completion versus Campaign completion.** ADR 0027 completes a Campaign only when the book is finished **and** the deck is reviewed; Primary Goal is framed as finishing a book. The UI cannot graduate vocabulary merely because reading finished.
2. **Primary Goal without a prepared deck.** The current Campaign requires one source book and one prepared deck. The new commitment concept should also support reading-only or pre-preparation goals.
3. **Fluid Journey versus learning queue.** Accepted design currently exposes queued Campaigns ordered for later. A provisional Journey and a commitment queue must not coexist as duplicate plans.
4. **My Books versus My Library.** Accepted navigation and terminology use My Library for acquired books. A broader My Books collection may also include metadata-only, unassessed, unsupported, or merely desired works.
5. **Learner-facing Campaign terminology.** Current terminology, workflows, screen inventory, and completion copy explicitly name Campaign and must eventually be amended if it becomes internal.
6. **Post-completion destination.** Existing Campaign flow returns to Campaign history; the proposed rhythm returns to the recalculated Journey and a fresh choice.

These are real contract changes, but they do not prevent another Stitch pass. Visual exploration should specifically test reading-only goals, book-finished/deck-not-reviewed state, consequential vocabulary copy, and the uncommitted “Where next?” moment.

## Freeze decision

**Recommend freezing the learner-facing conceptual direction here and returning to Stitch.**

The model is coherent if one boundary remains explicit:

> Primary Goal replaces Campaign as the learner's commitment concept, but it does not erase the internal state and epistemic rules that currently make vocabulary graduation honest.

The next visual exploration should therefore use only:

- **My Books** for the broad field;
- **Reading Journey** for provisional future order and lexical route comparison;
- **Primary Goal** for the present commitment;
- ordinary language for preparation, reading, evidence, and completion.

The completion experience should end not with progress toward a Journey finish, but with the changed books ahead and one open question:

> **Where next?**
