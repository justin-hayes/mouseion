# OGA v0.2.0 (oga_src) AGDT lineage and licence

**Research date:** 2026-10-26

**Scope:** Which Ancient Greek Dependency Treebank (AGDT) version or versions
the Opera Graeca Adnotata (OGA) v0.2.0 annotations in the pinned `oga_src`
file derive from, whether those inputs carry non-commercial (NC) terms, and
the deployer-register row for the annotation layer (ADR 0111, #2312). Primary
sources only: Zenodo records, GitHub repositories and tags, arXiv papers, and
the pinned file itself. No authors were contacted.

## Answer

- **The v0.2.0 lineage names no AGDT version.** The OGA v0.2.0 README says the
  morphosyntax comes from a parser and lemmatizer "trained on 1,2M+ tokens of
  AGDT data", documented in arXiv 2410.12055. That paper's training set is the
  AGDT (linked to the treebank_data releases page, with no version number), the
  Gorman Trees and the Pedalion Trees. "AGDT 2.1" is stated only for OGA
  v0.1.0, in arXiv 2404.00739.
- **The pinned Anabasis text is not in any AGDT release checked.** Xenophon
  appears in neither AGDT 1.7 nor the AGDT 1.0, 2.1 or master trees. Gorman
  Trees contain Anabasis trees (Books 1 and 3). The 2410.12055 dataset table
  lists `tlg0032.tlg006` (Anabasis).
- **NC lineage can be neither confirmed nor ruled out.** The layer declares
  CC BY-SA 4.0. The annotations are parser output, so the whole training set
  is their lineage, not only the trees for Anabasis. Two NC claims sit in that
  training set. AGDT 2.x includes the AGDT 1.7 texts, which the treebank_data
  repository states are CC BY-NC-SA 2.5, while its top-level README states
  CC BY-SA 3.0 US for all contents. The Gorman README says CC BY-NC-SA 4.0,
  but the licence file it points to is the CC BY-SA 4.0 legal code. Neither
  conflict is resolved in the sources.
- **Recommendation:** restriction kind `unclear`, flagged **unverified**, not
  flagged NC. The reasons are the unstated AGDT version and the unresolved
  licence conflicts in the training data.

## Evidence

### 1. The pinned file

- `internal/ancientgreek/ancientgreek.go` pins
  `tlg0032.tlg006.perseus-grc2_oga020_src.conllu`, 7,438,037 bytes, SHA-256
  `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61`.
- That file is at `data/` in the `oga_src` tag `v0.2.0` (commit
  `223d2258745ecf35e7659191080f93640fe900c1`, from
  `https://github.com/mgbilby/oga_src/tree/v0.2.0`). Its blob is
  `dabd7bd596d72e0f679a1eeb29fe86137ed11c94`. I downloaded it from
  `https://raw.githubusercontent.com/mgbilby/oga_src/v0.2.0/data/`. The size
  and SHA-256 match the pin.
- The file's only header fields are `# source`, `# text` and `# sent_id`.
  No line states a version, a parser, a treebank or a licence. The first
  source line is `# source = Xenophon, Xen. Anab._1.1.1`.
- The ADR calls the pin "the single-file CoNLL-U packaging" of Zenodo record
  15199546. That record is not a single CoNLL-U file. Its only file is
  `mgbilby/oga_src-v0.2.0.zip`, 761,489,280 bytes, MD5
  `714b9b52abc47c65f26f6f2fefc92520`
  (`https://zenodo.org/api/records/15199546`). The per-file pin is
  verifiable from the GitHub tag. I did not download the 761 MB zip, so I did
  not confirm that its contents match the tag.

### 2. Zenodo and upstream OGA v0.2.0

- Zenodo 15199546 (`https://zenodo.org/api/records/15199546`): `license.id`
  `cc-by-sa-4.0`. Its description says the morphosyntax is "based on the
  Ancient Greek Dependency Treebank" and gives no version. Its
  `relations.version` lists one version (`is_last: true`).
- Zenodo 14206061, the upstream OGA v0.2.0 corpus
  (`https://zenodo.org/api/records/14206061`): `license.id` `other-at`, with no
  licence text in the record. The file is
  `opera_graeca_adnotata_v0.2.0.zip`, 8,629,032,478 bytes. I did not download
  it.
- Zenodo 8158675, OGA v0.1.0: also `other-at`.
- Upstream repository `https://github.com/OperaGraecaAdnotata/OGA`: tag `v0.2.0`
  is commit `ca3528bc65907014af88580c2793fe684caddcd7`, released 2024-11-24
  (`https://api.github.com/repos/OperaGraecaAdnotata/OGA/releases`). The
  README at that tag (blob `74cb76b7c969f468a38bacddd1b913ec277d6204`) says:
  - "The morphosyntactic annotation and lemmatization are the outputs of (i) a
    parser and (ii) lemmatizer trained on 1,2M+ tokens of AGDT data", documented
    in arXiv 2410.12055.
  - Licence: "Creative Commons Attribution-ShareAlike 4.0 International License
    (for more details, look also at the repositories of the original texts
    mentioned above)."
- The `main` README of that repository is a different revision. The tag
  README is the one cited above.
- `mgbilby/oga_src` `license.md` (blob `e3e2f9a956b426109dd619e974f2252ebc0b2ad9`,
  the same at `v0.2.0` and `main`) says the corpus "was archived under a CC
  BY-SA 4.0 international license". This is the repackager's statement, not
  Celano's. The `v0.2.0` README links the AGDT 2.0 guidelines
  (`AGDT2/guidelines/Greek_guidelines.md`) for the annotation scheme. That link
  is not a statement of the training-data version.

### 3. OGA v0.1.0: AGDT 2.1

- Celano, "Opera Graeca Adnotata: Building a 34M+ Token Multilayer Corpus for
  Ancient Greek", arXiv 2404.00739, section 4.4 (`https://arxiv.org/abs/2404.00739`):
  "morphosyntactic annotation in OGA 0.1.0 is performed automatically using the
  COMBO parser ... trained on the data of the Ancient Greek Dependency Treebank
  2.1 (AGDT)." Table 1 compares "AGDT 2.1" scores with the UD conversion.
- That paper describes v0.1.0. It does not describe v0.2.0.

### 4. OGA v0.2.0: the parser paper's training set

- Celano, "A State-of-the-Art Morphosyntactic Parser and Lemmatizer for
  Ancient Greek", arXiv 2410.12055 (`https://arxiv.org/html/2410.12055`),
  section 3: the training set "consists of the following treebanks: (i) the
  Ancient Greek Dependency Treebank, [footnote] `https://github.com/PerseusDL/treebank_data/releases`
  (ii) the Gorman Trees ... and (iii) the Pedalion Trees." The footnote gives no
  version. The total is 1,277,310 tokens before normalization.
- The paper names no AGDT version anywhere in its text. Its code repository
  (footnote 3, `git.informatik.uni-leipzig.de/celano/morphosyntactic_parser_for_oga`)
  did not yield readable metadata when I checked it.

### 5. Anabasis in the training data

- No AGDT release contains Xenophon. I checked the git trees of
  `PerseusDL/treebank_data` at `v2.1` (287 entries), `master` (309 entries) and
  `v1.0` (287 entries), and the 43 MB `v1/greek/data/agdt-1.7.xml`, which
  contains no `tlg0032` URN. The AGDT 1.7 text list in the CLTK README has no
  Xenophon either.
- `vgorman1/Greek-Dependency-Trees` (git tree at HEAD) contains
  `xml versions/Xen_Anab_book_1.1-5.xml`, `Xen_Anab_book_1.6-9.xml` and
  `Xen_Anab_book_3.xml`.
- `perseids-publications/pedalion-trees` contains `public/xml/xenmem.xml`
  (Memorabilia). It contains no Anabasis.
- arXiv 2410.12055's dataset table lists `tlg0032.tlg006 Anabasis, 18,737
  tokens`. The table does not say which treebank each text came from. The
  Anabasis trees are in the Gorman set, so the inference is that the Anabasis
  text was in the training data, drawn from Gorman. I did not confirm this per
  text.

### 6. AGDT licence statements

- `PerseusDL/treebank_data` `README.md` (identical at tags `v1.0`, `v2.1` and
  `v2.1_IGDS`, and at `master`): "Contents of this repository are licensed under
  a Creative Commons Attribution-ShareAlike 3.0 United States License." The
  statement dates from commit `a8252b78ab` (2014-10-16, "added description and
  license"). No NC term appears in it. The repository has no LICENSE file.
- `v2.0/Greek/README.MD`: the AGDT 2.0 "comprises the data available in the
  older versions plus new texts". So AGDT 2.x includes the 1.x texts.
- `v2.1/Greek/README.MD`: "no change has been made to the texts already
  contained in the version 2.0."
- `v1/greek/docs/README.txt` (in the same repository, describing AGDT 1.7):
  "AGDT 1.7 is licensed under a Creative Commons Attribution-NonCommercial-
  ShareAlike 2.5 License: http://creativecommons.org/licenses/by-nc-sa/2.5".
  `cltk/greek_treebank_perseus` README section 1.2 says the same.
- So the treebank_data repository states BY-SA 3.0 US at the top level and
  NC-SA 2.5 for AGDT 1.7 inside itself. The 1.7 texts are included in AGDT 2.x.
  The conflict is unresolved in the source. I did not check the commit history
  of `v1/greek/docs/README.txt`.
- The tags are `v1.0` and `v2.1` at commit `21ea99c050effeb9ad40dc4bc21ac4f5979eeea0`,
  and `v2.1_IGDS` at `92de910336`. The tag names therefore do not
  identify separate contents for v1.0 and v2.1.

### 7. Gorman Trees licence

- `vgorman1/Greek-Dependency-Trees` `README.md` (line 8): "This work is licensed
  under a Creative Commons Attribution-NonCommercial-ShareAlike 4.0
  International License (see TREEBANK_LICENSE file)."
- The file `TREEBANK LICENSE` (content SHA `117cd8deb8`, 20,131 bytes, created
  in commit `b690fad297`, 2019-12-31, the only commit touching it) is the
  full CC BY-SA 4.0 legal code. Its title is "Attribution-ShareAlike 4.0
  International". It contains no "NonCommercial" text.
- ADR 0111 says terms are "read from the licence instrument actually in
  force, not from README prose". Under that rule the Gorman instrument is
  CC BY-SA 4.0. The README's NC statement is unresolved.

### 8. Other licences seen

- Pedalion Trees (`perseids-publications/pedalion-trees`): MIT (GitHub
  licence API).
- arXiv 2404.00739 and 2410.12055 carry arXiv licence CC BY-NC-SA 4.0 on the
  article. That licence attaches to the papers, not to the corpus or the
  parser. The 2410.12055 appendix heading "Texts License: CC BY-NC-SA 4.0"
  appears only in extracted text. I could not find it as a standalone statement
  about the texts.

## Unverifiable or open claims

1. **The AGDT version behind OGA v0.2.0 is unstated.** The v0.2.0 README and
   arXiv 2410.12055 name only "AGDT". The only version statement is AGDT 2.1
   for v0.1.0. A training manifest or Celano's repository would settle it. The
   Leipzig GitLab repository I checked yielded no readable metadata.
2. **Anabasis in the training data is inferred, not confirmed.** The
   inference rests on the Gorman Xenophon trees and the paper's dataset table.
3. **Gorman NC conflict.** The README says NC-SA 4.0. The licence file in force
   says BY-SA 4.0. The source does not resolve this. Contacting the author
   would.
4. **AGDT 1.7 NC versus repository BY-SA 3.0 US.** Both statements sit in the
   treebank_data repository. Which governs the 1.x texts inside AGDT 2.x is not
   stated.
5. **The OGA corpus licence is unverified.** Zenodo 14206061 says `other-at`
   and has no licence text. The upstream README says BY-SA 4.0. The 8.6 GB zip,
   which may hold its own licence file, was not inspected.
6. **Whether the annotation is a derivative of the training data** is a legal
   question this research does not answer.
7. **The Zenodo zip is not the single CoNLL-U file.** The ADR's description
   does not match Zenodo 15199546. The GitHub tag file is verified against the
   pin, but the zip's contents were not compared with the tag.

## Recommended register entry

Add a row to `doc/third-party-data.md` when #2312 creates it. The row is an
Analysis resource (ADR 0111). The ADR's conditions for NC or unclear Analysis
resources already hold for the import: the file is operator-supplied at a local
path, and the import never ships it.

| Field | Value |
| --- | --- |
| Layer | OGA v0.2.0 morphosyntax and lemmas for Xenophon, *Anabasis* Book 1, from `oga_src` (`tlg0032.tlg006.perseus-grc2_oga020_src.conllu`, SHA-256 `c51bba2d…eb61`) |
| Kind | Analysis resource |
| Terms | Declared CC BY-SA 4.0 (upstream v0.2.0 README; `oga_src` `license.md`; Zenodo 15199546 `cc-by-sa-4.0`). Zenodo 14206061 `other-at`, no text. Training lineage: AGDT (version unstated), Gorman Trees (README NC-SA 4.0; licence file BY-SA 4.0), Pedalion Trees (MIT). |
| Restriction kind | `unclear` |
| NC or unverified flag | **Unverified.** NC lineage is neither confirmed nor ruled out: the parser's training data includes AGDT 1.7 texts stated as CC BY-NC-SA 2.5 inside a repository that states CC BY-SA 3.0 US. |
| Delivery path | Operator-supplied local file, imported by `ancientgreek` (ADR 0099). Never in a repository, image or release artifact. |
| Reaches a deck | Yes: `Lemma` and `POS` fields of imported decks, with `AncientGreekSourceNotice` (ADR 0111, "Exported decks"). |

If #2312 prefers the declared licence to the lineage, the restriction kind
would be `share-alike` with the same unverified flag. I recommend `unclear`
because the lineage contains an unresolved NC claim and an unstated AGDT
version.
