# Latin batch 1: generator gate report

This is the gate report for the first Latin batch
([#2686](https://github.com/justin-hayes/mouseion/issues/2686), spec
[#2674](https://github.com/justin-hayes/mouseion/issues/2674)). It records
ADR 0132's four gates for each candidate Work, run on the real pinned archive.
It holds aggregates and IDs only. No source or translation text appears here.

Run on 2026-11-25. The inputs:

- **OLA:** v0.2.0, `opera_latina_adnotata_v0.2.0.zip` (Zenodo
  `10.5281/zenodo.15183688`, 3,857,914,226 bytes). Every byte the generator reads
  was fetched by HTTP Range request into a sparse local copy: the outer and inner
  zip directories and each Work's two members. Each member's CRC-32 matched its
  directory.
- **Translations:** `PerseusDL/canonical-latinLit@8cef5162`, the ADR 0133 pin.
- **Lewis & Short:** derived from `PerseusDL/lexica@56061ca1`
  `lat.ls.perseus-eng1.xml` with `nlp/scripts/derive_dictionary_index.py
  --ls-dir`.
- **Code:** the generator at `issue-2674-agile-lemur@4543b766`, with the
  thresholds in `internal/classicalmanifest/gates.go`: split-word limit 2%, join
  floor 98%, dictionary floor 90%.

## Committed result

No Latin Work is admitted yet. `make classical-manifest` with the committed
overrides rejects every Latin Work before reading it, because no spot check is
recorded (`<work> has a translation and needs a recorded spot check`). The
committed manifest therefore still holds only the 24 Greek definitions, and
`make verify-classical-manifest` passes.

## Measured gates

To measure gates (i)–(iv), the generator was run once more over the same pins,
outside the repository, with a placeholder spot check on each Work. The
placeholder's result is ignored below. Some Works also show a scratch result in
brackets. That run read the files with the reader defects listed under "Reader
defects" worked around in memory. It is evidence for those fixes, not a gate
result.

| Work | OLA member | Translation | (i) Citations | (ii) Split words | (iii) Join | (iv) Dictionary | Result |
|---|---|---|---|---|---|---|---|
| `caesar-gallic-war` | `phi0448.phi001.perseus-lat2` | `perseus-eng2` | **FAIL**: unreadable (D1) [60,404 tokens, 0 backwards] | [52/52,686, 0.099%] | [2,845/2,845, 100%] | [31,094/33,753, 92.1%] | Blocked by D1. Also fails tier B (T1) |
| `sallust-catiline` | `phi0631.phi001.perseus-lat3` | `perseus-eng2` | pass, 12,922 tokens | pass, 3/10,941, 0.027% | **FAIL**: unreadable (D2) [681/698, 97.56%] | pass, 7,157/7,552, 94.8% | Blocked by D2, then a clearable join flag |
| `sallust-jugurthine-war` | `phi0631.phi002.perseus-lat3` | `perseus-eng2` | pass, 25,705 tokens | pass, 12/21,824, 0.055% | pass, 1,316/1,316, 100% | pass, 13,649/14,902, 91.6% | **Passes**, awaiting spot check |
| `cicero-in-catilinam` | `phi0474.phi013.perseus-lat2` | `perseus-eng2` | **FAIL**: unreadable (D1) [14,862 tokens, 0 backwards] | [9/12,840, 0.070%] | [unreadable (D2, then D4)] | [7,702/8,190, 94.0%] | Blocked by D1, D2 and D4 |
| `cicero-pro-archia` | `phi0474.phi016.perseus-lat2` | `perseus-eng2` | pass, 3,616 tokens | pass, 2/3,169, 0.063% | **FAIL**: unreadable (D2) [144/144, 100%] | pass, 1,793/1,939, 92.5% | Blocked by D2 |
| `cicero-pro-milone` | `phi0474.phi031.perseus-lat2` | `perseus-eng1` | pass, 12,618 tokens | pass, 36/10,678, 0.337% | **FAIL**: unreadable (D3) | pass, 6,326/6,804, 93.0% | Blocked by D3 |
| `cicero-pro-caelio` | `phi0474.phi024.perseus-lat2` | `perseus-eng1` | pass, 10,141 tokens | pass, 15/8,655, 0.173% | **FAIL**: unreadable (D3) | pass, 5,162/5,561, 92.8% | Blocked by D3 |
| `cicero-de-amicitia` | `phi0474.phi052.perseus-lat2` | `perseus-eng2` | pass, 11,270 tokens | pass, 49/9,531, 0.514% | pass, 575/575, 100% | pass, 5,658/5,984, 94.6% | **Passes**, awaiting spot check |
| `cicero-de-senectute` | `phi0474.phi051.perseus-lat1` | `perseus-eng1` | pass, 9,958 tokens | pass, 54/8,427, 0.641% | **FAIL**: unreadable (D3) | pass, 5,066/5,450, 93.0% | Blocked by D3. Gate (ii) passes, so it needs no clearance |
| `tacitus-agricola` | `phi1351.phi001.perseus-lat1` | `perseus-eng4` | **FAIL**: unreadable (D1) [8,083 tokens, 0 backwards] | [28/6,912, 0.405%] | [443/443, 100%] | [4,409/4,850, 90.9%] | Blocked by D1. Also fails tier B (T1) |
| `tacitus-germania` | `phi1351.phi002.perseus-lat1` | `perseus-eng2` | **FAIL**: unreadable (D1) [6,746 tokens, 0 backwards] | [49/5,733, 0.855%] | [459/459, 100%] | [3,601/3,869, 93.1%] | Blocked by D1. Also fails tier B (T1). Gate (ii) passes, so it needs no clearance |
| `suetonius-julius` | `phi1348.abo011.perseus-lat2` | `perseus-eng2` | pass, 11,530 tokens | pass, 34/10,018, 0.339% | **FAIL**: unreadable (D5) | **FAIL**: 5,535/6,410, 86.3% | **Fails gate (iv)**: drops out unless cleared |
| `suetonius-augustus` | `phi1348.abo012.perseus-lat2` | `perseus-eng2` | pass, 16,372 tokens | pass, 55/14,263, 0.386% | **FAIL**: unreadable (D2) [732/761, 96.19%] | pass, 8,337/9,237, 90.3% | Blocked by D2, then a clearable join flag |

Alternatives measured and not pinned:

- **Sallust OLA `perseus-lat4`.** Its gates match `lat3`:
  - *Catiline*: dictionary 95.0%, scratch join 97.59%.
  - *Jugurthine War*: join 1,321/1,321, dictionary 92.4%.

  `lat3` is pinned because its CTS record names the edition (Ahlberg). The
  record for `lat4` has no bibliographic information.
- **Agricola, Murphy's abridged `perseus-eng3` (1883).** Scratch join only
  266/443 (60.0%). Church and Brodribb's `perseus-eng4` (1877) is pinned.
- **Life of Julius Caesar, Rolfe's `rolfe-eng1` (1914).** Unreadable for the
  same reason as `perseus-eng2` (D5), and it is not a `perseus-engN` file.

## Reader defects

These defects stop a Work before its gates can measure it. They are not source
failures, so a Work blocked only by them has not dropped out under ADR 0132.

- **D1. Underscored Citations.** OLA's PAULA layer writes multi-level Citations
  with underscores (`1_1_1`). The adapter (`parsePaulaCitations` →
  `classical.ParseCitation`) accepts only dots, so every multi-level Work is
  refused as a malformed Citation layer. This affects preparation as well as
  the generator. Single-level Works are unaffected.
- **D2. Editorial textparts.** A translation with an editorial textpart
  (`n="argument"`, `intro`, `chronology` or `note`) is refused outright with
  "textpart number is not a Citation component".
- **D3. Milestone-keyed translations.** At this pin, the `perseus-eng1` files
  for *Pro Milone*, *Pro Caelio* and *De Senectute* mark sections with
  `<milestone unit="section">` and have no textpart divs. The reader finds "no
  cited sections".
- **D4. Speech-keyed translation.** The *In Catilinam* translation numbers
  its sections inside `subtype="speech"` textparts. The reader first refuses
  the editorial `n="argument"` textpart (D2). With that removed, it still
  finds a speech textpart without a cited section ("textpart has no cited
  section"), because it does not read the speech level.
- **D5. Finer translation.** OLA cites the *Divus Iulius* by chapter only, but
  both translations are keyed chapter.section. A translation finer than the
  Latin is neither tier A nor tier B (ADR 0133), and the reader refuses it
  ("textpart deeper than citation depth").

## Tier eligibility

- **T1. Segments over 400 words.** ADR 0133's tier B, a translation keyed one
  level above the Latin, allows segments of at most 400 words. Gate (iii)
  measures join coverage only, not this limit. The bracketed 100% joins above
  therefore do not make these pins admissible. Word counts below are of
  segment text with footnotes left out, from a scratch reader:

  | Work | Translation | Segments | Over 400 words | Largest |
  |---|---|---|---|---|
  | *The Gallic War* | `perseus-eng2` (book.chapter) | 404 | 10 (first 1.31, 1.40, 7.77, 1.44) | 750 |
  | *Agricola* | `perseus-eng4` (chapter) | 46 | 1 (18) | 457 |
  | *Germania* | `perseus-eng2` (chapter) | 46 | 1 (45) | 460 |

  Every other pinned translation is keyed like its Latin (tier A), apart
  from *Life of Julius Caesar*, whose translation is finer than the Latin
  (D5). A reviewer decides whether these Works enter or drop out. No
  `perseus-engN` alternative at the pin is keyed at the Latin's depth.

## Lewis & Short citation author references

These figures come from the derived index and the pinned TEI. The filter is
the card's Work-author citation filter, `cardexport.latinAuthorMatches`.

- **The whole index.** It holds 376,678 citations, and 360,509 of them (95.7%)
  carry an author reference: an abbreviation, an "id." resolved within the
  entry, or a CTS URN author.
- **The batch's authors.** This is a proxy for the usable-reference share,
  measured on the TEI. For each author it counts the `<bibl>` elements whose
  CTS URN names the author's textgroup. It then counts how many of those carry
  an author reference the card's filter keeps, with "id." resolved as the
  derivation resolves it. "ib." and "ibid." are not resolved, so the filter
  drops them:

  | Author | Textgroup | `<bibl>` elements naming it | Usable and kept | Share |
  |---|---|---|---|---|
  | Caesar | `phi0448` | 8,017 | 7,702 | 96.1% |
  | Sallust | `phi0631` | 4,179 | 4,174 | 99.9% |
  | Cicero | `phi0474` | 63,319 | 63,083 | 99.6% |
  | Tacitus | `phi1351` | 8,837 | 8,832 | 99.9% |
  | Suetonius | `phi1348` | 8,404 | 8,400 | 100.0% |

- **Citations the filter keeps per author** across all citations in the index,
  including those with no URN: Caesar 9,125; Sallust 4,190; Cicero 70,830;
  Tacitus 10,353; Suetonius 8,694.

The author-prefix rule is enough for this batch. Its known gap, an
abbreviation that is not a prefix of the author's name (such as `Verg.` for
Virgil), does not affect any of these five authors.

## Still to do

1. Fix D1–D3 (and D4 and D5 if those Works are to enter), then rerun.
   *Gallic War*, *Agricola* and *Germania* also need a tier decision (T1).
2. Justin records each author's spot check (`spotCheck` in each Work's
   override), and any reviewer notes:
   - *Catiline* and *Augustus* if their join stays under 98%;
   - *Life of Julius Caesar* for gate (iv), or it drops out.
3. Regenerate the manifest, then do the deployment check: prepare one real
   Work and confirm that Reading and a deck work.
