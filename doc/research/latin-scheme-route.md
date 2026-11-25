# Annotation scheme route for Latin: reuse the AGDT stack, or the reserved UD adapter

**Research date:** 2026-11-25

**Scope:** What each annotation scheme costs Mouseion for classical Latin
(map: classical Latin in Mouseion; ticket: annotation scheme route for Latin).
The lead candidate is Opera Latina Adnotata (OLA) v0.2.0, which is the
AGDT-scheme output of a published model. It is assessed against the Greek
importer (`internal/ancientgreek`, [ADR 0099](../adr/0099-ancient-greek-imported-analyses.md))
and the native-AGDT stack ([ADR 0122](../adr/0122-ancient-greek-native-agdt-syntax.md),
[ADR 0123](../adr/0123-constructions-are-a-derived-reading.md)). The UD
fallbacks are the Scaife/Perseus6 LatinCy shards and UD Latin-CIRCSE. The
corpus and licence findings of the sibling tickets are cited, not repeated:
[`latin-oga-equivalent.md`](https://github.com/justin-hayes/mouseion/blob/3c1318cfafc5f95b4868d071fa317f37ab8969c5/doc/research/latin-oga-equivalent.md),
[`latin-gold-treebanks.md`](https://github.com/justin-hayes/mouseion/blob/12c3b707d67392dfe154fc79e9de5df54a38adc8/doc/research/latin-gold-treebanks.md),
[`latin-grc-generalisation.md`](https://github.com/justin-hayes/mouseion/blob/16303b380a1a66a58f0a415933b78e5d862186c5/doc/research/latin-grc-generalisation.md).

Primary sources only: the real files of the pinned release, read from the
Zenodo archive by HTTP range request, the Greek code at `a2317215`, and the UD
and Scaife repositories at the tags and commits named below. Words are
non-punctuation tokens unless stated. "Classical" and the class of each Work
follow the sibling ticket's table
([`ola-conllu-manifest.tsv`](https://github.com/justin-hayes/mouseion/blob/3c1318cfafc5f95b4868d071fa317f37ab8969c5/doc/research/latin-oga-equivalent/ola-conllu-manifest.tsv),
SHA-256 `05c76d7cd0d7d3152157d6a307b5777b05c95b10a5f8320c71bc150420d42ec0`).
Only counts, IDs, digests and offsets are committed; scripts are in
[`latin-scheme-route/`](latin-scheme-route/), results in
[`latin-scheme-route/results/`](latin-scheme-route/results/).

## Answer

- **The Greek AGDT stack is reusable at its seams, not at its parser.** What
  carries over unchanged: the source-neutral `analyzer.Result`, the stored
  `syntax_state = 'native_agdt'` with verbatim relations and heads, the
  `corpus_tokens` columns, the `syntax.Tree` and construction interface, and
  ADR 0123's derived-table machinery. OLA's labels are the AGDT label set and
  every one of 328,687 sentences has exactly one root with every head
  resolving. What does not carry over: the OGA CoNLL-U parser (it refuses
  every OLA line), the positional-postag feature mapper, the POS validator
  (it would refuse 160 of 209 Work files), the Greek punctuation and
  normalisation rules, and the AGDT adapter's exact-match relation mapping.
  The last one matters most: **11.6% of OLA tokens, in 59.3% of sentences,
  carry a `_CO`, `_AP` or `_ExD_…` suffix** that the adapter files under
  `CategoryOther`. Greek OGA has none of these.
- **OLA needs a new Latin adapter, not a new architecture.** It is
  `syntax_state = 'native_agdt'`, no migration, and the same source-neutral
  tree. The adapter is new work in four places: a parser for the OLA dialect
  joined to the Citation layer, a key-value feature mapper with the Latin
  values, a relation reader that understands the suffixes, and an `la`
  normalisation profile. Latin Constructions are new detectors and a new
  lexicon on top; nothing Greek-specific in `internal/constructions` is
  reusable except the structure.
- **The Citation join works and is exact.** The Citation layer is a separate
  PAULA file, joined by the `t_N` token id. On 209 Work files (5.27M tokens)
  every CoNLL-U token has a Citation value, and on the 12 Works checked
  against the token layer no surface differs. The Citation values are not
  Greek-clean: 125 of 209 files parse as numeric dotted references after
  stripping a URN prefix that 10 files carry; 72 contain letter suffixes
  (`331a`), named sections (`pr`, `ep`, `index`) or both, 9 have uncited
  sentences and 30 are not monotonic. A Reading unit is a whole Work (ADR
  0128), so no mid-Work boundary is needed; the Citation layer feeds only the
  Work's start and end, the constituent-book count and per-sentence Citations.
- **Fetch per Work is possible and cheap, but is a new delivery mode.** The
  3.86 GB archive stores its inner archives uncompressed, so one Work costs two
  range requests of about 176 KB at the median (CoNLL-U plus Citation member,
  compressed; 1.2 MB at the 90th percentile, 10.9 MB for Livy), and the whole
  209-file classical BY-SA set costs 93.0 MB. A manifest can pin each member by
  absolute offset, size, CRC-32 and SHA-256, and a member then needs no zip
  directory (verified for two Works below). The Greek path fetches a file by
  URL and SHA-256; Latin needs an archive-member fetch kind in `layers.json`
  and `classical_preparation`.
- **The UD routes are cheaper on tags and dearer on everything else.** UD input
  is the ordinary `syntax_state = 'ud'`, which switches on the existing UD
  consumers (lexical sense ordering, generic Sentence-study relations) that
  ADR 0122 kept off for Greek. Scaife shards give a UD parse with the Citation
  inline and a per-file GitHub fetch that fits the Greek path, but the parse is
  tagged one citation chunk at a time with no sentence boundaries, and its
  measured LAS on unseen gold is 37.3. UD Latin-CIRCSE is gold but is five Works,
  has no punctuation, no Citation, and splits enclitics into multiword tokens.
- **The same Work is available in both schemes, independently.** The five
  CIRCSE Works are each in three forms: OLA (AGDT, predicted), CIRCSE (UD, gold)
  and Scaife (UD, predicted). Neither OLA nor CIRCSE is derived from the other.
  The Perseus UD treebank is an automatic conversion of LDT with passages left
  out, by the same person who made OLA.
- **Recommendation:** take OLA's AGDT route, with a Latin adapter reading the
  suffixes, as the first-slice annotation path, and treat CIRCSE as the gold
  yardstick for measuring it rather than as a source. Keep the Scaife shards
  as a cold-start fallback only if the corpus grilling rejects OLA's licence
  lineage. The decision on gating Latin Constructions waits for a precision
  measurement; the five Works in two schemes give a way to run it (see
  Recommendation).

## What was measured, and on what

| Layer | Files | Coverage |
| --- | --- | --- |
| OLA CoNLL-U, `workspace/conllu.zip` | all 736 read; the 209 classical BY-SA files, 16 classical NC-text files and 12 borderline files profiled | whole layer, not a sample |
| OLA Citation, `workspace/ola.zip`, `…tok01_cts01.xml` | 209 classical BY-SA files | whole classical BY-SA set |
| OLA token layer (`tok01.xml`), text, postag, dep | 12 Works (Sallust *BC*, Caesar *BG*, Cicero *Cat.*, Tacitus *Agricola* and *Germania*, Virgil *Aeneid*, two Seneca tragedies, Livy, Ovid *Met.*, Petronius, Pliny *Ep.*) | sample, for the join check |
| UD Latin-CIRCSE `r2.19` (`5fff3a77`) | all three files | whole treebank |
| Scaife `tagging-shard-03` (`21dd17dd`), `-04` (`6eab631a`) | 5 Work files (Sallust *BC*, Caesar *BG*, Virgil *Aeneid*, Cicero *Cat.*, Tacitus *Germania*) | sample, for the format |
| UD Latin-Perseus `r2.19` | all files | sentence-number gaps only |

The archive is Zenodo 10.5281/zenodo.15183688, `opera_latina_adnotata_v0.2.0.zip`,
3,857,914,226 bytes, MD5 `543305da911c91b9ade5cfa3e82d659f` (from the sibling
ticket; this ticket read it by range request and did not re-download the whole
file). "209 BY-SA files" are the 205 BY-SA classical Works plus four second
Editions of the same Works. The 16 files with NC-SA text and the 8 borderline
Works are reported separately where they differ. Whole-layer token totals are
5,270,178 tokens (4,413,485 non-punctuation) in 328,687 sentences for the BY-SA
files, and 441,410 tokens in 27,016 sentences for the NC-text files.

## 1. OLA, the AGDT route

### The file dialect against the Greek parser

The OLA CoNLL-U is the same AGDT family as OGA but a different dialect. The
Greek parser (`parseOgaSrc`, `ogasrc.go:47`) is strict and stops with
`source_malformed` on the first line.

| Element | Greek `oga_src` | OLA CoNLL-U (measured) |
| --- | --- | --- |
| Sentence headers | `# source`, `# text`, `# sent_id`, in order | `# absolute_id`, `# clause_id`, `# file` on every sentence; no text, no sent_id, no source |
| Columns | 10 | 10 on every one of 5,270,178 lines |
| Column 5 | nine-character positional postag, required | `_` on every token (`xpos` is empty) |
| Morphology | positional letters inside the postag | column 6, `Key=value` with AGDT letter values (`Case=a|Gender=m|Number=p`); 37 lines have an empty column 6 |
| MISC | `t_n|head|ref=<label>_<citation>` | `t_N` only, on every token |
| Citation | in MISC, required per token | none; in the PAULA layer |
| Ellipsis | tokens with POS `-` | no unprinted nodes (see below) |

So the Greek parser and `citationFromMisc` cannot be adapted by a switch; a
Latin dialect parser is needed, and the format switch at
`classical_preparation.go:619-625` (the designed seam) takes a new source
format beside `oga-conllu`. Sentence text has to be rebuilt from tokens, as the
Greek importer already does with `reconstruct`.

### Parts of speech

The letters are the AGDT set. Counts are non-punctuation tokens in the 209 BY-SA
files.

| POS | Greek `mapUPOS` | OLA |
| --- | --- | --- |
| `n v a d c r p m i x` | mapped | present; `p` (491,095) holds pronouns and determiners alike, so UPOS `DET` cannot arise |
| `u` | `PUNCT` | present (856,693 punctuation tokens) |
| `l` (article), `g` (particle) | `DET`, `PART` | absent in Latin |
| `e` (exclamation) | **not in the set** | 6,218 tokens in 160 of 209 files |
| `-` | **ellipsis, dropped** | 1,709 real tokens in 51 files |
| `t` (particle) | not in the set | documented, none observed |

`validateNativePOS` checks the whole document before anything is imported and
refuses on an unknown value, so the `e` tokens alone would stop 160 of 209 Work
files. The 1,709 `-` tokens are not ellipsis nodes. Sampled, they are
fragments such as single letters and sigla (`aADRV`), the apparatus leaking
into the text that the sibling ticket warned about; `isEllipsis` would drop
them silently and `unprintedNodes` would then have to resolve their dependents.
`x` is present on 1,255 tokens in 140 files and is mapped to `X` already.

The proper-name rule differs in source. Greek computes `PROPN` from surface
capitalisation statistics (`properNameStats`) because AGDT has no proper-name
tag. OLA's lemma keeps its capital for names: 164,004 of 1,180,046 noun tokens
(13.9%) have a capitalised lemma, which is a direct signal, but 36,569 adjective
tokens also do (ethnic adjectives), so the rule still needs a decision.

### Morphology

The Greek mapper reads a nine-slot positional postag by index
(`featureSlots`, `mapFeatures`). Latin arrives as keys, so the reader is new, but
the value tables are mostly reusable. Values the Greek tables do not hold, over
the 4,413,485 non-punctuation tokens:

| Value | Meaning | Tokens | Effect if mapped by the Greek tables |
| --- | --- | ---: | --- |
| `Case=b` | ablative | 542,648 (12.3% of all tokens) | dropped and counted as unmapped |
| `Mood=g` | gerundive | 28,039 | `VerbForm` and `Mood` lost |
| `Mood=d` | gerund | 8,623 | same |
| `Voice=d` | deponent | 12,451 | dropped |
| `Degree=p` | positive | 7,065 | dropped |
| `_` as a value (`Degree=_`, `Number=_`, `Person=_` …) | placeholder | 38,362 for `Degree` alone | counted unmapped; Greek writes `-` |

In all, **592,990 tokens (13.4%) lose at least one meaning-bearing value** if the
Greek tables are applied as they are. `Mood=u` (supine) does not occur. The
tense letters (`p i r l t f`) are covered by the Greek table, and its perfect and
future-perfect Aspect rule matches Latin UD, so `Tense` can be reused. A Latin
table has to add `VerbForm=Ger` and `Gdv`, `Case=Abl`, and a policy for the
deponent voice and the placeholder.

### Relations, suffixes and the AGDT adapter

The base labels are the AGDT set: `ATR ADV OBJ SBJ AuxX PRED AuxK COORD AuxP
AuxZ AuxC AuxY PNOM AuxV AuxG ExD APOS ATV OCOMP AtvV`, plus a stray `XSEG` on
82 tokens (the README lists it among the treebank's mistaken labels). The
difference from OGA is the suffix, which OGA dropped and OLA keeps:

| Form | Tokens (of 5,270,178) | Share |
| --- | ---: | ---: |
| bare label | 4,657,205 | 88.4% |
| `_CO` (coordinated member) | 591,767 | 11.2% |
| `_AP` (apposition member) | 5,486 | 0.1% |
| `…_ExD_PRED`, `…_ExD_OBJ` etc. (ellipsis markers) | 15,720 | 0.3% |

`internal/syntax/agdt.Category` maps a relation by exact match, and says so:
"Anything else … is `CategoryOther`: the measured detector precision depends on
these semantics, so no new normalisation is added." Against OLA, **612,198
non-punctuation tokens (13.9%) fall to `CategoryOther`, and 195,056 sentences
(59.3%) contain at least one**. `EffectiveChildren` walks down through
`CategoryCoordination` nodes and returns children by category, so a coordinated
subject labelled `SBJ_CO` would not be found as a subject, which is the exact
shape detectors ask for. This is a defect of exact matching for Latin and not of
the OLA labels: the suffix carries information the Greek corpus lost (which
children are conjuncts and which are shared modifiers). The reader should strip
the suffix to the base category and keep the conjunct flag. ADR 0122 stores the
relation verbatim, so the suffix survives storage and the fix is at read time,
as ADR 0122 decision 3 already says ("COORD resolution and stray labels are
normalised at read time, in the AGDT adapter").

The AGDT-specific conventions carry over unchanged. Every sentence has one root
(328,687 of 328,687; no sentence has a dangling head), and punctuation is
attached under the first node, as in OLA's own documentation and ADR 0122's
normal form. 21,666 sentences (6.6%) have a punctuation token as root, the Latin
analogue of Greek's comma root (12.7%); 30.5% of roots are `COORD` and 61.1% are
`PRED`. Median sentence length is 13 tokens, 95th percentile 39, maximum 241,
and 201 sentences exceed 100 tokens.

### Ellipsis

OLA has **no unprinted nodes**. Its tokens come from the text, so no artificial
node exists, and the ellipsis is carried in labels instead: 31,122 tokens are a
bare `ExD` (the remnant of an elided structure, 0.59% of tokens), and 15,720 carry
a composite `…_ExD_PRED`-style label that names the relation to an elided
predicate. 36,490 sentences (11.1%) contain one of them. Against ADR 0122:

- The unprinted-node table, the `HeadUnprintedNode` column and the
  `sourceOrder` source-id inference stay unused for Latin, which makes the
  import simpler than Greek's (no 18.6% multi-root repair, no id inference).
- The cost moves to the detectors. ADR 0123's rule that a detector "reached only
  through an unprinted node … produces no hit" has nothing to fire on; an elided
  predicate is visible only as a composite label. A Latin detector set has to
  decide what `SBJ_ExD_PRED` means for a clause detector. The 4,145 root
  relations of the form `SBJ_ExD_PRED` and the 3,368 `ADV_ExD_PRED` are the
  population.
- `isEllipsis("-")` must not be applied to Latin (see parts of speech).

### Lemma identity and normalisation

OLA lemmas have no homograph discriminator in the Greek sense. 7,674 tokens
(0.15%) end in a digit, and the ones sampled are apparatus sigla (`D1`, `V1`) and
numerals, not Morpheus numbers, so `decodeLemma` would split them wrongly into a
`morpheus:N` identity; Latin needs the opposite rule, to reject them. No `la`
canonicalization profile exists (the registry holds `grc`), and the data say
what one needs:

- 95,426 distinct lemma types on 4.41M tokens fold to 87,458 under
  case, diacritic, `j`/`i` and `v`/`u` folding; 6,630 folded groups (7.6%) have
  more than one spelling. Of these 2,788 differ only in case, 2,769 involve a
  non-ASCII character (accents, Greek script) and 282 are `i`/`j` splits;
  44,613 tokens (1.0%) have a `j` in the lemma and every one of the 209 files
  has some.
- Plautus carries metrical accents on 500 to 1,200 tokens per play, and some
  lemmas are in Greek script (embedded Greek words).
- The lemma spelling uses `v` and `j` as the editions do, so the profile has to
  choose whether `uenio` and `venio` are one headword. This is ADR-level lemma
  identity and belongs to the dictionary and lemma-identity tickets; this
  ticket only records that the data need folding.

### Reconstructing the Reading unit text

The Greek `reconstruct` joins tokens with spaces and omits the space before
`, . ; · ;` (`noSpaceBefore`). Applied to the 856,693 Latin punctuation tokens,
that rule is right for 85.6% (the comma, full stop and semicolon) and wrong for
the rest: colon (36,708), question mark (31,431), exclamation mark (5,745),
closing brackets and quotation marks. The text of the Reading unit would show a
stray space before each. OLA's own text layer is the token stream, so there is
no original typography to recover. Quote marks are one undirected `"` on 4,259
tokens, so an open/close rule needs context.

### The Citation layer and the join

The PAULA layer `ola.<URN>.tok01_cts01.xml` holds one `<feat>` per token, keyed
`#t_N`. The CoNLL-U column 10 is `t_N`, so the join is by integer id. Measured:

| Check | Result |
| --- | --- |
| CoNLL-U tokens with no Citation entry | 0 of 5,270,178 (209 files) |
| Citation value empty | 11,571 tokens (0.22%), 9 files |
| Sentences with no cited token | 2,294 (0.70%), 9 files |
| Surface in CoNLL-U equals the token layer's surface for the same id | 0 differences in 12 Works, 1,000,000+ tokens; ids strictly increasing in all 12 |
| PAULA tokens the CoNLL-U lacks | 1 in *BG*, 20 in Petronius, none in the other ten |
| Sentences with one distinct leaf Citation | 264,721 (80.5%); 39,104 with two; 22,568 with three or more |
| Sentences crossing a top-level (book) value | 11,721 (3.6%) |
| Files whose Citation runs decrease in numeric order | 30 of 209 (96 decreases in all) |

The depth structure of a value (`1_2_3`, joined by `_`) is 1 level for 938,545
tokens, 2 for 1.93M, 3 for 2.20M and 4 for 31,177. Known structure checks out:
Sallust *Bellum Catilinae* has 61 top-level values, *Germania* 46, Caesar *BG* 8
(*BG* 1 to 7 and Hirtius's book 8, in one Work file), Livy 35, and
*In Catilinam* 4.

What Greek's `parseCitation` (`ogasrc.go:780`) would accept: only dotted
non-negative integers. After stripping a CTS URN prefix that 10 files carry
(Lucan, nine Ovid files, Tacitus *Historiae*; 305,119 tokens), **125 of 209
files are clean**. The other 72 hold a non-numeric component on 158,251 tokens
(3.0%): a number with a letter suffix on 87,657 tokens in 53 files (`331a`,
Plautus and Cicero), named sections on 70,359 tokens in 24 files (`pr`, `ep`,
`epilogus`, `appendix`, `index`), and 274 other tokens in 6 files. A Latin
citation type is therefore needed, not a reuse of `[]int`, or the citation
reading must refuse these Works (64 of the 145 files of at least 5,000 words).
This is Latin-specific work in `internal/classical/citation.go`, which also
orders and formats Citations.

For Reading-unit boundaries nothing needs cutting. ADR 0128 makes the unit the
whole Work in one Edition, so the unit's `citationStart` and `citationEnd` are the
first and last values, the constituent-book count is the number of distinct
top-level values, and a sentence belongs to the one unit. The sibling ticket
recorded that the Citation is auto-extracted; I checked it only for the five
structural counts above, not token by token against the TEI.

### Archive size and pinning

| Quantity | Value |
| --- | --- |
| Whole archive | 3,857,914,226 bytes (3.86 GB), one Zenodo file, MD5 `543305da…` |
| Needed inner archives | `workspace/conllu.zip` 214,687,770 bytes; `workspace/ola.zip` 1,410,289,884 bytes; both stored uncompressed in the outer zip |
| Zip directories to read | outer 1.25 MB, `conllu.zip` 0.25 MB, `ola.zip` 2.65 MB: 4.15 MB in 6 requests |
| Per-Work fetch (CoNLL-U + Citation, compressed) | median 176,282 bytes; 90th percentile 1,234,004; maximum 10,935,346 (Livy, `phi0914.phi001.perseus-lat2`) |
| All 209 classical BY-SA files | CoNLL-U 64.0 MB + Citation 29.0 MB compressed; 323 MB + 333 MB inflated |
| Greek comparison | one 0.5 to 11 MB file per Work from GitHub raw at a pinned commit, SHA-256 in `layers.json` |

Zenodo serves HTTP range requests (`206`, `Accept-Ranges: bytes`) on both the
`records/…/files/…?download=1` and the API `…/content` URL. It rate-limits
anonymous clients (`429` after roughly 130 requests a minute, `Retry-After: 60`),
so a deployer-side fetch must back off.

The pin has three levels, none of which Zenodo provides below the archive:
the outer MD5, the member's CRC-32 from the zip directory, and a SHA-256 that
Mouseion computes when it builds the manifest. [`ola-bysa-classical-pins.tsv`](latin-scheme-route/ola-bysa-classical-pins.tsv)
holds, for all 209 files, the absolute byte offset of each member's local header
in the Zenodo file, its compressed size, CRC-32 and the SHA-256 of the inflated
bytes. I fetched two Works (Sallust *BC* and *Germania*) by **one range request
per member from those numbers alone, with no directory read**, inflated them and
matched the SHA-256 in all four cases. A manifest that records offset, size and
digest per Work therefore makes a Latin fetch the same shape as the Greek one,
plus a range header. Two details a spec must settle: the archive member is
`deflate` (not stored), so the fetch must inflate it; and a Work's CoNLL-U and
Citation members come from two inner archives, so a Work has two fetches where a
Greek Work has one.

What does not fit is the `layers.json` model (`baseURL` plus file name plus size
and SHA-256), which assumes one file per URL. A `zenodo-member` kind or a
repackaging step is needed. Repackaging per-Work files for a Mouseion-hosted
mirror would redistribute annotation of unclear lineage and is not recommended.
OLA has one release, no change since 2025-04-10 and one maintainer, which
matters for the pin only in that the DOI is immutable.

### Stored `syntax_state`, code touched, and what it rules in or out

| | OLA, AGDT route |
| --- | --- |
| Stored `syntax_state` | `native_agdt`; relations stored verbatim including suffixes |
| Schema | none; `analysis_runs_syntax_state_check` already allows it; no unprinted-node rows |
| New code | OLA-dialect parser with the Citation join; Latin POS, key-value feature mapper and value tables; `la` normalisation profile; Latin citation type; archive-member fetch kind; Latin reconstruct rules |
| Changed code | `parseClassicalSource` (format switch); `SourceFormat*` constants and the manifest `format` value; `layers.json` loader; `classical.Language` and the per-Definition language already listed by the generalisation ticket |
| Adapter change | `internal/syntax/agdt.Category` to read suffixes (the Greek golden outputs must not change) |
| Constructions | a Latin detector set and lexicon; ADR 0123's "one detector version per syntax scheme" does not fit two languages on one scheme, so the version is per scheme and language |
| Rules in | the AGDT adapter, the Word relations disclosure, Concordance construction parallels, once detectors exist; ADR 0125's A&G reference (shape only today) |
| Rules out | none of the UD consumers; the UD adapter stays unneeded for Latin |
| Quality | published LAS 77.53; held-out LDT LAS 81.6 (Wilson 80.3 to 82.8); construction precision unmeasured |

## 2. The UD fallbacks

### UD Latin-CIRCSE (`r2.19`, tag `5fff3a77`)

Native UD, hand-annotated syntax, CC BY-SA 4.0 (`LICENSE.txt`). Measured over
all three files:

| Work | CIRCSE tokens | Sentences | OLA URN | OLA non-punct tokens | Form alignment | Lemma agreement |
| --- | ---: | ---: | --- | ---: | ---: | ---: |
| Tacitus *Germania* | 5,674 | 299 | `phi1351.phi002.perseus-lat1` | 5,733 | 96.7% | 88.5% |
| Seneca *Hercules Furens* | 7,714 | 555 | `phi1017.phi001.perseus-lat2` | 7,698 | 98.5% | 87.1% |
| Seneca *Agamemnon* | 5,580 | 409 | `phi1017.phi007.perseus-lat2` | 5,585 | 98.1% | 87.0% |
| Seneca *Oedipus* | 5,931 | 401 | `phi1017.phi006.perseus-lat2` | 5,945 | 98.5% | 86.6% |
| Seneca *Phoenissae* | 4,156 | 308 | `phi1017.phi003.perseus-lat2` | 4,152 | 98.4% | 89.3% |

Alignment is a sequence match on case- and `u`/`v`/`i`/`j`-folded forms;
lemma agreement on the aligned pairs is an agreement figure, not an accuracy
(different lemma conventions).

Facts that bear on the adapter:

- **No punctuation and no Citation.** There are zero `PUNCT` tokens in the
  treebank and the text is lower-case (`# text` is unpunctuated), because the
  text was received tokenised from LASLA. The sentence id is a serial number
  (`Latin_SenecaYounger_HercF_poetry-261`, `SenPhoen-P-17-97`,
  `TacGerma-Q-01-92`) with three `-bis` variants. A Citation would have to be
  joined from OLA by form alignment, which is lossy at the 1.5% to 3.3% that
  do not align.
- **Enclitics are multiword tokens.** 768 `N-M` rows (216 in *Hercules Furens*)
  split `-que` and similar. OLA does not split them (`demitque` is one token), so
  any join between the two must map one token to two.
- **A fuller UD than the Greek UD Perseus.** UPOS includes `DET`, `AUX` and
  `PROPN`; the relation set has 59 values including subtypes (`advcl:abs`,
  `obl:lmod`, `acl:relcl`, `nsubj:pass`), LASLA `XPOS`, and `Speaker` comments.
  No empty nodes, no enhanced dependencies, one root in every sentence.
- **Lineage.** The README says text, lemmas, POS and morphology were received
  from LASLA, whose own repository is CC BY-NC-SA 4.0. The sibling ticket flagged
  this as unverified; it is why CIRCSE is an Analysis resource of unclear lineage,
  and why its text layer would be Source content only if the NC-SA text is
  excluded and OLA's BY-SA edition text used instead.

### Scaife/Perseus6 LatinCy shards

Five Work files were profiled (commits: shard-03 `21dd17dd…`, shard-04
`6eab631a…`).

| Work file | Tokens | Citation chunks | Median chunk | Chunks with 2 or more roots | Roots |
| --- | ---: | ---: | ---: | ---: | ---: |
| Sallust *BC* `phi0631.phi001.perseus-lat4` | 12,917 | 61 | 172 | 61 of 61 | 707 |
| Caesar *BG* `phi0448.phi001.perseus-lat2` | 59,978 | 2,150 | 26 | 681 | 3,033 |
| Cicero *Cat.* `phi0474.phi013.perseus-lat2` | 19,092 | 115 | 160 | 114 of 115 | 1,364 |
| Virgil *Aeneid* `phi0690.phi003.perseus-lat2` | 79,050 | 9,897 | 8 | 1,531 | 11,453 |
| Tacitus *Germania* `phi1351.phi002.perseus-lat1` | 6,802 | 185 | 37 | 163 | 545 |

The format is a nine-column TSV (`ref, index, form, UPOS, XPOS, morph, lemma,
deprel, head`) with a 0-based head within the chunk and `ROOT` as the root
label, so it is not CoNLL-U despite Perseus's write-up.

- **The Citation is inline**, on every row, in dotted form (`1.1`), so no join and
  no Citation layer. Citations are also numeric in these five.
- **Fetch fits the Greek model.** One file per Work at a commit-pinned GitHub raw
  URL, SHA-256 by Mouseion; the shard repositories have no tags or releases, so
  the pin is a commit (shard-04 holds all four Seneca tragedies).
- **No sentence boundaries.** A chunk holds one citation unit, which may be many
  sentences (Sallust: 707 roots in 61 chunks) or a fragment of one (a verse line
  of the *Aeneid*: 9,897 chunks for 11,453 roots). Sentences have to be derived
  from the roots, and a sentence crossing a chunk edge is cut. The text carries
  punctuation, and 120 Aeneid tokens have a blank UPOS.
- **Enclitics are split** (`que` as its own token: 3,245 in the *Aeneid*), unlike
  OLA, and the UPOS set includes `DET`, `AUX` and `PROPN`.
- **Quality.** On unseen UD Latin-Perseus gold: lemma 89.6, UPOS 85.8, UAS 56.5,
  LAS 37.3 (sibling ticket). It is the best-fitting route for the existing
  fetch path and the worst for the parse.

### The UD adapter ADR 0122 reserved, and the existing UD consumers

A UD route stores `syntax_state = 'ud'`. Greek's `native_agdt` state exists to
keep the UD readers off Greek: ADR 0122 decision 7 says no generic UD code
path reads AGDT labels. Those readers are
`internal/enrichment/lexical.go:270` (it adds a token's relation to the sense
context for `ud` runs), the sense ordering in `cardexport/presentation.go`, and
the generic UD relation display in Sentence study. A Latin UD run turns all of
them on for Latin. That is plausible behaviour but it is a product choice, not
an accident to inherit. The reserved adapter has to provide, beyond the existing
format switch: UD relation to `syntax.Category` (including subtypes and `cop`,
`aux`, `mark`, `det` which AGDT folds into `AuxV`, `AuxC`, `ATR`), the same head
and cycle handling, an enclitic multiword-token policy, an empty-node policy
(none occur in either source), and its own construction detector version
(ADR 0123).

## 3. The same Work in both schemes

| Work | OLA (AGDT, predicted) | LDT 2.1 (AGDT, gold) | UD Perseus | CIRCSE (UD, gold) | Scaife (UD, predicted) |
| --- | --- | --- | --- | --- | --- |
| Tacitus *Germania* | yes, whole | no | no | yes, whole | yes |
| Seneca, four tragedies | yes | no | no | yes, whole | yes (shard-04) |
| Sallust *Bellum Catilinae* | yes, two Editions | whole, 11,087 words (licence unclear) | 336 of at least 699 numbered sentences | no | yes |
| Cicero *In Catilinam* | yes | partial | 137 of at least 327 | no | yes |
| Virgil *Aeneid* | yes | partial | 68 of at least 175 | no | yes |
| Caesar *BG* | yes | no | 24 of at least 69 | no | yes |

The LDT figures are from the sibling ticket; the UD Perseus figures are the
sentence numbers present in its `sent_id`s against the highest number, so they
are a floor for what is missing. The README calls the treebank "an automatic
conversion of a selection of passages from" LDT 2.1, so I attribute the gaps to
selection and not to ellipsis, unlike Greek where the conversion research
([AGDT-to-UD conversion cost](https://github.com/justin-hayes/mouseion/blob/0e458333932b4a059120f29127ee6cd0ec8aca5b/doc/research/grc-agdt-ud-conversion.md))
found elliptical sentences omitted. Derivation:

- **UD Perseus is derived from LDT**, by Celano's converter, which the Greek
  research found unpublished. It is lossy and selective, and carries no Citation.
- **OLA is not derived from it**: LatinPipe predicts AGDT directly (`la_giuseppe`
  is AGDT-labelled, per the sibling ticket). Its relation to LDT is training
  data (94.2 LAS on seen sentences, 81.6 held out), not conversion.
- **CIRCSE is annotated natively in UD** by CIRCSE. It does not derive from OLA
  or LDT.
- **Scaife derives from a UD-trained model** (LatinCy) over `canonical-latinLit`.
- No tool in hand converts OLA's AGDT to UD without Greek's problems (the
  published converter is not available; Treex needs the suffixes OGA lacks, but
  OLA has them, so a conversion from OLA is more feasible than from OGA. I did
  not test one).

## 4. ADR 0111 classification, by layer

From the instruments actually in force. OLA terms and the OLA text layers are
from the sibling ticket and unchanged.

| Layer | Instrument | Terms | Kind | Restriction |
| --- | --- | --- | --- | --- |
| OLA CoNLL-U (lemma, POS, morphology, relations) | README and Zenodo record; no licence file in the zip | CC BY-SA 4.0 declared | Analysis resource | `unclear`, unverified (unpublished AGDT training set including LDT) |
| OLA PAULA Citation layer (`cts01`) | same | CC BY-SA 4.0 declared; "retrieved automatically from the original texts" | Analysis resource (structural metadata of the BY-SA edition) | `unclear`, unverified; the values are references |
| OLA text, `canonical-latinLit`, `Latin`, `priapeia` | repository `license.md` | CC BY-SA 4.0 | Source content | admitted |
| OLA text, `additional-texts`, `digiliblt` | `LICENSE.md` | CC BY-NC-SA 4.0 | Source content | excluded (16 classical Works) |
| UD Latin-CIRCSE annotation | `LICENSE.txt` at `r2.19` | CC BY-SA 4.0 | Analysis resource | share-alike; lineage unverified (LASLA NC-SA lemmas, POS, morphology) |
| UD Latin-CIRCSE text | none separate; LASLA-derived | LASLA repository is CC BY-NC-SA 4.0 | Source content | excluded unless the BY-SA claim holds; OLA's text is the alternative |
| UD Latin-Perseus | `LICENSE.txt` | CC BY-NC-SA 2.5, both layers | Analysis resource and Source content | NC: annotation deployer-fetched, text excluded |
| Scaife shards, annotations | none in the repository | unlicensed; MIT model trained on NC-SA data | Analysis resource | `unclear`, unverified |
| Scaife shards, text | `canonical-latinLit` `license.md` | CC BY-SA 4.0 | Source content | admitted |

No route needs a Mouseion-run analyser: OLA and the Scaife shards are third
parties' published output, which the map admits.

## Recommendation

1. **Take the AGDT route on OLA as the first-slice annotation path.** It is
   the only route with whole classical Works at scale (205 BY-SA Works), a
   Citation layer and the scheme the shipped stack already stores. Its cost is
   one new adapter family and a new fetch kind, no migration, and a read-time
   suffix reader that the Greek goldens must not notice.
2. **Plan the Latin adapter as five pieces,** each testable on its own: the
   OLA dialect parser and Citation join; the Latin POS, feature and value
   tables; the `la` profile and reconstruct rules; a Latin citation type that
   admits letter suffixes and named sections; and the archive-member fetch kind
   pinned by offset, size, CRC-32 and SHA-256. The corpus grilling should decide
   whether to pin whole-archive MD5 plus per-member digests.
3. **Use CIRCSE as the yardstick, not a source.** The five Works exist in three
   forms, so a measurement of Latin Constructions can compare the AGDT adapter on
   OLA with a UD adapter on gold CIRCSE detection for the same text, and
   alignment is 96.7% to 98.5%. That would give a human-checked sample for
   ADR 0125's bar (precision at least 0.90, Wilson lower bound at least 0.84,
   120 hits) on five Works, one prose, without trusting OLA's parse. It needs a
   small UD adapter regardless, so the UD adapter's first use is measurement.
4. **Keep the Scaife shards as the contingency** if OLA's lineage is rejected:
   they fit the Greek fetch exactly (per-file, commit-pinned) and carry the
   Citation inline, but need the UD adapter, a sentence splitter, a chunk-edge
   policy and acceptance of a LAS of 37.3 on unseen gold, so they would only
   support lemmas, morphology and no Constructions in slice 1.
5. **Decide in the spec** whether a Latin run is `native_agdt` only, and keep
   the `ud` consumers off for Latin; the alternative (a UD run) turns on lexical
   sense ordering, generic relations display and the sense-context relation for
   Latin, which is a choice and not a default.

**What would change this:** the corpus grilling rejecting OLA's lineage (then the
shards or no-go); a measurement showing OLA constructions fail the bar on the
CIRCSE Works (then Latin ships vocabulary and cards only, as the generalisation
ticket's slice 1 already allows); or the 72 files with non-numeric Citations
turning out to hold the texts the learner wants (then the Citation type work
moves up); or a published OLA-to-UD converter.

## Limits

- I did not read the whole 3.86 GB archive, only the central directories, the
  whole CoNLL-U layer, the Citation members for 209 files and the full token
  layer for 12 Works. The `tok01`, postag and dependency PAULA layers for the
  other Works were not read, and the 16 NC-text and the 12 borderline Works
  were profiled for tags only.
- The Citation join was checked structurally (counts, structure, surface
  equality for 12 Works), not against the source TEI token by token.
- The Scaife profile is five Works, not all five shards. UD Perseus was read for
  sentence numbers only; its LDT parent was not downloaded. LDT, PROIEL and LASLA
  were not re-measured.
- Pronoun and determiner coverage, the proper-name rule and the `i`/`j` folding
  were counted, not evaluated for accuracy.
- Detector precision on Latin was not measured; ADR 0125's bar is untouched.
- OLA's held-out figures are the sibling ticket's and carry its caveat that gold
  exists only for Works the model saw.

## Reproduction

Scripts in [`latin-scheme-route/`](latin-scheme-route/); each takes the input
paths as arguments and writes counts only. `rangezip.py` is a seekable
range-request file for Python's `zipfile`; it retries on `429`.

| Script | Produces |
| --- | --- |
| `ola_index.py` | member sizes from the zip directories; fetch cost |
| `fetch_sample.py`, `fetch_cts.py` | the 12-Work sample, the 209 Citation members |
| `measure_conllu.py` | tag, relation, feature, ellipsis and lemma inventories |
| `measure_affected.py` | tokens and sentences hit by the Greek mapping |
| `measure_lemmas.py` | lemma spelling variation |
| `measure_citation.py`, `measure_join.py` | Citation shapes and the join checks |
| `measure_circse.py` | CIRCSE profile and OLA overlap |
| `measure_scaife.py` | shard TSV profile |
| `ola_pins.py` | `ola-bysa-classical-pins.tsv` and the two-member direct fetch check |

Inputs: the sibling ticket's `ola-conllu-manifest.tsv` (cited above) for the
Work classes and licence instruments; the CoNLL-U layer and Citation members read
from the Zenodo record.
