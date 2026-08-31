# Stitch prototype brief — Explore / Reading Horizon

Status: **Superseded visual-exploration prompt; retained as historical evidence.**
Its Explore / Reading Horizon destination, active Campaign language, and
threshold-led structure are not canonical. See
[`information-architecture.md`](../../../doc/design/information-architecture.md)
and the later connected prompt/synthesis.

Paste the **Initial prompt** and **Sample content** together into Stitch. Use the companion `DESIGN.md` as persistent design context.

## Initial prompt

Design a responsive first concept for Mouseion’s **Explore** destination, headed **Your reading horizon**, for an Italian learner planning at a selected **97% token-coverage threshold**.

This is a calm, serious, scholarly reading environment—not a dashboard. The screen should help the learner understand, in this order:

1. which literature they care about;
2. the one book they are undertaking now;
3. what currently meets the selected planning threshold or would require preparation;
4. how much preparation desired books may require;
5. what would change **after the active campaign**;
6. where they might deliberately choose to go next.

Make learner desire and commitment visually primary. Readiness is exact preparation evidence, never a recommendation, literary judgment, or score. Default ordering should preserve **Want to read**, **Planned**, and **Active** intent rather than ranking the cheapest books first. Include a secondary sort by preparation required.

Explore a restrained visual field rather than a table alone: stable bibliographic rows or book groups may use a quiet horizontal measure, depth bands, or aligned positions to show exact additional lemma identities. For works affected by the active campaign, show a subtle current marker, projected marker, short connecting trace, and textual delta. Do not blend projected and current values. Keep an accessible semantic list as the clear reading structure.

Use a compact top area with language, the 97% threshold selector, and a two-state lens: **Current** / **After the active campaign**. Give the active campaign a quiet but prominent anchored region. Then foreground wanted/planned works; demote low-interest discoveries and separate works whose evidence is unavailable or needs review. Every book shows title, author, desire/commitment, concise interpretation, exact evidence when trustworthy, and one appropriate action or evidence link.

Create one desktop composition and show how it collapses on mobile. Typography and bibliographic identity should carry the design. Use cards sparingly, thin rules, modest corners, no shadows, no decorative gradients, no fantasy map, no game metaphor, no XP, no achievements, no “unlocking,” and no composite difficulty score.

The governing principles are:

> The reward is not a score. It is more world.
>
> Desire determines direction; analysis informs preparation.

Use the realistic illustrative content below. Do not treat the numbers as intrinsic book difficulty; they belong to this learner, edition, confirmed scope, and vocabulary state.

## Sample content

**Comparison context**

- Language: Italian
- Planning marker: 97% token-weighted coverage
- Learner name: Elena
- Active campaign: *Il sentiero dei nidi di ragno* by Italo Calvino
- Campaign state: Reading in progress; book not yet marked finished; Anki review in progress
- Conditional effect: 142 eligible campaign lemmas would become known only when the campaign is completed
- Evidence copy: “Additional lemmas” means exact whole-scope lemma identities needed to reach Elena’s selected 97% marker; it is not the Anki deck count

**20 works**

1. **Il sentiero dei nidi di ragno — Italo Calvino** · **Active campaign** · current scoped coverage 92.8% · 142 campaign lemmas eligible to graduate on completion · action: **View current campaign**.
2. **Il Gattopardo — Giuseppe Tomasi di Lampedusa** · **Want to read · high personal priority** · 91.6% current coverage · 286 additional lemmas · after campaign: 94.1%, 164 additional (−122) · trusted evidence.
3. **Se una notte d’inverno un viaggiatore — Italo Calvino** · **Want to read · high personal priority** · 95.8% · 61 additional · after campaign: **within 97% marker**, 0 additional (−61) · trusted evidence.
4. **La coscienza di Zeno — Italo Svevo** · **Planned** · 96.2% · 43 additional · after campaign: **within 97% marker**, 0 additional (−43) · trusted evidence · action: **Review plan**.
5. **Il nome della rosa — Umberto Eco** · **Want to read · long-held ambition** · 83.4% · 1,580 additional · after campaign: 84.6%, 1,438 additional (−142) · trusted evidence with note: dense Latin phrases remain outside the Italian lexical measure.
6. **La Storia — Elsa Morante** · **Want to read** · 89.7% · 412 additional · after campaign: 91.4%, 288 additional (−124) · trusted evidence.
7. **Se questo è un uomo — Primo Levi** · **Want to read** · **within 97% marker** · 97.4% · 0 additional · trusted evidence · action: **Plan to read**.
8. **Marcovaldo — Italo Calvino** · **Interested** · **within 97% marker** · 98.1% · 0 additional · trusted evidence.
9. **Novecento — Alessandro Baricco** · **Interested** · **within 97% marker** · 97.6% · 0 additional · trusted evidence.
10. **Il barone rampante — Italo Calvino** · **Want to read** · 95.2% · 74 additional · after campaign: 96.8%, 9 additional (−65) · trusted evidence.
11. **Una questione privata — Beppe Fenoglio** · **Interested** · 94.5% · 103 additional · trusted evidence; high lexical concentration means a relatively small set has broad effect.
12. **Accabadora — Michela Murgia** · **Want to read** · 93.6% · 168 additional · trusted evidence; Sardinian terms are separately noted, not folded into a score.
13. **Uno, nessuno e centomila — Luigi Pirandello** · **Little current interest** · 96.7% · 18 additional · trusted evidence · show as a quiet low-preparation discovery, never above wanted books.
14. **Le città invisibili — Italo Calvino** · **Want to read** · **Not assessed** · source acquired; scope has not been reviewed · action: **See what preparation might help**.
15. **Lessico famigliare — Natalia Ginzburg** · **Interested** · **Not assessed** · bibliographic record saved; no source edition attached · action: **Find a source**.
16. **La luna e i falò — Cesare Pavese** · **Want to read** · **Needs review** · previous evidence showed 93.1% and 201 additional, but the source edition changed · do not place quantitatively until scope is confirmed · action: **Review evidence**.
17. **Cristo si è fermato a Eboli — Carlo Levi** · **Interested** · **Needs review** · 94.0% and 137 additional are available, but extraction may include notes and index material · keep metrics visible but qualify comparison · action: **Review scope**.
18. **Il piacere — Gabriele D’Annunzio** · **Maybe later** · **Target not currently reachable** · 88.9% current coverage; no invented lemma distance because excluded passages prevent a valid 97% calculation · action: **Understand the evidence**.
19. **L’amica geniale — Elena Ferrante** · **Want to read · high personal priority** · **Not currently assessable** · Mouseion has no assessable EPUB/source for this work · no coverage or distance · action: **Keep on horizon**.
20. **Il deserto dei Tartari — Dino Buzzati** · **Interested** · assessment running · durable status: **Analysis running** · no speculative readiness placement · action: **View analysis status**.

**Useful compact disclosures**

- “Within 97%” means no additional whole-scope vocabulary is required for this selected planning marker. It does not promise comfort or measure general proficiency.
- “After the active campaign” is conditional on completing *Il sentiero dei nidi di ragno* and graduating its eligible vocabulary; current knowledge is unchanged.
- Structural signals and evidence quality remain separate from vocabulary preparation.
- Each assessed work can link to its exact edition, scope, and analysis evidence.

## Follow-up exploration prompts

1. **More bibliographic / typographic**

   Reduce visible containers by half. Make the page read like a contemporary scholarly catalogue: stronger serif book titles, author and edition alignment, fine rules, marginal evidence notes, and desire expressed through typographic emphasis rather than badges. Preserve all states and current/projected values.

2. **More spatial / horizon-oriented**

   Keep the semantic list and desire-led ordering, but make changing possibility more perceptible through a restrained horizontal field. Use an explicit preparation scale, quiet threshold line, paired current/projected positions, textual deltas, and reduced-motion behavior. Do not introduce landscape, routes, territories, constellations, or game-board imagery.

3. **Stronger learner desire**

   Redesign the hierarchy so Elena’s Want to read, Planned, and long-held ambition signals dominate before any coverage number. Make the distant but deeply desired *Il nome della rosa* feel more important than low-preparation, low-interest *Uno, nessuno e centomila* without hiding exact preparation evidence.

4. **Less dashboard-like**

   Remove summary tiles, KPI cards, and most pills. Replace them with a single coherent document flow: current undertaking, desired literature, changing possibilities, and evidence needing attention. Use spacing, typography, and thin dividers for structure; keep only one primary action per region.

5. **Clearer current versus projected movement**

   Strengthen the distinction between **Current** and **After the active campaign** without making two separate dashboards. Show the conditional nature of projection, paired values and exact deltas for affected works, a clear unchanged state, and a mobile pattern that does not rely on animation or horizontal position alone.
