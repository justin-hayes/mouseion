# Basic poetry support: Faust I acceptance case

Status: **Accepted specification — implementation pending** · Date: 2026-10-17

Accepted live by Justin in [Decision: accept the Faust I poetry specification and
handoff](https://github.com/justin-hayes/mouseion/issues/2151), completing the
[Poetry support in Mouseion: Faust I decision map](https://github.com/justin-hayes/mouseion/issues/2143).
Acceptance of this specification is not acceptance of an implemented feature.
The architecture and consequential persistence choices are recorded in
[ADR 0103](../adr/0103-source-structure-and-poetry-evidence.md).
Implementation is separately tracked in
[Implement basic poetry support from the accepted Faust I specification](https://github.com/justin-hayes/mouseion/issues/2186).
It is not another decision-map child and has not been executed.

## Purpose and scope

Support basic poetry in the existing vocabulary, Concordance, sentence-study
and Anki workflows without treating a source line as a linguistic sentence.
German *Faust I* is the acceptance case: verse, prose, dialogue and stage
directions coexist. The learner reads the complete work in their ebook reader;
Mouseion gains neither a full-work reader nor a new acquisition format.

Preserve meaningful source lines, line groups, declared speakers and directions,
not a printed-page facsimile. Apply the same rules to any EPUB with matching
markup; do not classify a Book or passage as poetry. Existing prose without
authorial breaks keeps its extraction, vocabulary and card outcomes.

## Decision ownership

The repository promotes the accepted contracts below; it does not reopen their
owners or merge their exploratory branches.

| Owning decision | Contract carried forward |
| --- | --- |
| [Decision: distinguish verse structure from linguistic sentences](https://github.com/justin-hayes/mouseion/issues/2147#issuecomment-6237112588) | One text/offset basis; breaks, roles, provenance and structural fallbacks. |
| [Decision: poetic study context, vocabulary eligibility and card quality](https://github.com/justin-hayes/mouseion/issues/2148#issuecomment-6237391469) | Complete analyzer sentence; role eligibility, bounded card quality and exact provider payload. |
| [Decision: poetry evidence compatibility and frozen-reading boundaries](https://github.com/justin-hayes/mouseion/issues/2150#issuecomment-6238677596) | New extraction under unchanged bytes, explicit adoption, immutable old evidence and recovery. |
| [Decision: Faust source notices in study and Anki](https://github.com/justin-hayes/mouseion/issues/2154#issuecomment-6238654704) | Ordinary bibliographic attribution; acceptance-only edition and repository boundary. |
| [Prototype: line-aware Faust excerpts in sentence study and Anki](https://github.com/justin-hayes/mouseion/issues/2149#issuecomment-6239826608) | Human-reviewed source-line presentation and separate Anki attribution. |
| [Decision: accept the Faust I poetry specification and handoff](https://github.com/justin-hayes/mouseion/issues/2151) | Persistence, mixed-role safety, measured-example corrections, validation limits, scheduling and rollout. |

## Acceptance source and acquisition

- Input: Goethe, *Faust: Eine Tragödie [erster Teil]*, Gutenberg eBook 21000,
  EPUB 2 without images, reporting an 1808 first-edition base text. This is
  transcription provenance, not independent collation against a scan.
- Fetch `https://www.gutenberg.org/ebooks/21000.epub.noimages`; require SHA-256
  `93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01`
  (179,103 bytes). A changed digest is a different input, not a passing rerun.
- Exercise the existing Calibre-Web/OPDS route: serve the unchanged EPUB from
  the catalog's own origin, declare German, sync metadata, then acquire through
  **Move to To Read** with the German NLP capability ready. A Gutenberg download
  link on another catalog origin is not an equivalent supported acquisition.
- Neither a new importer nor a public Gutenberg OPDS connection is required.
  This edition is a test input, not the learner's recommended reading edition.
- Keep the stored original byte-for-byte unchanged. Never commit or ship the
  complete EPUB. Committed fixtures contain only short excerpts of Goethe's
  text, without Gutenberg headers, licence text or Gutenberg references.
- Preserve ADR 0066's whole-snapshot fallback: this edition declares no usable
  main-text start, so readable header/licence matter also enters analysis.
  Evaluate literary examples separately; do not invent a main-text filter or
  disguise header/licence excerpts as source-attribution UI.
- No poetry-specific source notice or licence UI is added. Ordinary Book title
  and available chapter attribution remain; Anki keeps its Book title/deck/tag
  attribution. None enters the provider payload. This records the human's
  accepted use boundary, not a legal certification. Crossing the full-EPUB
  redistribution/serving boundary requires a fresh licence review.

## Source text, structure and analysis

1. The immutable readable-unit text is the sole source and offset basis.
   Locations are half-open, unit-relative Unicode-code-point spans. No separate
   normalized analysis text, offset map, per-line text copies or canonical verse
   numbers are introduced. Scene labels use existing navigation provenance;
   display line ordinals are not citations.
2. Under extraction contract v2, `<br/>` becomes `\n`; block boundaries and blank
   lines become `\n\n`. Incidental markup whitespace collapses to a space;
   viewport wrapping never enters source text. Source lines and line groups are
   derived from that text. Plain blocks without `<br/>` retain today's text.
3. Roles come only from the versioned convention set: `p.speaker`,
   `span.speaker`, the direction paragraph classes `entrance`, `sdcenter`,
   `sdsetting`, `dirlong`, `dirlongind`, and `span.sd`. Unrecognized or
   contradictory markup establishes no role. Store nestable role spans and
   heading-reset offsets in the snapshot, not guessed character identities.
4. Derive speaker attribution from the nearest preceding label in the same unit,
   reset at a heading. A direction carries no speaker attribution and does not
   itself clear the preceding speaker. No preceding label means no attribution.
5. Analyze the unit text unchanged through the existing RPC and production
   pipeline. Roles do not alter segmentation. Sentences may cross single source
   breaks, never group breaks. Punctuation is not a promised sentence splitter:
   one analyzer sentence can contain several punctuated utterances.
6. Preserve the existing per-unit, size-capped analysis boundaries (ADR 0013).
   No sentence or syntax is reconstructed across readable units or RPC chunks.
7. `<ins class="correction">` contributes only visible text, never its `title`
   alternative. Empty page markers are zero-width. Typography, alignment and
   indentation do not become linguistic structure. Shared metrical lines split
   between speakers remain separate source lines/groups in their speeches.
   Headings are their own groups (a two-part heading remains two groups);
   `<hr>` creates a group boundary without adding text.

## Vocabulary, study context and cards

- Speaker-overlapping tokens, including directions nested in a speaker label,
  contribute no vocabulary identity, effective vocabulary count or
  analyzable-token coverage denominator. This is structural non-vocabulary,
  not a learner Occurrence exclusion; an ordinary exclusion still does not
  remove a token from that denominator. Keep all analyzer evidence.
- Other stage directions are ordinary eligible source text, subject to existing
  POS/particle/lemma rules. `PROPN` remains ineligible. Counts and Reading
  candidate thresholds remain independent of card suitability.
- Keep eligibility in Go `selection` and pass exact-snapshot structural facts to
  it; do not reproduce the rule in SQL. Projection versioning, durable rebuilds
  and truthful **Updating** states follow ADRs 0084/0087. Rebuilding a projection
  is not changing a Book's published analysis or frozen Reading vocabulary.
- Study context remains exactly one complete analyzer sentence, including its
  authorial breaks. Never truncate, modernize, pad, paraphrase or borrow adjacent
  lines. A line, group or larger passage is not the study context.
- A sentence overlapping any speaker label is unavailable as study/card context,
  even when it also contains eligible direction words. Do not remove the label
  from the sentence or force a new boundary. Those other words still count and
  may obtain a card from another acceptable occurrence.
- Concordance keeps speaker-label occurrences as searchable raw evidence,
  subject to its existing ownership, query and occurrence-decision rules. Label
  them as speaker-label evidence. No sentence overlapping a label offers a Study
  action; a direct request cannot bypass that restriction. This is not deleting
  corpus rows or adding a vocabulary identity to a label.
- Card bounds remain 6–50 words and at most 400 code points. Retain complete
  boundary, finite-verb/subject, other noise and ranking rules, including current
  language-appropriate missing-syntax fallback. Source-order tie-breaks and
  first-encounter ordering remain unchanged.
- Only the newline quality policy changes: a newline alone is not structural
  noise. Apply the existing list/page-marker content checks to source lines;
  retain actual bullet, numbering and page-fragment rejection. Do not introduce
  a poetry-specific profile. New preparation uses this policy; a presentation-only
  rebuild never regrades a frozen decision.
- Short exclamations, fragments, interrupted/dash/ellipsis endings and over-limit
  contexts deliberately yield sentence-quality omissions, not invented cards.
  A target with no acceptable candidate is omitted. An indefensible contextual
  meaning remains a meaning omission; unusable provider responses remain provider
  omissions under the existing preparation contract.
- Historical-language mistakes use the existing learner correction/exclusion
  workflow before a new freeze. No model tuning, automatic modernization or
  new blocking-risk detector is part of this feature. Curated acceptance cards
  must have reviewed correct identities and target emphasis; measured incorrect
  assignments are limitations, never gold labels.

## Provider and presentation contracts

Send the exact complete study context, with `\n`, the tested target/lemma and
bounded lexical evidence. Never add speaker labels, role markers, headings,
titles or surrounding orientation. The English translation is one complete
prose rendering; no line correspondence is inferred. Gloss and validated English
target alignment retain their existing meaning and exact-translation identity.
Hash the exact source sentence: v2 contexts with `\n` legitimately miss v1 cache
entries. The shared source-target anchoring decision in ADR 0102 remains owned
there; poetry does not create a parallel target matcher.

The human-reviewed rules are also recorded in
[design components](../design/components.md#accepted-source-line-presentation-target).

- One-line Concordance summaries replace each authorial break with a muted
  ` / ` mark. A literal source slash is an accepted ambiguity; expansion resolves
  it. Existing boundary-space handling remains.
- Expanded context, sentence-study text and the Anki front render actual source
  lines. Soft wraps hang indented, distinct from authorial breaks. No-break prose
  retains its current layout. Do not show containing-group orientation or line
  numbers.
- Sentence-study morphology has one aligned-word row per source line at every
  width; row wraps hang indented. Keep source gaps, punctuation, existing safe
  alignment checks and local/whole-line fallback. The complete text alternative
  notes **end of source line** after the last word of a line; it does not invent
  a morphological word for an unaligned gap. Screen readers otherwise hear a
  continuous sentence.
- Quiet small-caps speaker metadata contains only the name: omit nested
  directions, optionally trim the terminal period, and escape it as text. A
  direction sentence instead shows **Stage direction**. Neither marker is part
  of the source sentence or a target.
- Freeze optional typed attribution in the deck specification. Append the Anki
  `Attribution` field after the existing fields and show it separately on the
  front. Emit explicit per-source-line HTML after escaping each source segment;
  raw LF is insufficient in Anki HTML. Target/particle emphasis stays inside the
  correct lines. No provider-supplied HTML is trusted.
- Bump manifest, render-input and presentation versions independently of source
  analysis identity. Old manifests remain readable and render without attribution;
  do not backfill them from newer source evidence. Preserve the existing required
  frozen-input checks, historical codecs and digests. Same-specification re-import
  keeps note/model identity; a new v2 specification can change card identity and
  leave old Anki notes requiring manual cleanup.

## Existing Books and frozen evidence

New acquisition writes v2. Existing EPUB sources receive extraction-only River
checks via a bounded startup scan and relevant source-evidence changes. Reuse
successful exact-key checks; no continuous rescanning or separate scheduler.
Imported analyses are excluded. A check does not publish or select a snapshot,
run NLP, change current evidence or rewrite any frozen artifact.

For unchanged v1 text with no roles, record **unchanged** and retain the current
snapshot and analysis. For changed extraction, offer **Source structure update**
at Book level, using the existing consequential-confirmation pattern. State the
old Reading/deck boundary and the number of confirmed corrections/exclusions
that will not apply to the new analysis. No per-card missing-structure banners.

Confirmation re-extracts retained bytes, verifies the checked result and expected
source, then atomically creates/selects the immutable v2 snapshot and queues a
snapshot-bound analysis. A stale confirmation must refresh, not adopt different
evidence. The same content revision remains; no reacquisition is required when
bytes are retained. Evidence becomes stale immediately and leaves current
cross-Book counts until reanalysis publishes. Double submits and retries converge
on one extraction key/run. Failures before commit leave v1 untouched; failure
after switching uses existing stale/failed analysis and **Re-analyze** recovery.
There is no revert-to-v1 action. Convention changes use the same flow, including
role-only changes; do not reuse a corpus across snapshots.

Checks distinguish **unchanged**, **update available** and **original
unavailable**. A failed check is a retryable failed job, never a successful
unchanged result; after bounded automatic attempts, use explicit retry. Legacy
records without original bytes cannot adopt in this MVP. Leave their analyses,
readings and decks usable; do not promise ordinary reacquisition repairs the
record, because acquisition can reuse it. No special repair workflow is added.

An active Reading may coexist with adoption but keeps its exact v1 evidence,
Reserved vocabulary and decks. Preparing from that Reading still uses v1.
Line-aware evidence requires explicit End, restart on completed v2 analysis and
re-preparation. Corrections never carry to another analysis automatically.
Completed Reading/Known facts, old corpora and manifests stay untouched. v1
means **structure not recorded**, not prose or absence of speakers; never infer
source lines from its old `\n\n` encoding. A presentation-only deck rebuild may
update styling, not selection, source text, translations or omissions.

## Acceptance matrix — implementation/release obligations, not passed tests

Bind excerpt fixtures to the checksum above and the packaged path prefix
`OEBPS/8980964947130898904_21000-h-`. Source line references below address the
packaged XHTML, not canonical verse numbers. Synthetic edge fixtures are separate
from edition evidence. All stored/queried artifacts remain owner-scoped.

| Check | Required end-to-end result |
| --- | --- |
| Acquisition | Sync/acquire through the catalog route; assert retained original digest and unchanged bytes, v2 snapshot identity and main-text fallback. A digest mismatch fails the acceptance setup. |
| Cross-line sentence (`0.htm.html:90–93`, Director) | First complete sentence spans four source lines, retains exact `\n` and offsets through NLP, corpus, study, provider request and Anki. Reviewed targets pass the unchanged remaining quality gate. The full speech is one 26-line group, not 26 sentences. |
| Analyzer-owned boundaries (`0:195`) | Do not require a split between `Was fällt euch an?` and `Entzückung oder Schmerzen?`; the measured pinned model joins them. Use a supplied-analyzer fixture to test the general case of two sentences sharing one source line. |
| Group boundaries (`3:141` onward, Thule) | Six groups of four source lines survive extraction. No sentence crosses a group; no group-orientation UI or forced one-card-per-stanza behavior appears. |
| Visible text and Unicode (`5:46–47`, correction markup) | Keep visible reading only; no `title` alternative enters text/payload. Code-point slices, entities, zero-width page markers and literal source slashes remain exact and safely escaped. |
| Role precedence (`4:579`, Valentin) | Raw markup is a speaker block containing a nested direction. Both words are excluded from vocabulary/denominator, still searchable as raw evidence, and supply no Study/card context. The earlier eligible-direction prediction is not an acceptance oracle. |
| Standalone direction (`0`, `Er schlägt das Buch auf …`) | Eligible content words count normally; accepted context displays Stage direction, not the preceding speaker. Count policy is independent of whether a particular candidate passes the gate. |
| Mixed-role paragraph (`2:346`) | Inline `Mephistopheles` is excluded; other eligible direction words still count. Any sentence overlapping that label supplies no study/card context and offers no Study action. No altered source sentence is sent to a provider. |
| Attribution | Derive nearest preceding label within the exact unit, reset at headings; directions do not clear it. Remove nested directions only from displayed/frozen speaker name. Unknown convention yields no attribution and no warning. |
| Deliberate omissions (`4:368`, `4:580`, `0:599–602`) | Short exclamations and the interrupted Geist speech fail honestly. No padding/truncation, no change to the frozen vocabulary snapshot, no generated provenance for omitted cards. |
| List/page noise | Positive multiline sentence plus bullet/number/page fragments on first and later lines; LF alone passes, actual noise still fails. v1/no-break card outcomes remain unchanged. |
| Vocabulary and corrections | Speaker exclusions agree across analysis aggregates, projection, Browse, coverage and Reading freeze; directions/names/particles follow ordinary rules. Correct/exclude measured bad historical forms using existing review, keeping source and analyzer assignments immutable. No duplicate SQL eligibility. |
| Web surfaces | At 375px and 1200px, both themes and enlarged text: `/` only in the summary, source lines elsewhere, distinguishable soft wraps, per-line morphology, target/particle emphasis, safe fallback, no horizontal page overflow. Preserve native disclosure, keyboard/return focus, no-JavaScript operation and the named text alternative; role/name browser assertions are not manual screen-reader verification. |
| Provider/translation | Fixed response fixtures prove exact newline-bearing context and absence of attribution/title/orientation; full English prose, validated contiguous/discontinuous alignment and unhighlighted fallback. Review representative actual provider meanings/alignment before release; no new prompt or line-translation scheme. |
| Anki | Inspect APKG fields/HTML and perform real import/re-import in a recorded client version. Same-spec notes update without duplicates or field shifts; source-line layout, front attribution and target/particle emphasis work in narrow/wide and dark presentation. Legacy manifests still render with empty attribution. |
| Compatibility | Checked unchanged prose retains snapshot/run; changed v1 offers confirmation only. Test double submit, crash, retry, source-change race, role-only convention change, absent bytes and imported-analysis exclusion against real persistence and the UI. No partial snapshot/switch. |
| Frozen artifacts | Adopt during v1 Reading: its vocabulary, corrections, reservations, completion/Known behavior and deck remain pinned. End/restart/re-prepare yields v2; old digests/cache identities stay readable. Presentation-only rebuild never fetches new attribution, regrades or calls a provider. |
| Prose regressions | Existing extraction/entity/offset, chunk reconstruction, corpus/parse, eligibility/count, correction, study fallback, complete-sentence, alignment/cache, APKG and historical-manifest suites remain green; unchanged prose comparison retains source text and verdicts. Authorial breaks in prose use the same new line-aware rule. |

Run existing project lint/unit/integration/browser gates for implementation,
including new-code coverage; a fake analyzer/provider or fixture server must not
be reported as a real NLP, catalog, provider or Anki acceptance run.

## Measured evidence and deliberately unsupported cases

[Pinned-model validation](../evidence/faust-pinned-model-validation.md) records
the real production Producer and bounded gate-ablation results, identities and
reproduction asset. Exact source alignment and paragraph boundaries were
observed; the measured historical lemma/dependency errors are accepted limits.
It neither implements v2 nor proves the matrix above passed. No accuracy target,
minimum card yield or whole-Book omission rate is claimed. Revalidate the curated
cases when the pinned producer/model/configuration changes.

Unsupported: recovering lost structure from v1 text; paragraph-per-line verse
whose sentences cross blocks; inferred speakers/genre/stanzas; contexts spanning
readable units or RPC chunks; automatic repair of historical spelling/parses or
missing-byte legacy records; joining shared metrical lines; canonical verse
citations; exact indentation/fonts/pages; metre/rhyme/scansion/commentary; PDF,
OCR, TEI or a full-work reader; general validation of other editions/languages.

Implementation rollout follows ADR 0103's additive, single-version cutover. This
document authorizes no SQL, backfill, application change or production operation.
