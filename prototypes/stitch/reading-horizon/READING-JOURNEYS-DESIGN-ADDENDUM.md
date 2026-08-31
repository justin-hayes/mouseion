# Reading Journeys: design addendum

Status: **Superseded as learner-facing architecture by [`EXPERIENCE-ARCHITECTURE-REASSESSMENT.md`](EXPERIENCE-ARCHITECTURE-REASSESSMENT.md); retained as exploration history.**

Date: 2026-08-31

This addendum extends the reviewed Corpus, Campaign, and Horizon discovery. It does not replace that proposal, define persistence, or create implementation work. It investigates one question:

> **If the learner chooses the destinations, can Mouseion help chart a route through them in which each milestone meaningfully prepares the way for those that follow?**

## Recommendation

**Reading Journey is a useful distinct concept when it is treated as an optional planning lens, not a new obligation container.** It gives Mouseion a coherent layer between literature the learner desires and the one work they have committed to undertake.

The concept is valuable because Horizon and Campaign represent different strengths of intent:

- Horizon can hold broad, durable, and even currently unassessable desire.
- A Journey expresses a foreseeable group of learner-chosen destinations and provisional sequencing preferences.
- A primary goal gives current planning one clear direction.
- A Campaign records an explicit undertaking and its actual preparation and reading state.

A Journey should not own works, campaigns, analyses, or vocabulary. It is a view over their relationships. A work remains on the Horizon whether or not it appears in a Journey; a milestone need not have a Campaign; and a Campaign can retain meaning after a route changes.

The strongest initial hypothesis is **collection-directed journey planning**: the learner selects works they already want to read, states any fixed positions, and asks Mouseion to compare understandable route alternatives. Goal-directed planning is also coherent when the destination and all candidate intermediaries come from learner-expressed desire. Mouseion should not insert unrelated “efficient” books.

The term **optimal route** is too broad on its own. The most defensible first objective is:

> **Among the selected works and learner-fixed ordering constraints, find the order with the fewest total additional lemma identities in the modeled preparation transitions required to reach the selected planning threshold at each prepared milestone.**

This is understandable and reproducible, but only when every modeled transition names its evidence and assumption. It is not a difficulty score, a recommendation about literary value, or a forecast that reading alone will produce knowledge.

## 1. Concepts and the learner intent each represents

| Concept | Learner intent | Strength of commitment | What Mouseion may infer | What Mouseion must not infer |
|---|---|---:|---|---|
| **Reading Horizon** | “This is literature I care about or may want to read.” | Broad and exploratory | These works deserve visibility and, where possible, readiness evidence. | That low preparation means high desire; that every work needs assessment or action. |
| **Reading Journey** | “These are works I want in my foreseeable reading future; help me consider an order.” | Deliberate but provisional | The selected works and stated ordering constraints define the route-planning problem. | That membership is a deadline, queue position, or commitment to prepare every work. |
| **Milestone** | “This work is one selected destination in this Journey.” | Provisional unless separately promoted | Its position and preparation policy may be compared with other selected milestones. | That it owns a Campaign or must produce vocabulary gain. |
| **Primary goal / next milestone** | “Orient my current planning around this work.” | Stronger immediate direction | This work should be visually dominant and can anchor current-versus-projected Horizon effects. | That a Campaign already exists or preparation has been chosen. |
| **Campaign** | “I intend to undertake this work and record its real preparation and reading state.” | Explicit undertaking | Actual Campaign, preparation, reading, and eligible vocabulary-transition semantics apply. | That every Journey milestone is already a Campaign. |

“Primary goal” and “next milestone” may coincide, but they answer slightly different questions. Primary goal is a direction expressed from the Horizon; next milestone is a role inside an ordered Journey. When an active Campaign exists, its work is the present undertaking and should be fixed ahead of provisional future milestones. The learner still explicitly chooses what becomes the next Campaign.

“Milestone” is useful in explanation, but prototypes should test whether repeated milestone language feels like project management. Book titles, authors, and plain actions should carry more weight than the noun.

## 2. Relationship model, not a hierarchy

```text
Reading Horizon / reading field
    contains learner concern for Works
             |
             +---- desire markers and primary-goal role
             |
             +---- 0..n Journey selections and order constraints
                              |
                              +---- milestone role for a Work

Work <---- exact source/scope/analysis evidence
  |
  +---- 0..n Campaigns ---- preparation and reading facts
  |
  +---- current readiness from current language vocabulary

Known vocabulary
  +---- current Horizon readiness
  +---- conditional Journey projections
  +---- recalculated from actual state after a justified transition
```

This model deliberately does not say that Horizon contains Journey, Journey contains Milestone, or Milestone contains Campaign as persisted parents and children. Those may be useful screen relationships without being ownership relationships.

## 3. Journey membership and ordering semantics

### Membership

Adding a work to a Journey should mean only:

> “Include this learner-selected destination in this planning exercise.”

It should not automatically:

- acquire a source;
- start scope review or analysis;
- create or queue a Campaign;
- choose a preparation mechanism;
- assign vocabulary;
- set a date;
- remove the work from the wider Horizon.

Removing a future milestone from a Journey is therefore not abandonment. It changes the plan, not the work’s desire state, evidence, or history. Whether a work can appear in several named Journeys is a persistence question and should remain unresolved until named Journeys themselves prove useful.

### Ordering

Treat order as an editable proposal with explicit constraints, not as a hidden priority score.

A milestone may be:

- **fixed first** — for “this book must be first” or an active Campaign;
- **fixed final destination** — the important work the route is meant to reach;
- **fixed relative to another milestone** — A must precede B;
- **reorderable** — Mouseion may place it anywhere consistent with the fixed constraints;
- **reading only / no modeled preparation** — meaningful in the human Journey but contributes no assumed lexical transition;
- **not comparable** — retained in the Journey while its route effect remains unavailable.

The learner should be able to compare a suggested order with their own order and swap two reorderable milestones without silently changing the accepted plan. “Use this order” should adopt a planning arrangement only; it should not create Campaigns.

A suggested first position is not automatically the primary goal. The interface should ask the learner to confirm **Make this my primary goal** or proceed to the explicit Campaign action when they are ready.

### Avoiding the obligation queue

- Later milestones remain visually quieter than the primary goal.
- Dates, overdue states, completion percentages, backlog language, and queue pressure are absent unless the learner explicitly introduces a separate scheduling need.
- Reordering is ordinary and reversible.
- A Journey can be paused or left incomplete without negative status.
- The canonical Journey view emphasizes books and preparation evidence, not task completion.

## 4. What “optimal route” can honestly mean

### Recommended collection-directed objective

For a single language, a selected planning threshold, a learner-selected set of works, and explicit fixed-order constraints:

1. start from the learner’s current known vocabulary;
2. at each milestone, calculate the exact additional identities required under the named planning rule;
3. apply only the milestone’s explicitly modeled, graduation-eligible vocabulary transition to the conditional state;
4. recompute readiness and preparation requirements for later milestones;
5. compare the sum of **new, distinct identities added by those modeled transitions** across valid orderings.

The route with the lowest total may be described as:

> **Least total planned vocabulary among these books at the selected threshold**

Do not label it simply **Best**, **Easiest**, or **Recommended reading order**. If several routes tie, show them as equivalent. A deterministic tie-break may keep output stable but is not a product preference.

This objective aligns with the learner’s request because every destination is fixed by desire and the only optimized quantity is disclosed preparation. It is still conditional: it says what a reproducible vocabulary-planning model would require, not what the learner will enjoy, finish, remember, or find comfortable.

### Goal-directed objective

When one important work is fixed as the final destination, two quantities can answer different questions:

- **Least total planned vocabulary through the complete Journey** — avoids hiding work that was merely front-loaded.
- **Least preparation remaining at the final destination** — shows which order creates the strongest lexical preparation before arrival, even if it requires more work earlier.

These should remain separate alternatives, not be combined into a weighted score. If one route minimizes total preparation and another minimizes final-stage preparation, present the trade-off in exact lemma identities and let the learner choose.

Initially, goal-directed optimization should use only works the learner has selected or already marked as desired. Mouseion may ask which desired works are eligible intermediaries; it should not recommend arbitrary literature because of lexical overlap.

### Objectives not recommended as the primary meaning

- **Maximize cumulative readiness gain.** This can favor long or lexically overlapping works without answering how much preparation the learner undertakes.
- **Maximize books crossing a threshold.** This rewards count and can subordinate important desired works to cheap threshold crossings.
- **Minimize preparation for the final work only.** Used alone, this hides preparation shifted into earlier milestones.
- **Balance desire and efficiency in one score.** Desire is learner authority, not a numeric feature for Mouseion to weight.
- **Structural difficulty plus lexical preparation.** Structural signals should remain separate cautions, not become hidden route weights.

## 5. Defensible calculations and projection language

### Three levels of evidence

Journey prototypes should distinguish three projection strengths:

1. **Supported Campaign projection** — the actual active Campaign has an exact set of identities eligible to graduate under an accepted completion transition. Mouseion may recompute later works using that set and say **After this Campaign, if its eligible vocabulary is added to known**.
2. **Named planning scenario** — no accepted active-Campaign transition exists yet, but Mouseion can deterministically identify a candidate set under a disclosed rule. A prepared but not active Campaign also remains conditional under the current one-active contract. Mouseion may say **If these 312 candidate identities later become known under an eligible preparation transition**. It must not say they will be learned.
3. **No modeled lexical effect** — the milestone is reading only, lacks comparable evidence, uses an unsupported language, has stale or questionable analysis, or has no justified vocabulary transition. Preserve the milestone and say why no later delta is projected.

Only the first level is already close to the accepted active-Campaign projection. A multi-step Journey built from future candidate sets requires a new accepted planning-scenario contract before it can become product behavior. The prototype should expose that distinction rather than smoothing it away.

### Stage calculation

For each milestone, keep these quantities separate:

- **Current now** — coverage from the work’s exact analysis and the learner’s actual known vocabulary today.
- **Projected on arrival** — conditional coverage after only the earlier modeled vocabulary transitions in this route.
- **Preparation at this milestone** — the exact new identity count needed under the selected threshold and named planning rule at that stage.
- **Modeled transition** — the exact subset that would enter the next conditional vocabulary state, if justified.
- **Effect on later milestones** — coverage delta and reduced additional-identity requirement, calculated independently against each later work’s exact evidence.

Do not blend projected-on-arrival values into current coverage. A route remains a chain of conditional scenarios until actual transitions occur.

### Reproducibility basis

A comparable route needs:

- one learner and one language-scoped vocabulary basis;
- an exact source, confirmed scope, and immutable analysis for every quantified work;
- a named 95%, 97%, or 99% planning threshold, or an explicitly selected per-milestone preparation policy;
- the accepted lemma identity and wildcard matching rules;
- the deterministic book-local occurrence ordering and reachability rules from ADR 0025;
- exact generated, reserved, graduated, imported, and excluded-vocabulary treatment;
- the exact identity set assumed to transition after each prepared milestone;
- current, projected, and comparison timestamps or revision identities.

The whole-scope threshold investment is not the same as an Anki Deck vocabulary count. A Journey must not use the former to imply that the latter has been prepared, nor use generated vocabulary as known. If a chosen preparation mechanism produces a different set, downstream projections must use that mechanism’s actual eligible transition set.

### Human Journey versus lexical model

The interface should present two related but distinct narratives:

```text
Human reading journey
Book A → Book B → Book C → destination work

Modeled lexical effect
current known vocabulary
  → eligible transition from A, if completed
  → eligible transition from B, if completed
  → no modeled change for reading-only C
  → recomputed destination readiness
```

Reading history, Campaign state, preparation state, and vocabulary knowledge remain separate ledgers. Finishing a reading-only milestone can be personally important and advance the human Journey while leaving the lexical projection unchanged.

## 6. Realistic six-work prototype scenario

All values below are deliberately illustrative. They describe one fictional learner, exact editions/scopes, a selected Italian 97% planning threshold, and the stated conditional transition model. They are not claims about intrinsic difficulty.

| Work | Journey role | Current now | Projected on arrival in the learner’s order | Preparation at that stage |
|---|---|---:|---:|---:|
| Italo Calvino, *Il sentiero dei nidi di ragno* | Primary goal; fixed first | 92.8% | 92.8% | +312 identities to 97% |
| Carlo Levi, *Cristo si è fermato a Eboli* | Reorderable | 91.6% | 94.1% after milestone 1 | +184 identities |
| Primo Levi, *Se questo è un uomo* | Reorderable | 93.4% | 95.8% after milestones 1–2 | +96 identities |
| Italo Calvino, *Il barone rampante* | Reading only; reorderable | 97.3% | Within threshold | No modeled transition |
| Italo Calvino, *Se una notte d’inverno un viaggiatore* | Reorderable | 88.9% | 93.1% after earlier modeled transitions | +328 identities |
| Umberto Eco, *Il nome della rosa* | Fixed final destination | 82.7% | 91.8% before final preparation | +760 identities to 97% |

The learner’s order models **1,680** distinct additional identities across the six stage transitions.

A route comparison swaps the second and third milestones:

```text
Learner order
Il sentiero (+312) → Cristo (+184) → Se questo è un uomo (+96)
→ Il barone (no modeled effect) → Se una notte (+328) → Il nome della rosa (+760)
Total modeled additional identities: 1,680

Alternative after swapping milestones 2 and 3
Il sentiero (+312) → Se questo è un uomo (+122) → Cristo (+139)
→ Il barone (no modeled effect) → Se una notte (+302) → Il nome della rosa (+711)
Total modeled additional identities: 1,586
Difference: 94 fewer identities under this scenario
```

The interface should explain *why* the counts changed: the exact overlap between the candidate or graduation-eligible identity set at one milestone and token occurrences in later exact analyses. It should also state that *Il barone rampante* remains a meaningful reading milestone even though this scenario assigns it no lexical benefit.

## 7. Experience concepts

These concepts are deliberately different interaction models. They use the same evidence and can be evaluated without deciding persistence.

### Concept A — The route ledger (recommended canonical concept)

A vertical, typographic planning document makes cumulative change legible one milestone at a time. It is closer to a scholarly itinerary than a schedule.

```text
YOUR ITALIAN READING JOURNEY                         97% planning threshold
Current vocabulary basis · updated today                 [Compare routes]

PRIMARY GOAL · FIXED FIRST
1  Il sentiero dei nidi di ragno — Italo Calvino
   Current now 92.8%  |  +312 identities to 97%
   If eligible vocabulary later graduates: carry this set forward
   [View exact evidence] [Change primary goal]

        ↓ conditional vocabulary state: current + 312

2  Cristo si è fermato a Eboli — Carlo Levi        REORDERABLE
   Current now 91.6%  |  Projected on arrival 94.1%
   +184 at this stage  |  62 fewer than preparing from today
   [Move earlier] [Move later] [View overlap]

        ↓ conditional vocabulary state: previous + 184

3  Se questo è un uomo — Primo Levi                 REORDERABLE
   Current now 93.4%  |  Projected on arrival 95.8%
   +96 at this stage

4  Il barone rampante — Italo Calvino               READING ONLY
   Within 97% now · no vocabulary transition modeled

…

FINAL DESTINATION · FIXED
6  Il nome della rosa — Umberto Eco
   Current now 82.7%  |  Projected on arrival 91.8%
   +760 at destination to reach 97%
```

**Primary interaction:** reorder future milestones with explicit move controls or an optional drag enhancement; every change recalculates a visible conditional chain. A “what changed” summary receives focus after a keyboard move without moving focus unexpectedly.

**Strengths:** the cause-and-effect sequence is explicit; current and projected values can coexist without blending; exact evidence sits near each claim; it degrades naturally to server-rendered ordered forms; narrow layouts preserve the same reading order.

**Risk:** a long ledger can feel procedural. Strong bibliographic typography, quiet labels, and the absence of dates/checklists are essential to keep it about literature rather than project tracking.

### Concept B — Two-route reading table

A dedicated comparison surface places two complete orders side by side on wide screens and one after another on narrow screens. It is optimized for questions such as “What happens if I swap these two?”

```text
COMPARE TWO ORDERS
Same six learner-selected books · same 97% threshold · same evidence basis

YOUR ORDER                         ALTERNATIVE: SWAP 2 AND 3
1  Il sentiero        +312         1  Il sentiero        +312
2  Cristo             +184         2  Se questo è…       +122
3  Se questo è…        +96         3  Cristo             +139
4  Il barone      no effect        4  Il barone      no effect
5  Se una notte       +328         5  Se una notte       +302
6  Il nome della rosa +760         6  Il nome della rosa +711

Total modeled: 1,680                  Total modeled: 1,586
Final arrives at: 91.8%               Final arrives at: 92.0%
                                           94 fewer identities

[Keep my order] [Use alternative order] [Inspect changed evidence]
```

**Primary interaction:** choose a swap or alternate objective, then inspect only the milestones whose stage calculation changed. “Use alternative order” updates the provisional Journey, not Campaign state.

**Strengths:** trade-offs and fixed constraints are easy to compare; it avoids an opaque “optimizer says so” result; near-ties can be shown honestly.

**Risk:** side-by-side numbers can become dashboard-like and visually dense. Use it as a focused comparison step, not the Journey home. On compact screens, provide sequential route summaries followed by a change table; do not force horizontal scrolling.

### Concept C — Horizon with a conditional route trace

The existing primary-goal Horizon remains the dominant discovery field. Selected works receive a quiet numbered route trace and paired current/projected positions. Activating **Plan a journey with these works** reveals the provisional order without turning the whole Horizon into a map.

```text
YOUR READING HORIZON

Primary goal
Il sentiero dei nidi di ragno
92.8% now ─────────────── 97% after +312 modeled identities

Selected future direction
2  Cristo             91.6% now ─── 94.1% after milestone 1
3  Se questo è…       93.4% now ─── 95.8% after milestones 1–2
4  Il barone           within threshold · no modeled lexical change
5  Se una notte       88.9% now ─── 93.1% projected on arrival
6  Il nome della rosa 82.7% now ─── 91.8% projected on arrival  FINAL

[Open route ledger] [Compare another order]

Other desired works remain in the wider Horizon below, unnumbered.
```

**Primary interaction:** select or remove desired works from a provisional route while keeping all desired literature visible. The trace shows how the primary goal’s effect expands into a multi-step scenario; route editing moves to the ledger.

**Strengths:** preserves the promising current-versus-projected Horizon concept and makes the relationship between possibility and selected direction immediately visible.

**Risk:** multiple paired positions can suggest a fantasy map, Gantt chart, or continuous score. Use exact horizontal measures only where one documented preparation quantity controls position; accompany every trace with text; omit position when evidence is unavailable. This concept should be a bridge into Journey planning, not the canonical editing surface.

### Recommended composition

Use Concept C to enter Journey planning from the Horizon, Concept A as the canonical planning and evidence surface, and Concept B as a deliberate comparison mode. This is more coherent than choosing one screen to handle discovery, planning, optimization, and evidence inspection at once.

## 8. Interaction with the primary-goal Horizon concept

The existing one-goal interaction remains the smallest useful unit:

1. the learner marks or selects a strongly desired work;
2. it becomes the primary goal;
3. Horizon shows the actual current state and the conditional effect of its justified vocabulary transition across other assessed works.

Journey planning expands that interaction only when the learner selects several foreseeable destinations:

```text
Horizon
  discover or mark desire
      → make one work the primary goal
      → select several desired works for a Journey

Journey
  state fixed and reorderable constraints
      → compare a learner order with one or more disclosed alternatives
      → retain one next milestone as the immediate direction

Campaign
  explicitly undertake the next milestone
      → prepare and/or read under accepted Campaign semantics
      → complete, abandon, or finish reading without a lexical transition

After the Campaign
  use actual known vocabulary, not the old forecast
      → recalculate the remaining Journey
      → show deviations from the prior scenario without calling them failure
      → update the wider Horizon as well
```

Completing a Campaign does not automatically create or activate the next one. The Journey advances from historical milestone to provisional next milestone; the learner confirms the next primary goal or Campaign. If the actual graduated set differs from the old scenario, all remaining projected values are replaced by newly calculated conditional values with an explanation.

## 9. Edge cases and epistemic limitations

| Case | Required behavior |
|---|---|
| Zero or one selected work | Horizon and primary goal are sufficient; do not manufacture a Journey or optimization. |
| Two selected works | Offer a simple swap comparison if both are reorderable; avoid “optimization” theater. |
| Active Campaign is in the Journey | Fix it as the present undertaking. Reordering future milestones must not mutate or abandon it. |
| Milestone is reading only | Preserve it in the human order; show **No modeled vocabulary transition** and leave later lexical projections unchanged at that step. |
| Work already meets the threshold | Show zero threshold preparation. Do not treat reading it as a vocabulary gain. |
| No preparation mechanism or eligible transition is selected | Show a candidate planning scenario, not an after-Campaign claim. |
| Campaign is abandoned or completes without graduation | Recompute from actual vocabulary; remove the forecast benefit and explain the changed route. |
| Vocabulary import or manual known-vocabulary change occurs | Recompute every later stage from the new actual basis; the prior route is stale. |
| Several languages appear | Keep the human Journey if useful, but partition or omit lexical optimization. Vocabulary effects do not cross language boundaries. |
| Analysis missing, running, failed, stale, or materially questionable | Retain the milestone and its learner-fixed position; show no comparable numeric route effect until trustworthy evidence exists. |
| Target unreachable under ADR 0025 exclusions | Show the reason and no invented lemma distance. The route cannot claim that milestone reaches the target. |
| Unsupported or source-unavailable final destination | The destination remains meaningful, but no numeric “optimal route to 97%” is available. |
| Source, edition, scope, or comparison analysis changes | Invalidate affected route calculations and require a visible new basis; do not silently substitute a “latest” result. |
| Two routes tie or differ by very few identities | Present them as equivalent or nearly equivalent in exact terms; do not create a false winner. |
| Learner constraints remove all freedom | Show the learner’s fixed route and its evidence; do not pretend to optimize. |
| Structural warning conflicts with lexical efficiency | Keep the warning separate. Do not hide or weight it into the route objective. |
| A lemma has different parts of speech across works | Preserve the accepted `(lemma, UPOS)` identity and wildcard semantics; do not count spelling overlap alone. |
| Very large selected collection | Ask the learner to narrow the Journey or compare bounded alternatives; Horizon remains the broad field. Do not render a massive route canvas. |
| Repeated work or different editions | Preserve exact work/edition/evidence identity. Do not deduplicate solely by title. |

Projection language should avoid certainty words such as **will know**, **will learn**, **unlocks**, or **guarantees 97%**. Prefer **if these eligible identities later become known**, **projected under this route**, and **no modeled lexical effect**.

## 10. Implications for the experience-architecture proposal

Reading Journeys refine the relationship-graph proposal rather than replace it.

### Explore remains the possibility center

The Reading Horizon remains the default place to answer:

- What literature do I care about?
- Which evidence is trustworthy?
- What can I read or prepare for now?
- What changes after the primary goal or active Campaign?

Journey planning is a secondary mode reached from selected desired works, not automatically a new primary navigation destination. Prototype **Plan a reading journey** within Explore before considering a permanent **Journeys** destination.

### Reading remains the undertaking center

Campaigns remain under the Reading experience. A Campaign is stronger than Journey membership and should not be created for every future milestone. This distinction may simplify the earlier proposal’s ambiguous “planned campaigns” area:

- Journey = provisional future sequence;
- primary goal = immediate planning direction;
- planned Campaign = explicit undertaking not yet active;
- active Campaign = present undertaking.

The overlap between a planned Campaign and a next milestone needs a settled product boundary before planner handoff. The interface must not expose two competing queues.

### Proposed experience flow

```text
Explore / Reading Horizon
    → express desire
    → make primary goal
    → select desired works for Journey planning
    → compare constrained route alternatives
    → adopt a provisional order
    → explicitly plan or begin the next Campaign

Reading / Campaign
    → conduct real preparation and reading
    → complete or abandon under accepted semantics
    → apply only justified vocabulary transitions

Journey and Horizon
    → recalculate from actual state
    → show what changed from the prior conditional route
    → invite, but do not force, the next commitment
```

### Architecture decisions this addendum does not make

This exploration does not decide:

- whether Journey is persisted, named, singular, language-scoped, or shareable;
- whether order is stored as positions, constraints, or derived alternatives;
- whether route calculations are persisted or recomputed;
- URL structure, schemas, APIs, jobs, or optimization algorithms;
- changes to current Campaign persistence or ADR 0027;
- whether the existing planned-Campaign queue remains.

Those decisions follow only if prototype evaluation supports the concept and product semantics are accepted.

## 11. Decisions required before planner handoff

1. **Is Journey membership a durable learner state or a temporary planning selection?** Validate the experience before deciding persistence.
2. **Does “Reading Journey” feel inspiring but serious, or does it imply travel/gamification?** Test **Journey**, **Reading plan**, and plain **Selected books** language.
3. **What exact action distinguishes Want to read, Add to Journey, Make primary goal, and Plan to read?** Each needs a clear strength of intent.
4. **Can a Journey contain only one language for route calculations?** If human cross-language Journeys are allowed, define their non-comparable presentation.
5. **What is the initial preparation policy?** One shared threshold, learner-selected per-milestone thresholds, or explicit read-without-preparation choices.
6. **Which identity set powers a future-milestone transition before an actual prepared Campaign exists?** A whole-scope threshold prefix, future mechanism candidate set, or no multi-step projection until preparation is real.
7. **What accepted evidence makes a planning candidate eligible to become conditionally known?** Do not reuse deck assignment or reading completion as proof.
8. **Which route objective is offered first?** Recommend least total planned vocabulary for all learner-selected works; validate wording and comprehension.
9. **Should goal-directed comparison also offer least preparation remaining at the final destination?** If so, keep it separate from total Journey preparation.
10. **May Mouseion omit a desired intermediary, or only reorder all selected works?** Recommend reorder all selected works initially; omission changes the learner’s destinations.
11. **How are fixed first, fixed final, relative-order, and reorderable constraints expressed accessibly?** Drag cannot be the only control.
12. **When does a suggested first work become the primary goal?** Recommend explicit confirmation, never silent promotion.
13. **How do Journey and planned Campaign lists coexist?** Avoid duplicate representations of the same future intent.
14. **What happens when actual Campaign graduation differs from the modeled set?** Define recalculation, stale-plan messaging, and whether prior projections need any historical visibility.
15. **How much evidence is visible in the route versus disclosed on demand?** Learners must understand the cause without every row becoming an analysis report.
16. **How should near-ties and incomplete routes be described?** Exact differences and unavailable segments must prevent false certainty.
17. **Does the route ledger remain comprehensible with missing evidence, reading-only milestones, long titles, narrow viewports, keyboard reordering, and reduced motion?** Validate these states before visual polish.
18. **Does the concept improve decisions over the simpler primary-goal Horizon?** If learners do not understand or use multi-step effects, keep the one-goal model and do not add another noun.

## Reference context

- [Corpus, Campaign, and Horizon discovery](../../../doc/design/corpus-campaign-horizon-discovery.md)
- [Mouseion design principles](../../../doc/design/principles.md)
- [Learning Campaign workflow](../../../doc/design/workflows/learning-campaign.md)
- [ADR 0025: analysis coverage and threshold metric contract](../../../doc/adr/0025-analysis-coverage-threshold-metrics.md)
- [ADR 0027: Campaigns and vocabulary graduation](../../../doc/adr/0027-learning-campaigns.md)
- [Stitch design context](DESIGN.md)
- [Initial Reading Horizon prototype brief](STITCH-PROTOTYPE-BRIEF.md)

## Conclusion

The hypothesis is coherent, with an important limit:

> **Mouseion can chart a route only through destinations the learner has chosen, and only along lexical transitions it can name and justify.**

A Reading Journey can make the cumulative value of preparation visible without turning literature into a score or queue. The most promising experience is a bibliographic route ledger entered from the primary-goal Horizon, paired with an explicit two-route comparison. It should show the human sequence of books and the narrower modeled lexical sequence as related but separate layers.

If the projections require Mouseion to pretend that reading equals learning, that generated cards equal knowledge, or that lexical efficiency determines desire, the concept should be rejected. If learners can instead see that one chosen undertaking may make later chosen literature more approachable—and can inspect exactly why—then Reading Journey provides a credible new layer between **the literature I desire** and **the book I am undertaking now**.

> **The learner chooses the destinations; Mouseion helps chart the route.**
>
> **Desire determines direction; analysis informs preparation.**
>
> **The reward is not a score. It is more world.**
