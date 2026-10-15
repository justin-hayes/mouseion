# OGA Anabasis Book 1 import proof: binding verified, lexical decoding stopped

## Latest result: mapping completion (#2052, 2026-10-15)

**Both required mapping layers failed on measured, reproducible blockers, so
the pass stopped before the sentence, selection, cap, and omission checks.** The
retained binding stays verified. This is a bounded negative result: no Book 1
vocabulary is ready, and no learner-facing vocabulary was published.

Reproduce with `python3 -I mapping_completion.py run` from this directory. It
reads the retained `private/` inputs and a private Morpheus key index whose
SHA-256 is pinned in the script, and it writes `mapping-completion-summary.json`
(tracked) plus private ledgers. Tests: `python3 -m unittest discover -p 'test_*.py'`
(75 tests pass: 61 existing, 14 new).

### Inputs and pins

- Native nodes: 10,572 (`private/import/native-nodes.json`, sha256 `2a5aca90…`).
  All printed nodes match the retained CoNLL-U on surface, lemma, and native POS.
  110 empty nodes are unprinted.
- CoNLL-U (Book 1 subset, sha256 `0f9cbba8…`).
- Morpheus Greek stemlib at commit `b1b33c56ef2338fe0dcd1893628ed638f00c0986`
  (<https://github.com/PerseusDL/morpheus>). Full index of all 84 `stemsrc` files:
  188,173 records (lemlist 49,090; `:le:` blocks 126,862; `vbs.cmp.ml` compounds
  7,180; `vbs.cmp.lsj` compounds 5,041). The index is private, sha256
  `57f81c86…`; the per-file hash manifest is `private/mapping-completion/morpheus-manifest.json`.
- Undecodable keys (Latin or non-beta names) are kept only as loose records.
  They never resolve an identity: 293 `:le:` and 197 lemlist keys, plus 1 compound.

### Identity layer: failed (7 of 142 occurrences blocked)

| Raw lemma | Occurrences | Decision | Basis |
| --- | ---: | --- | --- |
| πρόσειμι1 | 6 | ambiguous-shared-key | Same key `pro/seimi1` from two derivation records, `vbs.cmp.ml:5639` (`pro/s-ei)=mi`) and `:5643` (`pro/s-ei)mi/`). |
| σύνλέγω2 | 1 | not-established-no-key | Derivation `su/n-le/gw2` yields `sulle/gw` with no digit (`vbs.cmp.ml:6459`). No digit-bearing key. `vbs.cmp.lsj` has no `su/n-le/gw` entry. |
| 19 other labels | 135 | key-resolved | One producer key with this headword and digit (see limit below). |

Blocked occurrence IDs: `t_302` (σύνλέγω2); `t_5468`, `t_6606`, `t_7682`,
`t_7815`, `t_9906`, `t_9922` (πρόσειμι1).

σύνλέγω2 is "not established by the pinned source", not "shown impossible".
Only the spelled prefix-compound derivation was checked, not the full LSJ table
for every possible compound.

**Limit on the 135 resolved occurrences.** No retained producer document defines
how OGA's digits relate to Morpheus's. The "key-resolved" decision therefore
assumes the digits correspond. A paradigm-POS check on nine sampled keys supports
that assumption at the POS level. Morpheus's own records agree with OGA's
native POS for `le/gw3` (verb), `h)/1` (conjunction), `ce/nos1` (masculine
noun), `eu)qu/s2` (adverb), `de/w2` and `xra/w2` (verbal), `qei=os1` and
`pisto/s2` (adjective), and `a)/rwma1` (neuter noun). This does not establish
sense-level correspondence, and it was not run for all 19 labels. The identity
verdict does not depend on this limit, because the layer already fails.

### Canonical POS layer: failed (938 of 10,462 printed nodes blocked)

Status totals: resolved-eligible 4,865; resolved-outside-AllowedPOS 4,659; blocked 938.
Eligibility uses the default `AllowedPOS` (NOUN, VERB, ADJ, ADV) from
`internal/selection/selection.go`. A blocked node has candidate UPOS tags that
mix eligible and ineligible values, or have two eligible values.

| Blocking rule | Nodes | Candidates and why unresolved |
| --- | ---: | --- |
| pos-n-adverb-status-conflict | 411 | Native `n` with ADV relation. AGDT 2.6 makes noun-with-adverb-status an adverb, but native POS cannot show that status. NOUN and ADV are both eligible. |
| pos-n-proper-undecided | 398 | Native `n` where Morpheus evidence is both starred (proper) and unstarred (common), or absent. NOUN is eligible; PROPN is not. Example: κῦρος (158 nodes) is both a proper name and a common noun. |
| pos-a-adverb-status-conflict | 113 | Native `a` with ADV/AuxY/AuxZ relation (AGDT 2.6). ADJ and ADV are both eligible. |
| pos-d-function-conflict | 11 | Native `d` with COORD, AuxC, or AuxP. ADV versus CCONJ, SCONJ, or ADP. |
| pos-d-unrecognised-relation | 4 | Relation `AuxΖ` (Greek capital zeta), absent from the consulted AGDT tables. ADV versus PART. |
| pos-c-adverb-function-conflict | 1 | Native `c` (πρίν) with ADV relation. SCONJ versus ADV. |

Resolved rules:

- `u`→PUNCT (1,157); `i`→INTJ (7); `l`→DET (1,047); `p`→PRON or DET (721; both ineligible).
- `c` COORD→CCONJ (473); AuxC→SCONJ (277); AuxY→CCONJ (5).
- `r` AuxP→ADP (577).
- `d` with ADV, ATR, OBJ, OCOMP, PNOM, AuxY, or AuxZ→ADV (1,476).
- `a`→ADJ (815).
- `n`→NOUN (723) when Morpheus has only unstarred keys; `n`→PROPN (278) when only starred.
- `v`→VERB (1,788). `v` εἰμί with a PNOM dependent in the same sentence→AUX (178, ineligible).

Limits and product consequences:

- **Particles become eligible.** `d` AuxY and AuxZ resolve to ADV, which is
  eligible under AllowedPOS. Particles such as μέν and οὐ would therefore be eligible vocabulary.
  ADR 0005 excludes function words, so this needs a product decision before any publication.
- **PROPN rule.** Starred Morpheus keys carry proper-name tags (e.g. `pers_name`).
  Whether starred-only lemmas match UD PROPN convention is not established. For
  example, ἕλλην (53 nodes) is starred-only and resolves to PROPN.
- **Copula rule.** An εἰμί is AUX only with a PNOM dependent in the same
  sentence. The head lookup was sentence-scoped after a bug fix during this pass.

### Downstream: not run

Sentence, selection, cap, and omission measurements were not run, because both
required layers failed. No tested subset is claimed as a ready Book.

### Not run

- Repository-wide `make lint` and `make coverage-new` (not part of this research directory).
- No Greek provider, model, dictionary, or training-corpus call was made.
- No archive redownload, and no edits to `private/archive*`, `private/inputs`,
  `private/text`, `private/import`, or accepted documents.

## Prior stop after explicit archive authorization (historical, 2026-10-14)

**Acquisition and complete Book 1 text binding succeeded; vocabulary-ready
publication stopped at required producer lexical decoding.** This is a bounded
negative full-workflow result, not a successful import into learner vocabulary,
source/MVP acceptance, or evidence that OGA is linguistically unusable.

Justin [explicitly authorized the official archive](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178890938),
superseding only the original no-archive acquisition limit. The same research
task/branch continued. No fallback corpus/edition, accounts/challenge bypass,
analyzer/model execution, provider calls, learner-state changes, cards, production
changes, PR or merge occurred. The previous gate evidence remains immutable at
`4104c8be238fe0d4b5500f8e18522e80dd516467`; `gate.py`, `evidence.json`, `ledger.json`
and the historical sections below still reproduce **that initial assessment**.
The current verdict is [import-ledger.json](ancient-greek-oga-anabasis1-import-proof/import-ledger.json).

### Exact acquired components and rights

The one ordinary official request completed: **8,629,032,478 bytes**, published
MD5 **`d4117ae52f1cc6319c6ba20af0c1cf3b`**, locally computed SHA-256
**`fffc246e9833f7ef265f0daffd1f4cc679489cb6de1fd9b652474a6313ecf6c3`**.
The source is [Zenodo record 14206061](https://zenodo.org/records/14206061),
`opera_graeca_adnotata_v0.2.0.zip`. A documentary subagent mistakenly unlinked
the partial file. Its original open inode was retained and restored locally
after the original request completed; size and checksums were independently
verified again. There was **no second download, network range extraction,
alternate source or bypass**. This incident is not an upstream access failure.

Only nested local ZIP directories and allowlisted Anabasis members were read;
no corpus-wide decompression or analysis occurred. See
[archive-members.json](ancient-greek-oga-anabasis1-import-proof/archive-members.json)
for every exact container/member, size, CRC and SHA-256, and
[unit-inputs.json](ancient-greek-oga-anabasis1-import-proof/unit-inputs.json)
for acquisition provenance. Important containers/members:

- `original_Greek_files.zip`: the original
  `canonical-greekLit-0.0.11376425141/data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`
  (within `original_Greek_files/`) is **byte-identical** to the upstream pinned
  work XML, SHA-256 `865cf2c761e55a0f603a2ddbb58a08e94934f963724552b268a1493e4b9ae2ec`.
- `workspace/merge.zip`: `merge/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`.
- `workspace/oga.zip`: `oga/oga_v0.2.0_2/tlg0032.tlg006.perseus-grc2/`
  carries primary PAULA text/ranges/features; `oga/laula/tlg0032.tlg006.perseus-grc2/`
  carries original-text references and native sentence groups.
- `workspace/conllu.zip`: the matching work's native-format file is retained,
  not mistaken for canonical UD merely because its container says CoNLL-U.

The archive's own README **99–107** explicitly grants CC BY-SA 4.0. Its README
**12–40** documents the PAULA, LAULA, original and normalized representations.
The original XML header identifies **Xenophon, Anabasis, E. C. Marchant,
Opera Omnia volume 3, Clarendon Press 1904/reprint 1961**, Perseus/Tufts and
preparation contributors, without a component-specific rights exception.
The upstream repository's explicit default BY-SA grant remains evidence.
[NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md) now separately
assesses these exact components: private inspection and bounded attributed
diagnostics proceed; full text/annotations/maps stay Git-ignored locally.
Exports/bundling, provider use and semantic/artifact acceptance are not approved.

### Full Book 1 binding and explicit reading projection

The proof measures **Book 1 only**, retaining the indivisible work unchanged:

| Measurement | Verified result |
| --- | --- |
| Extent | **1.1.1–1.10.19**, 10 chapters / 205 sections |
| Native nodes | **10,572**, required lemma/POS/CTS targets and text ranges resolved |
| Printed nodes | **10,462**, `t_1`–`t_10462` |
| Unprinted nodes | **110**, `e_1`–`e_110`; 95 literal `[0]`, 15 literal `[1]` |
| PAULA Book 1 source span | Unicode code points **[0, 59793)** |
| Marker-free reading | **59,353** code points; UTF-8 SHA-256 `0f18cae963899218fda6c62726d6a0b0429763f390597c1781a93d80c2ddd907` |
| Exact non-whitespace correspondence | **48,892 characters**, every section verified against both original and merged TEI |

Rules are versioned **`oga-anabasis1-native-binding/1`**. PAULA annotation-carried
text is the declared reading-text authority (ADR 0093); it is not casual joining
of lemma/token strings. Every 1-based native range is verified as a half-open
Unicode-code-point span. All adjacent node spans have exactly one source space;
there are no overlapping ranges, unexplained gaps or omitted printed characters.
Printed referents agree exactly with the separate LAULA token attributes.

Remove **only** verified `e_*` source spans and their following single separator:
each also has native POS `-` and a zero-length, empty LAULA referent. No regex
deletes lookalike printed content. All printed characters, polytonic marks and
remaining PAULA spacing are unchanged, including spaces around punctuation.
Every reading character maps back to its exact PAULA source coordinate; empty
nodes remain retained evidence with **no invented reading span**.

The original and merged Book 1 section string-values are exact, not merely NFC
equivalent. TEI comparison omits explicit `del` text, chooses `corr` inside
`choice`, retains `add` and standalone `sic`, and permits **only whitespace
reflow**. Significant accents, case, apostrophes and punctuation are not folded.
Every printed character maps monotonically to its original TEI section
string-value coordinate, including split forms; this is not fuzzy alignment or
an inferred lexical span. Suppressed editorial material remains in the originals
and the diagnostic element spans; it is not silently made a learner exclusion.
These rules describe this exact release's correspondence, not universal Greek
tokenization or an accepted production adapter.

Optional LAULA pointers are separately qualified: **486 nodes in 19 sections**
do not reproduce the claimed referent when their literal element path is
resolved and the element string-value is whitespace-folded. For example,
`t_876` at 1.2.5 restarts at position 1 but targets the section rather than its
later paragraph; `t_3328` at 1.3.20 resets after editorial markup. These are
representation diagnostics, **not failures of the independently verified PAULA
binding** or a claim that every possible LAULA implementation behaves identically.
Also, **47 empty nodes** carry a citation different from their preceding printed
node. Their raw metadata is retained, not used to reorder text or invent words.
The ledger lists exact IDs/sections for both qualifications.

### Required lexical stop, not a vocabulary result

**142 printed occurrences have 21 distinct raw lemma strings ending in digits**,
including 49 `λέγω3` predictions, nine `πάρειμι1`, one `πάρειμι2`, three each of
`ἄπειμι1` and `ἄπειμι2`. The ledger lists **all affected native IDs, citations,
raw lemma values and native POS**; they are not hidden by defaults or exclusions.

The release's `query/README.md:32` documents lowercased NFC lemmas, and its
`query/morphology.md` documents native POS meanings. The pinned OGA README
[66–78][oga] attributes automatic parser/lemmatizer outputs to AGDT-trained
models, linking the [producer paper](https://arxiv.org/abs/2410.12055).
That is provenance, **not a verified decoding table or lexical namespace for
these exact predicted numeric strings**. The investigated documentary evidence
has not established their source-declared discriminator meanings, namespace,
or complete citation-headword decoding. This is a bounded verification gap,
not a claim no upstream explanation can exist or that numeric predictions are
necessarily errors. An unpinned mapping draft was not promoted into authority.

The main session completed the draft's documentary gaps, retaining exact
originals/hashes in [producer-docs.json](ancient-greek-oga-anabasis1-import-proof/producer-docs.json):

- [Producer paper v1 §3.1](https://arxiv.org/html/2410.12055v1#S3.SS1)
  describes heterogeneous training conventions and normalization, including
  numbered ellipsis placeholders; §4.4 describes character-sequence lemma
  prediction. It does **not** define these lexical numeric endings as verified
  dictionary IDs. Aggregate model scores do not certify Book 1 predictions.
- [Lemmatizer README at `267dbf045dfca1129115aef05ba9ac6ebc49f4db`](https://git.informatik.uni-leipzig.de/celano/lemmatizer_for_oga/-/blob/267dbf045dfca1129115aef05ba9ac6ebc49f4db/README.md)
  identifies **GreTa**, its 91.41 score and inference interface, but supplies
  no numbered-lemma citation/discriminator decoding. Only the README was
  retrieved, not code, models or training data; no commands in it were run.
- [AGDT2 guidelines §2.5 at `0ebc9289bc6c8eb1069fac3433955f83f442762f`](https://github.com/PerseusDL/treebank_data/blob/0ebc9289bc6c8eb1069fac3433955f83f442762f/AGDT2/guidelines/Greek_guidelines.md#vrb)
  say Morpheus numbers **should be deleted in the annotated lemma field**.
  This is genuine annotation-policy evidence, not a dictionary namespace or
  proof that a learned OGA prediction of either contrasting numbered form is
  a valid source-declared identity. Mechanically importing this deletion rule
  into canonical identity would erase the distinction ADR 0094 requires us
  to preserve, without verifying what the differing predictions signify.

An initial arXiv `v2` HTML request returned 404; the existing `v1` above was
read normally. No challenge, alternate corpus or archive transport was involved.

Do not strip the digits, silently merge the contrasting forms, manufacture an
`OGA`/`Morpheus` homograph equivalence scheme, or publish numbered raw predictions
as verified citation headwords. This blocks required identities under ADR 0094.
**No canonical mapping was adopted for the other nodes either**: native `n/v/d`
are not automatically universal UD NOUN/VERB/ADV, and no capitalization-based
PROPN or blanket εἰμί→AUX exclusion was imported. Native POS counts in the ledger
are **native**, not canonical vocabulary/eligibility denominators.

Per the task's stop condition, downstream sentence completeness/practical input
judgments, Go selection/statistics, independent model/Known comparison, 3/15
selection, cap distribution, deterministic inputs and omissions are **unperformed,
not zeros**. Native sentence groups and optional annotation files are retained
but not promoted into optional-capability success to bypass this required stop.
Gloss/translation, dictionaries, Anki/client artifacts and their semantics remain
unassessed. No learner state or supported-language implementation changed.

### Comparison and decision returned

OGA now has a verified, maintainably pinned complete Book 1 **text-binding
result**, beyond its preserved Stage 1 example. Its marker-free authority and
original-text transformation are explicit; optional link metadata needs care.
Required lexical decoding remains unresolved, so there is no comparable accepted
selection or dry-input yield. This research does not select or reject OGA for
the live [source decision](https://github.com/justin-hayes/mouseion/issues/2030).
Any continuation must first establish the exact producer decoding/mapping
evidence, not repeat the archive download, thin Book 1 or switch editions.

The retained GLAUx proof remains provisional: 1,294 proposed identities, 373
selected and 372 dry inputs are **not accepted vocabulary or a ready deck**.
Retain its 63 lexical words in editorial deletions, presentation/headword/POS
and sentence-boundary qualifications, and its unsupported blanket εἰμί→AUX
rationale affecting 180 occurrences (cited UD evidence: 1,864 AUX and two VERB).
No count difference establishes superiority or a prediction error by itself.
The exact OGA original/merged release has a `sic/corr` choice at **1.3.20**;
GLAUx's retained `text_error_propagated` example instead targets **1.3.14**.
That particular row is **not a verified witness** to this 1.3.20 correction.
The general revision qualification remains; neither earlier branch is modified
and no GLAUx remediation becomes a prerequisite.

### Reproduce the current bounded result

Python **3.13 / Unicode 15.1.0**, standard library only. With the retained official
archive in Git-ignored `private/archive/`, one offline command rechecks the whole
archive size/MD5/SHA-256, extracts only allowlisted work members, checks member
and retained producer-document hashes, validates the Book 1 binding/projection, compares the current
ledger **byte-for-byte**, reconstructs private coordinate artifacts and runs all
focused tests:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 doc/research/ancient-greek-oga-anabasis1-import-proof/import_gate.py --from-archive
```

Without `--from-archive`, it verifies retained selected-member hashes and reruns
the same binding/ledger/tests, without rereading 8.63 GB. `--write` explicitly
regenerates the ledger, never overwrites differing private projections, and is
not used for verification. The official archive request is not retried by any
reproducer; independent acquisition needs explicit access/scope authorization.
The original one-shot acquirer and extraction helper remain inspectable code.
The supplemental producer originals must also be present under
`private/producer-docs/` at the hashes in `producer-docs.json`; they are small
documentary inputs, not part of the corpus archive. No supplementary document
or archive retrieval occurs implicitly in this offline command.

**61 tests pass**: the 17 historical metadata tests, 10 acquisition tests, eight
invented-ZIP tests and 26 invented-Unicode/XML binding/identity-stop tests. These
exercise wrong ranges/bases/references, missing nodes/interior sections/chapters,
duplicate targets, Unicode NFD/non-BMP coordinates, typed versus printed marker
lookalikes, edition/character disagreement, lexical omission, entity declarations,
ZIP bounds/path safety and unsupported numbered identity defaults. Invented
fixtures are not claimed as source observations or sentence-quality tests.

Full originals, the complete native-node manifest, reading text and coordinate
maps stay private; public hashes/element spans/blocker IDs allow inspection of
the bounded claims. Producer model/version/configuration specific to each
released layer is not reconstructed or executed; undocumented details stay
unknown. The older documentary-only command below remains a reproduction of
the **initial** acquisition assessment, not the present verdict.

Validation on **2026-10-14**: archive-inclusive and retained-member-only commands
reproduce byte-identical current diagnostics; every selected member's original
size/SHA-256 was independently checked again, and public section/native/blocker
totals reconcile. `git diff --check` passes. Reattempted `make lint` is blocked
by missing **golangci-lint 2.14.0**; `make coverage-new` stops at the missing
**`.venv`/pytest** preflight. Neither whole-project check is claimed to pass.

## Historical initial gate result

**Result: no complete-unit import was attempted.** The examined official download
inventory supplies one whole-corpus archive, not an individual Anabasis package.
That route is explicitly prohibited by
[Prove a complete OGA Anabasis Book 1 import into Mouseion vocabulary workflows](https://github.com/justin-hayes/mouseion/issues/2052).
No permitted individual-work package was established, so the task stops at Gate 1.
This is a bounded negative acquisition result, **not evidence that OGA lacks
Anabasis or is linguistically unusable**, and not source/MVP acceptance.

Companion [registry](ancient-greek-oga-anabasis1-import-proof/inputs.json),
[documentary evidence](ancient-greek-oga-anabasis1-import-proof/evidence.json),
[verdict ledger](ancient-greek-oga-anabasis1-import-proof/ledger.json),
[operation-specific notice](ancient-greek-oga-anabasis1-import-proof/NOTICE.md), and
[reproducer/tests](ancient-greek-oga-anabasis1-import-proof/gate.py).

## Exact availability and acquisition assessment

Assessment date: **2026-10-14**. OGA documentation is pinned to
**`a69d917dbe117a307829977dd556e22f05ce0715`**, also the observed `main` HEAD.
This pins documentation and the example, **not a full-unit release payload**.

1. [OGA README, lines 11–41][oga] names the source releases, links the corpus
   download to Zenodo, and separately advertises hosted ANNIS queries and a
   desktop corpus. Its [example README][example] directs users to Zenodo too.
   The [complete, non-truncated Git tree][tree] has no Anabasis payload path;
   its annotation example is `ggm0001.ggm001.1st1K-grc1`, not Xenophon. No
   example was substituted for Book 1.
2. [Zenodo record 14206061][zenodo] lists exactly one file:

   | Field | Published value |
   | --- | --- |
   | Release | OGA `0.2.0`, DOI `10.5281/zenodo.14206061`, 2024-11-24 |
   | File | `opera_graeca_adnotata_v0.2.0.zip` |
   | File UUID | `96c1ae43-3193-4386-94ec-4f62263b1cae` |
   | Size | **8,629,032,478 bytes** (8.63 GB decimal) |
   | Published checksum | `md5:d4117ae52f1cc6319c6ba20af0c1cf3b` |
   | Scope | Whole-corpus archive; **not acquired** |

   These are inventory declarations, not an inspected archive-member manifest.
   No archive SHA-256, member path, unit size or complete Book 1 coverage is
   invented. The archive content endpoint is deliberately absent from the
   reproducer's request allowlist. No HEAD, GET, range/stream extraction, mirror
   or alternative transport request to that file was made.
3. [OGA's bibliographic list, lines 5562–5570][bibliography] has one target
   record: `tlg0032.tlg006.perseus-grc2`, Xenophon, *Anabasis*, Marchant,
   *Xenophontis Opera omnia Volume III Expeditio Cyri*. This establishes an
   advertised **work/edition lead**, not Book 1 endpoints, interior coverage,
   original text correspondence or annotation availability in an inspected unit.
4. The original-text lead is canonical-greekLit release `0.0.11376425141`.
   Its [tag reference][tagref] identifies annotated **tag object**
   `90ac67b935af3b46d6f910c1e173ffd99c657d78`; [peeling that object][tag] gives
   **commit** `476fb6133498e5710de7eec0b492791da86be4b4` (not the tag object).
   The [work-directory listing][listing] advertises
   `data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`, **886,347 bytes**,
   Git blob `a220f4b8accd1448a0189a595493a78aa09c707b`.
   The [CTS metadata][cts] agrees on the edition identifier and bibliography.
   **The Greek work file was not fetched.** Its internal structure, full extent,
   SHA-256 and relationship to OGA remain unverified. An individual original
   text without the accompanying OGA annotation does not satisfy this task.
5. The [official website][site] sends downloads to the same Zenodo record.
   Its [ANNIS page][annis] documents querying/visualization, not an advertised,
   revision-pinned, source-preserving individual Anabasis package. No hosted
   corpus query/export, endpoint probing or reconstruction by paginating matches
   was performed. This does **not** establish that ANNIS can never export such
   data, or that every query is a prohibited survey. A suitable supported unit
   export remains **unestablished**, not silently treated as archive access.

**Gate decision:** no demonstrated acquisition route satisfies the existing
individual-unit/work-package boundary. The observed archive route is out of
scope regardless of download feasibility. No package was acquired while deciding
its permission, and no Gate 2 work followed the stop. The bounded inventory is
not a global claim that no other file or future upstream export exists.

## Component rights and the actual decision needed

OGA [README lines 162–170][oga] explicitly declares **CC BY-SA 4.0** and refers to
the original-text repositories. The declaration expressly accompanies a README
that identifies the corpus/release. Zenodo's `other-at` shorthand is recorded
without treating it as a competing prohibition, a missing license or a veto.
Likewise, the upstream [README][upreadme] explicitly licenses repository contents
BY-SA 4.0 unless otherwise indicated, while retaining Tufts's general copyright
and varying-component notices. Neither notice automatically cancels that grant
or makes contacting anyone a prerequisite.

Those declarations support this documentary assessment; they do not establish
which exact uninspected text/header/annotation components a full-unit package
would contain or their verified derivation. **Exact package/component clearance
is unperformed, not a finding that private computation is prohibited.** No
alternative license was guessed, and no legal or Greek-classification question
is delegated to Justin. Local inspection, supported acquisition, public source
redistribution, exports/bundling and external-provider operations have separate
dispositions in [NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md),
following [Choose the licensing and distribution boundary for classical data](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026).

The concrete scope decision returned to Justin is whether to **leave this OGA
proof deferred under the current no-archive boundary**. Continuing is not
authorized on present evidence. It would require an identified permitted unit
package (with its component review), or a separately explicit change of scope.
This task does **not** offer an archive retry as an executable next step, create
an access/survey prerequisite chain, contact anyone, or choose another corpus,
edition or Reading unit. The live source/usefulness decision remains
[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030).

## Unperformed layers and comparison

| Layer | This OGA task |
| --- | --- |
| Acquisition | **Stopped**: no demonstrated in-scope individual package |
| Exact full unit / edition | Bibliographic lead only; completeness and exact edition unverified |
| Binding / clean reading-text projection | Not performed |
| Required lemma / UPOS / discriminators / headwords | Not performed |
| Complete sentences / verified targets | Not performed |
| Native morphology / syntax / optional capabilities | Not assessed on Book 1 |
| Real Go selection / independent model / Known comparison | Not run |
| Counts / 3–15 selection / selected occurrence coverage | **Unmeasured, not zero** |
| Deterministic sentence inputs / cap distribution / omissions | **Unmeasured, not zero** |
| Practical examples / prediction quality | No Book 1 examples acquired or judged |
| Dictionary / Gloss / translation / Anki / client rendering | Not performed |

Consequently there are no Book 1 node, selection, sentence, card-input or omission
manifests masquerading as successful empty results. The ledger uses `null` for
those measurements. No Greek service/analyzer/model, provider call, dictionary
acquisition, card construction or learner-state change occurred.

The retained [OGA official-example format probe][stage1] and the retained
[GLAUx complete-unit proof][glaux] are unchanged. This task does not redo their
tests or promote their conclusions beyond the independent parent verification:

| Comparison axis | OGA evidence | GLAUx evidence |
| --- | --- | --- |
| Acquisition / maintenance | Documentary pin and linked sample work; no demonstrated complete-unit package under this task's limits | Individually addressable pinned annotation and text files; real full-unit reproduction exists |
| Coherent complete text/analysis | Stage 1 shows linked-format feasibility only; no Book 1 binding | Parent checked all 10,645 native-node correspondences, 10 chapters/205 sections and full source-span coverage |
| Required repair / practical quality | Full-unit requirements and prediction quality unassessed, not presumed better or worse | 63 unannotated lexical words in editorial deletions; editorial-sign presentation, headword/POS quality, a boundary case and the annotated 2019 text's 1.3.14 typo remain qualifications |
| Vocabulary / dry inputs | Unmeasured | Proposed interpretation: 1,294 identities, 373 selected, 372 deterministic inputs; **not accepted vocabulary or a ready deck** |
| Rights | Explicit declarations preserved; unacquired exact components unassessed | Exact-file declarations and prior operation-specific assessment exist, not blanket source/export approval |

In particular, GLAUx's blanket εἰμί→AUX rule affects 180 occurrences. The cited
UD evidence has **1,864 AUX and two VERB** εἰμί occurrences; a supported
rationale/refinement is still needed. Neither all 180 exclusions nor all 180
being errors follows mechanically. No such rule is copied into OGA.
Edition-aware output comparison is impossible without OGA's exact acquired text.
Different future counts would not by themselves establish errors or superiority;
neither GLAUx nor human annotation is assumed always correct. Parent
[comparison/qualifications](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178553143)
remain controlling. No candidate or whole-Anabasis readiness is accepted here.

## Reproduce and independently inspect the stop

Python **3.13 / Unicode 15.1.0**, standard library only. One command, repo root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 doc/research/ancient-greek-oga-anabasis1-import-proof/gate.py --fetch
```

This fetches **only 13 allowlisted documentation/inventory inputs**, validates
their hashes, derives the evidence/ledger, compares both **byte-for-byte**, and
runs **17 focused tests**. It never follows response URLs, XML references or
redirects, never retries automatically, and cannot fetch archive/text/annotation
payloads through its allowlist. `--fetch` means documentary retrieval, **not
Gate 2 acquisition**. Without that flag it is offline. `--cache PATH` selects
retained inputs; `--write` explicitly regenerates the two diagnostic files.

The registry records original response byte sizes/SHA-256. API JSON also has a
canonical-JSON SHA-256 over **every parsed field**, because GitHub returns
equivalent JSON with different whitespace/content negotiation. Evidence hashes
declare that distinction. Raw documentary originals are unchanged in Git-ignored
`ancient-greek-oga-anabasis1-import-proof/private/inputs/`; no full-unit source
or normalized reading text is there. The full bibliographic inventory is not
republished: only the target record is projected publicly. Published checksums
of unacquired payloads are clearly distinguished from locally verified hashes.

API/website responses are dated snapshots, not immutable upstream objects.
Changed metadata must fail verification; it must not silently refresh this
assessment. Independent reviewers can inspect the cited inventories and the
public target-specific evidence even if raw response snapshots later change.
The Git tree and raw repository files use the declared revisions. No global
inventory size is used as Book 1 coverage evidence.

Validation on 2026-10-14: retained-input and fresh-documentary retrieval both
reproduce exact diagnostics and pass all 17 tests. Initial raw-byte-only API
verification rejected equivalent JSON formatting; canonical whole-JSON checking
was added and tests cover both serialization equivalence and changed values.
Mutation/failure tests additionally reject wrong/incomplete tree revisions, new
unreviewed target packages, missing/duplicate/wrong work or edition references,
changed release inventories, tag/commit confusion, missing declarations,
non-allowlisted payload URLs, corrupt inputs and redirects. They are **metadata
gate tests**, not span/mapping/sentence validation or claims of corpus correctness.

`git diff --check` passes. Whole-project `make lint` stops because
`golangci-lint` 2.14.0 is missing; `make coverage-new` stops because the project
`.venv`/pytest environment is absent. No full-tree lint or application coverage
success is claimed for this standalone research artifact. No production code,
schema, dependencies, ADR, glossary, earlier research branch, PR or merge changed.

[oga]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/README.md
[example]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/README.md
[tree]: https://api.github.com/repos/OperaGraecaAdnotata/OGA/git/trees/a69d917dbe117a307829977dd556e22f05ce0715?recursive=1
[bibliography]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/urn_cts/texts/urn_cts_plus_date_label.xml#L5562-L5570
[zenodo]: https://zenodo.org/api/records/14206061
[site]: https://varro.informatik.uni-leipzig.de/oga/en/index.html
[annis]: https://varro.informatik.uni-leipzig.de/oga/en/annis.html
[tagref]: https://api.github.com/repos/PerseusDL/canonical-greekLit/git/refs/tags/0.0.11376425141
[tag]: https://api.github.com/repos/PerseusDL/canonical-greekLit/git/tags/90ac67b935af3b46d6f910c1e173ffd99c657d78
[listing]: https://api.github.com/repos/PerseusDL/canonical-greekLit/contents/data/tlg0032/tlg006?ref=0.0.11376425141
[cts]: https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/data/tlg0032/tlg006/__cts__.xml
[upreadme]: https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/README.md
[stage1]: https://github.com/justin-hayes/mouseion/blob/f0034fea98458491ce24413c8a24717c41b3cfd8/doc/research/ancient-greek-oga-format-probe.md
[glaux]: https://github.com/justin-hayes/mouseion/blob/957c99f25de9ea16e139055e6a87bbcda1996cc9/doc/research/ancient-greek-glaux-anabasis1-import-proof.md
