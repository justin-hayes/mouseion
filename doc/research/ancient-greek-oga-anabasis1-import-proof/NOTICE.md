# Attribution, rights and modification notice

## Fresh-owner completion pass (#2052, 2026-10-15; v2 review revision)

### What was acquired, and how

`opus-completion-inputs.json` records every new input with its URL, revision,
size, SHA-256, licence statement and how it was obtained.

- **Fetched over the network on 2026-10-15** (one ordinary request per file):
  - From **PerseusDL/morpheus** at `b1b33c56…`:
    - eight complete stem-source files, each matching the hash in the existing
      manifest;
    - three documentation files: `doc/morpheus.html`,
      `stemlib/Greek/xsl/lemmaRegistry.xsl` and `stemlib/Greek/makefile`.
  - From **vgorman1/Greek-Dependency-Trees** at `d823e96d…`: two **complete
    annotation XML files** (710,529 and 619,008 bytes, covering Anabasis 1.1–1.10)
    plus the README and the licence file. These are training-data lineage
    material, not short lemma extracts. Fetching them goes beyond the brief's
    "narrow lexical records" wording, so it is recorded here as a scope
    qualification for parent review. No other training file was fetched.
  - From **UniversalDependencies/docs** at `24add7d0…`: Ancient Greek POS pages
    and the universal AUX, cop and nonverbal-predication pages.
- **Copied locally, not downloaded:** UD_Ancient_Greek-Perseus `331ddef9…`
  train, dev and test files, README and LICENSE. These are byte-identical copies
  of the #2050 retained inputs, checked against that registry's hashes.
- **Refused and not retried:** one Perseus Hopper request for a single LSJ entry
  returned HTTP 405. No other route was tried, and no LSJ file was acquired.
- The v2 review revision fetched nothing.

### Terms per source

Each source keeps its own terms. Nothing here relicenses a source.

| Source | Stated terms | What this pass publishes |
| --- | --- | --- |
| OGA v0.2.0 (archive README) | CC BY-SA 4.0 | Counts, node IDs, citations, raw lemma labels and native POS, as bounded diagnostics under CC BY-SA 4.0 with this notice. These are OGA-derived material. |
| Morpheus (README) | CC BY-SA 3.0 US unless otherwise indicated; Tufts' general notice | Short key, line and derivation citations, attributed to Morpheus under its own terms. No file is republished. |
| Gorman Trees | The README says CC BY-NC-SA 4.0; the licence file holds the CC BY-SA 4.0 text. This conflict is not resolved here. | v2 publishes only counts and agreement categories. The v1 summary (`opus-completion-summary.json`, kept unchanged as the 0e839900 record) does contain short Gorman lemma labels for the numbered-label tokens and the numbered-label tally. That is flagged here for the parent's rights review, not removed. |
| UD Ancient Greek-Perseus | CC BY-NC-SA 2.5 | Counts only. |
| UD docs repository | GitHub reports Apache-2.0 | Cited, not republished. |

Full ledgers, sentence text, examples and the Go probe input stay private. The
#2035 operation boundaries are unchanged: no export, bundling or provider use is
approved.

### Mouseion transformations

None of these changes upstream text or annotation, and none certifies a
prediction.

- Lemma labels are interpreted against Morpheus keys and compound records.
- Native POS is mapped to UPOS through the UD Perseus conversion. For εἰμί, UD
  definitions are applied to the native syntax.
- Sentences are segmented on `.` and `;` over the unchanged reading text.
- Identities are serialized as `key#morpheus:n` for the Go probe.
- Selection is modelled and sentence inputs are picked.

## Current component assessment after verified acquisition

The official v0.2.0 archive is now acquired and locally verified (size, published
MD5 and SHA-256). Local recovery completed using the same request's retained
inode; no retry occurred. Originals are unchanged under Git-ignored
`private/archive/` and `private/archive-members/`. `archive-members.json` records
every selected member's exact container/path, size, CRC and SHA-256.

The **archive's own README, lines 99–107**, explicitly declares CC BY-SA 4.0 for
this work and refers to the original-text repositories. The selected original
Anabasis XML is byte-identical to the independently pinned upstream file under
canonical-greekLit's explicit default BY-SA 4.0 declaration. Its header credits
Xenophon, E. C. Marchant, the Perseus Project/Tufts and the named preparation
contributors; it names *Opera Omnia*, volume 3, Clarendon Press 1904/reprint 1961.
No component-specific rights exception was found in that header or in the
selected PAULA/LAULA headers. This is affirmative component evidence, not
permission inferred merely from public access or a missing prohibition.

**Operations now assessed:** private non-commercial inspection of these exact
components and public **bounded, attributed research diagnostics** (member hashes,
Book 1 structure/span counts, native node IDs/citations, editorial element spans,
and raw numbered-lemma predictions with native POS) proceed under CC BY-SA 4.0.
These diagnostic projections/adaptations are offered under CC BY-SA 4.0 with
the citations below and this modification notice. Whole source files, complete
native annotations, marker-free reading text, and full coordinate maps remain
private, not redistributed in Git. No inference of source acceptance, public
dataset/app bundling, decks/exports, provider use or semantic acceptance follows.
No dictionaries, models or analyzer code were acquired or executed.

Supplementary producer documentation is separately pinned/hashed in
`producer-docs.json`, retained privately, and cited rather than republished:
Celano's *A State-of-the-Art Morphosyntactic Parser and Lemmatizer for Ancient
Greek*, arXiv **2410.12055v1** (paper page declares **CC BY-NC-SA 4.0**, not the
corpus's BY-SA grant); Celano's AGDT2 guidelines at treebank_data commit
`0ebc9289bc6c8eb1069fac3433955f83f442762f`; and the lemmatizer README at
`267dbf045dfca1129115aef05ba9ac6ebc49f4db`. The README's model Apache-2.0
declaration is not assessed or exercised as model permission: no model/code
payload was fetched or run, and no text/annotation rights are inferred from it.
Only bounded factual findings/citations about these documents are published.

**New Mouseion transformations, 2026-10-14:** select only the Anabasis work from
nested local ZIP containers without unpacking the corpus; measure Book 1;
validate native code-point ranges; suppress only typed, independently empty
`e_*` placeholder spans and their following separator; retain printed characters
and other PAULA spacing unchanged. Compare against explicit TEI `del` omission
and `choice/corr` selection, retaining `add` and standalone `sic`; map every
non-whitespace character exactly, without accent/case/punctuation normalization.
Report optional LAULA pointer disagreements separately and stop before canonical
lexical publication. These changes are not upstream text/annotation corrections
or certification of model predictions. Research code/invented fixtures remain
Mouseion-authored under the repository's unchanged code-license arrangements.

The sections below retain the authorization history and **initial-gate** operation
assessment. Statements there that acquisition was unperformed are historical,
superseded by this exact-component assessment, not current restrictions.

## Live scope amendment: official archive authorized

After the initial gate stop, Justin explicitly authorized downloading the archive
in the live session: “Feel free to download the archive if that unblocks you.”
The official v0.2.0 Zenodo archive may now be acquired for **private inspection**
under OGA's explicit corpus CC BY-SA 4.0 declaration. This overrides only the
earlier no-archive acquisition boundary; it does not infer public redistribution
of unreviewed components, accept a source, authorize challenge/account/mirror
bypass, or change analysis scope beyond Anabasis Book 1. The previous gate
assessment remains immutable at commit
`4104c8be238fe0d4b5500f8e18522e80dd516467`.

Proposed acquisition: one ordinary request to the official record's content
endpoint for `opera_graeca_adnotata_v0.2.0.zip`, expected **8,629,032,478 bytes**,
published MD5 `d4117ae52f1cc6319c6ba20af0c1cf3b`. Verify size and checksum,
compute SHA-256 locally, retain the original privately, and use the ZIP central
directory to locate the individual Anabasis work and accompanying notices.
Do not unpack or analyze the corpus. Access errors/challenges stop the request;
there is no automatic retry or alternate transport. Approximately 54 GiB free
disk space was observed before download, sufficient for the archive plus the
bounded selected-work extraction without corpus-wide decompression.

A documentary subagent erroneously unlinked the partial download despite the
new authorization and separate file ownership. The main session retained the
still-open local inode through the downloader's file descriptor. A local
recovery process holds that inode until the original request exits, then checks
the complete size/MD5 and computes SHA-256 before restoring the archive path.
This is recovery of the **same original response bytes**, not another network
request, archive range extraction, alternate source or access workaround. No
verified acquisition is claimed until recovery and checks complete.

Exact component declarations/headers will be inspected before any source-derived
public examples or text publication. The historical operation table below records
the **initial** gate only and must not be read as undoing this authorization.

The individually addressable canonical-greekLit Anabasis work file at commit
`476fb6133498e5710de7eec0b492791da86be4b4` is also eligible for private comparison
under its repository's explicit default BY-SA 4.0 grant and personal-use notice.
Expected size: 886,347 bytes; Git blob
`a220f4b8accd1448a0189a595493a78aa09c707b`. This is the one accompanying
edition lead already scoped by the issue, not another corpus or substitute
edition. Retain the indivisible work file but measure only Book 1. Inspect its
own header for exceptions; do not infer exact OGA correspondence from the grant
or CTS identifier, and do not publish the full original at this stage.

Bounded documentary assessment, not legal advice, dataset acceptance or an
archive-acquisition authorization. The acquisition-scope stop is sufficient;
license uncertainty is not used to manufacture another reason to reject OGA.

## Actual declarations

- **Giuseppe G. A. Celano, Opera Graeca Adnotata v0.2.0 (2024)**.
  [Documentation](https://github.com/OperaGraecaAdnotata/OGA/tree/a69d917dbe117a307829977dd556e22f05ce0715),
  commit `a69d917dbe117a307829977dd556e22f05ce0715`.
  The README explicitly declares
  [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/), referring also
  to the repositories of the original texts. This is real permission evidence;
  missing GitHub license detection or Zenodo's `other-at` shorthand does not
  negate it. The README identifies its corpus/release as well as the example.
- Requested citation: Celano, *Opera Graeca Adnotata: Building a 34M+ Token
  Multilayer Corpus for Ancient Greek*, [arXiv:2404.00739](https://arxiv.org/abs/2404.00739).
  Corpus citation: Celano (2024), *Opera Graeca Adnotata (v0.2.0)*,
  [doi:10.5281/zenodo.14206061](https://doi.org/10.5281/zenodo.14206061).
  Citation and release metadata only: **archive not acquired**.
- **Perseus Digital Library / Tufts University**, canonical-greekLit release
  `0.0.11376425141`, commit `476fb6133498e5710de7eec0b492791da86be4b4`.
  [README](https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/README.md)
  declares contents BY-SA 4.0 unless otherwise indicated and retains general
  overall-copyright/varying-status notices.
  [license.md](https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/license.md)
  supplies the legal text. The general notices do not automatically defeat the
  explicit grant or require outreach. The Greek work's own header was not read;
  exact file/component assessment and OGA derivation remain unperformed.
- [OGA website](https://varro.informatik.uni-leipzig.de/oga/en/index.html) also
  declares its content BY-SA 4.0. The published record metadata, not an archive
  member manifest, supplies the archive filename, size and checksum.

## Operations: distinct dispositions

| Component / operation | Disposition in this task |
| --- | --- |
| Local documentation and metadata copying/inspection | Performed as the authorized public-evidence gate; explicit documentary declarations retained. Raw originals unchanged and private. This is not an inference of source redistribution rights from public access. |
| Full-unit private non-commercial inspection | **Not performed**: no demonstrated in-scope acquisition package. No assertion that private copying/adaptation is categorically prohibited by BY-SA or by a metadata shorthand. |
| Supported direct acquisition of Anabasis text + OGA annotation | **Blocked under this issue's scope**, independently of eventual exact-component clearance. An individual upstream text listing alone supplies no accompanying annotation. |
| Public research findings, registry, target-specific inventory projection and verdict | Published with attribution here. Treat OGA-derived documentary projections conservatively under CC BY-SA 4.0; the registry/ledger are not asserted to be automatically unrestricted statistics. No full bibliography, Greek reading text, full-unit annotations or normalized source is redistributed. |
| Eventual public diagnostics including source strings; original/normalized source redistribution | Not approved by this gate. Requires review of the actual selected package/components and the particular output. |
| Personal exports/decks, public exports, application bundling, mirrors/container datasets | Not performed or approved. Require operation/component review, retained attribution/license/modification notices, and any applicable adaptation/database conditions; a local acquisition never grants blanket public distribution permission. |
| External-provider / Gloss / translation operations | Not performed or approved. No Greek input was sent for enrichment. A source license or local inspection alone is not approval for another operation. |
| Producer code/models, training data, dictionary acquisition | Not acquired/run or assessed. No license or quality conclusions inferred for those separate components. |

**Mouseion modifications, 2026-10-14:** project the Anabasis bibliographic lead
and download-listing metadata; preserve exact original hashes and all-field
canonical-JSON hashes where appropriate; derive acquisition/capability stop
diagnostics. No original artifact was edited; no Greek text was reconstructed,
mapped or corrected. Source-derived documentary evidence is offered under
CC BY-SA 4.0, without warranty or upstream endorsement. Research code and invented
test fixtures are Mouseion-authored; the repository's application-code license
is unchanged. Dataset terms do not automatically relicense application code.

## Mapping-completion pass (2026-10-15): Morpheus and UD documentation

- **Morpheus Greek stemlib** (<https://github.com/PerseusDL/morpheus>, pinned
  at `b1b33c56ef2338fe0dcd1893628ed638f00c0986`). The repository is licensed
  CC BY-SA 3.0 US. Morpheus is Tufts University / Perseus Digital Library work.
  This pass uses the stemlib only to derive lemma-key identity and proper-name
  evidence. The committed summary records only short witnesses (file, line,
  key, derivation) and per-file SHA-256 hashes. The full index stays private.
  Mouseion does not relicense Morpheus data. Share-alike applies to any
  redistribution of the stemlib itself.
- **UD Ancient Greek-Perseus README** and **UD UPOS index**: documentation
  only, retained privately with SHA-256 hashes in `producer-docs.json`. The
  README is listed as CC BY-NC-SA 2.5. No treebank data, model, or code is redistributed.
