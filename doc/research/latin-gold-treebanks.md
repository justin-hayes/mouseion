# Hand-annotated classical Latin treebanks and lemmatised corpora

**Research date:** 2026-11-25

**Ticket:** [#2659](https://github.com/justin-hayes/mouseion/issues/2659), under the
map [#2658](https://github.com/justin-hayes/mouseion/issues/2658).

**Question:** Which classical Latin texts have gold, hand-annotated lemmas (and ideally
dependency syntax) in a machine-readable release, under what licence, and how much of
that is a usable Reading unit? Only the classical portion of each resource counts.

**Method.** Primary sources only: each release's own licence instrument and metadata,
the data files themselves, and the repositories' git history. Every figure below was
measured on the real pinned files with
[`latin-gold-treebanks/measure.py`](latin-gold-treebanks/measure.py) (it prints counts
only). No third-party text or annotation is committed. README claims were not used for
counts, with one stated exception (the LDT loci list, which is README prose). "Tokens"
are syntactic words. "Non-punctuation tokens" excludes punctuation nodes (UD `PUNCT`,
LDT postag `u`); resources whose files carry no punctuation tokens have equal figures.
No resource below needs an analyser run by Mouseion. Where a lemma layer began as
analyser output (LDT: Morpheus, EvaLatin 2020: UDPipe), it was published after human
correction, which is the OGA pattern.

## Answer

- **Whole Works with hand-checked lemmas, morphology and gold syntax under an
  ADR 0111-admissible licence: five, 29,055 tokens.** All of them are in
  UD_Latin-CIRCSE (CC BY-SA 4.0, `LICENSE.txt` and metadata agree): Seneca's
  *Hercules Furens*, *Agamemnon*, *Oedipus* and *Phoenissae* (verse) and Tacitus'
  *Germania* (prose, 5,674 tokens). The Seneca plays alone are 23,381 tokens. The same
  two texts (*Germania*, *Hercules Furens*) are EvaLatin 2024 gold (CC BY-SA 4.0). One
  caveat: CIRCSE's lemmas, tags and tokens were received from LASLA, whose own
  repository says CC BY-NC-SA 4.0 and "data-sharing agreements". The in-force
  instrument is clear; the relicensing chain is not verifiable from the release.
- **Everything bigger is NC or unclear.** The classical syntax-bearing resources
  are LDT (70,361 tokens), PROIEL (84,587) and UD Perseus (27,220), but PROIEL and UD
  Perseus are NC on both layers, and LDT is unclear on both layers (its own v1 README
  says NC-SA 2.5; the repository README says BY-SA 3.0 US). **No syntax-bearing
  resource has a whole, classical, prose Work under a clearly admissible licence except
  *Germania*.** The one whole prose Work in AGDT style is Sallust, *Bellum Catilinae*
  (LDT, 11,087 non-punctuation tokens), and its licence is unclear.
- **The only large gold lemma corpus is LASLA: 134 Works, 1,839,371 tokens, no syntax,
  CC BY-NC-SA 4.0 on text and annotation.** About 110 of those Works are whole and
  longer than 1,000 tokens (72 prose, 1,285,971 tokens; 38 verse, 536,064 tokens). Its
  text layer is excluded as Source content. Its annotation layer would be an NC
  Analysis resource that a deployer may fetch, but the text forms are inseparable from
  it in the same file.
- **Counted as Mouseion Reading units** (a Reading unit is a whole Work in one
  Edition, so partial coverage does not count): CIRCSE 5; LDT 2 (*Bellum Catilinae*,
  *Res Gestae*); UD Perseus 0; PROIEL 0 (every classical Work is partial); LASLA about
  110 (lemma and morphology only).
- **What would change this:** a CC BY-SA text layer aligned to LASLA (so the NC text is
  not needed) would turn LASLA into the only large route, at the price of syntax, of
  an alignment step and of its NC annotation layer; or an LDT licence clarification
  from Perseus/Leipzig. Neither was found in the sources.

## Candidates at a glance

Pins (what a spec would pin, and what was measured). Digests are SHA-256.

| Resource | Pin | Scheme | Gold or corrected |
|---|---|---|---|
| LDT 2.1 | `PerseusDL/treebank_data` commit `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` (master HEAD 2024-06-20), `v2.1/Latin/texts/` | AGDT-style XML, guidelines GSALT v1.3, 9-character `postag`. Res Gestae converted from the Harrington scheme; the original labels are kept in `@hrngtn` | Syntax manual. Lemma and morphology "semi-automatic", Morpheus-assisted, then corrected |
| UD Perseus | `UD_Latin-Perseus` tag `r2.19` = `a296f012949ef766ffdc5898ded59ffe0a7c3c6a` | UD v2, automatic conversion of a selection of LDT 2.1 (Celano) | As LDT, then converted |
| PROIEL (XML) | `syntacticus/syntacticus-treebank-data` tag `20230428` = `525cee4fb40590d7d514376c11acaed1bdd91c15`, `proiel/*.xml` (the `proiel/proiel-treebank` repository is frozen at `8e388967`, pointing here) | PROIEL scheme, schema 2.1; `reviewed` sentences only counted | Manual; `reviewed` status is the second-annotator-checked state |
| UD PROIEL | `UD_Latin-PROIEL` tag `r2.19` = `5acbf8efa572ce37890768df5b72f1ad46194c44` (built from the 20180408 PROIEL release, so older than the XML pin) | UD v2, converted by Haug | Converted from manual |
| UD CIRCSE | `UD_Latin-CIRCSE` tag `r2.19` = `5fff3a77c4386006e7e0f0db8dd6fd5c0f4d7523` | UD v2. Relations manual and native; lemma, UPOS, XPOS, features received from LASLA and converted "with corrections" | Gold (manual) |
| LASLA | `CIRCSE/LASLA` commit `c2e2bb6500b5ef9c6f5f31df6774423e9bb9fddc` (HEAD 2024-10-15; the latest tag `v1.0.1` is `d54a3044`, older). Digest of `sha256sum conllup/* \| sha256sum`: `781f125c941a2be6c79ddd4ba74ddaa5efb07d640393e36515351fe6d2cfcd35` | CoNLL-U-Plus, UD UPOS/features, LASLA XPOS, LiLa lemma IDs. No syntax | "Semi-automatically lemmatized … full manual verification" (README) |
| EvaLatin 2022 | `CIRCSE/LT4HALA` commit `2ac2af22127c33fbf07cf1bf2ae2055415228f3e`, `2022/data_and_doc/`: `EvaLatin2022-training-new.zip` `fec39d88…9708`, `gold-test-EvaLatin2022.zip` | UD UPOS/features/lemma, no syntax | Converted LASLA |
| EvaLatin 2020 | same commit, `2020/data_and_doc/` | Same, no syntax | UDPipe output **manually revised**, not LASLA |
| EvaLatin 2024 | same commit, `2024/data_and_doc/EvaLatin_2024_Syntactic_Parsing_test_data_gold.zip` `64e550a0…8d4a` | UD v2 with syntax | Gold (same annotation as UD CIRCSE) |

File digests for the files that matter:

| File | SHA-256 |
|---|---|
| `la_circse-ud-train.conllu` | `56d92508d9d1a7d29f32ad4cdde79377b04f78a0392cfdee2235d99eee44ee6e` |
| `la_circse-ud-dev.conllu` | `b9d9b609841e630c86a582b1679a220b92edc7e4ed1b7d88d6e2fb132196e414` |
| `la_circse-ud-test.conllu` | `f492948f750925f597fd0cb8054e20bd6e628d75737cf0065259ca81f0f6a1cb` |
| `la_perseus-ud-train.conllu` | `49d2e3813fc34595fe8cf569f263df4396e4b21aba9429fcdb0eea3ae09e64fa` |
| `la_perseus-ud-test.conllu` | `e0e53fdcc8040a2a6d3b6e7b5dfa69fa35d355ce11cae0682b61e6848745d386` |
| `la_proiel-ud-{train,dev,test}.conllu` | `4bcb4705…f5cc`, `7a753106…7fa4`, `bd3d538c…05bc` |
| PROIEL XML `caes-gal` | `1c666ea866167fa9dd3e0302b69a839a8bce88b90ca68b5ad2cd0e63a0e189f3` |
| PROIEL XML `cic-att` | `9a60fe6f47514caaf2cd3b954813ef4ca92163a0a2bbdc3661dff05fca5c2339` |
| PROIEL XML `cic-off` | `51b0844969d714627814a47b3f5ea3a1494194f8fdc9fda4acfb611ba1ad34b5` |
| LDT `phi0631.phi001` (Sallust) | `07febef0f140f508a1978a94a79d3c82a672a8c2ef094c1e49c63f36ef715ea1` |
| LDT `phi1221.phi007` (Res Gestae) | `9130f3e018588eafa7295b717507104cc409d38d887b39d318aae85c460e1760` |
| LDT `phi0448.phi001` | `74b0fd8486ca71501a5f57c4363bfb321330bb5678923f6dbe08e48075f6dd75` |
| LDT `phi0474.phi013` | `55b12883b48e5c8489e8cd7ebc0f34fc1892642571121f1783bae5f1df43d24a` |
| LDT `phi0620.phi001` | `a61ffec0631c8dd46a2109c0a4e7e715d08dd16460dc17829d7b8e3e2a3c2b29` |
| LDT `phi0690.phi003` | `cb2a94852cd8320e8f4ac63cb941899a7381e581fc62138c52d9eb1bbbed24c0` |
| LDT `phi0959.phi006` | `3372ca14b792d4e888307b77eb6f4175b0dec4be56d0953d01ac627a8f6c32e1` |
| LDT `phi0972.phi001` | `edcaa56832a116904905c6fc5bb66dc8fbfd34b8597a38f8ac3ff0f82c7dfa46` |
| LDT `phi0975.phi001` | `5e2d99af8fd3a7fd56d7cd14c21cd23271247712785c29155cb6a69d43a4a929` |
| LDT `phi1348.abo012` | `e974a30eb87cac19ad4ccedc37f9784d11f663ebab25f6b62134ba703e667aaa` |
| LDT `phi1351.phi005` | `eb1cd0d7ea6bfb7a9184e8ac4eb6405d2c6a0e547fde06b4a194a835668696ab` |

## Licences, layer by layer (ADR 0111)

Terms are read from the instrument in the pinned release, not from a catalogue page.
"Source content" must be public domain, CC0, CC BY, CC BY-SA or GFDL. "Analysis
resource" may be NC, unclear or research-only if the deployer fetches it.

| Resource | Text layer: instrument and class | Annotation layer: instrument and class |
|---|---|---|
| **LDT 2.1** | No per-file licence in any of the 11 classical files (grep for licence terms: none) and none in `v2.1/Latin/README.md`. Repository `README.md`: "Contents of this repository are licensed under CC BY-SA 3.0 US". But `v1/latin/docs/README.txt` says LDT 1.5 is CC BY-NC-SA 2.5, and v2.1 "comprises the data available in the older version". **Unclear → Source content excluded** until resolved | Same instruments. **Unclear (NC lineage unresolved), flagged unverified.** Allowed as a deployer-fetched Analysis resource. Same conflict as `oga-src-agdt-lineage.md` found for Greek |
| **UD Perseus** | `LICENSE.txt` and metadata: CC BY-NC-SA 2.5. **NC → excluded** | Same. **NC Analysis resource**, deployer-fetched |
| **PROIEL XML** | Per-file `<electronic-text-license>`: "Free, non-commercial" for Caesar, Cicero *Off.*, Cicero *Att.*; text from the Perseus Digital Library. **NC → excluded.** (Palladius: "Probably in the Public Domain"; Aetheria: "Public domain"; both out of scope) | `<license>` CC BY-NC-SA 4.0 in every file. **NC Analysis resource** |
| **UD PROIEL** | `LICENSE.txt`: CC BY-NC-SA 4.0. README metadata says 3.0. The two disagree on the version, not on NC. **NC → excluded** | Same. **NC Analysis resource** |
| **UD CIRCSE** | `LICENSE.txt`: CC BY-SA 4.0; metadata agrees. **Admissible as Source content on the instrument. Flag: the text was received from LASLA** (`README.md`: "received with tokenisation, and annotation … from the Opera Latina corpus built by the LASLA laboratory"), and LASLA's own repository is NC-SA with "data-sharing agreements". The release does not show the right to relicense. **Unverified** | CC BY-SA 4.0. **Admissible; same unverified flag** on lemmas and morphology. The syntax is CIRCSE's own manual work |
| **LASLA** | `README.md`: all "these resources" CC BY-NC-SA 4.0; "texts are available through data-sharing agreements". The Latin forms are in the `conllup` files. **NC → excluded** | Same. **NC Analysis resource**, but not separable from the text forms |
| **EvaLatin 2020, 2022** | `LICENSE.txt` in each: CC BY-NC-SA 4.0. **NC → excluded** | Same. **NC Analysis resource** |
| **EvaLatin 2024** | `2024/data_and_doc/LICENSE.txt`: CC BY-SA 4.0. Same flag as CIRCSE, because it is the same text | CC BY-SA 4.0. Same flag |

The ADR 0111 consequence: of the whole set, only UD CIRCSE and EvaLatin 2024 have a text
layer that may ship. Everything else could, at most, be an NC or unclear Analysis
resource fetched at deploy time, and its text forms could not ship. Greek works because
`oga_src` is one BY-SA file with both layers; no Latin source has that property except
CIRCSE.

## Per-resource measurements

All counts are on the pinned files. "Classical" means the working definition
(authors through roughly the 2nd century AD).

### UD_Latin-CIRCSE `r2.19`

| Work | Genre | Sentences | Tokens | Whole Work? |
|---|---|---|---|---|
| Tacitus, *Germania* | prose | 299 | 5,674 | Yes (LASLA's copy has 5,671 tokens for the same text) |
| Seneca, *Hercules Furens* | verse (tragedy) | 555 | 7,714 | Yes |
| Seneca, *Agamemnon* | verse | 409 | 5,580 | Yes |
| Seneca, *Oedipus* | verse | 401 | 5,931 | Yes |
| Seneca, *Phoenissae* | verse | 308 | 4,156 | Yes (the play itself is unfinished) |
| **Total** | | **1,972** | **29,055** | train 14,574, dev 2,978, test 11,503 |

Lemma and head are present on every token. There are no punctuation tokens: the sentence
text is lowercase except names, with LASLA orthography (`u` for `v`, `I` for `J`). The
files carry `sent_id` only (`TacGerma-Q-01-N`, `Latin_SenecaYounger_HercF_poetry-N`)
and `speaker` comments. **They carry no chapter, section or line citation.** The
`sent_id` is identical to LASLA's `sent_id` (checked on *Germania*: first, 100th and last
sentence), and LASLA's `conllup` carries `CitationHierarchy=Capitulum_n,Paragraphus_n`
(prose) or `Versus_n` (verse). A Citation therefore has to be joined in from LASLA, an NC
file, or rebuilt by the pin. That join was not tested beyond the three ids.

### EvaLatin 2024 gold

`EvaLatin_2024_prose_gold.conllu` (299 sentences, 5,674 tokens, every head set) and
`EvaLatin_2024_poetry_gold.conllu` (555 sentences, 7,714 tokens, every head set). The
counts equal CIRCSE's *Germania* and *Hercules Furens*, so this is a second copy of the
same annotation, not more coverage.

### LDT 2.1

Classical total: 11 files, 70,361 tokens, 59,209 non-punctuation tokens. The Vulgate
file (`tlg0031.tlg027`, 9,309 tokens) is excluded as Jerome. Figures include
"artificial" (elided) nodes, which have no form and, in some files, no lemma, so the
lemma-missing counts below are mostly those nodes.

| Work (file) | Genre | Sentences | Tokens | Non-punct | Coverage |
|---|---|---|---|---|---|
| Sallust, *Bellum Catilinae* (`phi0631`) | prose | 699 | 13,177 | 11,087 | Whole Work (subdoc 1 to 61) |
| Augustus, *Res Gestae* (`phi1221`) | prose | 116 | 3,035 | 2,714 | Whole Work (converted from Harrington labels; no `subdoc`) |
| Cicero, *In Catilinam* (`phi0474`) | prose | 327 | 6,652 | 5,672 | Partial: 1.1 to 2.11 |
| Petronius, *Satyricon* (`phi0972`) | prose (prosimetrum) | 1,120 | 14,171 | 11,392 | Partial: 26 to 78 (the *Cena*) |
| Suetonius, *Divus Augustus* (`phi1348`) | prose | 347 | 8,313 | 7,298 | Partial (README: chapters 1 to 55) |
| Tacitus, *Historiae* (`phi1351`) | prose | 197 | 3,531 | 3,034 | Partial: 1.1 to 1.21 |
| Phaedrus, *Fabulae* (`phi0975`) | verse | 583 | 6,588 | 5,508 | Partial: books 1 to 3 |
| Propertius, *Elegiae* (`phi0620`) | verse | 364 | 5,297 | 4,450 | Partial: 1.1 to 1.22 |
| Vergil, *Aeneid* (`phi0690`) | verse | 177 | 2,839 | 2,326 | Partial: 6.1 to 6.295 by `subdoc` (README says 6.336) |
| Ovid, *Metamorphoses* (`phi0959`) | verse | 317 | 5,202 | 4,350 | Partial: 1.1 to 1.778 |
| Caesar, *Bellum Gallicum* (`phi0448`) | prose | 71 | 1,556 | 1,378 | Partial: eleven short passages of book 2 |

The coverage column rests on the repository README and each file's `subdoc` range. I
did not count the full Work to compute a percentage. Citation: each file's root has a
`cts` URN (`urn:cts:latinLit:phi….perseus-lat1.tb`), and sentences carry `@subdoc`
(book.line or chapter.section; 9 of 11 files). Word-level `@cite` URNs occur only in the
Caesar and Ovid files. *Res Gestae* and *Historiae* have no `subdoc` at all, and the
Suetonius file has one `subdoc` (`1.1-154.4`) for the whole text. Lemmas carry homograph
numbers (`sum1`).

The treebank tags `format` as `aldt`, `gclat`, `glat` or `harrington` per file. Nine
files, including Sallust, have `format="gclat"`.

### UD Perseus `r2.19`

Classical: 27,220 tokens (22,874 non-punctuation) in 11 documents, 2,119 of the 2,273
sentences; the Vulgate document is 2,003 tokens and is excluded. It is a **selection**
of LDT, not LDT: for example Sallust has 4,999 tokens against LDT's 13,177, and *Res
Gestae* 713 against 3,035. So UD Perseus holds no whole Work. Its `sent_id` is
`phi….xml@N` and it has no citation at all. Lemmas carry the LDT homograph number in the
`LId` MISC field. Every token has lemma and head. The README states 29,138 tokens; I
measured 29,223 in total.

### PROIEL, classical part only

| Work (XML file) | Genre | Reviewed sentences | Annotated tokens (XML) | UD r2.19 tokens | Coverage |
|---|---|---|---|---|---|
| Caesar, *Bellum Gallicum* (`caes-gal`) | prose | 1,448 | 27,397 | 27,386 | Books 1 to 4 and 6; book 5 has 1,054 tokens and book 7 has 88 (README: more than half of 5.8 to 5.58, book 7 and book 8 unannotated) |
| Cicero, *Epistulae ad Atticum* (`cic-att`) | prose | 3,925 | 45,580 | 45,308 | Books 1 to 8 with gaps (README lists the gaps) |
| Cicero, *De officiis* (`cic-off`) | prose | 619 | 11,610 | 11,375 | Book 1 only, with gaps |
| **Classical total** | | **5,992** | **84,587** | **84,069** | |

The XML counts exclude 3,593 empty (elided) nodes across these three files. PROIEL's
own coverage figures (UD_Latin-PROIEL, LT4HALA 2026, from word counts of the full
texts) are 53.4 % of the *Gallic War*, 33.3 % of *De officiis* and 36.3 % of the letters
to Atticus. That is a secondary source; I verified the tag-level gaps from the PROIEL
README and the book counts above. In the XML every sentence in these three files is
`reviewed`. Citation: `citation-part` on every token (`1.1.1` book.chapter.section;
letters book.letter.section; *Off.* book.section), and the XML keeps punctuation in
`presentation-before/after`. UD PROIEL has no `PUNCT` tokens and carries `Ref=` in MISC.

Left out as non-classical: the Vulgate (`latin-nt` 110,774 tokens XML; 109,517 in UD),
Palladius, *Opus agriculturae* (11,979; 4th to 5th century), and the *Peregrinatio
Aetheriae* (17,554 XML tokens, not in UD; 4th century). The PROIEL licence applies to
all of them.

### LASLA (lemma and morphology, no syntax)

134 Works in 242 `conllup` files, 1,839,371 tokens, every file without punctuation
tokens. No dependency columns. Lemmas are missing on 385 tokens in total (0.02 %; the largest
counts are *Epistulae Morales* 63, *Casina* 51, *De rerum natura* 44 and *Satyricon* 43). The `LILA:`
columns give a LiLa lemma ID (`lilaLemma:N`) per token, which is a stable lemma
identity key. Citation: `CitationHierarchy` in MISC (`Liber_n,Capitulum_n,Paragraphus_n`
for prose, `Versus_n` for verse) plus a `sent_id`.

Authors (tokens): Caesar 79,039; Cato 20,065; Catullus 13,129; Cicero 478,569; Curtius
72,523; Hirtius 6,632; Horace 45,007; Juvenal 25,559; Livy 13,248; Lucretius 50,188;
Ovid 150,756; Persius 4,654; Petronius 30,978; Plautus (8 plays) 61,560; Pliny
(*Epistulae*, *Panegyricus*, *Naturalis Historia*) 115,408; Propertius 25,798;
Pseudo-Caesar (3 *Bella*) 30,292; Sallust 36,475; Seneca 313,885 (26 files); Tacitus
(20 files) 165,251; Tibullus 12,648; Vergil 87,707. (Per-author sums are from the
script's output; the full table is `measure.py` output.)

In Mouseion terms (grouping files into one Work per author and title): 134 Works, of
which 20 are fragment collections or under 1,000 tokens, leaving 110: **72 prose Works
(1,285,971 tokens; 56 of them 5,000 tokens or more) and 38 verse Works (536,064 tokens)**.
Several are partial: Livy is *Ab Urbe Condita* 8 only (13,248 tokens), Pliny's
*Naturalis Historia* is two files, and Curtius has 8 files. I did not check each Work's
extent against its full text, so "whole" for LASLA means "one Work as the release
groups it".

### EvaLatin, classical part

| Release | Texts | Tokens | Lemma | Syntax | Licence |
|---|---|---|---|---|---|
| 2022 training-new | Caesar *BG* 45,788; *BC* books 1 to 2 17,762; Cicero *Cat.* 12,727, *Phil.* 53,390; Pliny *Epist.* 60,128; Seneca *Ben.* 45,765, *Clem.* 8,303, *Prov.* 4,116, *Vit. Beat.* 7,339; Tacitus *Agr.* 6,839, *Germ.* 5,671, *Hist.* 52,527 | 320,355 | gold (LASLA) | none | NC-SA 4.0 |
| 2022 gold test | Livy *AUC* 8 13,248 (classical); Ovid *Met.* 9 to 10 10,723 (verse); Pliny *NH* 37 11,363 | 35,334 | gold (LASLA) | none | NC-SA 4.0 |
| 2020 gold + training | Caesar *BC* 1 and 2, *BG*; Cicero *Cat.*, *Phil.*; Horace *Carm.*; Pliny; Seneca; Tacitus | 329,862 (classical) | UDPipe output manually revised | none | NC-SA 4.0 |
| 2024 gold parsing | Tacitus *Germ.* 5,674; Seneca *HF* 7,714 | 13,388 | gold (CIRCSE/LASLA) | gold UD | **BY-SA 4.0** |

The 2022 training token counts equal LASLA's per-Work counts (for example *BG* 45,788,
*Hist.* 52,527), so it is a subset of LASLA, not another resource. The 2022 test zip
(`EvaLatin2022-test.zip`) has no lemmas; the gold is in `gold-test-EvaLatin2022.zip`.
The excluded Sabellicus (15th century) and the Aquinas file in EvaLatin 2020 are
out of scope. The 2020 release totals 341,418 tokens in sixteen files (nine gold, seven training);
the figure above leaves out the Aquinas file (11,556).

## Overlap: the same Work in several resources

| Work | Resources, in which scheme |
|---|---|
| Caesar, *Bellum Gallicum* | LDT (AGDT, 1,556 tokens, book 2 only); PROIEL (own scheme and UD, 27.4k); LASLA (7 books, 45,788); EvaLatin 2020, 2022 (LASLA subset) |
| Caesar, *Bellum Civile* | LASLA (3 books, 33,251); EvaLatin 2020 (bk 1 gold, bk 2) and 2022 (bks 1 to 2) |
| Cicero, *In Catilinam* | LDT (6,652); UD Perseus (1,915, a subset of LDT); LASLA (4 speeches, 12,727); EvaLatin 2020, 2022 |
| Cicero, *De officiis* | PROIEL (book 1); LASLA (3 books, 34,857) |
| Cicero, *Ad Atticum* | PROIEL only |
| Sallust, *Bellum Catilinae* | LDT (13,177, whole); UD Perseus (4,999, subset); LASLA (10,781) |
| Vergil, *Aeneid* | LDT (6.1 to about 6.295/336); UD Perseus; LASLA (12 books, 67,143) |
| Ovid, *Metamorphoses* | LDT (1.1 to 1.778); UD Perseus; LASLA (3 files, 49,549); EvaLatin 2022 (bks 9 to 10) |
| Propertius | LDT (1.1 to 1.22); UD Perseus; LASLA (4 files) |
| Petronius, *Satyricon* | LDT (26 to 78); UD Perseus; LASLA (30,978, whole) |
| Phaedrus, *Fabulae* | LDT, UD Perseus only |
| Suetonius, *Divus Augustus*; Augustus, *Res Gestae* | LDT, UD Perseus only |
| Tacitus, *Historiae* | LDT (1.1 to 1.21); UD Perseus (867); LASLA (52,527); EvaLatin |
| Tacitus, *Germania* | CIRCSE (UD, gold syntax); EvaLatin 2024; LASLA; EvaLatin 2020, 2022 |
| Seneca tragedies (*HF*, *Ag.*, *Oed.*, *Phoen.*) | CIRCSE (UD, gold syntax); LASLA; *HF* also in EvaLatin 2024 |

LDT and UD Perseus are the same annotation in two schemes (UD is the automatic
conversion), so a Mouseion route would pick one, not both. PROIEL XML and UD PROIEL
are likewise one annotation in two schemes (the UD set predates the XML pin).
CIRCSE's lemma and morphology are LASLA's, with corrections; its syntax is not.

## Coverage in Mouseion terms

A Reading unit is a whole Work in one Edition (ADR 0128 amendment to ADR 0116), so a
partial Work cannot be a Reading unit.

| Route | Whole Works (Reading units) | Tokens | Genre | Syntax | Licence verdict (ADR 0111) |
|---|---|---|---|---|---|
| UD CIRCSE | 5 (Germania; 4 Seneca tragedies) | 29,055 | 1 prose, 4 verse | UD, manual | Text and annotation admissible; unverified LASLA lineage |
| LDT | 2 (Sallust, *Res Gestae*) | 16,212 | prose | AGDT, manual | Unclear on both layers |
| UD Perseus | 0 | 0 | | UD, converted | NC |
| PROIEL | 0 (Caesar 1 to 4, 6 and 5.1 to 5.7; *Off.* book 1; *Att.* partial) | 0 whole (84,587 partial) | prose | PROIEL/UD | NC (text and annotation) |
| LASLA | about 110 (72 prose, 38 verse) | 1,822,035 | both | none | NC (text and annotation) |

Where several resources cover the same Work the table counts the Work once per route.
Overall, with syntax required and a clean licence, the answer is 5 Reading units (one
prose). With the LDT lineage question settled in LDT's favour, it is 7 (three prose).
With lemmas only and the NC text layer replaced, it is about 110.

## Borderline authors and placements

| Author or text | Date | Placement and reason |
|---|---|---|
| Suetonius, *Divus Augustus* | c. 120 | Classical (2nd century). In LDT and UD Perseus only. |
| Petronius | Neronian | Classical. In LDT, UD Perseus, LASLA. |
| Phaedrus | Tiberian | Classical (1st century AD). LDT and UD Perseus only. |
| Juvenal | c. 100 to 130 | Classical, at the edge of the cut. LASLA only. |
| Pliny the Elder, *Naturalis Historia* | 77 | Classical. LASLA (two files) and EvaLatin 2022 test only. |
| Curtius Rufus | 1st century (uncertain) | Classical on a 1st century dating. LASLA only. |
| Pseudo-Caesar (*Bella Africanum, Alexandrinum, Hispanum*) | 1st century BC to AD | Classical, in the Caesarian corpus. LASLA only. |
| Plautus, Cato the Elder, Lucretius, Catullus | before Augustus | Included under the "through the 2nd century AD" definition, but flagged archaic or late Republican; Plautus is early Latin metre. LASLA only. |
| Jerome, Vulgate | 4th century | Out of scope everywhere it occurs (LDT, UD Perseus, PROIEL). |
| Palladius, *Opus agriculturae* | 4th to 5th century | Out. PROIEL only. |
| *Peregrinatio Aetheriae* | 4th century | Out. PROIEL XML only. |
| Sabellicus (15th century), Aquinas *Summa contra Gentiles* | | Out. EvaLatin 2022 and 2020. |
| Apuleius, Gellius, *Historia Augusta* | | **Not present** in any resource checked: not in LASLA's 134 Works, nor in LDT, PROIEL, CIRCSE or EvaLatin. |
| ITTB, LLCT, UDante | | Out of scope by the map; named only to rule out. |

## Other resources considered

Nothing else turned up that is hand-annotated, classical and machine-readable. The UD
Latin set is exactly ITTB, LLCT, Perseus, PROIEL, UDante and CIRCSE (UD site). Archimedes
Latinus and Sabellicus are not classical. The LiLa knowledge base republishes LASLA and
UD PROIEL as Linked Open Data, so it is a different encoding of these same annotations
and not a new source. Stanza `la`, LatinCy, CLTK, UDPipe and `latinpipe-evalatin24` are
analysers and are excluded by the map's hard constraint (the `latinpipe` model is also
CC BY-NC-SA 4.0). I did not contact any author or maintainer.

## What I could not settle

- **Whether CIRCSE may relicense LASLA-derived text and lemmas as BY-SA.** The release
  states BY-SA; LASLA's repository states NC-SA and data-sharing agreements.
- **Whether LDT 2.1 is NC.** The repository README and the v1 README disagree, and the
  v2.1 files carry no licence of their own.
- **Whether the CIRCSE-to-LASLA `sent_id` join is complete.** It was checked on three
  ids for *Germania*, not for the Seneca plays.
- **Whole-Work status of LASLA's Works** and the percentage coverage of each LDT
  passage. I used the release's own file grouping and the README loci.
- **The default branch of `UD_Latin-CIRCSE`** is newer than `r2.19` (HEAD
  `751d21140362d503de8d6108058d7bb70cd7ef8e` at research time). The README at `r2.19`
  already lists the five texts above; I did not diff HEAD against the tag.
- **No UD tag after `r2.19`** exists on `UD_Latin-Perseus` at research time, although
  the default branch moved on 2026-11-10.

## Sources

- <https://github.com/UniversalDependencies/UD_Latin-CIRCSE/tree/r2.19>, `UD_Latin-PROIEL` and `UD_Latin-Perseus` at `r2.19`: `LICENSE.txt`, `README.md`, `.conllu`.
- <https://github.com/PerseusDL/treebank_data/tree/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f>: `README.md`, `v1/latin/docs/README.txt`, `v2.1/Latin/README.md`, `v2.1/Latin/texts/*.xml`.
- <https://github.com/syntacticus/syntacticus-treebank-data/tree/20230428>: `README.md`, `proiel/*.xml` headers.
- <https://github.com/proiel/proiel-treebank> (frozen at `8e388967`; its README points to Syntacticus).
- <https://github.com/CIRCSE/LASLA/tree/c2e2bb6500b5ef9c6f5f31df6774423e9bb9fddc>: `README.md`, `conllup/`.
- <https://github.com/CIRCSE/LT4HALA/tree/2ac2af22127c33fbf07cf1bf2ae2055415228f3e>: `2020`, `2022`, `2024` `data_and_doc/LICENSE.txt` and data zips.
- <https://circse.github.io/LT4HALA/2022/EvaLatin.html> and the EvaLatin 2022 overview (Sprugnoli et al., LREC 2022): training texts, LASLA provenance, conversion.
- <https://universaldependencies.org/treebanks/la_circse/> and `la_perseus/`, `la_proiel/`: annotation provenance tables.
- <https://aclanthology.org/2026.lt4hala-1.36.pdf>: PROIEL coverage percentages (secondary, used once).
- Prior research: `doc/research/grc-agdt-ud-conversion.md` at commit `0e45833` (Latin routes table, now updated here with measured counts); `doc/research/oga-src-agdt-lineage.md` (the AGDT NC conflict).
