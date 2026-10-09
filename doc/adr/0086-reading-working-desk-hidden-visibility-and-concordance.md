# ADR 0086: Reading Working desk, independent Hidden visibility, and corpus-wide Concordance

Status: **Accepted; code implementation complete in the repository** · Production cutover and corpus-scale acceptance evidence pending · Date: 2026-10-08 · Author: Justin + OpenCode

Specified by [issue #1580](https://github.com/justin-hayes/mouseion/issues/1580) and the planning decisions it cites (#1570–#1579); promoted by [issue #1581](https://github.com/justin-hayes/mouseion/issues/1581). This ADR records the **accepted target**. Its code is complete in the repository; the production cutover (section 5) and corpus-scale acceptance evidence are pending, so production behavior is not yet claimed. Until that cutover is performed, ADRs 0078, 0050, 0043, 0081, 0084, and 0085 remain in force for deployed data except where this record states otherwise. Accepted by Justin on 2026-10-08, this ADR permits dependent SQL work in the implementing slices; it does not authorize any production maintenance operation.

Supersedes the lifecycle and disposition clauses of [ADR 0078](0078-book-dispositions-and-current-reading.md) listed below. Reconciles [ADR 0050](0050-active-study-language.md) and [ADR 0043](0043-study-languages-derived-settings-removed.md). Amends the clauses of ADRs 0081, 0084, and 0085 listed below. Preserves [ADR 0083](0083-concordance-server-rendering-and-htmx-4.md).

## Context

The current Book, its vocabulary inventory, and corpus investigation are split across competing surfaces: Reading shows the commitment beside other To Read Books, the full inventory lives under Vocabulary, and Concordance exposes Book, Grammar, and evidence-mode controls the learner rarely needs. The lifecycle also conflates intentions. Set Aside is a durable disposition, Finish writes it, and there is no way to hide a Book without changing its reading intent. Stop is framed as a pause although nothing is paused. Stale tabs can act on a newer reading of the same Book because mutations are bound to the Book rather than the commitment.

## Decision

### 1. Surface ownership

- **My Books** owns collection browsing, Inbox/To Read disposition, and independent Hide/Unhide.
- **Reading** owns Start, Switch, End, Finish, the Working desk (compact Book masthead, the only full Vocabulary Browse, quiet preparation/evidence notes), and, between Books, the neutral chooser.
- **Vocabulary** owns Concordance and explicit Known-vocabulary import. Primary navigation and the Vocabulary landing open Concordance. Browse is no longer a Vocabulary peer or duplicate preview.
- **Catalogs** retains catalog management.
- Opening a page never starts a reading, submits an import, or changes the stored active study language.

### 2. Disposition contracts to two values; Hidden is independent

- Persisted disposition is **Inbox** or **To Read** only. Inbox means *no explicit future-reading intent*, not "never triaged". **Set Aside is retired** as a value and action; requests to set it are rejected, not translated into Hide or End.
- **Hidden** is a durable, independent, per owner-and-Book visibility choice with no expiry. It is not a bucket, shelf, disposition, pause, deletion, or membership change.
- Visible bucket precedence: Current reading → To Read → Read (when history exists) → Inbox. Current reading is labeled distinctly within the To Read tab. Assertion and completion history both count; a historical To Read Book stays To Read.
- Hide/Unhide changes only visibility. It leaves disposition, current role, snapshot, reservations, history, Known vocabulary, eligibility, evidence, artifacts, queued work, study-language membership, and corpus participation untouched. End, Finish, Read again, and To Read choices preserve visibility, and Start never unhides.
- Default My Books and the between-Books chooser omit Hidden Books (including Needs language) without making them ineligible. **Show hidden books** is off by default, includes Hidden Books within the otherwise selected language/search/bucket scope, remains available in empty views, and bucket counts match the selected visibility scope. A Hidden Current reading remains in Reading.
- Newly discovered Books are visible Inbox. Sync refresh, disappearance/reappearance, alias reconciliation, and language correction preserve existing disposition and visibility, except for the incompatible-current-language handling in section 3.

### 3. Reading lifecycle

- At most one Current reading per owner and study language. Inactivity never expires it. There is **no pause/resume state**; **End is not pause**.
- **End current reading** replaces Stop and Set Aside: it clears the role and reservations, leaves the Book To Read, records no completion or Known acceptance, and preserves visibility, snapshots, history, and artifacts. Without a replacement Reading shows the chooser.
- **Switch** atomically releases the former reservation and freezes a fresh replacement snapshot; the former Book remains To Read. Failed or stale switches make no partial change.
- **Finish** atomically appends completion, accepts eligible frozen identities into Known vocabulary with set semantics, clears role and reservations, and sets the underlying disposition to Inbox; history projects it as Read. No automatic Hide or next Book. **Read again** sets To Read without starting or changing visibility or history. A previously-read assertion remains an idempotent assertion-time fact that changes neither disposition nor visibility and creates no acceptance.
- Return after End/Switch or a reread is a fresh start with a fresh snapshot, never adoption of a released one. The candidate rule of ADR 0085 (three in-Book occurrences, or exactly two with at least ten across the eligible same-language corpus) is unchanged, as are the count-readiness gate and its non-blocking behavior when no twice-occurring candidate exists. This ADR does not change thresholds or retroactively expand snapshots.
- An incompatible Book-language correction ends the old role with an explanation, releases reservations without completion, preserves history and artifacts, and requires a fresh commitment in the corrected language.

### 4. Expected-state and revision protection

- **Commitment-bound mutations.** End, Finish, Switch, and preparation actions bind to the exact expected commitment/snapshot, not Book ID alone. A stale action after a same-Book restart is rejected; safe replay changes nothing newer and creates no duplicate history or acceptance. Owner authorization and CSRF protection are unchanged.
- **Visibility and disposition writes** request a desired value and carry expected-state/revision protection. A stale write is rejected or, when the persisted state already equals the request, verified as a harmless replay. An old Hide cannot reverse a newer Unhide, and independent visibility and disposition writes must not reset each other.
- Physical column/table names, lock strategy, redirect codes, and component decomposition are implementation choices constrained by the above; they are not product decisions and are not tested as such. Consequential physical shape (new storage, constraints) is proposed in the implementing slice's PR and reviewed under [ADR 0038](0038-schema-change-governance.md) and [ADR 0070](0070-migration-and-documentation-reboot.md) before dependent SQL merges.

### 5. Structural versus data conversion; maintenance assumptions

- `migrations/000001_initialize` and shipped migrations are immutable. Structural successors (new visibility storage, disposition contraction) and the **data-only conversion** are separate stages with separate review; structural steps may commit separately.
- Justin is the responsible operator of **one backed-up maintenance window**. Old application writers and affected workers are stopped before conversion so old Finish/Set Aside/sync behavior cannot recreate retired state. **No mixed-version operating requirement and no compatibility shelf** is introduced.
- Before conversion: inventory owners, Book state, disposition values/revisions, history categories, and active commitments (not assuming a single learner), and write a cutover manifest of expected affected counts, preservation checks, and original changed-row values. Unexpected owners/state, invalid relationships, or unsafe volume/locking impact stop for review instead of silent repair.
- Mapping: preserve Inbox and To Read; map every still-legacy Set Aside to **Inbox**, initializing missing visibility as visible; never infer Hide or future intent. Unread legacy Books project Inbox; assertion-only and completed Books project Read with all source/snapshot facts preserved. No legacy Book becomes Hidden automatically.
- The conversion runs in one bounded transaction with writers stopped and inventory verified; committed data and checkpoint/audit completion must agree, and failure leaves no half-mapped state. Reruns map only still-legacy rows, initialize only missing visibility, guard original identity/revision, never reset an existing Hide, To Read, or later learner edit, and converge after failure or lost acknowledgement without duplicate history, acceptance, reservation changes, or artifact changes.
- The application stays closed until validation confirms: no legacy dispositions, writers, or routes remain; visibility initialization is complete; bucket mapping is correct across unread/assertion/completion categories; new/sync defaults work; and ownership, membership, snapshots, reservations, history, Known vocabulary, source evidence, and artifact provenance are unchanged apart from the intended mapping.
- **Recovery and preservation.** Before reopening, recover from the backup and manifest using the documented procedure. After activity resumes, prefer guarded forward correction only for state proven unchanged since cutover; never overwrite later learner choices or completions, and do not assume `down` migrations are safe for destructive production rollback. The implementing PR documents exact steps and completion criteria for human review. Nothing here authorizes production execution or deleting custom-deck records or artifacts.

### 6. Concordance contract (reconciles ADR 0083 and Browse ownership)

- Concordance searches **all owned Books with source-matching, current, completed analysis in the active study language and selected analyzed scope**, regardless of history, Hidden, or disposition. Stale, incomplete, failed, and superseded analyses do not contribute; frozen reading analysis never qualifies by virtue of commitment.
- One field, **Lemma or word form**, accepts a single term. A canonical lemma evidenced by non-excluded effective occurrences matches all its evidenced forms and POS; otherwise only the typed observed form matches. No query-time NLP, no POS chooser, no Book/Grammar/evidence-mode controls, and no phrase, substring, fuzzy, or advanced query. A Browse link supplies an exact lemma/POS restriction, cleared when the term field is submitted independently.
- Result order is captured with the result set: current-reading Book first, then others by case-insensitive title, exact title, and Book identity, then source order; 25 per page across one global sequence. Revision guards cover evidence affecting matches, interpretation, eligibility, or order (source/current-analysis identity, corrections/exclusions, titles); Hidden, history, disposition, and accounting do not change it.
- **Preserved from ADR 0083:** one server-rendered native occurrence list with no second client renderer, native sentence disclosures, and a visible Study link on every occurrence. Compact KWIC rows scan across Book boundaries with a static single-line source label and page-local count; this is a narrow exception to non-truncation of authoritative titles for that secondary label only, and Study shows the complete title. Immutable analyzer and source evidence stays unchanged and excluded evidence remains reachable in Study.

### 7. Browse inside Reading (reconciles ADR 0084)

Browse is scoped to the Current reading Book's source-matching current completed analysis and exact effective counts. The projection readiness and bounded durable rebuild guarantees of ADR 0084 are preserved: rows and across-Book totals stay gated until every relevant contributing Book has a ready matching projection, and partial totals, older analyses, or frozen snapshots are never substituted. Ordering (descending in-Book count, descending across-Book count, canonical lemma, POS), 25-row paging, prefix search, and accounted-for hiding are unchanged. Hidden Books contribute to cross-Book counts. Live Browse is distinct from the frozen snapshot and from deck contents.

### 8. Navigation, language precedence, and returns (reconciles ADRs 0050 and 0043)

- ADR 0050 stays in force (one active study language, derived set, stored selection, `/reading?language=` as non-persisting intent) **except** where it implies same-screen language handling: a deliberate language change lands on **My Books in the newly active language** with screen/filter/return state reset. Any supported old-language request, link, or submission redirects to My Books in the active language with a *Your study language changed* notice **before** restoring a reading or evidence or performing any mutation, and the selection is not switched back.
- ADR 0043's navigation and compatibility clauses (including the Vocabulary landing, `/settings`-style redirects, and retired-alias behavior) are superseded where they conflict: the Vocabulary landing means Concordance, the former known-vocabulary alias is retired without redirect, and no retired Browse-URL restoration, evidence-mode or Book-filter translator, or Set Aside-filter recovery is built. Ordinary unsupported-request handling applies to aged retired links. Supported stale-context safety remains required.
- Supported origins: a Browse origin names the exact reading/snapshot, language, controls, row, and live evidence revision; Back to book vocabulary restores them only if commitment, language, and evidence are unchanged, otherwise it explains and offers current Reading/My Books. Return pointers are owner/language-validated, grant no authorization, and cannot cause an external redirect. Ordinary browser Back/Forward is preserved; every subsequent request revalidates context.

### 9. Amended and preserved clauses

| Record | Treatment |
| --- | --- |
| ADR 0078 | **Superseded:** three-value disposition and Set Aside, bucket precedence ending in Set Aside, "stopping or setting aside", Reading-only stop wording, and the cutover/route clauses that assume Set Aside. **Retained:** one Current reading per language, no automatic next Book or recommendation, snapshot-on-start, reservation-as-not-Known, append-only completion, atomic switch, neutral chooser, historical `/journey` handling. |
| ADR 0050 / 0043 | Reconciled in section 8; otherwise retained. |
| ADR 0081 | **Amended:** "explicitly stopping without completion" now reads End current reading. Corrections still never rewrite active snapshots; a reread freezes anew. |
| ADR 0084 | **Amended:** Browse is hosted by Reading, not Vocabulary; projection and readiness guarantees unchanged. |
| ADR 0085 | **Amended:** "Browse and Concordance remain" means Browse in Reading and Concordance in Vocabulary; "any Book disposition" counts Inbox and To Read, Hidden or not. Candidate threshold, snapshot-based preparation, and artifact retention unchanged. |
| ADR 0083 | **Preserved** (section 6). |

Accepted records are not rewritten. When an implementation slice ships, it adds an amendment pointer to the affected ADR's header rather than editing its decision text.

### 10. Guarantees preserved

Immutable source and analyzer evidence, immutable vocabulary snapshots, Known-vocabulary semantics (generation never marks Known, per ADR 0036), exact-count readiness, historical artifact ownership and download, and owner authorization/CSRF all remain unchanged. Deck-independent Finish and the optional, separately consented translation of ADR 0078 are retained.

## Governance and identified follow-up documents

Accepted target versus shipped behavior must stay distinguishable until rollout. The implementing slices own these updates, each marking itself shipped only when the behavior is:

- Feature contracts: `reading-workflow.md`, `reading-owned-book-vocabulary.md`, `vocabulary-browse-and-concordance.md`, `collection-browsing.md`, `language-mode.md`, `lemma-review-and-correction.md`.
- Design docs: information architecture, screen inventory, components, and the design-system title exception for secondary Concordance source labels.
- `CONTEXT.md` glossary: independent **Hidden**, two dispositions, **Inbox** meaning, derived buckets, **End current reading** wording, reservations, and historical Set Aside/selection terms. It stays a glossary, not a migration manual.
- `doc/product.md`: status prose for this ADR as slices ship.

## Explicitly not authorized

A mixed-version compatibility shelf, retired-link translators or redirects, deletion of custom-deck records or artifacts, production execution of the cutover, changes to candidate thresholds or completion knowledge semantics, additional dispositions, a Hidden shelf, pause/resume, inactivity expiry, implicit starts, or Concordance phrase/fuzzy/advanced search.

## Consequences

- Visibility, intent, commitment, history, and artifacts become independent state with independent stale-write protection.
- Implementers get an accepted durable shape without reopening approved product choices; dependent SQL still goes through the schema-change review of ADR 0038.
- The cutover is a deliberate, single-operator, backed-up maintenance event rather than a rolling migration.
- Older learner links to retired surfaces stop working instead of being translated.

## Related

- [Issue #1580: Reading Working desk and corpus-wide Concordance](https://github.com/justin-hayes/mouseion/issues/1580)
- [Issue #1581: Accept the redesigned durable contract](https://github.com/justin-hayes/mouseion/issues/1581)
- [ADR 0038: Schema-change governance](0038-schema-change-governance.md)
- [ADR 0070: Migration and documentation reboot](0070-migration-and-documentation-reboot.md)
- [ADR 0078](0078-book-dispositions-and-current-reading.md), [ADR 0083](0083-concordance-server-rendering-and-htmx-4.md), [ADR 0084](0084-browse-effective-count-projection.md), [ADR 0085](0085-reading-owned-book-vocabulary.md)
