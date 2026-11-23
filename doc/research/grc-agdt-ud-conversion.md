# AGDT-to-UD conversion cost and Latin generalisation

**Research date:** 2026-11-23

**Ticket:** [#2599](https://github.com/justin-hayes/mouseion/issues/2599), under the
map [#2597](https://github.com/justin-hayes/mouseion/issues/2597).

**Question:** What would converting OGA's AGDT syntax to Universal Dependencies (UD)
cost and lose? Which representation (native AGDT, or UD) generalises to the Latin
routes Mouseion might later take?

**Method.** Primary sources only: UD treebank repositories at tag `r2.19`, the Treex
source at a pinned commit, the PerseusDL `treebank_data` repository at a pinned
commit, the `oga_src` v0.2.0 file pinned in `internal/ancientgreek/ancientgreek.go`
(SHA-256 verified), the OGA papers on arXiv, and the Stanza resources index. Three
measurements were run on local copies with the scripts in
[`grc-agdt-ud-conversion/`](grc-agdt-ud-conversion/). They print aggregate counts only.
This file contains no Greek text and no per-token OGA annotation.

## Answer

- **No converter fits OGA.** The released UD Greek conversion (UD_Ancient_Greek-Perseus)
  is Giuseppe Celano's, and its code is not published. The only public AGDT-to-UD
  converter is Treex/HamleDT (Perl; the GPL v2 and Perl-licence files are listed under
  "Converters" below). It expects `_CO`/`_AP` suffixes and AGDT XML ellipsis markup,
  and OGA has neither.
- **UD Perseus never solved ellipsis. It left the sentences out.** The seven AGDT 2.1
  prose files checked have 1,432 sentences with artificial (elliptic) nodes, and none
  of them is in UD Perseus. It keeps 4,486 of the 5,446 sentences without ellipsis.
  In the pinned *Anabasis*, 539 of 3,813 sentences (14.1 %) carry ellipsis nodes. So
  "converting like UD Perseus" means either losing about 1 sentence in 7 or writing
  the ellipsis-to-`orphan` step that nobody has published for Greek.
- **Missing `_CO` costs about 1 coordination in 6.** HamleDT's fallback guesses
  conjuncts by majority label. Run on gold AGDT 2.1 (Herodotus and Thucydides, 3,051
  coordinations) with the suffixes removed, it reproduces the gold conjunct set for
  82.6 % of coordinations, with member precision 0.894 and recall 0.980. In UD, a
  wrong conjunct set turns into wrong `conj` relations and wrong heads. That error
  comes on top of OGA's own parser error.
- **UD loses the distinctions a Greek construction detector cares about.** In the
  observed UD Perseus mapping:
  - OCOMP, ATV and AtvV all become `xcomp`.
  - PNOM is split across `xcomp`, `root`, `advcl` and `obj`.
  - AuxY sentence particles and AuxZ focus and negation particles become plain
    `advmod`. That relation already holds ordinary adverbs.
  - UD Perseus has **no language-specific subtypes** (no `advcl:abs`, `acl:relcl` or
    `advmod:neg`).

  UD preserves one thing: under AuxC, ADV becomes `advcl` and OBJ becomes `ccomp`. AGDT
  already carries this distinction as the label on the clause's verb, so UD adds
  nothing that AGDT lacks for the first-slice constructions.
- **Latin does not settle the choice.** Every UD Latin treebank (Perseus, PROIEL, ITTB,
  LLCT, UDante, CIRCSE) and the Stanza `la` parser arrive as UD. The Latin Dependency
  Treebank (LDT) and LLCT1 arrive as AGDT-style. LASLA has no dependency syntax at all.
  The useful Latin UD treebanks have been harmonised with construction subtypes
  (`advcl:abs`, `acl:relcl`, `advmod:neg`). Greek UD has not. "UD" therefore does not
  give one cross-language construction vocabulary in practice.
- **Recommendation: store native AGDT (a `native_agdt` syntax state, as ADR 0099
  anticipated), and write the construction detector against a small source-neutral
  construction interface, not against either label set.** An AGDT adapter feeds it
  now. A UD adapter (for Latin or a future Greek parser) can be added later without
  redesign. Do not convert OGA to UD. Dual storage costs the converter work and
  inherits its errors, while the UD copy has no reader in the first slice.

## Comparison

| | Native AGDT (store as is) | AGDT to UD (store UD only) | Dual-store (AGDT + derived UD) |
|---|---|---|---|
| **Fidelity** | Full: OCOMP, ATV/AtvV, PNOM, AuxY/AuxZ, ExD and ellipsis nodes are kept. Coordination membership is implicit, with no `_CO` suffix, but the tree is intact. | Lossy. 14.1 % of *Anabasis* sentences have ellipsis that needs `orphan` and promotion with no published method. About 17 % of coordinations get the wrong conjunct set. Five AGDT distinctions merge into `xcomp`/`advmod`. | AGDT copy as in the left column. The UD copy is as lossy as the middle column. |
| **Effort** | New interpretation version, new syntax state, an ellipsis-node representation, and a detector on AGDT labels. | All of the left column's parsing, plus writing or porting a converter (Treex is Perl, and Mouseion has no Perl runtime), an ellipsis policy and a validator. | The sum of both, plus keeping the two copies consistent across interpretation versions. |
| **Converter licence** | None needed. | Treex: GPL v2 / Perl licence. Celano's converter is unpublished. | As the middle column. |
| **Latin fit** | Directly fits LDT/LLCT1 (AGDT-style). UD Latin needs a second adapter. | Fits UD Latin and Stanza `la` directly. LDT would need the same converter. | Fits both, at the highest cost. |
| **Existing Mouseion consumers** | None read it. Gated by `syntax_state`. | Sentence study would start showing relation and head. Everything else stays gated by language (see below). | As the middle column. |

## Evidence

### 1. Existing converters

**UD_Ancient_Greek-Perseus** (`r2.19` = `84daf01a020539d6e1e44bb286b8f6352719d869`,
<https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/tree/r2.19>):

- Its README describes "an automatic conversion of a selection of passages from the
  Ancient Greek and Latin Dependency Treebank 2.1" and says "The current UD data have
  been converted by Giuseppe G. A. Celano".
- Metadata: `License: CC BY-NC-SA 2.5`, `Contributors: Celano, Giuseppe G. A.; Zeman,
  Daniel`, `Contributing: elsewhere`. It has 25 relations, "of which 0 language
  specific".
- The commit history shows the first data "from AGDT / HamleDT 3.0" (Dan Zeman,
  `7bac676d9b`, 2015-10-06). Celano replaced it ("newdata", `7f8bdf5659`, 2015-10-29,
  and UD v2 uploads in February 2017).
- Since then only post-hoc fixes have been made (Udapi `ud.FixPunct` in v2.8; the
  `advmod` to `obl`/`advcl`/`discourse` and copula-AUX relabelling in v2.11). The
  repository tree holds only data plus one `not-to-release/fix_too_many_subjects.py`.
  It contains no converter.
- Celano's public GitHub repositories (`gcelano/*`, listed via the API) contain UDPipe
  models (`AncientGreekUD`, `LatinUD`) but no AGDT-to-UD conversion code. **The
  converter behind the current release is not public.**

**Treex/HamleDT**, the only public converter
(<https://github.com/ufal/treex>, commit `f5889e47701e83dcec80ce6ae5e5fff1d456e711`,
last pushed 2026-11-11, so it is still maintained):

- The pipeline is `HamleDT::GRC::Harmonize`, which extends `HamleDT::HarmonizePerseus`
  (shared with `HamleDT::LA::Harmonize`), then `HamleDT::PragueDeprelsToUD`, then
  `HamleDT::Udep` (uses `Tool::PhraseBuilder::PragueToUD`,
  `coordination_head_rule => 'first_conjunct'`).
- Licences: `HarmonizePerseus.pm` says "distributed under the GNU General Public License
  v2". `Udep.pm` says "under the same terms as Perl itself". The repository has no
  top-level licence file.
- It runs on OGA's label set only partly:
  - `HarmonizePerseus::convert_deprels` takes conjunct membership from the `_CO`/`_AP`
    suffix ("The _CO suffix signals conjuncts"). OGA deleted those suffixes. Celano
    (arXiv 2410.12055, §3) writes that "this and similar suffixes, such as _AP, were
    deleted".
  - Without the suffix, `fix_annotation_errors` falls back to
    `Harmonize::identify_coap_members`: up to 2 candidates are all members, otherwise
    the most frequent label wins. Its own comment says that shared `Atr` modifiers are
    not handled ("This has not yet been implemented"). It is measured in §3 below.
  - Ellipsis in HamleDT is handled through chained `ExD` labels (for example
    `ADV_ExD0_PRED_CO`) and AGDT XML artificial words. OGA uses neither. It writes
    `[0]`, `[1]`, … placeholder rows at the end of the sentence (Celano 2023 ellipsis
    modelling, arXiv 2410.12055 §3). The converter would need a new reader that turns
    OGA placeholders into its artificial-node model, or a new ellipsis step.
    `PragueDeprelsToUD` itself notes "ExD with chains of orphans should be
    stanfordized!" (TODO).
  - `OCOMP => 'Obj', ###!!!` and `AtvV? => advcl:pred` are marked as known
    approximations in the source.
- Treex is a Perl framework with Interset. Mouseion has no Perl runtime (Go + Python),
  so using it means either a new runtime or a port.

**Other converters checked.** Gorman's trees and Keersmaekers' GLAUx are AGDT-style
resources. Celano (arXiv 2410.12055 §2) reports that Keersmaekers & Van Hal (2023)
transformed coordination and deleted ellipsis nodes before training, and that "the
models, however, have not been released". I found no released AGDT-to-UD converter
from either.

### 2. OGA's input as a converter would see it

Measured on the pinned file (`tlg0032.tlg006.perseus-grc2_oga020_src.conllu`, SHA-256
`c51bba2d…eb61`, matches the pin) with `oga_probe.py`:

| Measure | Value |
|---|---|
| Sentences / printed tokens | 3,813 / 64,400 |
| Ellipsis placeholder rows | 622 |
| Sentences with ≥1 ellipsis node | 539 (14.1 %) |
| Printed tokens headed by an ellipsis node | 1,445 (2.2 %) |
| Ellipsis nodes headed by an ellipsis node | 2 |
| Relations of ellipsis nodes | PRED 311, ADV 204, OBJ 69, ATR 20, SBJ 14, APOS 3, PNOM 1 |
| Relation labels carrying any `_` suffix | 0 |
| COORD heads | 2,839 |
| COORD with >2 non-delimiter children of differing labels | 742 (26 %) |
| COORD with 2 children of differing labels | 182 (6 %) |
| A relation spelled with a non-Latin character | 4 tokens: `Aux` + Greek capital Zeta (U+0396), a variant of `AuxZ` |

The last row matters for any adapter: a strict label allowlist would refuse the file,
and a lax one would carry a homoglyph label into storage. The Crito probe on the map
found 21 labels. *Anabasis* has 19 distinct plus the homoglyph.

### 3. Information loss, measured

**What the published conversion does to each AGDT label.** UD Perseus `r2.19`
(train+dev+test) was aligned with the seven AGDT 2.1 prose files it was drawn from
(`treebank_data` commit `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`, `v2.1/Greek/texts/`:
Thucydides, Plutarch ×2, Athenaeus ×2, Herodotus, Diodorus) using `align_agdt_ud.py`.
All 4,486 shared sentences aligned token-for-token (83,085 tokens). Main rows of
"AGDT relation (suffix stripped) to UD relation":

| AGDT | n | UD (top) | Construction-relevant consequence |
|---|---|---|---|
| ATR | 18,811 | det 10,037 · nmod 7,831 · acl 432 · amod 314 | Article vs attribute recoverable (UPOS). |
| ADV | 10,570 | obl 4,099 · advcl 3,618 · advmod 2,525 | Clause/phrase/adverb split is *added* by UD, but AGDT recovers it from POS. |
| OBJ | 9,041 | obj 5,400 · obl 1,106 · xcomp 1,023 · ccomp 704 · iobj 485 | Infinitive objects become `xcomp`/`ccomp`. |
| AuxY | 5,670 | advmod 5,664 | Sentence particles merge into `advmod`. |
| AuxZ | 2,285 | advmod 2,187 · discourse 89 | Focus particles and negation merge into `advmod`. |
| PNOM | 1,500 | xcomp 566 · root 367 · advcl 159 · obj 158 · acl 75 | Predicate nominal is scattered over five relations. |
| OCOMP | 210 | xcomp 210 | Object complement is indistinguishable from controlled infinitive. |
| ATV / AtvV | 181 / 127 | xcomp 180 / 126 | Circumstantial attributive participle merges into `xcomp`. |
| AuxC | 1,219 | mark 1,219 | Subordinator kept as `mark`. |
| (child of AuxC) ADV / OBJ | 812 / 128 | advcl / ccomp | Clause type preserved, as on the AGDT verb. |
| ExD | 162 | vocative 162 | Only vocatives survive, because elliptical sentences are absent. |
| COORD | 3,212 | cc 3,173 | Non-first conjuncts lose their function label (to `conj`). |

So the AGDT distinctions that vanish or become ambiguous are OCOMP / ATV / AtvV / PNOM
(all partly `xcomp`), AuxY / AuxZ / negation (all `advmod`), ExD and ellipsis (absent),
and the non-first conjunct's function. A construction detector needs most of them:

- the circumstantial participle (ATV/AtvV);
- the predicate nominal and copula (PNOM);
- the object complement (OCOMP);
- men ... de style particle structure (AuxY);
- clauses with an elided verb (ExD and ellipsis).

UD gives no first-slice distinction that AGDT lacks. The `advcl`/`ccomp` split is the
AGDT ADV/OBJ label on the clause verb under AuxC. The `obl`/`advmod` split comes from
UPOS. UD's only real gain is the explicit `cop` and the content-word-head convention,
and both are derivable from AGDT.

**Ellipsis selection.** `ellipsis_selection.py` over the same seven files:

| AGDT 2.1 sentences | in UD Perseus | not in UD Perseus |
|---|---|---|
| with artificial nodes | **0** | 1,432 |
| without artificial nodes | 4,486 | 960 |

The README's "selection of passages" is, at least for these prose files, a selection
that excludes every elliptical sentence.

**Coordination without `_CO`.** `coord_heuristic_eval.py` applies HamleDT's
`identify_coap_members` rule to gold AGDT 2.1 trees with the suffixes ignored, and
compares the result with the gold suffixes. It was bounded to two prose files:
Herodotus `tlg0016.tlg001.perseus-grc1.1` (1,555 sentences) and Thucydides
`tlg0003.tlg001.perseus-grc1.1` (942 sentences).

| COORD/APOS group | n | exact conjunct set | member precision | member recall |
|---|---|---|---|---|
| all | 3,051 | 0.826 | 0.894 | 0.980 |
| ≤2 candidates | 2,166 | 0.856 | 0.918 | 1.000 |
| >2 candidates | 885 | 0.753 | 0.847 | 0.940 |

This is the floor on gold trees. On OGA's parser output (reported LAS 76.67 for its best
model, arXiv 2410.12055 Table 2) the errors compound. The detector does not depend on
this step if it reads AGDT directly. It can treat coordination as "children of COORD"
and test the label it is looking for, which is what the heuristic would have to guess.

### 4. Published conversion accuracy

I found **no published loss or accuracy figure for AGDT-to-UD conversion of Greek**.
Neither the UD Perseus README nor arXiv 2404.00739 / 2410.12055 reports one. 2410.12055
§5 says only that the UD and AGDT schemes differ enough that score comparisons "can only
be loose", that UD "do[es] not contain elliptical nodes", and, citing Keersmaekers
(2021), that UD-style coordination parses better. For Latin, Cecchini et al. 2018
(<https://aclanthology.org/W18-6004/>) and Gamba & Zeman 2023
(<https://aclanthology.org/2023.udw-1.2/>) describe conversion challenges and
harmonisation. I did not extract numeric loss figures from them, so this is
**unverified**. The §3 measurements above are the only figures here.

### 5. Latin routes

All the UD rows below are at tag `r2.19`, with metadata from each README.

| Route | Arrives as | Licence | Notes |
|---|---|---|---|
| LDT (PerseusDL `treebank_data` v2.1/Latin) | AGDT-style XML | Repo README: CC BY-SA 3.0 US (the AGDT 1.x NC conflict noted in `oga-src-agdt-lineage.md` applies) | Same converter question as Greek. |
| UD_Latin-Perseus (`a296f012`) | UD | CC BY-NC-SA 2.5 | Celano conversion, harmonised in v2.12 (Gamba & Zeman 2023): adds `advcl:abs`, `acl:relcl`, `advmod:neg`, `advcl:cmp`; 9 `orphan`, no empty nodes. |
| UD_Latin-PROIEL (`5acbf8ef`) | UD (from PROIEL scheme, Haug) | CC BY-NC-SA 3.0 | Not AGDT. |
| UD_Latin-ITTB (`99da4cee`) | UD (from PDT-style via HamleDT) | CC BY-NC-SA 3.0 | Changelog: "ellipsis and ExD afun's are now addressed; where heuristics might fail, warnings are issued". |
| UD_Latin-LLCT (`ea247fca`) | UD (from LDT format) | CC BY-SA 4.0 | Perl script adapted from ITTB. "223 sentences have been left out … as their conversion requires further consideration". |
| UD_Latin-UDante (`f366aa0e`) | UD, manual native | CC BY-NC-SA 3.0 | |
| UD_Latin-CIRCSE (`5fff3a77`) | UD, manual native | CC BY-SA 4.0 | Morphology from LASLA. |
| LASLA (`CIRCSE/LASLA` `c2e2bb65`) | CoNLL-U-Plus, **no syntax** (HEAD/DEPREL `_` in the file sampled) | CC BY-NC-SA 4.0 | Texts "available through data-sharing agreements". |
| Stanza `la` (resources 1.15.0, `stanza-resources` `bc9e4f54`; Stanza v1.15.1) | UD | Model follows training data | Default package `ittb_nocharlm` (NC). Non-NC options: `circse`, `llct`. |

The pattern is the same for Latin as for Greek. Every route that converted an AGDT/PDT
source dropped or flagged elliptical sentences (UD Perseus, LLCT) or used heuristics
with warnings (ITTB). An NLP-service route for Latin would arrive in UD. An imported
pre-annotated route would arrive in either. A detector bound to one label set therefore
fits only half the Latin routes. One behind a construction interface with one adapter
per scheme fits all of them.

### 6. Mouseion's existing UD consumers

Checked at `origin/main` `45ffbcfd`:

- **Sentence study** (`internal/webapp/sentence_study.go:393-399`): relation and head
  are hidden when `SyntaxState` is `native_agdt` or `absent`. If Greek were stored as UD
  with state `ud`, the existing secondary disclosure would start showing raw UD labels
  for Greek. With a new `native_agdt` state it stays hidden until the feature adds its
  own view.
- **sentencequality / GDEX** (`internal/sentencequality/sentencequality.go:145-178`):
  the dependency overlay runs only for `de` (`isGerman`). It would not read Greek in
  either form.
- **German `advmod` recovery** (`internal/analyzer/proto.go:104-124`): runs only on NLP
  proto results with language `de`. The import path does not pass through it.
- **Language-neutral `compound:prt` checks**: `selection.OccurrenceEligible`
  (`internal/selection/selection.go:339`) and card particle bolding
  (`internal/cardexport/cardexport.go:364`). UD Perseus produces no `compound:prt`, so
  UD Greek would not trip them. AGDT labels never equal it.
- The schema allows `syntax_state ∈ {ud, native_agdt, absent}`
  (`migrations/000036_greek_import_identity.up.sql:22`), and ADR 0099 already reserves
  `native_agdt` with "fields that are never read as UD".

**Conclusion.** Storing UD would switch on one consumer (the Sentence study
relation/head view) by accident. Storing native AGDT switches on none.

## Open and unverifiable claims

- The current UD Perseus converter is unpublished, so its ellipsis policy is inferred
  from the data: 0 of 1,432 elliptical sentences across seven prose files. I did not
  check the verse files.
- The coordination figure is measured on two gold prose files only. It is an estimate
  for OGA, not a measurement of OGA.
- I extracted no numeric conversion-loss figures from Cecchini et al. 2018 or Gamba &
  Zeman 2023.
- The LASLA "no syntax" finding comes from one sampled file plus the README.
