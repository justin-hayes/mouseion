# ADR 0116: A curated Classical catalog of Ancient Greek Reading units

Status: **Accepted — implemented (#2401/#2419)** · Date: 2026-11-09 · Author: Justin + OpenCode

Decided in [#2401](https://github.com/justin-hayes/mouseion/issues/2401)
(Spec: Curated Classical catalog for learner-chosen Ancient Greek texts) and
recorded under [#2405](https://github.com/justin-hayes/mouseion/issues/2405).
Amends [ADR 0099](0099-ancient-greek-imported-analyses.md),
[ADR 0058](0058-catalog-maintenance-principal-destination.md) and
[ADR 0107](0107-analysis-pipeline-identity-and-analysis-updates.md), and
changes the `ReadingChooser` contract in the design documents. The spec holds
the full behaviour; this ADR records the decisions that change accepted rules
or the schema.

## Context

Ancient Greek reaches a learner today through one compiled Reading-unit
definition, *Anabasis* Book 1, which an operator registers with a command
(ADR 0099). The learner cannot choose what to read. #2401 replaces that path
with the **Classical catalog**: a reviewed, installation-wide collection of
Reading-unit definitions that ships in the release and from which a learner
adds Books to My Books.

Several accepted rules stand in the way:

- ADR 0099 refuses a sentence that straddles a citation boundary. That is fine
  for one hand-checked Book, but it would keep whole works out of the Classical catalog,
  and it disagrees with the glossary's rule that a sentence belongs to the
  Reading unit that holds its first word.
- ADR 0058 and the information architecture keep Catalogs a configuration and
  sync destination, "not a book-browse surface". The Classical catalog has no
  connection to configure and no metadata to sync, so a learner has to browse
  it before adding anything.
- ADR 0107 excludes imported analyses from Analysis updates. A classical Book
  whose source pin or interpretation version changes would then keep stale
  evidence with no way to refresh it.
- A work such as the *Iliad* is 24 Constituent books. Without work-level rules,
  My Books and the Reading chooser show 24 rows, and the product would be
  tempted to steer the learner to "the next Book".
- Each Book stores the whole work file as its source, and
  `reference_translations` is keyed by owner. A second Book or a second learner
  would download and store the same bytes again, and two translation pins for
  one learner conflict.

## Decision

### Sentence ownership replaces the straddle refusal

A sentence belongs to the **Reading unit that holds its first token** (the
glossary's "first word"). The import no longer refuses a sentence that crosses
a citation boundary; the whole sentence belongs to the unit that holds its
first token. It still refuses a sentence that belongs to no unit or to two
units. The maintainer generator
lists every crossing sentence for the reviewer. *Anabasis* Book 1 has no
crossing sentence, so its result is unchanged.

A **Reading unit** is one Constituent book when the pinned Edition's citation
scheme names books, otherwise one whole-work unit. Units cover the Edition
exactly: no gaps, no overlaps, no extra whole-work unit and no hand-picked
passages. A changed citation range is a new Reading unit with a new key, never
an edit of an existing one.

### Reading-unit definitions ship in the release

Reading-unit definitions ship as a generated, reviewed `manifest.jsonl`
embedded in the binary, with a hand-maintained overrides file for curatorial
fields and a `layers.json` that pins every upstream layer and records its
ADR 0111 licence class. There is no runtime fetch of definitions and no
operator command. The compiled *Anabasis* definition and the registration
command are retired; the `xenophon-anabasis-1` key and the import adapter
remain.

A definition's key is permanent. Curatorial fields (Work title, author,
Greek title, Edition label, reviewer note) are corrected in place. A
content-bearing change (source pin, translation pin or interpretation version)
requires a new definition revision; the generator refuses one without it, and
startup rejects a manifest that reduces a revision or changes content without
one. A removed entry becomes a withdrawn tombstone, so existing Books never
dangle. Startup reconciles the manifest into the Work, Edition and Reading unit
registry in one idempotent transaction and never touches learner state.

### Catalogs browses curated sources that do not sync

The Catalogs rule changes from "a configuration and sync destination, not a
book-browse surface" to: **Catalogs browses only curated sources that do not
sync into My Books.** The Classical catalog is a built-in, read-only record
above the learner's OPDS connections, with author, author-detail and
work-detail pages. It is not a fifth destination and is not scoped to the
active study language. It shows word counts and "In My Books", never
preparation state, coverage or difficulty.

OPDS connections keep ADR 0058's rule: Catalogs configures and syncs them, and
My Books remains the only browse surface for synced metadata.

Adding a Reading unit creates one metadata-only Inbox Book, linked to its
Reading unit; adding a whole Work creates one per current Reading unit of the
Work. Adding downloads nothing and starts no job. A learner has at most one
Book per Reading unit.

### A Book's place in its Work is a fact, never a pointer

A Constituent book's place, such as "Book 2 of 24", is a plain bibliographic
fact shown on Reading and on chooser candidates. The count comes from the
reviewed definition, not from how many Books the learner added. A whole-work
unit shows no position.

Mouseion never names, offers, links to or starts the next Book of a Work, even
when the learner has already moved it to To Read. The completion receipt stays
chooser-only. Interleaving works is ordinary choosing. This is the general rule
for a Book's place in its Work; OPDS series are out of scope.

### To Read is a bulk Work action over per-Book disposition

Disposition, visibility and preparation stay per Book. My Books groups a
learner's Books of one Work into one row when the learner owns more than one,
and offers Work-level actions over them:

- **Move the work to To Read** moves every Book without reading history and
  skips the current Book;
- **Move the work back to Inbox**, under More actions, leaves the current Book
  To Read;
- with mixed states, **Move the other N books to To Read**;
- **Hide all N books** and **Unhide all** set each Book's visibility.

There is no Work-level state. To Read is the preparation trigger for an
unprepared classical Book: it changes disposition and enqueues one ensure-once
preparation job per Book and definition revision, in Constituent book order,
on a dedicated `classical_preparation` River queue with one worker. Classical
preparation never uses the shared `analysis` queue and never calls the NLP
service. Leaving To Read or hiding a Book does not cancel its preparation.

### Preparation chooses its adapter by the definition's source format

Preparation selects the import adapter by the definition's source format rather
than hard-wiring the OGA CoNLL-U adapter, so a later adapter (such as the
graded-reader proposal in [PR #2402](https://github.com/justin-hayes/mouseion/pull/2402))
adds an adapter, not a new preparation job. For now there is one adapter, for
the OGA CoNLL-U dialect, and one interpretation version for every work in
the Classical catalog. That version recognises ellipsis only by the source's ellipsis
convention, keeps a native `x` token as an ordinary token mapped to UD `X`, and
accepts repeated identical `# source` headers. A file whose headers or
citations cannot be recovered is refused and kept out of the Classical catalog. Per-work
interpretation variants are not decided.

### Classical Analysis updates

ADR 0107's "imported analyses are excluded" rule is amended. A classical Book
is offered an Analysis update when the source pin or interpretation version it
was prepared under differs from its current definition. The comparison uses the
per-Book prepared-definition record, not an NLP pipeline identity. A change to
curatorial metadata or to the translation pin alone gives no offer.

ADR 0107's other rules apply unchanged: eligibility is per Book (To Read Books
outside the current Reading; never Read or Inbox, since an Inbox Book is
prepared under the current definition when it moves to To Read), nothing is queued without
confirmation, the published analysis stays in effect until its update
publishes, and earlier analyses, Lemma corrections, Occurrence exclusions,
current-reading snapshots, Known and Reserved vocabulary, completions and
prepared decks keep their frozen evidence. A grouped Work row shows one notice
and one bulk action; the current Book is skipped and named; one confirmation
states the corrections and exclusions that will not carry over; each Book is
revalidated separately on confirmation. Update jobs run on
`classical_preparation`.

### One copy of source and translation per installation

**Verified sources.** A new installation-wide table stores each verified work
file once, keyed by its source pin's SHA-256, with layer key, file name, byte
size and content, and no learner data. The first preparation of any Book of a
Work downloads the pinned file lazily, verifies size and SHA-256, and inserts
it; later Books and later learners reuse it. Bad bytes are never stored. The
existing 512 MiB acquisition limit is the hard ceiling. At startup, after
reconciliation, stored files that no current, non-withdrawn definition pins are
deleted. There is no on-disk cache.

**Unit-only source material.** A Book's source material stores only its own
Reading unit's text. Its source identifier is the Reading-unit key and its
content digest is the unit's digest. The work file's provenance is in the
prepared-definition record.

**Prepared-definition record.** A new per-Book record, keyed by owner and Book,
holds the Reading-unit key, definition revision, source pin SHA-256,
interpretation version and published analysis run. Classical preparation writes
it when it publishes; each successful update rewrites it.

**Translations.** `reference_translations` becomes one copy per installation,
keyed by translation pin SHA-256 and citation, with no owner. A translation file
is never stored; its parsed sections are. Translation import is a separate,
non-blocking step after the Book's analysis publishes; failure leaves the Book
analyzed and deck preparation abstains from passage context. Prepared decks
keep the translation pin and passage text they froze. The "conflicting pins per
owner" refusal is removed because pins now coexist.

### Migrations

Three migrations carry the schema change, under ADR 0038 and ADR 0070:

1. adds the installation-wide verified source table and the per-Book
   prepared-definition record;
2. replaces the owner-scoped key of `reference_translations` with the
   installation-wide (translation pin SHA-256, citation) key, copying existing
   rows, deduplicating identical pin-and-citation rows and keeping different
   pins. Deck-frozen passage fields are untouched.

Both migrations are **forward-only**. Their down migrations are development
conveniences, not a production rollback. Structural DDL and data movement stay
separable for review.

A third, additive migration adds `classical_preparation_failures`, the per-Book
record of a preparation failure. It is keyed by owner and Book, stores only a
stage and a stable ASCII code, and carries no source text or Greek. The record
is written when an attempt settles, and also when a submission is refused
because the definition is withdrawn, so that refusal remains visible on the
jobs surface without a River job. It is needed because a terminal source refusal
must stay visible until a newer definition revision arrives, which is longer
than River retains a discarded job; a successful preparation deletes it. The
current Reading unit's revision is compared with the recorded revision, so a
newer definition revision clears a terminal failure without rewriting the row.
This table is not a per-pin failure record.

The existing *Anabasis* Book 1 is converted during startup reconciliation, not
in a migration. The conversion is one-time, idempotent and retry-safe. It
reproduces the published analysis under the current interpretation and, when
the evidence matches, rewrites the Book's source material to unit-only content
under the Reading-unit key while keeping the Book's identity, analysis, Reading
history, corrections, exclusions, Known vocabulary, snapshots and prepared
decks. When the evidence differs, it leaves the published analysis in place and
offers the ordinary Analysis update. It also maps the old one-key Work and
Edition rows to the shared Work and Edition keys without changing any Book's
Reading-unit link.

ADR 0099's legacy `grc` refusal becomes a fail-closed Classical catalog
precondition. While un-converted legacy rows exist, adding and preparation are
unavailable with a stable reason and code; existing Books stay readable, and
OPDS and the modern languages are unaffected.

### The `ReadingChooser` contract

The chooser no longer groups candidates into coverage-band sections. It lists
To Read candidates in the existing neutral title order:

- a Work with more than one To Read Book is one entry. Its identity is Work
  title, author, Greek title and "N books" from the reviewed definition. Its
  margin gives the To Read and assessed counts and one Known coverage range.
  Inside the entry, a band table lists that Work's candidate Constituent book numbers in
  Constituent book order, one row per band in the shipped band order, omitting
  empty rows. Each number opens that Book's ordinary candidate entry inline,
  with its position, band, coverage, Start or Switch confirmation and Retry
  when not assessed;
- a single-Book Work stays an ordinary candidate row whose margin names its
  band.

Only To Read candidates appear; Read, current and hidden Books have no cells.
Book numbers carry no coverage figures, so the chooser is still not a ranking.
The page title, description and intro copy are unchanged.

## Implementation approach

The decision shipped as one vertical slice from the manifest to the learner
surfaces, and the code follows the seams above:

- `internal/classical` embeds and validates the generated
  `manifest.jsonl`, `overrides.jsonl` and `layers.json`, builds the read-only
  browse view, and reconciles the release into the Work, Edition and Reading
  unit registry. `internal/classicalmanifest` is the maintainer generator; its
  verify mode reproduces the committed bytes and CI asserts no diff.
- `migrations/000048_classical_manifest_reconciliation` adds the registry;
  `000049_classical_preparation` adds `classical_sources` and
  `classical_prepared_definitions`; `000050` and `000051` replace the
  owner-scoped translation key with the installation-wide
  `(translation_pin_sha256, citation)` key and copy existing rows;
  `000052_classical_preparation_failures` adds the per-Book failure record.
- `internal/analysis` owns the `classical_preparation` and
  `classical_translation` workers, the OGA adapter hand-off through the
  existing Greek import writer, the prepared-definition comparison used by
  Analysis updates, and the startup legacy conversion.
  `cmd/server` reconciles the manifest and runs the conversion before the web
  application starts.
- `internal/webapp` renders the Classical catalog on Catalogs, the grouped My
  Books row and its Work actions, the grouped Reading chooser, and the grouped
  Analysis update notice and confirmation. Preparation status and stable
  failure codes are shared between those surfaces and `/jobs`.
- `internal/webapp/classical_complete_loop_integration_test.go` proves the whole loop
  through the real HTTP routes, PostgreSQL and in-process River workers with no
  NLP service, including the seeded pre-upgrade conversion, the legacy-row
  precondition and one-fetch-per-installation request counts. The failure
  matrix, catalog browsing, grouping, chooser, update and conversion seams keep
  their dedicated slice tests.
- The glossary entries for **definition revision**, **withdrawn definition**
  and **Classical preparation** live in `CONTEXT.md`, and the design documents
  name the new Catalogs browse rule and chooser contract instead of the
  superseded ones.

## Alternatives considered

- **Keep the straddle refusal and let curators exclude affected works.**
  Rejected: many works have crossing sentences, and the glossary's first-word
  rule already gives every sentence one owner without guessing.
- **A fifth destination or a My Books facet for the Classical catalog.**
  Rejected: ADR 0058 rejected a catalog facet on My Books, and a new
  destination costs navigation for a surface used only when adding texts.
  Catalogs is the home for sources; this source just has nothing to sync.
- **Work-level disposition or a reading plan for a Work.** Rejected: a
  Work-level state duplicates per-Book disposition and invites "next Book"
  recommendations, which the product refuses.
- **Keep imported analyses excluded from Analysis updates.** Rejected: a
  corrected source pin or interpretation would never reach existing Books.
- **Cache work files on disk or store one per Book.** Rejected: an on-disk cache
  needs a new volume outside database backups; per-Book copies multiply storage
  by the number of Books and learners.
- **Hard-wire the OGA adapter in preparation.** Rejected: a second
  source format, such as graded readers, would then reshape the preparation
  job instead of adding an adapter.

## Consequences

- Adding a work to the Classical catalog is a reviewed manifest change, not Go code plus an
  operator command. An operator runs nothing for definitions, downloads,
  imports or the conversion.
- Modern-language analysis, OPDS sync and My Books rows for non-classical
  Books are unchanged; classical preparation never delays EPUB analysis.
- `CONTEXT.md`, `doc/design/information-architecture.md`,
  `doc/design/components.md`, `doc/design/design-system.md`,
  `doc/design/screen-inventory.md`, `doc/design/terminology.md` and the
  deployer register are reconciled with this decision when the surfaces ship,
  so the old Catalogs, chooser and imported-analysis rules do not stay live.
- New layers are added to `layers.json` and the deployer register in the same
  change, under ADR 0111.
- The two migrations need human review under CODEOWNERS and must not land before
  this ADR is accepted.

## Related

- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md)
  and [ADR 0070: Reboot migration history and consolidate superseded documentation](0070-migration-and-documentation-reboot.md)
- [ADR 0058: Catalog maintenance is a principal destination](0058-catalog-maintenance-principal-destination.md)
  — Catalogs browse rule amended.
- [ADR 0099: Ancient Greek through imported analyses in the existing pipeline](0099-ancient-greek-imported-analyses.md)
  — straddle refusal, operator registration, To Read and legacy-row rules amended.
- [ADR 0107: Analysis pipeline identity and learner-confirmed Analysis updates](0107-analysis-pipeline-identity-and-analysis-updates.md)
  — imported-analysis exclusion amended.
- [ADR 0111: One licence posture for third-party data in every language](0111-third-party-licence-posture.md)
  — the admissibility rule `layers.json` records.
