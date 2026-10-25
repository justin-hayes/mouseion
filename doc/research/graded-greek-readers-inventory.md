# Graded Ancient Greek Readers: Licensed Digitisation Inventory

**Research date:** 2026-10-25

**Ticket:** #2300 (map #2299, *public-domain graded Ancient Greek readers in Mouseion*)

**Scope:** Inventory of 19th–20th c. graded Greek readers (and born-digital learner texts) that exist in machine-readable form, with the licence, transcription status, pin, format, and linguistic annotation of each digitisation. Standing rule from the map: a digitisation, treebank, model or lexicon is usable only when its licence is confirmed **not NC**.

**Method:** Repository LICENSE files, READMEs and file trees were read at the pinned commits below via the GitHub API (`gh api`). Archive.org item metadata was read from `archive.org/metadata/<id>`. Model and treebank licences were read from the UD README metadata blocks, Hugging Face model cards and tool repositories. Downloaded content was treated as data and inspected only with `head`, `wc`, and `cat`.

---

## 1. Executive summary

- **Usable now (not NC, verified licence, machine-readable Greek):** six GLTP-style repositories, all **CC BY-SA 4.0**: Edwards *Salamis in Easy Attic Greek*, Morice *Stories in Attic Greek*, Young *Tutorial Greek Reader*, Beresford & Douglas *A First Greek Reader*, Farnell *Tales from Herodotus*, and Rouse *A Greek Boy at Home* (fhardison repo, CC BY-SA stated in README only). Also usable: Rouse *A First Greek Course* (ryanfb, CC BY-SA 4.0, partial), Heyden/Ruland *Formulae* (CC BY-SA 4.0, Greek translation only, not a graded reader), Colson and Chambers (CC BY-SA 4.0, but still in progress).
- **Excluded by licence:**
  - The **Exocortex** digitisations (23 readers) are **CC BY-NC 4.0** for the digitisation layer. The texts are public domain, but the digitisation is NC. Discovery and cross-check only.
  - **LGPSI** (*Linguae Graecae Per Se Illustrata*) has **no LICENSE file and no licence statement**, so all rights are reserved by default. Not usable until the author grants a licence.
  - **EasyGreekReaders** (seumasjeltzz) has **no LICENSE file and no licence statement**. Not usable.
- **Lemmatisation is the open problem.** Every treebank-trained Greek model checked is NC-tainted or unverifiable: UD Perseus is CC BY-NC-SA 2.5 and UD PROIEL is CC BY-NC-SA 3.0. That covers Stanza's Greek models, the PROIEL/Perseus-trained GreCy models, and Sōtēr Megas. Dilemma is MIT-licensed code, but its lookup and training draw on treebanks. Morpheus (MIT wrapper) is the only candidate whose data licence might be clean, and that still has to be confirmed.
- **Recommendation:** pilot **Edwards, *Salamis in Easy Attic Greek*** (`sleeptillseven/edwards-salamis-in-easy-attic-greek` @ `6937f4f`). It is the smallest and cleanest candidate, its main text is fully corrected, and it is in GLTP structure. Morice is the runner-up for scale.

---

## 2. Licence rules applied

| Category | Verdict rule used here | Result |
|---|---|---|
| Repo LICENSE file present with CC BY-SA 4.0 | Not NC | Usable (verified by file) |
| Licence stated only in README (no LICENSE file) | Not NC if the stated licence is not NC; flagged as weaker | Usable with a note (fhardison Rouse) |
| No licence file and no licence statement | Default copyright, not usable | Excluded (EasyGreekReaders, LGPSI) |
| CC BY-NC (any version) | NC | Excluded (Exocortex digitisation) |
| UD treebank CC BY-NC-SA | NC | Excluded (Perseus, PROIEL) |
| Model trained on NC data | NC-tainted | Excluded unless the training data is shown to be clean |

**Print status.** A print is public domain in the US if published before 1931 (as of 2026). EU and UK status depends on the author's death date, which is **not verified** for any editor below. The dates below are publication dates from the GLTP table, Archive.org metadata, or the scan record.

---

## 3. Digitisations of graded readers

Word counts are `wc -w` on the text file (includes reference tokens, so approximate). Exocortex counts are as published on its page.

### 3.1 Usable (non-NC, verified licence)

| # | Reader (author, date) | Digitisation & pin | Licence (verified how) | Print PD status | Transcription status | Format & structure | Annotation present |
|---|---|---|---|---|---|---|---|
| A1 | **Edwards**, *Salamis in Easy Attic Greek* (G. M. Edwards, 1913) | `sleeptillseven/edwards-salamis-in-easy-attic-greek` @ `6937f4f` (2020-05-15). Scan: archive.org `salamisineasyatt00edwa` (date 1913). | **CC BY-SA 4.0** (LICENSE file, 20,131 bytes, README) | 1913 publication; US PD by date. UK/EU unverified. | OCR 100%; preface, introduction and main text corrected (main text 22/22 chapters); notes and vocabulary "maybe". | GLTP format: `text/edwards-salamis-main.txt` (66,891 bytes), `…-introduction.txt`, `…-preface.txt`. Lines `N.M text`, chapter titles on `N.title`. Greek only in the main text; English is in the `orig/salamis-greek-grc+eng.txt` source file, not the GLTP text. ~5,400 words. | None verified in repo (no lemma files). |
| A2 | **Morice**, *Stories in Attic Greek* (F. D. Morice, 1883) | `greek-learner-texts/morice-stories` @ `17cc958` (2021-11-10). Scan: archive.org `storiesinatticg00unkngoog` (archive status NOT_IN_COPYRIGHT). | **CC BY-SA 4.0** (LICENSE file, README) | 1883; archive status NOT_IN_COPYRIGHT. | OCR and corrections done (README: "scanned, OCR'd and corrected"); notes not yet in main text; vocabulary tool not yet added. | `text/morice.txt` (320,404 bytes): `NNN.CC.PPP` IDs, chapter headings at `NNN.00.000`, Greek text on the paragraph lines, ~26,000 words. | `orig/vocabulary.tsv` (83,665 bytes) and `orig/Morice's Stories in Attic Greek.xlsx` exist in repo; not yet integrated. |
| A3 | **Young**, *The Tutorial Greek Reader* (A. W. Young, 1898) | `sleeptillseven/youngs-tutorial-greek-reader` @ `3e0df16` (2020-05-15). Scan: archive.org `tutorialgreekrea00younuoft` (NOT_IN_COPYRIGHT). | **CC BY-SA 4.0** (LICENSE, README) | 1898; NOT_IN_COPYRIGHT (archive). | Preface and main text 100% (README); vocabulary "maybe". | `text/young-tutorial-greek-main.txt` (174,779 bytes): `N.M text` lines, headings `N.title`, parts ("PART I.") as `0.0`. Preface in two separate files. ~15,000 words. | None in repo (vocab not done). |
| A4 | **Beresford & Douglas**, *A First Greek Reader* (1902) | `seumasjeltzz/Beresford` @ `09f627c` (2021-07-13). Scan: archive.org `BERESFORDDOUGLAS-AFirstGreekReader` (date 1903). | **CC BY-SA 4.0** (LICENSE, README) | 1902/1903; US PD by date. | Transcribed, checked and validated with `greek-normalisation` (README). Running vocabulary and English translation not yet done. | `text/Beresford.PP.CC.txt`, one file per chapter, lines `Part.Chapter.Line` (e.g. `1.8.1`); two parts (`01.*`, `02.*`), ~9,000 words. | Lemmatisation "now lemmatising" (README); no lemma files in repo yet. |
| A5 | **Farnell**, *Tales from Herodotus* (G. S. Farnell, 1895) | `FergusJPWalsh/farnell-herodotus` @ `7d7d345` (2022-07-03). Scan: archive.org `talesfromherodo00herogoog` (archive date recorded as 1912, creator "Herodotus"). | **CC BY-SA 4.0** (LICENSE, README) | 1895 print (MacMillan); US PD by date. | Transcribed by Nathan Binns (README); edited by Fergus Walsh. GLTP progress: "incorporate notes into main text or as own page; add vocab tools". | `text/farnell-herodotus.txt` (142,984 bytes, `NNN.CC.PPP` IDs, English chapter titles on `NNN.00.000` lines), `text/preface.txt`. ~12,300 words. | Notes (`orig/farnell_tales_from_herodotus_notes.txt`), proper-name vocabulary (`…_vocabulary_of_proper_names.txt`) and full vocabulary (`…_vocabulary.xlsx`) exist in `orig/`; not yet in the text. |
| A6 | **Rouse**, *A Greek Boy at Home* (W. H. D. Rouse, 1909) | `fhardison/rouse-a-greek-boy-at-home` @ `bcf0a64` (2024-02-13). Scan: archive.org `greekboyathomest01rousuoft` (NOT_IN_COPYRIGHT). | **CC BY-SA** stated in README only; **no LICENSE file** (GitHub `license: null`). Treat as usable with a note; confirm before pilot. | 1909; NOT_IN_COPYRIGHT. | Mixed: README says 19/43 chapters proofed (marked "OLD"). Published text is Nathan Binns' OCR, formatted by Fletcher Hardison. | `docs/` HTML and `src/rouse_text.txt`; `src/greek-boy-chNN.md` per chapter (Markdown). ~30,000 words. | Vocabulary folder `vocab/` and `convert_vocab.py` present; contents not checked. |
| A7 | **Rouse**, *A First Greek Course* (1916) | `ryanfb/rousefirstgreekcourse` @ `709a4e0` (2020-05-29). Scan: archive.org `FirstGreekCourse` (licenseurl `publicdomain/mark/1.0`). | **CC BY-SA 4.0** (LICENSE file) | 1916; archive scan marked PDM 1.0. | Partial: selected pages OCR'd (Tesseract, `eng+grc`), manual correction in progress (README). | `text/rousegreekcourse.NNN.txt`, one file per reading (about 30 files shown). Partial. | "Lemmatize… frequency dictionary" listed as goal; nothing in repo yet. |
| A8 | **Heyden / Ruland**, *Formulae Colloquiorum Puerilium* (1527/1556) | `FergusJPWalsh/heyden-formulae` @ `af58349` (2022-03-11, default branch `main`). | **CC BY-SA 4.0** (`LICENSE.md`, 20,131 bytes; README badge) | 1527/1556; PD by age. | Recension and digitisation done; English translation "coming soon". | `text/heyden-formulae-graece.txt` (31 KB) and Latin text. Bilingual colloquy, not a graded reader. | None. |

**Colson and Chambers (CC BY-SA 4.0, incomplete):**

| Reader | Digitisation & pin | Status |
|---|---|---|
| Colson, *Stories and Legends* (1908) | `IvyJL/colson-stories` @ `b8353dd` (2020-04-28). Licence CC BY-SA 4.0 (LICENSE, README). | "Texts being added" (README). Only the first story verified in detail (97 words). Scan: Google Books (README); archive.org `storieslegendsfi00cols` has no metadata in the API response. |
| Chambers, *The Greek War of Independence* (1906) | `seumasjeltzz/ChambersGreekWar` @ `0f313be` (2026-10-03). Licence CC BY-SA 4.0. | OCR complete, core text under review ("proofreading extremely welcome"). Core text: 48 chapters, `chapter.paragraph.line`. `analysis/` has seven lemmatiser outputs of the core text (see §5). Notes and vocabulary still in `drafts/` (OCR, not yet formatted). |

### 3.2 Excluded

| Reader / resource | Digitisation & pin | Licence finding | Verdict |
|---|---|---|---|
| **Exocortex** *Ancient Greek Readers* (23 readers) | `exocortex-ancientgreek/exocortex-greek-readers` @ `39dfc14` (2026-07-19). Site: exocortex-ancientgreek.github.io. | README, "Licence" section: **texts are public domain**; "the digitisation layer … is licensed **CC BY-NC 4.0**". Repo has no LICENSE file; the NC statement is in the README and governs the digitisation. | **Excluded (NC).** Discovery and cross-check only. Do not copy transcriptions. |
| **LGPSI** *Linguae Graecae Per Se Illustrata* | `seumasjeltzz/LinguaeGraecaePerSeIllustrata` @ `53014d1` (2026-09-17). Markdown source in `src/001.md`–`019.md`, with epub/PDF builds. | GitHub `license: null`; no LICENSE file; README has no licence statement. Born-digital, so copyright is held by its author(s). | **Excluded (unlicensed).** Needs a written licence from the author. Structure is `## I`–style headings with running Greek sentences, useful as a long-term option. |
| **EasyGreekReaders** (seumasjeltzz) | `seumasjeltzz/EasyGreekReaders` @ `7cc057c` (2020-02-29; pushed 2023-04-15). ~27 readers in `metadata.csv`. | GitHub `license: null`; no LICENSE file; README has no licence statement. The file contains Rouse GBH and Beresford text copies. | **Excluded (unlicensed).** Same texts exist with clear licences in the GLTP repos above. Use the GLTP repos instead. |
| Moss, *First Greek Reader* (1900) | No GLTP repo; text exists only as `EasyGreekReaders/Moss.01.0{1..7}.txt` (unlicensed). GLTP: "transcription 20%". | Unlicensed (via EasyGreekReaders). | **Excluded (no clear licence).** Scan-only until transcribed. |

---

## 4. Discovery list: readers with no clean digitisation found

These readers appear in the Exocortex list or in GLTP's metadata. For each, the public-domain print is on archive.org. Self-digitisation from the scan (not from Exocortex) would be a new, non-NC digitisation. Archive status is from the Archive.org metadata API.

| Reader | Archive.org ID | Archive date / status | Notes |
|---|---|---|---|
| Mayor, *First Greek Reader* | `firstgreekreade00mayogoog` | 1873 · NOT_IN_COPYRIGHT | Exocortex: 4,691 lines, 31,072 words |
| Rouse, *A Greek Reader* | `greekreader00rousuoft` | 1907 · NOT_IN_COPYRIGHT | Exocortex: 3,717 lines |
| Abbott, *Easy Greek Reader* | `easygreekreader00abbogoog` | 1886 · NOT_IN_COPYRIGHT | Short (88 lines per Exocortex); in EasyGreekReaders (unlicensed) |
| Rouse, *Greek Boy at Home* vocabulary | `greekboyathomest00rousuoft` | 1909 · NOT_IN_COPYRIGHT | Companion vocabulary |
| Moss, *First Greek Reader* | `firstgreekreader00mossrich` | 1886 (archive) vs 1900 (GLTP table) · NOT_IN_COPYRIGHT | Date mismatch; may be a different edition. Verify before use. |
| Lowe, *The Wars of Greece and Persia* | `warsofgreecepers0000hero` | archive date 1990 (edition), no status | Exocortex flags the copy as borrow-only; text is long out of copyright. Use an older print. |
| Smith, Coy, Bell, Freeman & Lowe, Herwig, Moore, Phillpotts & Jerram, Cherny (×3) | Listed in Exocortex | Not checked individually | Exocortex lists scan sources; no archive metadata checked in this pass. |

Exocortex is the only source that lists the Russian-language readers (Cherny) and Herwig's German reader. These were not investigated further.

---

## 5. Lemmatisation and morphology: licence check

Mouseion's rule is that analyses are imported, not generated by the NLP service for `grc` (ADR 0099). The treebank-trained tools below were checked for licence contamination.

| Tool / resource | Licence of code | Licence of data or model | Verdict |
|---|---|---|---|
| UD Ancient Greek **Perseus** treebank | — | **CC BY-NC-SA 2.5** (UD repo README, machine-readable metadata) | **NC.** Excludes any model trained on it. |
| UD Ancient Greek **PROIEL** treebank | — | **CC BY-NC-SA 3.0** (UD repo README) | **NC.** Excludes any model trained on it. |
| Stanza (`stanfordnlp/stanza`) | Apache-2.0 (LICENSE file, verified) | Greek models use Perseus and PROIEL (resource list: `perseus_nocharlm` default) | **NC-tainted** for Greek. Do not use the grc models. |
| CLTK (`cltk/cltk`) | MIT (verified) | Pipeline wraps Stanza and OdyCy models | Inherits the Stanza/OdyCy problem. |
| OdyCy `chcaa/grc_odycy_joint_trf` / `_sm` | HF card: MIT | Card says evaluated on UD Perseus and PROIEL test sets; the training data is **not** stated on the card read | **Unverified, probably NC-tainted.** Needs the training-data statement. |
| GreCy (`jmyerston/greCy`) `grc_proiel_*`, `grc_perseus_*` | MIT (LICENSE file, verified) | Models named after PROIEL and Perseus; model card not checked | **NC-tainted (likely).** Do not use the PROIEL/Perseus models without confirmation. |
| Dilemma (`open-greek/dilemma`) | MIT (LICENSE file, verified) | README: lookup built from "Wiktionary + LSJ + Sophocles + GLAUx + treebanks"; "gold treebanks" as training labels | **Unverified.** Lookup and training include treebanks. The treebank identities and model-weight licence were not confirmed. |
| Sōtēr Megas (`Zual/soter-megas`) | Apache-2.0 (HF card, verified) | Card: trained on "UD grc train splits" (CC BY-NC-SA per the card) and DeepSeek V4-Flash annotations | **NC-tainted** (UD) and **unverified** (LLM-annotation terms). Excluded. |
| Morpheus (`perseids-tools/morpheus-perseids-api`) | MIT (LICENSE, verified) | Wraps the Perseus Morpheus morphological data; **data licence not verified** in this pass. Perseus texts are CC BY-SA (per the JOHD article on Perseus). | **Candidate, unverified.** Only candidate with a plausibly clean data path. Confirm the Morpheus data licence before adopting. |

**Consequence.** No lemmatiser in this inventory is confirmed non-NC. The pilot can proceed without lemmas (the reader's own vocabulary and notes are the first use). A non-NC lemma path needs a separate check: confirm Morpheus data licence, or find a lexicon-based lemmatiser with a documented non-NC licence.

---

## 6. Other sources checked

| Source | Result |
|---|---|
| Greek Learner Texts Project table (`greek-learner-texts.org/texts/`) | Lists all the GLTP repos above. Moss has no repo. Heyden is listed under easy readers. |
| Open Greek & Latin / Perseus / First1KGreek | Perseus Greek texts are CC BY-SA. OGL corpora are open. **No graded learner readers found** in a single search. Negative result, limited search. |
| Wikisource (el) | Search for "Greek reader": one hit (*The New Testament in the Original Greek*, 1894). No graded reader found. |
| Wikisource (grc) | API call returned a non-JSON response; **not verified**. |
| Archive.org (full text) | Full-text (`_djvu.txt`) files exist for every item listed in §3–4; metadata checked, not the text itself. |

---

## 7. Format notes for ingestion

- **GLTP ref-text format.** `NNN.CC.PPP text` (Morice, Farnell, Chambers) or `N.M text` (Edwards, Young), or `P.C.L text` (Beresford, Colson). Headings use `.00` or `.title`. Line breaks from the print are kept as `{.2}` markers in Colson. The repo README (`text/README`, Colson) states: each story is numbered; the title is on the first line of each part.
- **Validation.** Each GLTP repo has `text-validator.toml` and a `text-validation.yml` workflow (`.github/workflows/`). Not run in this pass.
- **Notes and English.** Notes and vocabulary are in `orig/` or `drafts/` files, not in the GLTP text. English is in separate files (Edwards `…+eng.txt`). None of the GLTP texts embeds notes in the Greek text yet.
- **Dialect.** Titles and the GLTP descriptions say "Attic" (Morice, Edwards, Farnell "Attic dialectical forms", Young). Not checked against the text for dialect; treat as Attic by title.

---

## 8. Ranked pilot shortlist

Ranking: licence verified and not NC; structure clean; modest size; text corrected.

1. **Edwards, *Salamis in Easy Attic Greek*** (`sleeptillseven/edwards-salamis-in-easy-attic-greek` @ `6937f4f`). CC BY-SA 4.0. Main text fully corrected. ~5,400 words, 22 chapters. Clean GLTP structure. Best first pilot for import mechanics. Needs vocabulary built.
2. **Morice, *Stories in Attic Greek*** (`greek-learner-texts/morice-stories` @ `17cc958`). CC BY-SA 4.0. Full text in GLTP format, ~26,000 words. A vocabulary TSV is already in `orig/`. Best candidate for scale testing.
3. **Beresford & Douglas, *A First Greek Reader*** (`seumasjeltzz/Beresford` @ `09f627c`). CC BY-SA 4.0. Validated text, ~9,000 words, per-section files. Vocabulary and lemmas in progress.
4. **Young, *Tutorial Greek Reader*** (`sleeptillseven/youngs-tutorial-greek-reader` @ `3e0df16`). CC BY-SA 4.0. Main text done, ~15,000 words. No vocabulary yet.
5. **Farnell, *Tales from Herodotus*** (`FergusJPWalsh/farnell-herodotus` @ `7d7d345`). CC BY-SA 4.0. ~12,000 words. Notes and vocabulary exist in `orig/` but are not integrated. Adapted from Herodotus, so it is a Classical Book whose Work is the reader.
6. **Rouse, *A Greek Boy at Home*** (`fhardison/rouse-a-greek-boy-at-home` @ `bcf0a64`). Licence only in README; partial proofing. Do not pilot until the LICENSE gap is resolved.

Not yet ready: Colson (in progress), Chambers (in progress, notes in drafts), Rouse *First Greek Course* (partial), Heyden (bilingual colloquy, not a graded reader).

---

## 9. Unverifiable or unconfirmed claims

- **Print PD status in the UK and EU** for every reader: author death dates were not checked.
- **Edwards, Morice, Beresford, Farnell print dates:** archive dates differ slightly from the GLTP table (Beresford 1902 vs 1903; Farnell 1895 vs archive 1912). The print date is taken from the GLTP table.
- **Moss** archive date (1886) conflicts with the GLTP date (1900). Unresolved.
- **Dialect:** Attic is taken from titles and GLTP descriptions, not checked against the Greek.
- **Lemma-tool model licences:** OdyCy training data, GreCy model cards, Dilemma model weights and treebank list, and Morpheus data licence were not confirmed.
- **Wikisource grc:** not searched successfully.
- **Exocortex** page counts, word counts, and licence statement are as published by the repo; the NC licence statement was read from the README, not from a LICENSE file.
- **Rouse GBH** licence: README says CC BY-SA; no LICENSE file in the repo. Treat as declared, not verified.
- **LGPSI** and **EasyGreekReaders** have no licence in the repo; I did not contact the authors.
