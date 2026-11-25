# An OGA equivalent for classical Latin: published whole-corpus parses

**Research date:** 2026-11-25

**Scope:** Whether a third party has published whole classical Latin Works,
lemmatised and ideally parsed by a model trained on hand-annotated trees, in a
form Mouseion could import without running an analyser (map: classical Latin
in Mouseion; ticket: OGA equivalent for classical Latin). The hand-annotated
gold resources are the sibling ticket's subject (Research: hand-annotated
classical Latin treebanks and lemmatised corpora) and are cited, not repeated.
Primary sources only: Zenodo records, GitHub repositories and tags, Hugging Face
model repositories, the licence files in each release, and the real data files.
No authors were contacted. The Greek template is
[`oga-src-agdt-lineage.md`](oga-src-agdt-lineage.md) and
[ADR 0099](../adr/0099-ancient-greek-imported-analyses.md); licences are
classified under [ADR 0111](../adr/0111-third-party-licence-posture.md).

"Classical" follows the map: authors through roughly the 2nd century AD. Words
are non-punctuation tokens unless stated. All figures below are reproducible
with the scripts in [`latin-oga-equivalent/`](latin-oga-equivalent/); only
counts, IDs and digests are committed.

## Answer

- **Yes: Opera Latina Adnotata (OLA) v0.2.0 is the Latin sibling of OGA.** Same
  author (Celano, Leipzig), same AGDT label set and root convention,
  CTS-keyed. It holds 736 Works and 17.9M tokens; **221 are
  classical Works (4.54M words), plus 8 borderline (0.34M)**. Of the classical
  Works, **205 (4.17M words) have a text layer under CC BY-SA 4.0**; 16 (0.37M
  words, including Tacitus *Annales*, Velleius, Seneca *Naturales quaestiones*
  and Varro *De lingua Latina*) have CC BY-NC-SA text and are excluded as Source
  content.
- **It is a published model's output, not an analyser run by Mouseion.** Lemma,
  POS, morphology and dependency trees are LatinPipe output from a model
  fine-tuned on an unpublished AGDT-scheme set named `la_giuseppe` (the README
  credits F. Riemenschneider for the model files). Reported test
  figures: lemma 91.38, UPOS 97.15, feats 93.30, UAS 84.10, LAS 77.53. The
  Greek OGA bound is UAS 82.60 / LAS 77.10, so Latin parse quality is
  comparable.
- **My recomputation on held-out gold:** on 3,968 gold tokens from LDT 2.1
  sentences the model never trained on, the released OLA output scores lemma
  90.1, UPOS 96.0, all-tags 89.5, UAS 88.4, LAS 81.6. The same Works scored
  over all sentences (including training sentences) give LAS 94.2, so the
  figure for an unseen Work is lower than any number measurable on LDT. AGDT
  gold exists only for Works the model has seen, so true out-of-sample syntax
  is unmeasured.
- **ADR 0111 classification:** the annotation layer is declared CC BY-SA 4.0,
  but its training data is unpublished and the part that can be identified
  (LDT) carries the same unresolved BY-SA 3.0 US / NC-SA 2.5 conflict as the
  Greek lineage. It is an **Analysis resource, restriction kind `unclear`,
  flagged unverified**, exactly like the Greek `oga_src` layer. The
  text layer is Source content only where it comes from `canonical-latinLit`,
  `Latin` or `priapeia` (CC BY-SA 4.0).
- **The pin is usable but awkward.** Zenodo 10.5281/zenodo.15183688 is one
  3.86 GB archive, MD5-verified. The parse is a CoNLL-U set inside it
  (`workspace/conllu.zip`, 215 MB, 736 files). The CoNLL-U carries **no
  Citation and no sentence text**; Citations are in a separate PAULA layer
  (`workspace/ola.zip`, 1.41 GB) and join by token id. There is no
  `oga_src`-style repackaging for Latin, so Mouseion would need its own fetch
  step.
- **Second-best route: Scaife/Perseus6 LatinCy shards.** Perseus published
  spaCy `la_core_web_trf` output for all of its Latin (204 classical Works,
  5.13M words) as TSV, in UD scheme, tagged one citation chunk at a time (a
  verse line for the *Aeneid*), with no licence file, no tag and no per-file
  digest beyond git. UAS is 56.5 on an unseen UD sample. It needs the reserved
  UD adapter, and it adds only one classical Work OLA lacks (Pliny the Elder).
- **Lemma-only fallback: `latin-lemmatized-texts` 0.1.2** (Pie-Extended
  LASLA model; TEI; CTS). No syntax. Its instrument and its text sources
  contradict each other (CC BY 4.0 file; CC BY-NC-SA text repositories inside).
- **Recommendation:** take OLA to the corpus grilling as the leading route for
  the annotation layer and BY-SA text, with a no-go on the 16 NC-text classical
  Works. Weigh it against gold-only coverage using the sibling ticket's figures.
  Recommend a measurement on EvaLatin 2024 gold or a human-checked sample before
  any construction work, as for Greek ([Research: OGA parser accuracy and prior art for Greek construction detection](https://github.com/justin-hayes/mouseion/issues/2600)).

## Candidates

| Candidate | Output | Scheme / format | Syntax | Classical Works / words | Text licence | Annotation licence | Verdict |
| --- | --- | --- | --- | ---: | --- | --- | --- |
| **OLA v0.2.0** (Celano) | LatinPipe fine-tuned on unpublished AGDT data | AGDT; CoNLL-U + PAULA/ANNIS | yes | 221 / 4.54M (+8 borderline) | BY-SA for 205 Works; NC-SA for 16 | declared CC BY-SA 4.0; lineage unverified | **leading route** |
| Scaife `tagging-shard-01…05` (Perseus) | LatinCy `la_core_web_trf` 3.7.7 | UD; per-chunk TSV | yes (per chunk) | 204 / 5.13M (+4 borderline) | BY-SA (`canonical-latinLit`) | none stated (shard repos have no licence) | fallback, UD adapter |
| `latin-lemmatized-texts` 0.1.2 (Clérice) | Pie-Extended LASLA+ | LASLA tags; TEI + CTS | no | 222 / 5.93M by its word counts (+8) | mixed, inc. NC-SA | repo `LICENSE.md` CC BY 4.0; README CC BY-SA 4.0 | lemma-only |
| LatinISE v6 (McGillivray) | Haug analyser + TreeTagger | vertical text | no | not CTS-keyed; not measured | mixed (IntraText, LacusCurtius, Musisque Deoque) | CC BY-SA 4.0 (LINDAT) | not a fit |
| GreLa v0.7 / Corpus Corporum | TreeTagger lemmas | DuckDB; TEI at source | no | not measured | mixed | mixed | not a fit |
| LiLa/LASLA, UD Latin-Perseus/PROIEL/CIRCSE, EvaLatin | gold | UD / LASLA | partial | see sibling ticket | see sibling ticket | see sibling ticket | gold, not model output |

Ruled out as sources: Morpheus (Perseus) and Lemlat are analysers that return
ambiguous analyses on request; LatinCy, CLTK, Stanza and UDPipe are tools. A
third party's *published output* of such a tool counts, and the Scaife shards
are exactly that. UD Latin-Perseus, LatinPipe's own LINDAT model
(`11234/1-5671`, CC BY-NC-SA 4.0) and the EvaLatin shared-task data are
gold or models, not whole-corpus parses.

## Evidence

### 1. OLA v0.2.0

**Identity and pin**

- Zenodo record 15183688, DOI `10.5281/zenodo.15183688` (concept
  `10.5281/zenodo.15183687`), created 2025-04-09; one version only. File
  `opera_latina_adnotata_v0.2.0.zip`, 3,857,914,226 bytes, MD5
  `543305da911c91b9ade5cfa3e82d659f` (matches Zenodo; I downloaded the
  file). SHA-256
  `df0327a5d1b51a6960cad5707986d3d652f5247ad2ebde536bddb85f567a0f3e`.
- GitHub `OperaLatinaAdnotata/OLA`, tag `v0.2.0` = commit
  `5cd6b0655311a84cf7b4376016c7735fa72ef309` (released 2025-04-10); `master` is
  one commit later (`0bdcb7cf`, 2025-04-10). Last push 2025-04-10; no issue
  open; no later release, 7 months before this research date. The repository
  holds documentation only (README, query guides, `urn_cts`, tokenisation
  helpers), not the data.
- Zip layout (47 entries, read from its central directory): `README.md`,
  `annotation_example/`, `query/`, `urn_cts/`, `abbreviations/`, `tokenize/`, and
  five archives, all stored uncompressed: `workspace/conllu.zip` (214,687,770
  bytes, 736 CoNLL-U files, SHA-256
  `c7904bd77b696dc4d0426222d5286cb382441a665684c08fc3160ff5dd6cbfa4`),
  `workspace/ola.zip` (1,410,289,884; the PAULA layers), `workspace/merge.zip`,
  `graphannis.zip` (1.86 GB, the ANNIS database), and `original_Latin_files.zip`
  (301 MB, the base TEI). Stored entries mean one archive can be fetched with
  an HTTP range request; I fetched `conllu.zip` that way.
- Per-Work digests: [`ola-conllu-manifest.tsv`](latin-oga-equivalent/ola-conllu-manifest.tsv)
  (736 rows: URN, class, text source repository and its licence instrument,
  SHA-256 of the CoNLL-U file, tokens, words, sentences, citation depth).
  Manifest SHA-256
  `05c76d7cd0d7d3152157d6a307b5777b05c95b10a5f8320c71bc150420d42ec0`.

**Model and training data**

- The README says the annotation "was performed using Latinpipe", the model on
  Hugging Face as `bowphs/latinpipe-evalatin` (thanks to F. Riemenschneider).
  Repository commit `0e1ef4f9c61480a3c64c0fb757f67d428abd79b9`
  (2025-02-19); `model.weights.h5` is 10,420,897,228 bytes, LFS SHA-256
  `09caa2573769253da0b91fe08c961d84e077ac02efcfcad066dd1bb2a9e9e05e`.
  The repository has **no model card and no licence**. It is not the
  published LatinPipe model (`latinpipe-evalatin24-240520`, LINDAT
  `11234/1-5671`, CC BY-NC-SA 4.0, UD-trained).
- `options.json` shows `train: la_giuseppe/train.conllu`, `dev`/`test` likewise,
  encoders `bowphs/LaBerta`, `bowphs/PhilBerta`, `FacebookAI/xlm-roberta-large`.
  **`la_giuseppe` is not published and not described anywhere I found.** Its
  held-out sentences, carried in the repository's `conllus/dev.120.conllu` and
  `test.120.conllu`, are AGDT-labelled (`SBJ`, `AuxC`, `ATR`…) and come from the
  LDT 2.1 Works (Augustus, Caesar, Cicero *In Catilinam*, Propertius, Sallust, Virgil,
  Ovid, Petronius, Phaedrus, Suetonius, Tacitus, Jerome) **and from Works not in
  any published treebank**: Livy (`phi0914.phi001`), several digilibLT late
  texts, and the Vulgate (`tlg0031.tlg002.zefania`). So the model was trained
  on more gold than anyone has released, and which classical Works it has seen
  is unknown beyond the LDT set.
- Reported test scores (README; also the repository's
  `conllus/test.120.conllu.eval`): UPOS 97.15, UFeats 93.30, AllTags 91.53,
  Lemmas 91.38, UAS 84.10, LAS 77.53. The test set is 637 sentences, 14,127
  lines, split by sentence from texts that are also in training.

**Scheme and format**

- Dependency syntax "following the AGDT annotation scheme" (`query/syntax.md`
  cites the AGDT guidelines and lists the labels). Sampled CoNLL-U shows
  AGDT labels and one-letter AGDT parts of speech, with morphology as
  `Case=n|Gender=f|Number=s` letter values (no UD features). Every one of the
  355,703 sentences in the 225 classical files has exactly one root; punctuation
  attaches to the first node under the root, as in OGA (ADR 0122's normal
  form). 0.31% of classical tokens (17,527 of 5.71M) carry a label outside the
  AGDT set, the Greek-letter lookalikes and typos that `query/syntax.md` lists;
  ADR 0122 already treats the Greek-zeta `AuxZ` case.
- CoNLL-U columns 1–8 are as above; column 10 (MISC) holds only the PAULA token
  id (`t_N`). The file has no `# sent_id`, `# text` or Citation. The CoNLL-U
  files hold 17,919,142 tokens against the README's 17,919,596. In 13
  classical Works the CoNLL-U has fewer tokens than the PAULA layer, by 1 to 30
  (worst: Cicero *Timaeus*, 30 of 5,160, 0.58%); every CoNLL-U token id exists
  in the PAULA layer, so the join works.

**Citation scheme**

- The PAULA `cts01` layer gives each token a value such as `1_2_3` (levels
  joined by `_`). Modal depth per classical file: 1 level for 106 files, 2 for
  81, 3 for 34, 4 for 4. 0.20% of classical tokens have an empty value. The
  README says the CTS fields "were retrieved automatically from the original
  texts (and therefore they may contain errors and inconsistencies)".
- File names carry the CTS URN and Edition (`phi0448.phi001.perseus-lat2`).
  The Works are keyed by Perseus/Stoa identifiers; `urn_cts/texts/urn_cts.xml`
  lists author, title and printed edition for all 736.

**Coverage**

| Class | Works | Words | Of which BY-SA text | NC-SA text |
| --- | ---: | ---: | ---: | ---: |
| Classical | 221 | 4,537,171 | 205 Works / 4.17M | 16 Works / 0.37M |
| Borderline | 8 | 335,082 | 4 / 0.21M | 4 / 0.13M |
| Out (late, medieval, commentary) | 474 | 9,229,547 | | |

(Works are counted once, at their longest Edition; three classical Works and
29 others exist in more than one Edition. The licence columns count files.)
The 205 BY-SA classical Works split into 141 of at least 5,000 words and 64
shorter. Across all 205 BY-SA Works prose is 3.04M words and verse 1.12M. Reading-unit size, per ADR 0128 the whole Work in one Edition, for
the 221 classical Works: under 5k words 71; 5–20k 94; 20–60k 37; 60–200k 18;
over 200k 1 (Livy, one 518k-word Work). Of the 141 plausible BY-SA units
(≥ 5k words), 80 are prose and 61 are verse; the verse set is Plautus, Terence, Lucretius, Catullus,
Virgil, Horace, Ovid, the elegists, the epic poets, Martial, Juvenal, Persius,
Phaedrus and the Seneca tragedies. By author, the largest are Cicero (60
Works, 1.17M words), Livy (0.52M), Celsus (0.32M), Seneca (0.37M with the
*Dialogi*), Quintilian (0.24M), Ovid (0.23M), Plautus (0.17M), Tacitus (0.16M).
Per-Work counts: `latin-oga-equivalent/results/ola_inventory.json`.

The 16 classical Works with NC-SA text (repository `additional-texts`, which
also packages Thayer, Remacle, Latin Library and Corpus Corporum editions) are
Tacitus *Annales* (91k words), pseudo-Quintilian *Declamationes maiores*
(68k), Seneca *Naturales quaestiones* (49k), Varro *De lingua Latina* (40k),
*Rhetorica ad Herennium* (31k), Velleius Paterculus (27k), Scribonius Largus
(25k), Pomponius Mela (17k), Suetonius's three minor lives (8k), *Calpurnius
Siculus* (5k), the two Sallust pseudepigrapha (5k), *Laus Pisonis* (2k) and an
*Appendix Vergiliana* piece (1k). Gaius *Institutiones* and Hyginus *Fabulae*
are the two NC-SA Works on the borderline side.

**Text quality hazard**

OLA's text is the source TEI flattened to plain text, and apparatus markup can
leak in. The release's own `annotation_example` (Cicero *Pro Tullio*) shows
split words and noise lemmas: its text begins "A ntea sic hanc causam a gere s
tat ue ra m", where the source TEI wraps restorations in `<add>`. This is
Work-specific. Of the 233 classical and borderline editions in the release's
`original_Latin_files.zip`, 104 contain at least one `<add>` element, 23 at
least 50 and 6 at least 200 (Celsus 1,713; Seneca the Elder 645; Valerius
Maximus 400; Varro 349). I did not measure how many are mangled. Single-letter
non-abbreviation tokens, a crude symptom, stay under 1.4% in every classical
Work above 2,000 words, so this is not corpus-wide.

**Agreement with gold (interim)**

Method: align OLA's classical CoNLL-U to gold by normalised surface forms (case,
diacritics, u/v and i/j folded); punctuation excluded; heads and labels scored
only where the gold head also aligns; lemmas compared after stripping sense
digits and case. Scripts: `measure_ola.py`, `measure_proiel.py`.

| Comparison | Tokens | Lemma | UPOS | All tags | UAS | LAS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| LDT 2.1, **held-out sentences** (test list of the OLA model, 11 Works) | 3,968 | 90.1 (89.2–91.0) | 96.0 | 89.5 | 88.4 (87.3–89.4) | **81.6 (80.3–82.8)** |
| LDT 2.1, the model's dev sentences | 4,039 | 90.7 | 95.8 | 89.4 | 86.5 | 79.5 |
| LDT 2.1, all aligned tokens (mostly training sentences) | 54,049 | 95.5 | 99.1 | 97.7 | 96.0 | 94.2 |
| UD Latin-PROIEL, Caesar *BG*, Cicero *Att.*, *Off.* (UD scheme; lemma and POS only) | 72,062 | 88.6 | 94.3* | n/a | n/a | n/a |

Intervals are 95% Wilson. \* UPOS compared coarsely, with determiner/pronoun
lenient; PROIEL lemma conventions differ (`alioqui(n)`), so the 88.6 is an
agreement figure, not an accuracy. Sallust appears in two Editions, counted
once. Per feature on the held-out set: case 96.1, gender 96.1, number 98.9,
person 99.2, mood 95.9, tense 94.7, voice 92.0. The three PROIEL Works are not
in the published treebanks the model used for the checkable part of its
training, but **I cannot rule out that `la_giuseppe` contains them**, and UD and
AGDT attach differently so I did not score syntax against PROIEL.

For comparison, Greek OGA's local agreement on its (fully training) gold texts
was LAS 0.810 / UAS 0.858 (Research: OGA parser accuracy and prior art for
Greek construction detection). OLA's held-out LAS of 0.816 and the paper-scale
0.775 sit in the same band. Attachment is the weakest layer, as for Greek.

**Maintenance.** One maintainer, one release, no change in 7 months. Licence
and lineage questions go to one person, as for OGA. The ANNIS demo
(`annis.varro.informatik.uni-leipzig.de/ola020`) is the author's host; I did
not test its availability.

### 2. Scaife/Perseus6 tagging shards (LatinCy)

- `scaife-viewer/tagging-pipeline` (MIT; README and `tag.py`): extracts
  passages from CapiTainS repos, tags them with spaCy, and stores TSV. Latin is
  shards 01–05 of 23 (`tagging-shard-NN`); the sharding table and `repos.tsv`
  are in the pipeline repo. Source repos for Latin: `canonical-latinLit` and
  `csel-dev`.
- Model: `la_core_web_trf`, wheel renamed `3.7.7` in the pipeline README. The
  current LatinCy `la_core_web_trf` 3.9.9 card (MIT) reports UPOS 97.38, lemma
  97.39, UAS 87.56, LAS 82.69 on a UD test set; the 3.7.7 weights that tagged
  the shards were not scored in anything I found. The current LatinCy card lists the six UD
  Latin treebanks and LASLA among its sources; the UD treebanks are CC BY-NC-SA
  or BY-SA, and LASLA is CC BY-NC-SA 4.0 (`CIRCSE/LASLA` README).
- Release: shard repositories have no tags, releases or licence file (GitHub
  reports no licence); they are single-branch checkouts last pushed 2024-11-12.
  Commits read: shard-03 `21dd17ddae07c7be7ebdacf88f392361359eaa97`, shard-04
  `6eab631a6368f7cee4652561da5c19a232fa39f9`, shard-05
  `57078f8114308162cc2eb7fc065bdd0b31535763`. Perseus's own write-up
  (Crane et al. 2025, *The Sixth Generation of the Perseus Digital Library and a Workflow for Open Philology*)
  describes this layer as a default, automatically generated treebank "stored in
  the CoNLL-U format"; what is actually on GitHub is a nine-column TSV, not
  CoNLL-U (`ref, index, form, UPOS, XPOS, morph, lemma, deprel, head`).
- Format and Citation: one `.tagged.tsv` per Work and Edition, each row keyed
  by the lowest citation reference. Chunk lengths are the citation units: the
  *Aeneid* median is 8 tokens (one line), Caesar *BG* 26, Cicero *In Catilinam*
  160. The pipeline README concedes that "lowest-level citeable chunks are
  passed into spaCy individually which is often not desirable (at least for the
  dependency annotation)". Heads therefore stop at the chunk edge. 12,801 of
  7,003,363 rows (0.18%) in shards 03–04 are blank-form tokens.
- Coverage: shards 01–05 hold 204 classical Works (5.13M words, three in more
  than one Edition) and 4 borderline, plus 127 out-of-scope. 203 of the 204 are
  also in OLA; OLA alone adds 18 classical Works (mostly the NC-SA texts);
  Scaife alone adds one, Pliny the Elder's *Naturalis historia* (about 1.2M
  tokens, a quarter of Scaife's classical total).
- Agreement with UD Latin-Perseus gold, **test split (unseen by LatinCy)**:
  4,175 tokens in three Works (*Aeneid*, *Metamorphoses*, Phaedrus): lemma
  89.6, UPOS 85.8, UAS 56.5, LAS 37.3. The train split, which LatinCy saw
  (12,063 tokens): lemma 91.6, UPOS 97.1, UAS 85.4, LAS 73.5. All three test
  Works are verse, tagged a line at a time, and UD Latin-Perseus is
  unharmonised while LatinCy trained on harmonised UD (Gamba and Zeman 2023
  report gains of up to 9 points from harmonisation), so the syntax gap
  overstates the model's deficit. Against UD Latin-PROIEL (seen) lemma agreement is 91.7–92.9 and
  coarse POS 94.3–96.5.
- ADR 0111: text layer CC BY-SA 4.0 (`canonical-latinLit`; admitted); annotation
  layer: no instrument at all in the shard repositories, over a model licensed
  MIT whose training data includes NC-SA sets. Unlicensed output counts as
  unclear for an Analysis resource; the resource would be fetched at deploy
  time.

### 3. `latin-lemmatized-texts` (Pie-Extended LASLA)

- Repository `lascivaroma/latin-lemmatized-texts`, release 0.1.2 (2021-05-01,
  commit `0fccf668a1021470ee5a2662e6a7f1122b66607a`; Zenodo 4337145 version
  0.1.2, `lascivaroma/latin-lemmatized-texts-0.1.2.zip`, 161,874,590 bytes, MD5
  `c4f140bd40818d3b786445a5896aacc9`, Zenodo licence `other-open`). `main` has
  moved on (README names 0.1.3, last pushed 2025-03-16) with no release.
- Content: 853 CTS-keyed TEI files (`corpus.csv`), 21.2M tokens, TEI `<w>` with
  `lemma`, `pos` (LASLA tags) and `msd`; `ab` elements carry the CTS passage
  reference. Enclitics are not split (a `{…}` duplicate token carries `que`).
  Classical: 222 Works, 5.93M by the `corpus.csv` word counts (+8 borderline).
- Model: Pie-Extended LASLA+ 0.0.6 (`PonteIneptique/latin-lasla-models`,
  MPL-2.0 file), trained on LASLA (about 1.5M tokens) plus a late-Latin corpus
  and the aligned PROIEL Vulgate. LASLA data is CC BY-NC-SA 4.0
  (`CIRCSE/LASLA` README); the LiLa catalogue lists it as CC BY-SA, which
  disagrees (sibling ticket).
- Lemma accuracy on its own test set per the model README: 0.973; I did not
  verify it. Agreement with gold (seen, since LASLA contains these Works):
  vs LDT 2.1 lemma 89.7–92.8 and coarse POS 92.0–95.7 (ten Works); vs
  PROIEL 93.0–95.4 and 92.8–97.1.
- Licence: `LICENSE.md` is CC BY 4.0, the README says text "same licence as the
  original" and annotations "CC-BY-SA 4.0", and the 6 source repositories
  include `digiliblt` and `additional-texts` whose `LICENSE.md` is CC BY-NC-SA
  4.0. ADR 0111 reads the instrument, so the corpus-level terms are
  contradictory: unclear, with NC-SA Source content for some Works.
- Verdict: a lemma and morphology source with CTS URNs, no syntax, trained on NC
  data; useful as a cross-check or lemma fallback, not as the primary route.

### 4. Not a fit

- **LatinISE v6** (LINDAT `11372/LRT-6140`, 2026-04-29, `latin14.txt` 216 MB,
  MD5 `15aba54f63333f86a580ec5b7a0de724`, CC BY-SA 4.0 on the record; v4 was
  CC BY-NC-SA). 13.4M tokens, vertical format with author, title, century and
  line metadata but no CTS URN; lemmas from Haug's analyser and TreeTagger
  (trained on ITTB, LDT and PROIEL). Its texts come from LacusCurtius, IntraText
  and Musisque Deoque; I did not check per-text terms. No syntax. I did not
  download it.
- **GreLa v0.7** (Zenodo 18160596, 2026-01-06; 437M tokens) merges Corpus
  Corporum Latin, whose lemmas come from TreeTagger with Brandolini's data
  (Corpus Corporum site; the Umanistica Digitale overview, 2026). A
  DuckDB over 8 GB, mixed text licences, no syntax, not CTS-keyed, no gold-trained model.
- **Morpheus-based lemmas** (Perseus, Lemlat, LiLa Lemma Bank): analysers or
  lexica; a Mouseion that ran them would be running an analyser.

## Borderline authors

| Author or Work | Placement | Why |
| --- | --- | --- |
| Suetonius (b. c. 69; *Caesares* c. 121) | classical | Within the 2nd century, standard Latin reading. |
| Florus | classical | *Epitome* under Hadrian; 27k words. |
| Pliny the Elder | classical | 1st century; in Scaife, not in OLA. |
| Apuleius | borderline | *Apologia*, *Florida*, *Metamorphoses* written c. 158–170 AD; 85k words. |
| Aulus Gellius | borderline | *Noctes Atticae* c. 170–180; 120k words. |
| Gaius, *Institutiones* | borderline | c. 161; legal, NC-SA text; 88k. |
| Hyginus, *Fabulae* | borderline | Attribution and date uncertain; NC-SA text; 28k. |
| Calpurnius Flaccus; Lucius Ampelius | borderline | 2nd century or later; tiny. |
| *Historia Augusta* | out | 4th century, 30 Works, 109k words. |
| Servius, Porphyrio, Apicius, Ausonius | out | Late antique. |
| Tertullian, Minucius Felix, Jerome, Augustine | out | Christian and late; Tertullian c. 200 also sits near the line. |
| Pseudepigrapha (Sallust, Vergil, Quintilian) | classical in class, flagged | Counted with the attributed author. |

The sibling ticket records its own placements for gold authors.

## Overlap with gold

OLA's LDT Works are Caesar *BG*, Cicero *In Catilinam*, Propertius, Sallust
*Catilina*, Virgil *Aeneid*, Ovid *Metamorphoses*, Petronius, Phaedrus,
Suetonius *Augustus* and Tacitus *Historiae*; its release holds each in AGDT
scheme, so a gold LDT tree and an OLA tree exist for the same passages
(Augustus *Res Gestae* is not in OLA). UD Latin-PROIEL gold overlaps *BG*, Cicero
*Atticus* and *De officiis*. For LASLA, CIRCSE and EvaLatin overlaps see
[Research: hand-annotated classical Latin treebanks and lemmatised corpora](https://github.com/justin-hayes/mouseion/issues/2659);
I did not duplicate them. LDT 2.1 licence statements repeat the Greek
conflict: the repository README says CC BY-SA 3.0 US, `v1/latin/docs/README.txt`
says LDT 1.5 is CC BY-NC-SA 2.5, and UD Latin-Perseus's `LICENSE.txt` is CC BY-NC-SA 2.5.

## ADR 0111 classification by layer

| Layer | Instrument in force | Terms | Kind | Restriction |
| --- | --- | --- | --- | --- |
| OLA parse, lemmas, morphology | OLA README and Zenodo record. No licence file in the zip, the tag or `master`. | CC BY-SA 4.0 declared | Analysis resource | `unclear`, **unverified** (training data unpublished) |
| OLA model weights (not run by Mouseion) | none: no card or licence on `bowphs/latinpipe-evalatin` | unlicensed | not applicable | n/a |
| OLA text from `canonical-latinLit` (0.0.13267625203 in the zip) | `license.md` | CC BY-SA 4.0 (in the current `master`, 170 of 387 edition headers state it, none contradicts, 217 are silent) | Source content | admitted |
| OLA text from `Latin` 1.14.0, `priapeia` 1.1.18 | `license.md`, `LICENSE.MD` | CC BY-SA 4.0 | Source content | admitted |
| OLA text from `additional-texts` 1.0.195, `digiliblt` 0.0.67 | `LICENSE.md` | CC BY-NC-SA 4.0 | Source content | **excluded** (NC) |
| OLA text from `csel-dev` | no instrument | none | Source content | excluded; late texts only |
| Scaife shards: text | `canonical-latinLit` `license.md` | CC BY-SA 4.0 | Source content | admitted |
| Scaife shards: annotations | none in repository | unlicensed; MIT model over NC-SA training data | Analysis resource | `unclear`, unverified |
| `latin-lemmatized-texts`: text | repository `LICENSE.md` CC BY 4.0, README "same as original"; NC-SA repositories included | mixed | Source content | unclear for NC-SA Works |
| `latin-lemmatized-texts`: annotations | README CC BY-SA 4.0; `LICENSE.md` CC BY 4.0 | contradictory; LASLA NC-SA lineage | Analysis resource | `unclear`, unverified |

The text layer in the OLA CoNLL-U is the OLA token stream, not a copy of the
TEI; whether its flattened form is a faithful reproduction of a BY-SA edition
is a quality question (see the text hazard above), not one I settled.
Share-alike Source content carries the attribution notices ADR 0111 names.

## Fit to the existing import path

- **Scheme.** AGDT, so the native AGDT path of
  [ADR 0122](../adr/0122-ancient-greek-native-agdt-syntax.md) applies. The
  reserved Latin UD adapter would be needed for the Scaife shards only.
- **Delivery.** Neither route has an `oga_src`: no single-file package per Work
  and no GitHub tag at file level. OLA's CoNLL-U (215 MB) plus the Citation
  layer (1.41 GB) can be pulled from the Zenodo zip by range request (the
  entries are stored uncompressed); the whole zip is 3.86 GB. Pinning by the
  Zenodo MD5 and the per-file digests above is possible.
- **Citation.** OLA's Citation depth varies (1 to 4 levels) and is
  auto-extracted; the Perseus TEI in `canonical-latinLit` is the ground truth to
  validate it against.
- **Verse.** 61 of the 141 plausible BY-SA Works are verse; poetry presentation
  is out of scope for the map, but a prose-only first slice still has 80 Works
  and 3.04M words.
- **Constructions.** Not measured. The ADR 0125 shipping bar (precision 0.90,
  Wilson lower bound 0.84) applies per construction. OLA's held-out LAS of
  0.816 (reported 0.775) is in the same band as Greek OGA's production LAS of
  0.771, which needed per-construction checks before any promise; a Latin
  construction measurement would have to precede one here too.

## Unverifiable or open claims

1. **Training data of the OLA model.** `la_giuseppe` is unpublished, so the
   licence lineage, which Works were seen, and the true out-of-sample accuracy
   are all unknown. The held-out figures use the model's own dev and test
   lists, which carry only sentences from Works that are also in training.
2. **Which AGDT licence governs the LDT part of the lineage.** BY-SA 3.0 US at
   the repository top level against NC-SA 2.5 in the Latin docs and in UD
   Latin-Perseus. Same unresolved conflict as the Greek lineage note.
3. **The OLA zip carries no licence file.** The README and Zenodo metadata are
   the only statements; the README adds "look also at the repositories of the
   base texts". The `canonical-latinLit` snapshot in the zip is
   `0.0.13267625203` while the README links tag `0.0.12144726697`; I did not
   find which one was used for the parse.
4. **Text fidelity.** Fragmented words from apparatus markup are confirmed in
   one Work and bounded by `<add>` counts, not measured corpus-wide.
5. **Scaife shard model version.** `3.7.7` comes from the pipeline README's
   notes on getting `uv` working; the shards carry no model stamp.
6. **LatinISE and GreLa** were not downloaded; their rows rest on record
   pages and project descriptions.
7. **LASLA licence.** CC BY-NC-SA 4.0 (`CIRCSE/LASLA` README) against CC BY-SA
   (LOD Cloud record); not resolved here.
8. **No gold for Works the model has not seen.** EvaLatin 2024 gold and a
   human-checked sample are the two ways to measure it; neither is done.

## Reproduction

All inputs were downloaded into an empty directory and read with `python3 -I`.
Pins used: OLA Zenodo 15183688 (MD5 above); gold LDT from
`PerseusDL/treebank_data` `master` `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`,
`v2.1/Latin/texts`; UD Latin-PROIEL `56e6e8c8f6b1c6388dba17c807a9da19f6141b23`
(train, dev, test); UD Latin-Perseus `63aba6ffd132ea5c0f453cad901bbd4547e1e70a`
(test SHA-256 `e0e53fdc…`, train `49d2e381…`); the held-out lists
`conllus/dev.120.conllu`, `test.120.conllu` from
`bowphs/latinpipe-evalatin` `0e1ef4f9…` (SHA-256 `a8d1cebc…` and `129844da…`);
`canonical-latinLit` `8cef51623130f2412da410858e32c322eb796c0a`;
Scaife shards as above; `latin-lemmatized-texts` tag `0.1.2`.

| Script | Output |
| --- | --- |
| `ola_inventory.py` | `results/ola_inventory.json` |
| `cts_scheme.py` | `results/cts.json` |
| `manifest.py` | `ola-conllu-manifest.tsv`, `results/manifest_summary.json` |
| `coverage.py` | `results/coverage.json` |
| `measure_ola.py` | `results/ola_vs_ldt.json`, `results/ola_vs_ldt_total.json` |
| `measure_proiel.py` | `results/ola_vs_proiel.json` |
| `measure_scaife.py` | `results/scaife_vs_udperseus.json` |
| `measure_scaife_proiel.py` | `results/scaife_vs_proiel.json` |
| `measure_lll.py` | `results/lll_agreement.json` |

`classes.py` fixes the classical, borderline and out classification by author
identifier; `common.py` holds the readers and the alignment. Nothing committed
contains third-party text or annotations.
