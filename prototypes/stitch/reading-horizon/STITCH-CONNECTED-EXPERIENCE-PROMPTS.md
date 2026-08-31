# Stitch prompts — connected Mouseion experience

Status: **Historical visual-exploration prompt.** The architecture it explored is
now canonical; generated layouts and detailed interaction treatments are not.
Current guidance lives in
[`doc/design/`](../../../doc/design/README.md).

Use the companion [`DESIGN.md`](DESIGN.md) as Stitch's persistent design context. Paste the **Primary prompt** and **Sample dataset** together for the first generation.

## Primary prompt

```text
Design a connected, desktop-first prototype for Mouseion, a calm scholarly reading environment for language learners. Use only three principal learner-facing concepts:

- My Books: every book Mouseion knows about for this learner.
- Reading Journey: a fluid, provisional ordering of books the learner currently imagines reading.
- Primary Goal: the one book the learner has committed to finishing, when one exists.

Create a coherent flow rather than unrelated screens:
My Books → choose or view a Primary Goal → Reading Journey → compare and freely reorder routes → finish the Primary Goal → see the recalculated Journey → answer “Where next?”

Books, titles, authors, editions, and learner intent should dominate. Metrics are supporting evidence. Only the Primary Goal should feel committed. Every later Journey book must feel provisional, removable, and freely reorderable. The Journey has no destination, schedule, completion state, or progress percentage.

Show Mouseion offering a “Vocabulary-efficient alternative” for the same learner-selected books. Compare it with “Your order” using exact modeled additional vocabulary identities and a short explanation of the assumptions. Never call the alternative best, optimal, or recommended. Let the learner keep their order, adopt the alternative, or move one book manually. A manual preference change should trigger a neutral recalculation such as “Moving this book here adds approximately 63 modeled identities across the remaining Journey,” without warning styling.

Make cumulative effects legible: vocabulary preparation associated with an earlier book can change projected coverage and preparation requirements for later books. Keep current knowledge, conditional projections, reading completion, preparation state, and vocabulary graduation visually distinct. Provide access to exact evidence without turning the main experience into an analysis dashboard.

Include two connected completion branches for the same Primary Goal:

1. Book finished and vocabulary work complete: record the justified vocabulary transition, replace forecasts with values recalculated from actual known vocabulary, show which Journey books changed, then return attention to “Where next?” The first book is a natural candidate, not an automatic commitment.
2. Book finished but vocabulary work remains: acknowledge that the book was finished, keep the remaining vocabulary work visible as a separate fact, do not add its vocabulary to known, do not claim readiness gains, and show current Journey values unchanged with any future effect still clearly conditional.

Also include compact states for no Primary Goal and for no planned next book. Neither should feel like failure or an empty backlog.

Preserve Mouseion’s visual character: calm, serious, scholarly, bibliographic, information-rich but restrained. Express “the road continues beyond the book” through sequence, continuation, changed possibility, and learner choice—not themed copy or imagery. Let Stitch make the composition decisions, but keep an accessible semantic reading order and a responsive narrow-screen interpretation.

Do not use the learner-facing terms Reading Horizon, Campaign, Milestone, Destination, or Journey completion. Avoid fantasy maps, Tolkien references, quests, XP, levels, achievements, project-management timelines, backlog styling, generic SaaS dashboards, excessive cards, optimization-first hierarchy, and language implying that the learner ought to follow the vocabulary-efficient order.

Use the illustrative dataset below. Its numbers belong to Elena’s vocabulary, exact editions, confirmed scopes, and selected 97% token-weighted planning threshold; they are not intrinsic difficulty judgments.
```

## Sample dataset

### Learner and current state

- Learner: **Elena**
- Study language: **Italian**
- Selected planning marker: **97% token-weighted scoped coverage**
- Current Primary Goal: **Il sentiero dei nidi di ragno — Italo Calvino**
- Reading state: **74% through the book**
- Current scoped coverage: **92.8%**
- Vocabulary work: **in progress**
- Justified transition available only when that vocabulary work is completed: **142 exact lemma identities become known**
- Evidence disclosure: current coverage, conditional coverage, additional lemma identities, edition, scope, and analysis provenance are separate facts

### Current Reading Journey

Everything after the Primary Goal is provisional.

1. **Il Gattopardo — Giuseppe Tomasi di Lampedusa** · high personal priority · current 91.6% · 286 additional identities to 97% · after the Primary Goal’s justified transition: 94.1%, 164 additional.
2. **Se una notte d’inverno un viaggiatore — Italo Calvino** · high personal priority · current 95.8% · 61 additional · after the transition: within 97%, 0 additional.
3. **La coscienza di Zeno — Italo Svevo** · interested · current 96.2% · 43 additional · after the transition: within 97%, 0 additional.
4. **Il barone rampante — Italo Calvino** · wants to read · current 95.2% · 74 additional · after the transition: 96.8%, 9 additional.
5. **La Storia — Elsa Morante** · wants to read · current 89.7% · 412 additional · after the transition: 91.4%, 288 additional.
6. **Il nome della rosa — Umberto Eco** · long-held ambition, but not a final destination · current 83.4% · 1,580 additional · after the transition: 84.6%, 1,438 additional · Latin phrases remain outside the Italian lexical measure.

### Route comparison after completion case 1

Use the same six learner-selected books. These stage costs are illustrative conditional modeling, not promises that reading alone teaches vocabulary.

**Your order**

- *Il Gattopardo* +164
- *Se una notte d’inverno un viaggiatore* +0
- *La coscienza di Zeno* +0
- *Il barone rampante* +9
- *La Storia* +288
- *Il nome della rosa* +1,281
- **Total modeled additional identities: 1,742**

**Vocabulary-efficient alternative**

- *La Storia* +288
- *Il Gattopardo* +96
- *Se una notte d’inverno un viaggiatore* +0
- *Il barone rampante* +7
- *La coscienza di Zeno* +0
- *Il nome della rosa* +1,104
- **Total modeled additional identities: 1,495**
- **Difference: 247 fewer modeled identities across these books**

Elena prefers *Il Gattopardo* sooner and moves it back ahead of *La Storia*. Recalculated total: **1,558**, or **63 more modeled identities than the vocabulary-efficient alternative**. Present this neutrally; her preferred order remains the active order.

### Primary Goal completion branches

**Case 1 — Reading and justified vocabulary transition complete**

- Record the book as finished.
- Add the 142 eligible identities to known vocabulary.
- Replace the old forecast with actual recalculation.
- Show the six current-to-new changes listed above, including zero-preparation threshold crossings and books that remain far from the marker.
- Return to the Journey with **Where next?** and *Il Gattopardo* as the first candidate, plus choices to select it, reorder, add from My Books, remove books, or choose something else.

**Case 2 — Book finished, vocabulary work remains**

- Record the book as finished.
- Copy: **Reading finished. Vocabulary review is still in progress. No vocabulary has been added to known yet.**
- Keep all current Journey coverage values unchanged.
- Keep the 142-identity effect explicitly conditional rather than showing it as achieved.
- Return to **Where next?** while preserving access to the unfinished vocabulary work as a separate supporting activity.

### Other books in My Books

- **Se questo è un uomo — Primo Levi** · wants to read · within 97% now · not currently in the Journey.
- **Marcovaldo — Italo Calvino** · interested · 98.1%, within 97% · not in the Journey.
- **Accabadora — Michela Murgia** · interested · 93.6%, 168 additional · not in the Journey.
- **Uno, nessuno e centomila — Luigi Pirandello** · little current interest · 96.7%, 18 additional; low preparation must not outrank desired books.
- **Le città invisibili — Italo Calvino** · wants to read · not assessed; source acquired, scope not reviewed.
- **L’amica geniale — Elena Ferrante** · high personal priority · not currently assessable; no source edition available.
- **La luna e i falò — Cesare Pavese** · formerly interested · evidence needs review because the source edition changed.
- **Il deserto dei Tartari — Dino Buzzati** · interested · analysis running; do not invent readiness.

### Empty and transitional states

- **No Primary Goal:** the Reading Journey remains available and entirely provisional. Any Journey book or eligible book in My Books may be chosen; do not manufacture urgency.
- **No planned next book:** after finishing a Primary Goal, show **Where next?** with a calm path back to My Books. Do not show an empty queue, unfinished plan, or Journey completion message.

## Follow-up exploration prompts

1. **Bibliographic folio**

   Reinterpret the connected prototype as a contemporary scholarly catalogue and reading folio. Reduce containers dramatically; use serif titles, disciplined metadata, fine rules, and typographic changes to distinguish Primary Goal, provisional Journey, route evidence, and the post-completion “Where next?” moment.

2. **Measured continuation**

   Make sequence and cumulative vocabulary effects more spatial without becoming a map or timeline. Use a restrained continuous measure, paired current/projected positions, and clear textual deltas so the changed road ahead is perceptible. Preserve the semantic list and all no-evidence states.

3. **Editorial split workspace**

   Explore a desktop composition where My Books and the editable Reading Journey coexist in a calm split workspace. Keep the Primary Goal visually anchored, make adding/reordering lightweight, and reveal route comparison in context rather than as a separate dashboard. Provide a clear stacked mobile interpretation.

4. **Completion-led experience**

   Make Primary Goal completion and “Where next?” the emotional center. Explore a restrained before/after reveal that distinguishes the two completion cases, shows actual versus unchanged vocabulary state, and lets the recalculated Journey invite a new choice without celebration mechanics or automatic commitment.

5. **Evidence on demand**

   Strip the default experience to books, intent, order, and the next decision. Move most percentages, lemma counts, assumptions, and provenance into precise disclosures. Ensure the vocabulary-efficient alternative remains understandable and auditable without allowing metrics to dominate bibliographic identity.
