# OGA official-example format probe

**Result: conditional format feasibility, not a reading/vocabulary-ready unit.**
The official example can be read coherently as linked original text and native
automatic grammatical evidence. All supported references and original-text spans
resolve. Sentence completeness, full canonical vocabulary identities, marker-free
reading text and exact underlying-source rights remain unvalidated. This does not
choose OGA, assess Anabasis coverage, or authorize Stage 2.

Context: [Test OGA text and annotation import using the official public example](https://github.com/justin-hayes/mouseion/issues/2051),
under [Find the way to first-class Ancient Greek in Mouseion](https://github.com/justin-hayes/mouseion/issues/2021).
The [external-annotation scope reset](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6177588105)
supersedes the old candidate-specific human-only wording, not ADRs 0093/0094's
integrity contract. No accepted files or GLAUx artifacts are changed here.

## Reproduce and inspect

Python **3.13 / Unicode 16.0.0**, standard library only. From the repo root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 doc/research/ancient-greek-oga-format-probe/probe.py --fetch --check --test
```

`--fetch` is explicit acquisition of the 26 allowlisted OGA example/documentation
files for local inspection under the declaration reviewed in
[RIGHTS.md](ancient-greek-oga-format-probe/RIGHTS.md). It makes no archive request,
follows no redirect, retries nothing, and fails on a changed hash. Without it the
command is offline. Downloads/output default to
`~/.cache/scratch/greek-oga-format-probe/{inputs,output}/`; `--cache` and `--output`
can point to retained private evidence instead.

This session's originals, consulted specifications and full normalized output are
retained in `doc/research/ancient-greek-oga-format-probe/private/{inputs,output}/`
(Git-ignored, **not pushed**); temporary scratch copies were removed at handoff.

Inspect `output/normalized.json` (all nodes, raw layers, provenance, partial
canonical fields, native coordinates, edges and metadata) and `output/text.txt`
(exact decoded body, no appended newline). These **local** files are intentionally
not published. [inputs.json](ancient-greek-oga-format-probe/inputs.json) records
every consumed file's complete repository path, immutable GitHub URL, byte size
and SHA-256; `probe.acquire` is the per-file retrieval command implementation.
The registry's separate `references` list records consulted PAULA and CC
specifications; these are not reader runtime inputs. The CC 4.0 specification URL
is versioned and its initially retained bytes hashed rather than Git-pinned.
A fresh verification request to that endpoint returned **HTTP 403**: no retry,
alternate transport or challenge bypass was attempted. The registry includes the official bibliographic list solely for
its example entry at lines 2–10; no corpus counts/survey are computed from it.

[ledger.json](ancient-greek-oga-format-probe/ledger.json) publishes structural
diagnostics without source strings. [outputs.json](ancient-greek-oga-format-probe/outputs.json)
records exact hashes/sizes for it and both private outputs. `--check` compares
the regenerated ledger **byte-for-byte** and verifies the full output manifest.
Tests independently compare supplied coordinates against whitespace segmentation,
all 454 comment witnesses, and original-text slicing; hashes alone are not the
correctness argument. Invented fixtures test identity, Unicode, bounds, IDs,
bases, layer cardinality, punctuation/markers, mapping abstention and bad graphs.

## Exact text, edition and indexing

OGA is pinned to **`a69d917dbe117a307829977dd556e22f05ce0715`**, not mutable `main`.
All 16 example XMLs are preserved unchanged locally, including native headers,
DTD declarations, comments and raw feature values. No DTD is fetched or evaluated;
this is not a DTD-conformance claim. Entity declarations are rejected.

The text authority for this probe is the supplied [text XML, lines 3–6][text],
specifically its decoded `body` string-value, excluding markup/comments, without
trimming, spacing reconstruction or Unicode normalization. It has **2,666 code
points** (NFC); NFD has **3,157**. Its UTF-8 SHA-256 is
`25a887cc80701debc6752fda1d1e8037f39ffd11667a6fa02b9404faac0d5718`.
The original XML bytes have their own different registry hash.

Native identity is source-commit + document path + node ID, not offsets or CTS
alone. [Token marks, lines 5–33][tokens] declare a relative text base and
`string-range(//body,'',start,length)` coordinates. The implemented, deliberately
narrow **PAULA 1-based absolute start + length** profile converts to half-open
`[start−1,start−1+length)` Unicode-code-point spans, as described in
[PAULA documentation, lines 22–57][paula-spans]. It is not a generic W3C
XPointer evaluator: other selectors/ranges and zero-length tokens stop explicitly.
All actual example marks are positive-length. Actual comment witnesses agree
exactly; no substring search, accent stripping or lemma-length offsets are used.
NFD fixtures retain their own longer original spans; normalizing before resolving
coordinates fails the tests. Significant diacritics/subscripts remain distinct.

The official [bibliographic record, lines 2–10][bibliography] names Anonymous,
*Anametresis Pontou*, Müller, *Geographi graeci minores*, volume 1, Paris,
Ambroise Firmin Didot, 1855. The metadata XMLs separately carry
`ggm0001.ggm001.1st1K-grc1` and its work/edition components. README
[lines 11–23][readme] identifies upstream release families. These are declared
bibliography and digital identity, **not a collation**: the relation of OGA's
spaced/processed text to the exact upstream edition file is unverified. No
different edition was fetched to improve agreement.

## Links, exceptional nodes and structure

| Observed supported property | Result |
| --- | --- |
| Native marks | 454: 438 `t_*`, 16 `e_*` |
| Lemma / morphology / CTS / IPA links | Exactly one of each per mark, resolved through `xml:base` and native IDs |
| Dependency relations / function labels | 423 edges, exactly one function label per edge |
| Missing/duplicate required IDs, dangling/wrong-document links | 0 |
| Invalid spans, overlaps, uncovered non-whitespace | 0; 453 uncovered whitespace code points |
| Optional metadata feature IDs | Absent on 7 supplied features; identified by file/link, not invented IDs |
| Source punctuation | 56 actual punctuation strings, retained, not vocabulary |
| Native `u` with nonpunctuation form | 6, explicitly unresolved rather than blindly discarded |
| Graph under native source→target orientation | Acyclic forest, 31 weak components / 31 zero-incoming candidates; no multiple incoming edges |

Each `e_1`–`e_16` covers a literal three-character `[0]` in the supplied text,
has lemma `[0]` and POS `-`; [first example: tokens 26–27][tokens],
[morphology 57–59][postags]. OGA [documents `-` as elliptical, lines 72–77][morphology].
These are therefore accounted-for **native elliptical-node placeholders in the
processed text**, not printable supplied words or vocabulary occurrences. The
reader retains their **source** spans but gives them no lexical surface span,
headword or canonical lemma. It cannot establish whether OGA inserted the literal
marker or inherited it from earlier processing/edition evidence. It does not
delete it, guess an unexpressed word, or claim a clean reading-text transformation.
PAULA also supports genuinely zero-length tokens ([lines 51–83][paula-spans]);
that representation is **not** present in this example and is unsupported here.

[Relations 5–18][dependencies] and [function features 5–18][functions] are separate
layers. Preserve source/target orientation and native function labels; no UD tree
conversion or fabricated self-headed root is produced. PAULA's pointing relations
are generic directed edges ([documentation 3–15,43–72][paula-relations]); its
examples are not evidence of a universal dependency-head orientation for OGA.
The zero-incoming candidates are reported, not asserted to be annotated roots.

There is **no native sentence-boundary layer** in these 16 files. CTS values
`1_1`, `2_2` … `2_8` group nodes ([CTS 6,84–85,459][cts]); they are not certified
sentence extents. Six edges cross those groups: `r_218`, `r_219`, `r_388`,
`r_389`, `r_405`, `r_406` ([relations 223–224,393–394,410–411][dependencies]).
This is reported rather than rejected under an invented sentence-local rule.

## Minimal interpretation and honest limits

The [official morphology table][morphology] supports a deliberately partial,
versioned category mapping: `a→ADJ`, `l→DET`, `r→ADP`, `m→NUM` (**221 nodes**).
These are interpretations of attributed predictions, not verification that each
prediction is linguistically right. `n/v/c/d/p/x/u/-` remain native/unresolved:
no automatic noun/name, VERB/AUX, conjunction or particle reinterpretation.
All raw morphology and syntax are retained separately; no canonical FEATS/basic
dependency layer is fabricated.

**373** plain Greek lemma strings receive only the accepted
NFC → lowercase → final-sigma → NFC profile; usable citation spelling is retained
separately. This is not 373 accepted vocabulary occurrences. The actual suffix
lemma `λέγω3` at **`t_206`** ([lemma file line 1][lemmas], token [434–435][tokens])
stays intact and unresolved: no suffix stripping or invented homograph namespace.
Numeric lemmas and unexpected forms likewise stay inspectable. Discriminator
status is **unknown, not asserted absent**, even for plain strings; no complete
canonical vocabulary identity is emitted. Per-node statuses identify every
affected record in the private normalized output and public structural ledger.

OGA [README 53–90][readme] declares AGDT-style automatic parser/lemmatizer outputs,
automatic CTS/bibliographic extraction and experimental IPA; composition dates
have separate manual provenance. No per-node human review or exact model revision
is invented. Machine prediction accuracy is unassessed; import correctness and
model quality are different questions.

## Review boundary

Validation on 2026-10-14: **32 focused tests pass**, including byte-identical
ledger regeneration and checks of the normalized/text output manifest. All 26
runtime inputs were freshly acquired and verified; the CC reference limitation
above is separate. `git diff --check` passes. Whole-project `make lint` stops
because `golangci-lint` 2.14.0 is unavailable; `make coverage-new` stops because
the project `.venv`/pytest environment is absent. No whole-project lint or
coverage success is claimed for this standalone research reader.

[RIGHTS.md](ancient-greek-oga-format-probe/RIGHTS.md) records actual declarations,
attribution, modification notices and private/public/bundling dispositions.
The explicit OGA CC BY-SA declaration supports the bounded diagnostic output;
underlying text/component applicability remains unresolved for redistribution.
No source XML, full normalized sample or reading text is committed.

**Established:** executable linked-package consumption, exact supported spans,
complete accounting of this example's nodes and explicit partial interpretation.
**Not established:** complete reading sentences, marker-free edition text,
vocabulary-ready identities, prediction quality, a source choice, a permitted
corpus acquisition route, full Anabasis Book 1, vocabulary selection or cards.
No prerequisite chain is launched. Independent parent review owns any comparison
and any separately authorized Stage 2; this research branch has no PR and must
never merge.

[readme]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/README.md
[text]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/ggm0001.ggm001.1st1K-grc1.xml#L3-L6
[tokens]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/ggm0001.ggm001.1st1K-grc1.tok01.xml
[lemmas]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/ggm0001.ggm001.1st1K-grc1.tok01_lemma01.xml#L1
[postags]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/postag.ggm0001.ggm001.1st1K-grc1.tok01_postag01.xml
[dependencies]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/dep.ggm0001.ggm001.1st1K-grc1.tok01_dep01.xml
[functions]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/dep.ggm0001.ggm001.1st1K-grc1.tok01_dep01_dep-fnc01.xml
[cts]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/ggm0001.ggm001.1st1K-grc1/ggm0001.ggm001.1st1K-grc1.tok01_cts01.xml
[bibliography]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/urn_cts/texts/urn_cts_plus_date_label.xml#L2-L10
[morphology]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/query/morphology.md
[paula-spans]: https://github.com/korpling/paula-xml/blob/316e01d43b43f99f683d5d0ebe631bb5ffe4973f/src/spans-and-markables.md#L22-L83
[paula-relations]: https://github.com/korpling/paula-xml/blob/316e01d43b43f99f683d5d0ebe631bb5ffe4973f/src/pointing-relations.md#L3-L76
