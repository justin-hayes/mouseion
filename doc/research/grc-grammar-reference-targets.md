# Grammar-reference link targets: Smyth (Ancient Greek) and Allen & Greenough (Latin)

**Research date:** 2026-11-23

**Ticket:** [#2601](https://github.com/justin-hayes/mouseion/issues/2601), on the
map [#2597](https://github.com/justin-hayes/mouseion/issues/2597) (Ancient Greek
syntax and construction identification).

**Scope:** Which reference grammar a construction in Sentence study can link to.
For each candidate this note records the licence instrument actually in force,
whether a § number is enough to build the URL, whether that URL is stable, and
which edition's numbering it follows. The Latin counterpart is covered so the
link scheme generalises. It also gives the ADR 0111 classification of a link and
a § citation.

**Method.** Live HTTP probes (2026-11-23) of Perseus Hopper, Dickinson College
Commentaries (DCC), Alpheios, Wikisource, Internet Archive and Scaife. The TEI
headers came from Hopper's `dltext` export. GitHub repositories are cited at
commit SHAs. One secondary source, the SBL Handbook of Style blog, is cited for
the 1956 revision, because the revised edition is in copyright and not freely
readable. Twelve sampled §§ were compared with a 1920 scan. No Smyth or A&G text
is committed here; the excerpts below are a few words used to identify a section.

## Answer

| Language | Recommended target | URL template | Licence of the hosted text | Fallback |
|---|---|---|---|---|
| Ancient Greek | Perseus Hopper, Smyth, *A Greek Grammar for Colleges* (American Book Company, 1920), `Perseus:text:1999.04.0007` | `https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={§}` | Underlying book: public domain (1920, US). Perseus terms conflict (see below); irrelevant to a link | Hopper citation form `https://www.perseus.tufts.edu/hopper/text?doc=Smyth+grammar+{§}&fromdoc=Perseus:text:1999.04.0007` (same host). Off-host fallback: Alpheios `grammars.alpheios.net/smyth/xhtml/<file>#s{§}`, which needs a § → file map |
| Latin | DCC, Allen & Greenough, *New Latin Grammar* (Ginn, 1903) | `https://dcc.dickinson.edu/grammar/latin/category-search?field_gl_section_number_value={§}` | DCC credits page: "freely available for re-use under a Creative Commons Attribution-ShareAlike license" (version unstated) | Perseus Hopper `https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0001:smythp={§}` (or `doc=AG+{§}&fromdoc=Perseus:text:1999.04.0001`) |

- **One scheme serves both languages: `{base}{§}`.** A per-language template
  plus an integer § fully determines the link. For Latin, prefer DCC over
  Perseus only if a second host is acceptable. Perseus Hopper alone also covers
  both, with the same `smythp=` key.
- **Cite the 1920 edition explicitly** (for example "Smyth 1920 §2070"). The
  Perseus text and every free digitisation is the 1920 edition. The 1956 Messing
  revision kept "many", not all, of the § numbers, so a § taken from the 1956
  book can point to the wrong 1920 section.
- **Every § citation Mouseion authors is checked against the Perseus 1920 text**,
  which is the text the link opens. The definition stays Mouseion's own sentence.
- **ADR 0111: no register row and no notice.** An outbound link and a bare §
  citation reproduce no third-party expression, so they are not a third-party
  data layer. A row would be needed only if Mouseion copied Smyth or A&G text,
  headings included, or bundled a § → URL table derived from a host's files. The
  recommended templates need no such table.

## Smyth digitisations

### Perseus Hopper (recommended)

- **Edition.** The TEI header exported by
  `hopper/dltext?doc=Perseus:text:1999.04.0007` has
  `<sourceDesc><bibl>American Book Company, 1920</bibl>`. The page footer says
  "American Book Company, 1920." The `refsDecl` declares a `smythp` citation
  unit, which is the § number (`<milestone unit="smythp" n="1" />`).
- **§ coverage.** The Hopper browse bar for the work lists 3,262 distinct
  `smythp` keys: every integer from 1 to 3048 with no gaps, plus 214
  dialect-note keys such as `3 D`, `30 D. 1` and `44 a. D`. An integer § always
  resolves. Lettered subsections (`2016 a`) are inside the § page, not separate
  keys, so link at § level. Script: the navbar parse used
  `re.findall(r'smythp%3D([^"&]+)"', page)` on the §2016 page.
- **Predictable and checked.** `...:smythp=2016` opens §2016 ("The infinitive is
  used as the object of verbs of saying and thinking"). The citation form
  `text?doc=Smyth+grammar+2070&fromdoc=Perseus:text:1999.04.0007` opens §2070.
  The path form `...:part=4:chapter=52:section=2016` returns "No document found",
  so do not use structural citations.
- **Established use.** The AGDT annotation guidelines link Smyth with the
  `Smyth+grammar+{§}` form
  ([`PerseusDL/treebank_data@bf4334f0`, `AGDT2/guidelines/Greek_guidelines.md`](https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/AGDT2/guidelines/Greek_guidelines.md)).
  So does the UD Ancient Greek documentation
  ([`UniversalDependencies/docs@44e7d419`, `_grc/pos/VERB.md`](https://github.com/UniversalDependencies/docs/blob/44e7d419e31aba3d5e745699d6f45e0d6b8c03fd/_grc/pos/VERB.md)),
  which cites "Smyth … 1920 … American Book Company". The URL form has been
  stable long enough for two large projects to depend on it.
- **Licence instruments, which conflict.**
  - The TEI header has no `<availability>` or licence element.
  - [Hopper Open Source page](https://www.perseus.tufts.edu/hopper/opensource):
    "Texts are licensed under the Creative Commons ShareAlike 3.0 License".
  - [Hopper Copyright page](https://www.perseus.tufts.edu/hopper/help/copyright):
    materials "are provided for the personal use of students, scholars, and the
    public. Any commercial use or publication without authorization is strictly
    prohibited", and "Links to Perseus are always preferable to any copying of
    materials."
  - The underlying 1920 book is in the US public domain. The Internet Archive
    scan `smyth-greek-grammar-for-colleges-1920` carries the Public Domain Mark
    1.0, and `agreekgrammarfo02smytgoog` is marked `NOT_IN_COPYRIGHT`.

  The conflict would matter only if Mouseion copied the Perseus digitisation. A
  link copies nothing, and the copyright page explicitly prefers links.
- **Availability.** It responded on 2026-11-23, but not to a bare client.
  `curl` without a browser User-Agent gets HTTP 403. With one, Hopper answers
  302 to a "Please allow cookies" page and serves the text once the cookie is
  set. A learner's browser is unaffected. A server-side link checker would need
  a cookie jar and a browser User-Agent. Hopper is the legacy Java platform;
  whether Perseus will retire it in favour of Scaife is **unverified**. That is
  the main longevity risk, so the template belongs in one place, as data.

### Perseus Scaife / CTS

- **Unverifiable.** On 2026-11-23, `https://scaife.perseus.org/` returned 200
  but `/library/` and `/search/?q=Smyth` served a "Making sure you're not a
  bot!" challenge. Whether Scaife hosts Smyth, and under what CTS URN and
  passage scheme, was not established.
- **Probably no CTS URN at all.** Perseus's migration ledger
  ([`PerseusDL/tei-conversion-tools@c336b8db`, `summary_data/urnLessEntries.json`](https://github.com/PerseusDL/tei-conversion-tools/blob/c336b8dbcea084726ac4df865263b1e5483cd459/summary_data/urnLessEntries.json))
  records `1999.04.0007` (Smyth) and `1999.04.0001` (A&G) with
  `"status": "not migrated - NO URN"`. Neither was migrated to the CTS corpus
  that Scaife serves, so a Scaife deep link most likely does not exist. Live
  confirmation was blocked.
- The same ledger gives both source paths under a `copyright/` directory
  (`texts/Classics/Smyth/copyright/smyth_eng.xml`,
  `texts/Classics/Allen_and_Greenough/copyright/ag.gram_eng.xml`). That
  suggests Perseus files these digitisations as rights-restricted. It is one
  more reason never to copy them. Links are unaffected.

### Alpheios (possible fallback)

- [`alpheios-project/grammar-smyth@d7796df9`](https://github.com/alpheios-project/grammar-smyth/tree/d7796df9b3cbccd0a04cdc2b7aebe7d7869e1d28),
  README: "This work is licensed under a Creative Commons
  Attribution-Noncommercial-Share Alike 3.0 United States License … an HTML
  transformation of the Perseus XML, with corrections". There is no LICENSE
  file; the README is the only instrument, and it is NC. That does not matter
  for a link, but it rules out copying.
- **URL scheme.** Live at `https://grammars.alpheios.net/smyth/xhtml/`. Each §
  has an anchor `id="s{§}"` (for example `<div class="smythp" id="s1">`), but
  the files are named by document structure (`body.1_div1.1_div2.1.html`). A
  link is therefore `<file>#s{§}`, and the file **cannot be predicted from the §
  number**; it would need a § → file map built from the repository's 110
  `xhtml/` files. The repository was last pushed on 2020-05-13.

### Wikisource

- No main-namespace transcription exists: `A_Greek_Grammar_for_Colleges` returns
  404. There is a proofreading index,
  `Index:A Greek Grammar For Colleges.djvu`, with 808 `Page:` pages (API,
  2026-11-23), addressed by scan page, not by §. **Not usable** today.

### Internet Archive

- 1920 scans (`smyth-greek-grammar-for-colleges-1920`, PDM 1.0;
  `agreekgrammarfo02smytgoog`, University of Michigan/Google). Deep links are
  by scan page (`/details/<id>/page/n<N>`), so they **cannot be predicted from
  §**. Useful as the verification copy of the 1920 numbering (below), not as a
  link target.
- Trap: IA also holds the 1916 *A Greek Grammar for Schools and Colleges*
  (`greekgrammarfor00smyt` and others), a shorter book with different numbering.

### Dickinson College Commentaries

- DCC hosts Allen & Greenough but no Smyth. `dcc.dickinson.edu/grammar/smyth`
  and `/greek-grammar` return 404. Whether DCC links Smyth from another page was
  not checked further.

## Edition: 1920 versus Messing 1956

- **Perseus follows the 1920 numbering.** Twelve §§ spread across the syntax
  were compared: 1041, 1765, 1866, 2016, 2058, 2070, 2193, 2280, 2574, 2753, 2965
  and 3048. In all twelve, the opening words of the Perseus page match the § of
  the same number in the 1920 IA scan (`smyth-greek-grammar-for-colleges-1920`,
  djvu OCR). Examples: 2070 "Genitive Absolute.—A circumstantial participle
  …"; 2193 "Final clauses denote purpose …"; 3048 "Zeugma …". 12/12. The last §
  in both is 3048.
- **The 1956 revision is not guaranteed to match.** SBL Press
  ([SBLHS blog, 2018-03-08](https://sblhs2.com/2018/03/08/citing-smyths-greek-grammar/)):
  "Messing's revisions and corrections were relatively minor … many of the
  section numbers are also the same … Authors converting citations from the 1920
  edition to the 1956 edition should be careful to ensure that the section
  numbers remain the same." Which §§ differ is **unverified**. The 1956 text is
  in copyright, and IA has it only as controlled-lending items
  (`greekgrammar0000smyt_j6b4`).
- **Consequence.** Most English-language scholarship cites the 1956 edition as
  "Smyth". Mouseion's links open the 1920 text, so Mouseion should label its
  citation as the 1920 edition and take every § from the 1920 text, never from
  memory of the 1956 book. The interim measurement already checks judgments
  against Smyth; it should name the 1920 Perseus text as that reference.

## Latin: Allen & Greenough

- **Perseus Hopper** `Perseus:text:1999.04.0001`. The TEI header gives
  `<bibl>Ginn and Company, 1903</bibl>`, editors Greenough, Kittredge, Howard
  and D'Ooge, and also uses the `smythp` unit name for its §. `...:smythp=419`
  and `text?doc=AG+419&fromdoc=Perseus:text:1999.04.0001` both open §419 ("A
  noun or pronoun, with a participle in agreement, may be put in the Ablative
  …"). The Perseus licence situation is the same as for Smyth.
- **DCC** ([credits and reuse](https://dcc.dickinson.edu/grammar/latin/credits-and-reuse)):
  "Source: Allen and Greenough's New Latin Grammar … Boston: Ginn & Company,
  1903 … based on a digitization carried out by the Perseus Project … The
  content is freely available for re-use under a Creative Commons
  Attribution-ShareAlike license." The CC version is not stated. Sections
  607–629 (metrics) are omitted, which does not affect syntax.
  - Pages are topic slugs (`/grammar/latin/ablative-absolute`, covering
    §§419–420), so the slug cannot be predicted from §. DCC's "Go to Section"
    form is, though:
    `category-search?field_gl_section_number_value=419` returned 200 and
    rendered the §419–420 page, with in-page anchors such as `id="419.a"`.
    This is a Drupal view URL, a little more fragile than a content URL.
  - DCC is actively edited and modern; the trade-off is a second host and a
    query-string URL.
- **Recommendation for Latin:** no Latin is scheduled (ADR 0110), so record the
  template only. Either host fits the `{base}{§}` scheme. Prefer DCC for
  readability and an explicit CC BY-SA statement, with Perseus as fallback. If
  one host for both languages matters more, use Perseus for both.

## ADR 0111 classification

- ADR 0111 governs third-party **layers** whose expression or output Mouseion
  uses. An outbound `<a href>` reproduces nothing. A § number is a bare
  reference, and the one-sentence definition is Mouseion-authored expression,
  so neither is a third-party layer. **No register row and no deck notice.**
  Cards are out of scope on the map anyway.
- **Conditions under which that changes:**
  1. Showing Smyth's or A&G's own headings or text in the app, which is Source
     content. The Perseus terms conflict and Alpheios is NC, so this would fail
     ADR 0111's redistribution rule unless it were taken from the PD 1920 print
     rather than a digitisation.
  2. Shipping a § → URL map derived from a host's files, such as the Alpheios
     map. That map is derived data, and it would deserve a register row.
- Optional and not required: a one-line operator note in `doc/third-party-data.md`
  that Sentence study links out to Perseus/DCC, for deployers who care about
  outbound requests on a tailnet-only app. That is a product call, not a licence
  requirement.

## Open and unverifiable

- Live Scaife hosting of Smyth (bot challenge). Perseus's ledger says both
  grammars were "not migrated - NO URN".
- Which § numbers changed in Messing's 1956 revision.
- Hopper's retirement timeline.
- The CC version of DCC's A&G licence.
- Hopper's cookie and User-Agent gate is only relevant to automated link
  checking. A CI link check would be flaky, so prefer a one-time manual
  verification of each authored § when the spec lands.
