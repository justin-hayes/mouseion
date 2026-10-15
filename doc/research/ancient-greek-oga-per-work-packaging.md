# Versioned per-work OGA packages for on-demand acquisition (Anabasis fixture)

Research for
[Evaluate versioned per-work OGA packages for on-demand acquisition](https://github.com/justin-hayes/mouseion/issues/2053),
part of the [Ancient Greek wayfinder map](https://github.com/justin-hayes/mouseion/issues/2021)
and feeding the
[architectural handoff](https://github.com/justin-hayes/mouseion/issues/2031).
Date: 2026-10-15. Researcher: Claude Code Opus 5.5.

This is a packaging and rights investigation. It does not accept OGA as the
source; that decision stays with
[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030).
It also does not authorize redistribution, hosting or implementation, and it
does not reopen the completed
[Book 1 import proof](https://github.com/justin-hayes/mouseion/issues/2052).
It is not legal advice.

## Verdict: feasible with named conditions

**Technically, a small, versioned, byte-preserving per-work package works.** I
assembled a 31-member Anabasis "core" package privately from the retained
official archive. It is **4,234,495 bytes**, about 1/2,038 of the
8,629,032,478-byte archive. Assembly is deterministic. Every member is
byte-identical to its archive original. I then ran the reviewed proof code
from commit `e610a5aa` against that package alone. With the explicit
research-repair layer, it reproduces every whole-unit result byte-for-byte:

- 10,572 native nodes;
- 6,037 eligible occurrences and 1,426 identities;
- **438 selected identities and 434 sentence inputs**;
- the uncorrected 437/433 comparison;
- real-Go parity.

The run had no Gorman file present.

**The preferred route is one-time archive extraction plus reviewed, immutable,
Mouseion-hosted per-work packages.** It goes ahead only under these named
conditions:

1. **Source acceptance.** OGA Anabasis Book 1 is accepted in
   [#2030](https://github.com/justin-hayes/mouseion/issues/2030).
2. **Redistribution review.** An owning decision records that the existing
   exact-component evidence is enough to approve Mouseion redistribution of the
   specific core package, as rule 6 of the
   [licensing decision](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026)
   requires. Every package component has an explicit CC BY-SA 4.0 declaration
   and I found no file-level exception (see [Rights](#3-component-and-operation-rights)).
   **That approval has not been given. This research does not give it.**
3. **A separate interpretation artifact.** Mouseion's own interpretation layer
   (decoder and mapping rules plus the 13 research repairs) is versioned
   separately from the source package and gets its own rights review. Two of
   its inputs are third-party witnesses:
   - Morpheus-derived numbered-lemma decodings (CC BY-SA 3.0 US);
   - conjunction decisions that the proof derives at run time from
     **UD Perseus (CC BY-NC-SA 2.5)** counts.

   Neither witness may be bundled. The NC-derived decisions are the one
   **unresolved runtime-rights item** (see [Interpretation layer](#interpretation-layer-is-not-part-of-the-source-package)).
4. **Unit-scoped validation.** Only Book 1 is validated. The package carries the
   indivisible whole-work files, but Books 2–7 must not be offered as
   vocabulary-ready until validated.
5. **An operations owner.**
   [#2031](https://github.com/justin-hayes/mouseion/issues/2031) assigns
   hosting and release maintenance. No service is designed or provisioned here.

**The alternative, per-installation archive download and extraction, is also
technically feasible.** It has the simplest rights position: direct
acquisition of BY-SA material, with no Mouseion redistribution. But every
installation pays an 8.63 GB transfer, plus disk space if the archive is cached.

**No blocker prevents either route technically.** The blocked items are
specific:

- Mouseion-hosted redistribution remains *unapproved*.
- Rights are *unresolved* for NC-derived runtime interpretation data.
- Gorman stays *excluded*.

## 1. Package boundary and required components

### What the proof actually reads

I traced the proof code at
[`e610a5aa`](https://github.com/justin-hayes/mouseion/tree/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof).
Binding uses only seven OGA members and the upstream original, through
`import_gate.assess`
([source](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/import_gate.py#L293-L308)).
The later interpretation passes add the CoNLL-U member and pin the OGA
`query/README.md`
([`opus_completion.py` pins](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L56-L84)).
None of these passes reads the PAULA IPA or dependency layers, the LAULA
sentence segmentation, the `merge` copy, the plain-text rendering or the
ellipsis table.

The source package therefore has four tiers. All sizes and hashes below are
measured; see [`work-inventory.json`](ancient-greek-oga-per-work-packaging/work-inventory.json).
Archive paths are archive-internal, not public URLs.

| Tier (in core package?) | Members | Uncompressed bytes | Role |
| --- | --- | ---: | --- |
| `core` (yes) | 9 | 42,256,593 | Exact proof inputs. **PAULA** (`oga_v0.2.0_2`): primary text, `tok01`, `tok01_cts01`, `tok01_lemma01`, `postag`. **LAULA**: merged TEI and `tok01`. **Upstream TEI**: `tlg0032.tlg006.perseus-grc2.xml`. **CoNLL-U**: annotated lemma |
| `notice` (yes) | 5 | 36,485 | OGA `README.md` (licence and citation); OGA `query/README.md`; `original_Greek_files/README.md`; canonical-greekLit `README.md` and `license.md` |
| `metadata` (yes) | 10 | 4,035 | Eight PAULA `anno_*.xml` work-metadata files; canonical-greekLit `__cts__.xml` for the textgroup and the work |
| `schema` (yes) | 7 | 3,312 | PAULA DTDs. Every PAULA member names one in its `DOCTYPE` (`../../paula_dtd/…`). The proof parser does not load them, but they keep the package self-describing |
| `optional` (no) | 9 | 17,464,641 | PAULA IPA, `dep01` and `dep-fnc01`; LAULA sentence segmentation; `merge` TEI (same bytes as the LAULA merged TEI); upstream `.txt`; ellipsis CSV; `query/morphology.md` and `query/syntax.md` |

Three groups of same-work or same-release material are deliberately outside
every package:

- the English translation (`perseus-eng2` XML/TXT), which is a different
  Edition;
- the corpus-wide `graphannis.zip` (4,286,995,714 bytes), split into five
  partitions rather than per work;
- all other works.

Identity of the core data members (full list with CRC32 in the inventory):

| Archive container : member | Bytes | Archive-compressed | SHA-256 |
| --- | ---: | ---: | --- |
| `workspace/oga.zip` : `oga/oga_v0.2.0_2/tlg0032.tlg006.perseus-grc2/tlg0032.tlg006.perseus-grc2.xml` | 708,041 | 183,059 | `8a30df6e…2236570` |
| … `tlg0032.tlg006.perseus-grc2.tok01.xml` | 6,207,730 | 718,861 | `8ec7eb72…a4806` |
| … `tlg0032.tlg006.perseus-grc2.tok01_cts01.xml` | 3,988,307 | 361,906 | `d69b649d…514422` |
| … `tlg0032.tlg006.perseus-grc2.tok01_lemma01.xml` | 3,845,647 | 575,404 | `139cc291…bf7bf8` |
| … `postag.tlg0032.tlg006.perseus-grc2.tok01_postag01.xml` | 11,978,867 | 592,995 | `f6a57aac…cc529` |
| `workspace/oga.zip` : `oga/laula/tlg0032.tlg006.perseus-grc2/tlg0032.tlg006.perseus-grc2.xml` | 886,260 | 199,109 | `0a9b2491…bc3043` |
| … `oga/laula/…/tlg0032.tlg006.perseus-grc2.tok01.xml` | 9,165,286 | 775,673 | `a0cb6c38…f1d75` |
| `original_Greek_files.zip` : `original_Greek_files/canonical-greekLit-0.0.11376425141/data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml` | 886,347 | 199,139 | `865cf2c7…9ae2ec` |
| `workspace/conllu.zip` : `conllu/tlg0032.tlg006.perseus-grc2.tok01_sentence-seg01_annotated_lemma.conllu` | 4,590,108 | 840,588 | `0f9cbba8…542780` |

All 27 members recorded by the earlier proof
([`archive-members.json`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/archive-members.json))
match this independent inventory exactly in size, CRC32 and SHA-256.

### Source revision notices

- **Release.** Opera Graeca Adnotata v0.2.0, Zenodo record 14206061,
  doi:[10.5281/zenodo.14206061](https://doi.org/10.5281/zenodo.14206061). The
  archive file is `opera_graeca_adnotata_v0.2.0.zip`, 8,629,032,478 bytes, MD5
  `d4117ae52f1cc6319c6ba20af0c1cf3b`, SHA-256
  `fffc246e9833f7ef265f0daffd1f4cc679489cb6de1fd9b652474a6313ecf6c3`. The
  retained record lists this single file, published 2024-11-24, with creator
  Giuseppe G. A. Celano (ORCID 0000-0002-7699-2566). The record is retained
  privately in the earlier proof and hashed in its
  [`inputs.json`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/inputs.json).
- **Upstream text.** PerseusDL/canonical-greekLit release `0.0.11376425141`,
  commit `476fb6133498e5710de7eec0b492791da86be4b4`, as bundled in the archive.
  The archive's own `original_Greek_files/README.md` names this release tag. The
  bundled README, `license.md` and work `__cts__.xml` are byte-identical to the
  copies independently retained from GitHub at that tag (SHA-256 `9733e53b…`,
  `ccf0e8ce…`, `7d505da6…`).
- **Edition.** The TEI header names Xenophon, *Anabasis*, ed. E. C. Marchant,
  *Opera Omnia* vol. 3 (Oxford: Clarendon Press, 1904; repr. 1961). It credits:
  - sponsor: Perseus Project, Tufts University;
  - principal: Gregory Crane;
  - preparation: Lisa Cerrato, William Merrill, Elli Mylonas, David Smith;
  - funder: The Annenberg CPB/Project;
  - publisher: Trustees of Tufts University.

  Its `revisionDesc` runs from 1992 to 2016-11-14. It has no `availability` or
  licence element.
- **Text normalization.** OGA documents that its `merge`, LAULA and PAULA text
  are NFC- and apostrophe-normalized derivatives of the originals
  (`original_Greek_files/README.md`, SHA-256 `e6837dc6…`). The PAULA primary
  text is therefore OGA's adaptation. The upstream TEI is the Edition witness.
  This is the same text-authority relationship the earlier proof already
  verified character for character.

### Byte-preserving whole-work files are sufficient; subsetting is not

The package carries whole-work files. Book 1 is the bounded, validated Reading
unit:

- CTS `1.1.1`–`1.10.19`;
- 205 sections;
- PAULA code-point span `[0, 59793]` of the whole-work primary text;
- 59,353 reading code points
  ([`import-ledger.json`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/import-ledger.json)).

The PAULA layers are stand-off. Tokens address character ranges in one
whole-work text, and features address token IDs. A Book-1-only cut would
therefore rewrite offsets and IDs. That would produce new, Mouseion-transformed
artifacts that no longer match the pinned original hashes. Such a cut would need
its own version and a complete re-validation, and as an adaptation it would
trigger BY-SA ShareAlike. The whole-work core package costs only 4.2 MB, so
**original byte-preserving selection is sufficient and preferable**.

Carrying Books 2–7 is not validation of them. In the terms of
[ADR 0093](https://github.com/justin-hayes/mouseion/blob/bb438d2e80410554ac7ce00122b1aa8a261013b8/doc/adr/0093-classical-identity-and-reading-units.md),
one Text Source acquisition may support several Reading units. A whole-work
Book must not be presented as analyzed on the strength of Book 1.

### Interpretation layer is not part of the source package

The proof's interpretation consumes third-party evidence at run time. The code
shows it:

- `interpret_lemma` decodes numbered labels against the Morpheus index and stem
  files
  ([source](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L280-L323)).
- `interpret_pos` resolves native `c` (conjunction) by looking up
  (letter, lemma) UPOS counts in the UD Perseus treebank
  ([source](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L426-L451)).
  `ud_perseus_evidence` also fails closed unless UD Perseus confirms the fixed
  letter table
  ([source](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L351-L381)).

In the repaired node ledger:

- 142 printed nodes (20 raw labels) use a Morpheus key or compound record
  (`I-NUM-MORPHEUS-KEY` 141, `I-COMPOUND-RECORD` 1). Seven more use
  `R-CITATION-REPAIR`.
- **756 conjunction nodes (29 lemmas)** take CCONJ (541) or SCONJ (215) from UD
  Perseus pair counts (`P-C-PAIR`). All 756 are non-vocabulary either way, so
  selection does not depend on them. The canonical UPOS evidence does.
- The εἰμί v3 rule and the six POS repairs use only native CoNLL-U syntax.

The 13 repair records
([`opus-repair-records.json`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus-repair-records.json))
mix three things:

- **values:** node, raw value, repaired headword/UPOS;
- **Mouseion reasons;**
- **witnesses:** Morpheus lines, and UD Perseus lemma/form counts under NC.

A runtime acquisition should therefore pin three separate artifacts:

| Artifact | Contents | Owner | Shipped to installations? |
| --- | --- | --- | --- |
| Source package | Byte-identical OGA/upstream members, notices, manifest | Upstream creators; Mouseion packaging | Yes, if redistribution is approved |
| Interpretation artifact | Ruleset versions, frozen per-label decoding, conjunction decisions, repair *values* bound to component hashes | Mouseion research (owning decision needed) | Yes, after its own rights review |
| Proof witnesses | Morpheus stemlib/index, UD Perseus treebank, UD/producer docs, Gorman | Third parties | **No.** Proof-only, cited, kept private |

The proof run here staged 51 witness files outside the package, from the
retained proof. They are hash-checked against the existing
[registry](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus-completion-inputs.json).
They are not a runtime requirement.

## 2. Measurements

Everything below comes from the probe in
[`ancient-greek-oga-per-work-packaging/`](ancient-greek-oga-per-work-packaging/).
It runs on Python 3.13.5 with zlib 1.3.1 and the standard library only. Private
outputs go under its Git-ignored `private/`; disposable work goes under
`~/.cache/scratch/greek-oga-per-work-packaging/`.

| Package | Members | Uncompressed member bytes | ZIP bytes | ZIP SHA-256 | Manifest SHA-256 |
| --- | ---: | ---: | ---: | --- | --- |
| `oga-v0.2.0_tlg0032.tlg006.perseus-grc2_core.zip` | 31 | 42,300,425 | **4,234,495** | `19bc870c02ceb0bd2be08ef952ecb7945b879d1aaebee1cb0f63be8de14ead76` | `7bb405000ac2f4aabd0819bbe831451784af4e15aab059e6df1e610fa973273a` |
| `…_complete.zip` (adds `optional`) | 40 | 59,765,066 | 6,432,447 | `5f6eb2522295b50bc990cb32cbeda6c610a4384ade935f82c1e0b4b23dd693a7` | `9475fce090c25eb3593cd4b528fc26282fb9da32af7ebac25e81d501ec71ebda` |

Each package holds `MANIFEST.json` followed by `NOTICE.txt` and the members in
sorted order. Each member is stored at `<container>/<member>`. ZIP metadata is
fixed: timestamp 1980-01-01, mode 0644, deflate level 9. The manifest records:

- the source archive identity;
- the canonical-greekLit release and commit;
- every member's original container/path, tier, byte count and SHA-256;
- "byte-identical member selection";
- the Book 1 validated scope.

The notice gives creators, the citation, licence links and the modification
statement.

**Determinism.** Each build ran twice in memory with identical bytes. A separate
process reassembled both packages from the archive into a new scratch directory,
and `cmp` found them identical. The committed
[`package-manifests.json`](ancient-greek-oga-per-work-packaging/package-manifests.json)
and [`work-inventory.json`](ancient-greek-oga-per-work-packaging/work-inventory.json)
also reproduced byte-for-byte. Package verification fails closed when:

- a member hash differs;
- a member is missing or extra;
- the manifest or notice is absent;
- a path is unsafe;
- the source archive hash is wrong (see the tests).

**Caveat for #2031.** ZIP bytes depend on the deflate implementation. Use the
member hashes and the canonical `MANIFEST.json` hash as the *content identity*,
and the ZIP SHA-256 as the *transport identity* of one published build. An
independent re-derivation from the official archive with another zlib yields
the same member hashes. Its ZIP hash need not match.

**Proof reproduction from the package.**
[`reproduce_proof.py`](ancient-greek-oga-per-work-packaging/reproduce_proof.py)
checks out commit `e610a5aa` as a disposable detached worktree with an empty
`private/` tree. It then:

1. writes only verified core-package members as source inputs;
2. derives the native-node, reading and coordinate projections by calling that
   commit's own `import_gate.assess`;
3. copies the hash-pinned non-Gorman witnesses and the retained expected outputs
   (comparison targets only);
4. runs the unchanged `mapping_completion_v2.run` and `opus_repair.run`, with
   real Go parity.

One substitution is needed. `opus_completion.verify_inputs` hash-checks the four
Gorman registry entries, so it is replaced by an otherwise identical function
that skips them. **Gorman files were never copied or read.** The repaired
computation does not use them; the retained notice says the same. The result,
[`proof-reproduction.json`](ancient-greek-oga-per-work-packaging/proof-reproduction.json),
was written from one fresh worktree and reproduced byte-for-byte from two further fresh worktrees (the last after the final code formatting):

- `import-ledger.json` regenerated byte-identically from the package.
- `native-nodes.json` (`2a5aca90…`), `reading.txt` (`0f18cae9…`),
  `reading-to-paula.json` (`e4739c29…`) and `node-to-tei.json` (`1f9016a0…`) are
  byte-identical to the retained private projections.
- `mapping_completion_v2`: exit 0, no reproduction problems. Its verdict is
  still the historical "not-established".
- `opus_repair`: exit 0, no failing gates, no reproduction problems, Go parity,
  **438 selected / 434 card inputs**. Its summary is byte-identical to
  [`opus-repair-summary.json`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus-repair-summary.json).
  That summary also contains the uncorrected-v2 comparison and the 16 changed
  interpretations. All five private repaired outputs match too, including the
  node ledger (`f5a61b43…`) and the selection model (`855ea59e…`).

The run checks accounting and reproduction against the reviewed results. It
does not re-review or change mapping, normalization, selection, whitespace
projection or repairs.

**Unchanged historical command.** I also ran `import_gate.py --cache <complete
package members>` unchanged. It reproduced the ledger and projections, then ran
164 tests: 162 passed and two full-build tests errored with
`input missing: private/opus-completion/gorman/README.md`. Two findings follow:

- **Existing layout drift, independent of packaging.** The command reads
  producer documents only from `private/producer-docs/`. Its registry lists
  three documents that the retained layout keeps under
  `private/mapping-completion/producer-docs/`. The retained tree has no
  `private/producer-docs/morpheus-README.md`, so the command would stop there
  as-is. The probe copies the three pinned files for layout only, inside the
  disposable worktree.
- **Gorman presence.** The unchanged full-build tests and `opus_completion.py`
  need the Gorman files present to verify pins, even though the repaired result
  does not use them. Any runtime or CI reproduction should drop that pin rather
  than acquire training data.

**Corpus-level context, from central directories only.** No member is
decompressed, and these are not packages. The 1,999 works with PAULA core
layers have archive-native compressed sizes (core PAULA, LAULA, `anno_*` and
CoNLL-U; original TEI excluded) of:

| Statistic | Compressed bytes |
| --- | ---: |
| Median | 376,982 |
| p90 | 2,790,930 |
| Largest | 65,072,143 |
| Total | 2,562,161,565 |

Anabasis is 4,249,428. The release catalogue
`urn_cts/texts/urn_cts_plus_date_label.xml` (1,028,899 bytes, SHA-256
`560220c8…`) lists all 1,999 works, Anabasis included. It is byte-identical to
the retained GitHub copy at OGA commit `a69d917d`. It gives bibliographic
fields and dates, **but no partition, member paths, sizes or hashes**.

**Time and transfer.** On this host with a warm page cache, the full-archive
size/MD5/SHA-256 check plus inventory took 12.4 s. The package reproduction,
including Go parity, took about 5 s. Network transfer is arithmetic, not
measured:

| Link speed | 8.63 GB archive | 4.23 MB core package |
| --- | --- | --- |
| 100 Mbit/s | about 11.5 min | about 0.34 s |
| 1 Gbit/s | about 69 s | negligible |

## 3. Component and operation rights

Every row below is a declaration found in the retained or packaged files.
Nothing is inferred from public access, and no licence is chosen. The CC BY-SA
4.0 clause references point to the `license.md` legal code shipped in the
package (SHA-256 `ccf0e8ce…`).

### Declarations per component

| Component | Creators and attribution | Declaration found | Notes and evidence gaps |
| --- | --- | --- | --- |
| Upstream TEI text (Marchant, via canonical-greekLit `0.0.11376425141`) | Xenophon; ed. E. C. Marchant; Perseus Project / Tufts University; Gregory Crane; the preparers named above; Annenberg CPB; Trustees of Tufts University | [README](https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/README.md): "Unless otherwise indicated, all contents … are licensed under a Creative Commons Attribution-ShareAlike 4.0 International License. You must offer Perseus any modifications you make." | The TEI header has no exception. The README also carries Tufts' general "overall copyright … personal use" and "varying copyright status" notices. Under the licensing decision these are retained and shown, not an automatic defeat of the explicit grant. The "offer Perseus any modifications" sentence is not triggered by byte-identical packaging. Any future Mouseion text modification needs review |
| OGA annotation layers and OGA-normalized text (PAULA, LAULA, CoNLL-U) | Giuseppe G. A. Celano, *Opera Graeca Adnotata (v0.2.0)*, 2024, doi:10.5281/zenodo.14206061. Requested citation: arXiv:2404.00739 | Archive `README.md` lines 99–107 (SHA-256 `c59449d8…`) and the [GitHub README](https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/README.md) both declare CC BY-SA 4.0, "for more details, look also at the repositories of the original texts". The [OGA website](https://varro.informatik.uni-leipzig.de/oga/en/index.html) says the same | Zenodo metadata says `other-at`. Under the licensing decision that shorthand alone does not invalidate the README. The layers are automatic predictions; provenance is separate from rights. The producer paper's CC BY-NC-SA 4.0 covers the paper, not the data |
| Metadata (`anno_*.xml`, `__cts__.xml`, `urn_cts` catalogue) | OGA (Celano); Perseus for `__cts__.xml` | Covered by the respective release declarations above; no separate statement | Bibliographic fact. Treat conservatively under BY-SA 4.0 with attribution |
| PAULA DTDs | OGA archive | Covered by the OGA README | Schema files; no separate statement |
| Mouseion manifest, notice and hashes | Mouseion | Mouseion-authored | Facts about files. The application code licence is unchanged |
| Extracted/converted evidence (reading projection, coordinate maps, native-node JSON, ledgers, analysis, selection, sentence inputs) | Derived from OGA and Perseus by Mouseion | Adaptations of BY-SA 4.0 material; no separate declaration | Shared copies carry BY-SA 4.0 attribution and modification notices, and ShareAlike for adapted material (§3(b)). Private computation shares nothing |
| Interpretation artifact: rules and repair values | Mouseion research (#2052) | None yet | **Owning-decision gap.** Morpheus-derived decoded keys/discriminators may be adaptations of CC BY-SA 3.0 US material (not reviewed here; no 3.0 US legal code is retained). The 756 conjunction decisions are derived from CC BY-NC-SA 2.5 counts. **Whether those derived decisions carry NC terms is unresolved.** Bundling them under the redistribution policy waits on review, or on a separately authorized, re-validated non-NC rule |
| Morpheus witnesses | Tufts University / Perseus Digital Library | [README](https://github.com/PerseusDL/morpheus/blob/b1b33c56ef2338fe0dcd1893628ed638f00c0986/README.md): CC BY-SA 3.0 US unless otherwise indicated; "You must offer Perseus any modifications you make" | Proof-only; never packaged |
| UD Ancient Greek-Perseus witnesses | AGDT contributors (Tufts, Leipzig) under Gregory Crane; ed. Celano, Crane and Almas; UD conversion by Celano (with Zeman) | [LICENSE](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/LICENSE.txt): CC BY-NC-SA 2.5 | Proof-only; never packaged. Not eligible for Mouseion redistribution under the policy |
| Gorman trees (vgorman1/Greek-Dependency-Trees `d823e96d`) | As recorded in the retained notice; not re-read | Conflicting: README BY-NC-SA 4.0 vs `TREEBANK LICENSE` BY-SA 4.0 (retained notice) | **Excluded.** Not read, staged or required by the repaired result |

### Dispositions per operation

These are dispositions under the
[licensing decision](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026),
not grants.

| Operation | Disposition |
| --- | --- |
| Private extraction and inspection (this probe) | Performed within the earlier authorization of the archive for private inspection. CC licences put conditions only on *Sharing*, so no condition is triggered. Packages stay in Git-ignored `private/` |
| Self-hosted direct upstream acquisition (installation downloads the official archive and extracts the work) | Eligible under the policy's direct-acquisition mode. The components are BY-SA, not NC, so no extra opt-in category applies. Mouseion redistributes nothing. Notices still have to be retained and shown before acquisition |
| Mouseion-hosted redistribution (release assets/mirror of the per-work packages) | **Eligible for exact-component review; not approved.** Members are unmodified Licensed Material, so the attribution conditions of §3(a)(1) apply: creator identification, copyright notice where supplied, licence and disclaimer notices, and a URI. So does §2(a)(5)(C): **no additional terms or technological measures may restrict recipients' licensed rights**. The package must also state the repackaging (`NOTICE.txt` does) and carry the original repositories' notices, as the OGA README asks. ShareAlike (§3(b)) applies to *adapted* material, not to byte-identical members. Re-cutting files would trigger it. **Bounded gap:** whether §4 sui generis database terms apply to hosting many works as a substantial portion of the corpus database is not settled here. §4(c) attribution would apply in any case |
| Installation use (local acquisition, validation, Reading) | Private computation. Same-installation learner display is not treated here as public Sharing. Whether a multi-user household instance is "the public" is not assessed |
| Later exports (decks, analysis or sentence exports) | Adapted material from OGA and Perseus. Shared exports need BY-SA 4.0 attribution, modification notices and ShareAlike. Exports must not carry NC-derived values unless separately cleared. A personal download is not publication permission |
| Provider use (Gloss/translation) | **Not assessed and not performed.** It needs its own operation review. No Greek text was sent anywhere |

**No rights-holder clarification prerequisite was identified for the core
package's members.** Every one has an explicit declaration and no exception
turned up. The residual statements must be shown, not resolved:

- the Tufts general notices;
- the "offer modifications" sentence;
- Zenodo's `other-at`;
- the "look also at the original repositories" clause.

The open items are owning decisions, not licence choices:

- approval of Mouseion redistribution;
- review of the interpretation artifact, including the NC-derived conjunction
  decisions.

## 4. On-demand acquisition comparison

The rules applied are from ADRs
[0093](https://github.com/justin-hayes/mouseion/blob/bb438d2e80410554ac7ce00122b1aa8a261013b8/doc/adr/0093-classical-identity-and-reading-units.md),
[0094](https://github.com/justin-hayes/mouseion/blob/bb438d2e80410554ac7ce00122b1aa8a261013b8/doc/adr/0094-source-independent-annotation-and-provenance.md),
[0095](https://github.com/justin-hayes/mouseion/blob/bb438d2e80410554ac7ce00122b1aa8a261013b8/doc/adr/0095-greek-personal-counts-and-external-frequency.md),
[0096](https://github.com/justin-hayes/mouseion/blob/bb438d2e80410554ac7ce00122b1aa8a261013b8/doc/adr/0096-classical-catalogue-discovery-and-acquisition.md)
and
[0097](https://github.com/justin-hayes/mouseion/blob/bb438d2e80410554ac7ce00122b1aa8a261013b8/doc/adr/0097-classical-evidence-refresh-adoption-and-retention.md).

| | A. One-time extraction and reviewed immutable per-work hosting (**preferred**) | B. Each installation extracts from the archive and caches it |
| --- | --- | --- |
| Per-work transfer | 4.23 MB core package | 8.63 GB archive per installation per OGA release (again if the cache is lost) |
| Installation disk | Package plus validated evidence | +8.63 GB if the archive is cached for later works or restore; else re-download |
| Integrity root | Mouseion manifest: member SHA-256 tied to the official archive SHA-256; anyone holding the archive can re-derive the member hashes | Full official archive SHA-256 locally; member hashes from Mouseion's pinned adapter manifest |
| Rights posture | Mouseion is a redistributor: review, notices, no downstream restrictions | Direct acquisition; no Mouseion redistribution |
| Maintainer work | Per OGA release: fetch the archive once, extract deterministically, validate each *offered* unit, review rights, publish immutable versions, keep old versions and the archive | Pin the archive identity and member manifest in the adapter; ship nested-ZIP extraction code |
| Upstream dependency | At release-build time only; installations depend on Mouseion hosting | Every new installation and restore depends on Zenodo |
| Size and policy fit | Meets the artifact-size bounds | Must allow an 8.6 GB artifact, and acquires the whole corpus to prepare one Book. It must not register, analyze or count the other works |
| Range-fetch variant (B′) | n/a | **Untested and unauthorized.** It could avoid the full download, but cannot verify the whole-archive hash. It would need the operation proposed below |

**Recommended semantics for either route.** These need no service design.

- **Discovery.** Metadata-only sync reads a small, versioned offering index. It
  must not download packages.
- **What each offering entry carries.**
  - Work and Edition identity: `urn:cts:greekLit:tlg0032.tlg006.perseus-grc2`;
    Marchant 1904; canonical-greekLit `0.0.11376425141`/`476fb613`.
  - Declared Reading units, with exact bounds and validation status. Book 1 is
    validated; other books are "carried, not validated".
  - The annotation source: OGA v0.2.0, automatic provenance, layers.
  - Package identity: format, tier, manifest and member hashes, ZIP hash and
    size, availability.
  - Component licences, review outcome and notices.

  The OGA `urn_cts` catalogue can seed bibliographic fields. Mouseion's manifest
  must add member identity, because OGA ships none.
- **Selection-triggered acquisition.** Downloads happen only on To Read, with
  the package and interpretation-artifact versions pinned first. Verify the ZIP
  hash, then the manifest and every member hash and the source-archive identity.
  Retain the notices. Rerun the proof-equivalent binding and accounting gates.
  Publish atomically.
- **No collateral effects.** Sync and acquisition must not download or analyze
  other works or affect personal counts. Counts follow only validated,
  published, full-scope evidence.
- **Failure and retry.** Transient transfer failures get bounded retries
  against the *same* pinned hashes. A hash mismatch is an integrity failure, not
  a reason to take another version. Exact re-derivation from the official
  archive, or an identical verified mirror copy, counts as transport recovery.
  Never fall back silently to another Edition, source, unit, annotation release
  or text-only canonical-greekLit.
- **Upstream revisions.** A new OGA or text release is a new package version
  that discovery *offers*. Adopting it is the owner's explicit choice and needs
  fresh validation. Old packages, evidence and frozen learner facts stay
  retained and unchanged.

## 5. Proposed operations needing authorization (none performed)

1. **B′ only.** One bounded HTTP Range experiment against the official Zenodo
   content endpoint:
   - read the outer central directory;
   - read the nested `workspace/oga.zip`, `conllu.zip` and
     `original_Greek_files.zip` directories;
   - read the 31 core members, verifying each against `work-inventory.json`.

   The aim is to establish range support and the bytes actually transferred.
   It needs explicit authorization.
2. **None needed for route A** beyond the human and owning decisions in
   section 6.

There was no new acquisition, live ANNIS use, range fetch, Gorman read,
analyzer/model run, provider call or network request for data. The retained
GitHub tree and Zenodo record already show that the only evidenced annotation
route is the single archive. OGA's GitHub tree holds documentation and one
unrelated `annotation_example`.

## 6. Handoff to #2031

- **Preferred option.** Route A: one-time extraction from the verified official
  archive into reviewed, immutable, versioned per-work packages (core tier) with
  a Mouseion offering manifest. Route B is the direct-acquisition fallback.
- **Package identity.** Source archive SHA-256, package format
  `mouseion-oga-work-package/1`, tier, `MANIFEST.json` hash and member SHA-256s
  form the content identity. The ZIP SHA-256 is the transport identity of a
  build.
- **Separate pins.** The interpretation artifact is pinned separately: rulesets
  `oga-greta-lemma/2`, `oga-agdt-upos/3` with εἰμί v3, repairs
  `oga-anabasis1-research-repairs/1`, `oga-reading-terminal/2`,
  `ancient-greek/1`, `grc-earliest-capped/1`. Witnesses stay proof-only.
- **Maintenance responsibilities (owner to be assigned).**
  - Per-release builds and validation of each offered Reading unit.
  - Rights review and notices.
  - Immutable retention of published versions.
  - Hosting with no downstream restrictions.
  - Integrity monitoring.
  - Exact re-derivation for recovery.
- **Human or owning decisions.**
  - Accept the OGA Anabasis Book 1 source in
    [#2030](https://github.com/justin-hayes/mouseion/issues/2030).
  - Decide whether Mouseion will host, applying the existing policy. The
    evidence appears sufficient for that review.
  - Decide who owns hosting and maintenance.
  - Review the interpretation artifact's Morpheus-derived and NC-derived
    content.
  - Decide the initial offering: the Book 1 unit only, versus a whole-work Book
    shown as not analyzed.
- **Shipping-stage blockers.**
  - Redistribution approval.
  - The NC-derived conjunction decisions: clear them, or authorize and
    re-validate a replacement rule.
  - A Gorman-free reproduction command. Drop the unused Gorman pin and the
    producer-document layout drift.
- **Not established here.** Validation of Books 2–7, practical usefulness,
  Gloss, deck or export rights, and provider use.

## Reproduce

From `doc/research/ancient-greek-oga-per-work-packaging/`, with the retained
archive at
`…/issue-2052-happy-falcon/doc/research/ancient-greek-oga-anabasis1-import-proof/private/archive/`.
Set `R` to that proof directory, `A=$R/private/archive/opera_graeca_adnotata_v0.2.0.zip`,
`S` to a new owned scratch child, and `GOTMPDIR=$HOME/.cache/gotmp`, with Go on
`PATH`:

```sh
python3 -I package_probe.py inventory --archive "$A"              # compare with work-inventory.json
python3 -I package_probe.py assemble --archive "$A" --out "$S/pk" # new dir; compare with package-manifests.json
git worktree add --detach "$S/proof" e610a5aa6c77c25864114ef9604ac9d50515039b
python3 -I reproduce_proof.py --package "$S/pk/oga-v0.2.0_tlg0032.tlg006.perseus-grc2_core.zip" \
  --complete-package "$S/pk/oga-v0.2.0_tlg0032.tlg006.perseus-grc2_complete.zip" \
  --proof-worktree "$S/proof" --retained "$R" --scratch "$S/tmp"  # compare with proof-reproduction.json
python3 -I test_package_probe.py                                  # 12 focused tests
git worktree remove --force "$S/proof"
```

Every command compares against its committed JSON unless given `--write`.
Retained originals and outputs are opened read-only. A post-run check found the
retained proof worktree clean at `e610a5aa`, with no file newer than this
session's start.

Other checks run: `make lint` (golangci-lint 2.14.0: 0 issues; ruff and mypy
over `nlp/` clean) and ruff on the new research files. This worktree has no
`.venv`, so `make lint` used the retained proof worktree's `.venv` tools through
`VENV_BIN`. `make coverage-new` was not run: no Go or Python application code
changed, and the project coverage gate does not measure research scripts.

**Public files contain only:**

- paths, sizes, CRC32s, hashes and counts;
- Mouseion-authored notice text;
- short licence quotations.

No packages, source text, annotation bundles or examples are committed.
