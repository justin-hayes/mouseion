# Citation-aligned public-domain English translations for OGA works

**Research date:** 2026-10-25

**Ticket:** [#2294](https://github.com/justin-hayes/mouseion/issues/2294) (map [#2292](https://github.com/justin-hayes/mouseion/issues/2292))

**Scope:** Which English translations of the works in the Opera Graeca Adnotata
release (`oga_src`) are machine-readable, aligned by conventional citation with
the Greek, and public domain. This is a survey, not a decision. It informs the
curated classical shelf and the default Greek card of
[ADR 0101](../adr/0101-ancient-greek-cards-from-source-evidence.md).

## Executive summary

- **Coverage.** 686 of the 1,999 OGA documents (34.3%) have at least one
  English translation in PerseusDL `canonical-greekLit` or
  `OpenGreekAndLatin/First1KGreek`. All 686 are `tlg` works.
- **Public domain by date.** 503 works (25.2%) have a translation whose
  recorded source imprint is 1930 or earlier, which is US public domain in
  2026. Other jurisdictions need the translator's death date, which was not
  checked.
- **Citation-aligned.** 401 works (20.1%) have such a translation whose
  sections match at least 90% of the Greek's citation keys. Of these, 386 use
  only `book`, `chapter`, `section` and `subsection` divisions.
- **Brownson's parser does not read them unchanged.** `brownson.go` accepts
  only a `book > chapter > section` tree. It rejects any section without both a
  book and a chapter ancestor, and it ignores other textpart kinds. Of the 401
  works, only 13 have the three-level shape it accepts. The fix is to
  generalise the parser to the Greek's depth and textpart kinds, not to write a
  new parser.
- **Granularity.** Where the English tracks the Greek, it matches at section
  level. In 95.6% of the well-matching files, the English adds no keys beyond
  the Greek's. No file is sentence- or word-aligned.
- **Licence.** The Perseus and First1KGreek files are CC BY-SA 4.0 by their
  own statements. The translation text is public domain only where its source
  is old enough. See Licences below for what remains open.

## Sources

| Repository | Pin | Role | Licence |
| --- | --- | --- | --- |
| [PerseusDL/canonical-greekLit](https://github.com/PerseusDL/canonical-greekLit/tree/0189b9c8d8d1f148133f2f0af309b4ee90ea4332) | `master` = `0189b9c8d8d1f148133f2f0af309b4ee90ea4332` (same as ADR 0101's Brownson pin) | Perseus English (`perseus-engN.xml`) and Greek (`perseus-grcN.xml`) | CC BY-SA 4.0 ([LICENSE](https://github.com/PerseusDL/canonical-greekLit), README). Its README says "Tufts University holds the overall copyright" and that materials "have varying copyright status". |
| [OpenGreekAndLatin/First1KGreek](https://github.com/OpenGreekAndLatin/First1KGreek/tree/03776b39f4047c5cff06f5296fae4b2bae4b08fb) | `master` = `03776b39f4047c5cff06f5296fae4b2bae4b08fb` (checked out 2026-10-25) | English (`1st1K-eng*.xml`, `opp-eng*.xml`, one `split/` file) and Greek | CC BY-SA 4.0 ([license.md](https://github.com/OpenGreekAndLatin/First1KGreek/blob/master/license.md)) |
| [mgbilby/oga_src](https://github.com/mgbilby/oga_src/tree/v0.2.0) (Zenodo [10.5281/zenodo.15199546](https://doi.org/10.5281/zenodo.15199546)) | tag `v0.2.0` | The 1,999-document list (2,000 files, one split). It is a CoNLL-U derivative of OGA v0.2.0 | CC BY-SA 4.0 (Zenodo record) |

Two repositories were searched. The English in the OGA-covered works is
found only in these two. Other sources (Project Gutenberg, Wikisource, the
Internet Archive) were not searched, and no alignment was measured for them.

## Coverage by OGA work prefix

| OGA prefix | Works | With any English in the two repos |
| --- | ---: | ---: |
| `tlg` | 1,811 | 686 |
| `pta` (Patristic Text Archive) | 182 | 0 |
| `stoa`, `ggm`, `ogl` | 6 | 0 |
| **Total** | **1,999** | **686 (34.3%)** |

Patristic and other `stoa`/`ggm`/`ogl` works have no English in these two
repositories.

## Method

1. Listed the `oga_src` `v0.2.0` tree (`git/trees/v0.2.0?recursive=1`) and
   took the first two dotted fields of each `.conllu` name as the work ID.
2. Listed the full trees of `canonical-greekLit` and `First1KGreek`. Selected
   every file with `eng` in its name (834 files: 788 + 46).
3. Read each TEI header with a namespace-aware parser: the `titleStmt` translator
   (`editor[@role=translator]`), `publicationStmt/availability/licence`, and
   `sourceDesc/.../imprint/date` as the source date.
4. Extracted each file's citation keys: the dotted `n` values of its
   `div[@subtype]` textpart leaves. The Greek is the `grc` file in the same
   directory. A file is "well matching" when at least 90% of the Greek's keys
   appear exactly in the English.
5. Joined to the OGA work list by work ID.

Limits:

- The matching is by exact key string. A file that cites `1.01` where the
  Greek cites `1.1` will not match.
- Keys come from `div` leaves only. 37 OGA-covered English files cite by
  milestone alone (`unit="card"`, `unit="Para"`, `unit="section"`). They are
  not counted as matched.
- Another 37 OGA-covered English files have divisions with no `n` attribute,
  mostly drama and choral divisions. They are excluded.
- Some TEI headers note "not all headers have been checked for accuracy"
  (canonical-greekLit README). Imprint dates and translators are as recorded.
  They were not verified against bibliographic records.
- The survey did not check whether a translation is a reprint or a
  revised edition of an earlier one.

## Translator and date for public-domain status

Public domain is judged from the recorded source imprint date, not the Perseus
release date. The release date is when Perseus digitised the file. Imprint dates
of 1930 or earlier are US public domain in 2026. For other jurisdictions, life
plus 70 needs the translator's death date, which was not checked.

The most common translators among dated-1930-or-earlier files, by file count
(works are the titles seen in the files, not an exhaustive list):

| Translator (as in TEI) | Files | Example works seen |
| --- | ---: | --- |
| Bernadotte Perrin | 66 | Plutarch, *Lives* (e.g. `tlg0007.tlg019`) |
| Henry Watson Fowler and Francis George Fowler | 65 | |
| Austin Morris Harmon | 38 | Lucian (`tlg0062`) |
| Hugh G. Evelyn-White | 36 | Homeric Hymns (`tlg0013`) |
| W.R.M. Lamb | 34 | Lysias (`tlg0540`) |
| Charles Duke Yonge | 31 | Philo (`tlg0018`, First1KGreek, 1854 imprint) |
| Francis Adams | 21 | `tlg0627.tlg004` (1886 imprint) |
| James Herbert Vince | 18 | |

The Brownson file (`tlg0032.tlg006.perseus-eng2.xml`, Xenophon, *Anabasis*)
is also dated-1930-or-earlier. Its translator is Carleton L. Brownson.
Its TEI records `sourceDesc` imprint `1921-1922`, Harvard University
Press and William Heinemann Ltd., and wrapper licence CC BY-SA 4.0. Its
translator is Carleton L. Brownson.

Some dated-1930-or-earlier translations have no licence element on their file.
That file is then covered by the repository README's default CC BY-SA 4.0 "unless
otherwise indicated".

Translations dated after 1930 (`1931` and later) are excluded from the
public-domain counts. Rainbow Missions, Inc. (`2000–2024`) and similar modern
translations are excluded too.

## TEI structure and the Brownson parser

The Brownson file is `TEI/text/body/div[@type=translation]` with
`div[@type=textpart][@subtype=book|chapter|section]` and numeric `@n`. Its
parser (`internal/ancientgreek/brownson.go`) reads only that shape:

| Behaviour in `brownson.go` | Consequence across the OGA-covered English |
| --- | --- |
| `textpartKind` accepts only `book`, `chapter`, `section` | Other textpart kinds (`verse`, `card`, `episode`, `strophe`, `line`, `poem`, `fragment`) are not citation levels. Their text is not separately cited. |
| `emit` returns `cited section without book or chapter` when either ancestor is missing | Section-only files (`section` with no `chapter`) are refused. Of the well-matching files, 478 have only `section` divs and 15 have `book` and `section` only. |
| Citation keys are fixed to `book.chapter.section` | Single-level and two-level Greek keys do not match. |
| Milestone-only text is not read | 37 OGA-covered English files cite by milestone, not by div. |

Divisions used by the 754 OGA-covered English files that have citable divs:

| Divisions used | Files |
| --- | ---: |
| `section` only | 488 |
| `chapter` + `section` | 92 |
| `chapter` + `verse` | 65 |
| `book` + `section` | 23 |
| `book` + `chapter` + `section` | 18 |
| `book` + `chapter` | 10 |
| `chapter` only | 8 |
| other (`fragment`, `poem`, `card`, `tetralogy`, …) | the rest |

Among the well-matching works, the Brownson-shaped subset is small. Only 13
works (0.7%) have a public-domain translation with `book` > `chapter` >
`section` that matches the Greek. Generalising the parser to any depth and
any textpart kind gives 401 works (20.1%) of the same quality. Restricting it
to `book`/`chapter`/`section`/`subsection` gives 386 (19.3%).

**Answer:** the same section parser cannot read them unchanged. It needs the
citation depth and kind taken from the Greek's textpart structure, and it needs
the section-only and milestone cases handled. That is a generalisation of the
existing streaming state machine, not a new format.

## Citation granularity against the Greek

For the 663 files matching at least 90% of the Greek's keys:

- The English adds no keys beyond the Greek's in 95.6% of them (median extra
  keys: 0).
- Divisions match the Greek's citation level, so a sentence-level unit is not
  available. An English section covers the Greek section it matches.
- One file has more than 1.5 times the Greek's keys: `tlg0627.tlg004.perseus-eng2.xml`
  (Francis Adams, 1886 imprint), with 57 English keys against 18 Greek keys.

## Licences

- The Perseus wrapper (`availability/licence`) is CC BY-SA 4.0 on 612 of 834
  files. The remaining 222 have no licence element, and the repository README
  makes CC BY-SA 4.0 the default.
- First1KGreek files carry CC BY-SA 4.0 on 43 of 44 OGA-covered files.
- The wrapper licence attaches to Perseus's encoding. The English text inside
  is public domain only where its imprint is old enough. The current Greek
  card's notices (ADR 0101) name these licences and the pins.

## Findings that bear on the shelf

- A learner's shelf can offer a public-domain English for about one work in
  five (401 of 1,999). The other four in five cards keep the LSJ entry and
  abstain from passage context, as ADR 0101 already allows.
- A second parser is not needed. Generalising `parsePerseusEng2` to the Greek's
  depth and textpart kinds, and to milestone-only files as a separate case,
  would cover the 401 works measured here.
- Each candidate English file can be pinned the way the Brownson file is:
  repository, commit, path, size and SHA-256.
- Open decisions: (a) whether a file's citation depth is read from the English
  or the Greek; (b) how the parser handles a `verse` or `card` textpart that
  the Greek calls `chapter`; (c) how many dated-1930-or-earlier translations
  have translators who died after 1955 (non-US life plus 70).

## Not done

- Non-US public-domain status (translator death dates) was not checked.
- Whether the Perseus CC BY-SA 4.0 wrapper also applies to the public-domain
  English text inside it is a licensing question. It was not resolved.
- Other sources (Project Gutenberg, Wikisource, Internet Archive) were not
  searched.
- Milestone-based alignment was not measured. The 37 milestone-only files are
  counted only as unmatched.
- The `split/` file for `tlg0527.tlg048` (Opp.) was included in the survey but
  not examined by hand.
- Exact-key matching was used. A looser match could raise the figures.
