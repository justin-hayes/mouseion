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
- **Code:** the generator at `issue-2674-agile-lemur@83157a1e`, with the
  thresholds in `internal/classicalmanifest/gates.go`: split-word limit 2%, join
  floor 98%, tier B segment limit 400 words, dictionary floor 90%.

## Committed result

No Latin Work is admitted yet. `make classical-manifest` with the committed
overrides rejects every Latin Work before reading it, because no spot check is
recorded (`<work> has a translation and needs a recorded spot check`). The
committed manifest therefore still holds only the 24 Greek definitions, and
`make verify-classical-manifest` passes.

## Measured gates

To measure gates (i)–(iv), the generator was run once more over the same pins,
outside the repository, with a placeholder spot check on each Work. The
placeholder's result is ignored below. Gate (iii) is reported in two parts:
the join floor, and ADR 0133's tier check on the translation's keying.

| Work | OLA member | Translation | (i) Citations | (ii) Split words | (iii) Join | (iii) Tier | (iv) Dictionary | Result |
|---|---|---|---|---|---|---|---|---|
| `caesar-gallic-war` | `phi0448.phi001.perseus-lat2` | `perseus-eng2` | pass, 60,404 tokens | pass, 52/52,686, 0.099% | pass, 2,845/2,845, 100% | **FAIL**: tier B, 10 of 404 segments over 400 words, largest 750, first at 1.18 | pass, 31,094/33,753, 92.1% | Flagged: tier B limit, clearable by reviewer note |
| `sallust-catiline` | `phi0631.phi001.perseus-lat3` | `perseus-eng2` | pass, 12,922 tokens | pass, 3/10,941, 0.027% | **FAIL**: 681/698, 97.56% | pass, tier A | pass, 7,157/7,552, 94.8% | Flagged: join, clearable by reviewer note |
| `sallust-jugurthine-war` | `phi0631.phi002.perseus-lat3` | `perseus-eng2` | pass, 25,705 tokens | pass, 12/21,824, 0.055% | pass, 1,316/1,316, 100% | pass, tier A | pass, 13,649/14,902, 91.6% | **Passes**, awaiting spot check |
| `cicero-in-catilinam` | `phi0474.phi013.perseus-lat2` | `perseus-eng2` | pass, 14,862 tokens | pass, 9/12,840, 0.070% | pass, 670/670, 100% | pass, tier A | pass, 7,702/8,190, 94.0% | **Passes**, awaiting spot check |
| `cicero-pro-archia` | `phi0474.phi016.perseus-lat2` | `perseus-eng2` | pass, 3,616 tokens | pass, 2/3,169, 0.063% | pass, 144/144, 100% | pass, tier A | pass, 1,793/1,939, 92.5% | **Passes**, awaiting spot check |
| `cicero-pro-milone` | `phi0474.phi031.perseus-lat2` | `perseus-eng1` | pass, 12,618 tokens | pass, 36/10,678, 0.337% | pass, 636/636, 100% | pass, tier A | pass, 6,326/6,804, 93.0% | **Passes**, awaiting spot check |
| `cicero-pro-caelio` | `phi0474.phi024.perseus-lat2` | `perseus-eng1` | pass, 10,141 tokens | pass, 15/8,655, 0.173% | pass, 527/527, 100% | pass, tier A | pass, 5,162/5,561, 92.8% | **Passes**, awaiting spot check |
| `cicero-de-amicitia` | `phi0474.phi052.perseus-lat2` | `perseus-eng2` | pass, 11,270 tokens | pass, 49/9,531, 0.514% | pass, 575/575, 100% | pass, tier A | pass, 5,658/5,984, 94.6% | **Passes**, awaiting spot check |
| `cicero-de-senectute` | `phi0474.phi051.perseus-lat1` | `perseus-eng1` | pass, 9,958 tokens | pass, 54/8,427, 0.641% | pass, 522/527, 99.05% | pass, tier A | pass, 5,066/5,450, 93.0% | **Passes**, awaiting spot check. Gate (ii) passes, so it needs no clearance |
| `tacitus-agricola` | `phi1351.phi001.perseus-lat1` | `perseus-eng4` | pass, 8,083 tokens | pass, 28/6,912, 0.405% | pass, 443/443, 100% | **FAIL**: tier B, 1 of 46 segments over 400 words (chapter 18, 469) | pass, 4,409/4,850, 90.9% | Flagged: tier B limit, clearable by reviewer note |
| `tacitus-germania` | `phi1351.phi002.perseus-lat1` | `perseus-eng2` | pass, 6,746 tokens | pass, 49/5,733, 0.855% | pass, 459/459, 100% | **FAIL**: tier B, 1 of 46 segments over 400 words (chapter 45, 460) | pass, 3,601/3,869, 93.1% | Flagged: tier B limit, clearable by reviewer note. Gate (ii) passes, so it needs no clearance |
| `suetonius-julius` | `phi1348.abo011.perseus-lat2` | `perseus-eng2` | pass, 11,530 tokens | pass, 34/10,018, 0.339% | **FAIL**: unreadable (D5) | not measured | **FAIL**: 5,535/6,410, 86.3% | **Dropped out** (gate iv, not cleared; Justin's decision). Removed from the overrides |
| `suetonius-augustus` | `phi1348.abo012.perseus-lat2` | `perseus-eng2` | pass, 16,372 tokens | pass, 55/14,263, 0.386% | **FAIL**: 732/761, 96.19% | pass, tier A | pass, 8,337/9,237, 90.3% | Flagged: join, clearable by reviewer note |

Notes on the readings:

- *De Senectute*'s translation (`perseus-eng1`) marks section 35 with two
  section milestones. The reader merges both into section 35; that is the
  reading measured above (orchestrator decision).
- *In Catilinam*'s translation is keyed speech.section, like the Latin, so it
  is tier A. Editorial textparts (`argument`, `intro`, `chronology`, `note`)
  are skipped in every translation.
- The tier B segment counts are the generator's (segment text, whitespace-split
  words).

Alternatives measured and not pinned:

- **Sallust OLA `perseus-lat4`.** Its gates match `lat3`:
  - *Catiline*: join 688/705 (97.59%), dictionary 95.0%.
  - *Jugurthine War*: join 1,321/1,321, dictionary 92.4%.

  `lat3` is pinned because its CTS record names the edition (Ahlberg). The
  record for `lat4` has no bibliographic information.
- **Agricola, Murphy's abridged `perseus-eng3` (1883).** Join only 266/443
  (60.0%), and tier B with 1 of 29 segments over 400 words (chapter 18, 515).
  Church and Brodribb's `perseus-eng4` (1877) is pinned.
- **Life of Julius Caesar, Rolfe's `rolfe-eng1` (1914).** Unreadable for the
  same reason as `perseus-eng2` (D5), and it is not a `perseus-engN` file. The
  Work has dropped out on gate (iv) in any case.

## Reader defects

- **D5. Finer translation.** OLA cites the *Divus Iulius* by chapter only, but
  both translations are keyed chapter.section. A translation finer than the
  Latin is neither tier A nor tier B (ADR 0133), and the reader refuses it
  ("textpart deeper than citation depth"). It affects only *Life of Julius
  Caesar*, which has dropped out on gate (iv), so it is not being fixed for
  this batch.

## Tier eligibility

ADR 0133's tier B, a translation keyed one level above the Latin, allows
segments of at most 400 words. The generator now checks this in gate (iii)
(the "Tier" column above); a failure can be cleared by a reviewer note.
Three pinned translations are tier B and each has segments over the limit:
*The Gallic War* (10 of 404, largest 750), *Agricola* (1 of 46, 469) and
*Germania* (1 of 46, 460). No `perseus-engN` alternative at the pin is keyed
at the Latin's depth. Every other pinned translation is tier A.

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

1. Justin decides each flagged Work: his reviewer note clears it, or it
   drops out.
   - Join under 98%: *Catiline* (97.56%) and *Augustus* (96.19%).
   - Tier B segments over 400 words: *The Gallic War*, *Agricola* and
     *Germania*.
2. Justin records each author's spot check (`spotCheck` in each Work's
   override).
3. Regenerate the manifest, then do the deployment check: prepare one real
   Work and confirm that Reading and a deck work.
