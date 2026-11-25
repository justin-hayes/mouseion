# ADR 0132: Classical Latin from Opera Latina Adnotata

Status: **Accepted** · Date: 2026-11-25 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided by Justin in the closing
grilling of the map
[Wayfinder: classical Latin in Mouseion](https://github.com/justin-hayes/mouseion/issues/2658)
([resolution](https://github.com/justin-hayes/mouseion/issues/2671#issuecomment-7164215607)).
This is Latin's counterpart to
[ADR 0099](0099-ancient-greek-imported-analyses.md) and
[ADR 0122](0122-ancient-greek-native-agdt-syntax.md). It amends
[ADR 0116](0116-classical-catalog.md). Card behaviour for Latin is decided
separately, in [ADR 0133](0133-per-language-card-policy.md).

## Context

[ADR 0110](0110-philological-reading-competence-boundary.md) names Latin as a language
Mouseion builds toward. Ancient Greek arrived without Mouseion running an
analyser: OGA publishes whole-corpus lemmas, morphology and AGDT syntax, and
Mouseion imports them (ADR 0099). The same constraint holds for Latin. A source
whose only route needs an analyser run by Mouseion (Stanza `la`, LatinCy, CLTK,
UDPipe) counts as unavailable, whatever its licence under
[ADR 0111](0111-third-party-licence-posture.md).

The map's research found one route that meets the constraint and covers
classical prose at scale:

- **Hand-annotated treebanks are too small or too restricted.**
  UD_Latin-CIRCSE (CC BY-SA 4.0, 29,055 tokens) covers the *Germania* and four
  Seneca tragedies. LASLA, PROIEL and UD Perseus are NC. LDT's licence is
  unclear.
- **Opera Latina Adnotata (OLA) v0.2.0** publishes LatinPipe parses in the AGDT
  scheme for 221 classical Works (4.54M words), with the Perseus Latin text and
  a PAULA Citation layer. It declares CC BY-SA. The model's training lineage
  through LDT is unverified, exactly as OGA's is. On held-out LDT text its LAS
  is 81.6.
- **The scheme fits the stored stack.** OLA's AGDT annotation fits the
  `native_agdt` storage and construction interface of ADR 0122 at its seams.
  The only additions are a Latin adapter that reads the `_CO`, `_AP` and `_ExD`
  suffixes, and a fetch for archive members. No migration is needed.
- **OLA ships as one archive.** Each Work is a CoNLL-U member and a PAULA member
  inside a 3.9 GB zip on Zenodo. Zenodo answers HTTP range requests on it
  (checked 2026-11-25), so one Work's members can be fetched without downloading
  the archive.

## Decision

### Source, pin and eligibility

**Classical Latin comes from OLA v0.2.0**, pinned to Zenodo
`10.5281/zenodo.15183688` (`opera_latina_adnotata_v0.2.0.zip`, 3,857,914,226
bytes) and cross-referenced to GitHub tag `5cd6b06`.

**Eligible Works are the 205 classical Works whose text is CC BY-SA.**
- The 16 classical Works with NC-SA text are not Source content and are
  excluded. Among them are Tacitus's *Annales*, Velleius and Varro's *De lingua
  Latina*.
- CIRCSE, LDT, LASLA and the Scaife UD shards are not sources.
- **Classical** has no lower bound and runs to about the 2nd century AD.
  Apuleius and Gellius are admitted. The *Historia Augusta*, Tertullian and
  later Christian authors, Servius and Ausonius are not. A pseudepigraphon
  counts with its attributed author.

**The first slice is prose only.** Verse waits for basic poetry support and
[ADR 0103](0103-source-structure-and-poetry-evidence.md). So does a prose Work that mixes verse
into its text, such as the *Satyricon*.

### Licence posture (ADR 0111, unchanged)

- **Annotation.** OLA's lemmas, POS, morphology and syntax form an **Analysis
  resource** with restriction `unclear`. They are admitted on OGA's terms: the
  release ships no annotations, and the deployer fetches them lazily when a
  learner prepares a Book. The layer gets a deployer-register row and a source
  notice.
- **Text.** The Latin text (Perseus `canonical-latinLit`, `Latin` and
  `priapeia`) is CC BY-SA **Source content**. As for Greek, it may carry each
  Work's Opening on the catalog index.

### Scheme

- **Stored scheme.** Latin is stored under `syntax_state = 'native_agdt'`, with
  the UD consumers off, as for Greek (ADR 0122).
- **Latin adapter.** The adapter behind the construction interface reads
  OLA's relation suffixes (`_CO`, `_AP`, `_ExD`) as the base category plus a
  conjunct flag, at read time. The stored relations stay verbatim.
- **CIRCSE** is measurement gold only. It is never shipped or served.
- **Constructions.** Latin Constructions stay fail-closed until a Latin
  Construction meets the ADR 0125 bar. The ablative absolute measured 0.82
  strict agreement (Wilson 0.72–0.90), so it is not yet demonstrated.

### Source format and fetch

- **Source format.** A new source format, `ola-conllu`, sits beside
  `oga-conllu`. Preparation chooses its adapter by source format, as ADR 0116
  requires.
- **Pin.** An `ola-conllu` source pin names **two archive members** per Work:
  the CoNLL-U and the PAULA Citation layer. Each member is pinned by its offset,
  size, CRC-32 and SHA-256 within the pinned archive.
- **Fetch.** A new **archive-range** fetch kind in the layer pins retrieves each
  member with HTTP range requests. A server that does not honour the range, or
  a member that fails its pin, fails preparation exactly as a pin mismatch does
  for OGA. An unreachable server gives OGA's retryable fetch error.
- **Interpretation version.** Latin is prepared under its own interpretation
  version, working name `ola-agdt-syntax/1`.

### Amendments to ADR 0116

ADR 0116 is written for Ancient Greek. Latin changes four things:

1. **A Definition carries its language from a set.** The manifest accepts every
   classical language, and Add records each language's display name. Latin is
   study language `la`. The Catalog's language filter and Classical section
   cover every classical language in the manifest.
2. **`greekTitle` becomes `originalTitle`** in the manifest and the overrides.
   It is rendered with `lang` set to the Definition's language and shown only
   when it differs from the English title. The `greek_title` database column
   keeps its name, so no migration is needed.
3. **The Citation is widened.** A component is a number with an optional letter
   suffix, or a named section from a closed, reviewed list (`pr`, `ep` and so
   on) with a defined order. Livy `1.pr.1` and `331a` are therefore Citations.
   Inventing numbers for named sections, or refusing such Works, was rejected.
4. **The generator gates each Latin Work.** A Work is admitted only if all of
   the following hold:
   - every token has a Citation, and Citations never run backwards;
   - a split-word check stays under a threshold;
   - its reference translation passes the join and spot check of ADR 0133;
   - it passes the dictionary-match release check of ADR 0133.

   A flagged Work that Justin clears by reading carries a reviewer note. A Work
   that fails a gate drops out and is not replaced.

ADR 0128 holds unchanged. Livy is one Book of about 530,000 words, admitted only
behind a scale check of preparation, Reading, Browse and the deck at that size.

## Considered options

- **A UD route** (the Scaife shards, or CIRCSE served directly). Rejected as the
  primary route. The Scaife shards' LAS is 37.3. CIRCSE has five Works, none of
  them prose beyond the *Germania*, and carries no Citation. The Scaife shards
  remain the documented contingency if OLA is withdrawn, with lemmas and
  morphology only.
- **Running an analyser for Latin.** Excluded by the map's hard constraint, the
  same one that postponed the graded-reader proposal for Greek.
- **Downloading the whole OLA archive on first use.** Rejected: 3.9 GB to read
  one Work, when range requests deliver the two members directly.
- **Treating OLA's lineage as worse than OGA's.** Not supported by the evidence:
  both are model output with an unverified link to LDT or AGDT training data.
  A finding to the contrary reopens this ADR.

## Consequences

- **No migration.** Latin needs no schema change. The catalog, preparation and
  Constructions work gains a second classical language without a new table.
- **Greek is unchanged.** Greek behaviour must not change. The neutral
  `originalTitle` and a language-neutral preparation error replace
  Greek-specific names only where the code is shared. The `ancientgreek`
  package name and the `greek_title` column stay.
- **Availability.** A Latin Book depends on Zenodo for preparation, as a Greek
  Book depends on GitHub for OGA. Once prepared, it works with the source
  offline. Because no NLP capability applies, a Latin Book never counts as
  outdated in analysis updates.
- **Reopening.** This ADR is reopened by any of:
  - a ruling that OLA's lineage is worse than OGA's;
  - OLA being withdrawn from Zenodo;
  - the PAULA Citation join failing for most prose Works;
  - a rights finding against the Perseus text;
  - the split-word gate flagging most of the first batch.

## Related

- [ADR 0099: Ancient Greek imported analyses](0099-ancient-greek-imported-analyses.md)
- [ADR 0110: Bound Mouseion by philological reading competence](0110-philological-reading-competence-boundary.md)
- [ADR 0111: One licence posture for third-party data in every language](0111-third-party-licence-posture.md)
- [ADR 0116: A curated Classical catalog](0116-classical-catalog.md) — amended
- [ADR 0122: Ancient Greek syntax is stored as native AGDT](0122-ancient-greek-native-agdt-syntax.md)
- [ADR 0125: Construction parallels in Concordance](0125-construction-parallels-in-concordance.md)
- [ADR 0128: A classical Book is a whole Work in one Edition](0128-a-classical-book-is-a-whole-work.md)
- [ADR 0133: Each language has one card policy](0133-per-language-card-policy.md)
