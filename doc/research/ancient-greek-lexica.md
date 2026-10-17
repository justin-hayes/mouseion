# Open Ancient Greek lexica for Anabasis Book 1

Findings for [Research: openly licensed Ancient Greek lexica for contextual word sense](https://github.com/justin-hayes/mouseion/issues/2133),
feeding [Prototype: word sense in context for Ancient Greek without an LLM](https://github.com/justin-hayes/mouseion/issues/2130)
under [Ancient-language cards without an LLM: public-domain translations, alignment and word sense](https://github.com/justin-hayes/mouseion/issues/2127).
Sources checked 2026-10-17. This is source discovery and a preliminary lookup
measurement, **not acceptance of a lexicon or validation of contextual sense selection**.
No production behaviour changes. Research assets stay on an unmerged research branch.

## Recommendation for the prototype

- **Test pinned English Wiktionary/Kaikki raw `grc` data as the baseline.** It supplies
  explicit POS, English glosses, occasional native sense identifiers, usage tags,
  inflections and quotations. Preserve its source entries and full gloss breadcrumbs:
  today's compact Mouseion index loses information needed for this experiment.
- **Test Perseus LSJ as the citation-rich comparison**, at
  `PerseusDL/lexica@56061ca127f4a2844980baffc5f2b6d1332897b3`.
  Its explicit component grant permits distribution under CC BY-SA 4.0, with the
  additional notices/offer-back wording below. It is the strongest inspected source
  for attributable passage evidence. Do not treat this investigation as a clearance
  of the underlying 1940 edition in every jurisdiction or permission to change exports.
- **Do not select Middle Liddell for the unrestricted-data path yet.** Its XML says
  non-commercial, whereas its institutional web page and third-party fork say BY-SA.
  Preserve the pin for comparison/rights clarification; do not silently choose the
  most permissive statement. DGE's inspected online edition is also NC. The inspected
  Bailly derivative is NC-ND; rights in the original digitisation remain unconfirmed.
- **Never identify an OGA homograph from equal dictionary digits.** Keep unresolved
  candidates separate. No chosen-sense accuracy, abstention quality, or human sense
  labels were measured here. The prototype must obtain its own live human sense review.

Exact input hashes, full pins and commands: [SOURCES.md](ancient-greek-lexica/SOURCES.md).
Print/digital rights and export obligations: [NOTICE.md](ancient-greek-lexica/NOTICE.md).

## Editions, digitisations and rights

| Candidate | Edition and machine-readable witness | Applicable evidence and disposition |
|---|---|---|
| English Wiktionary / Kaikki | Continuously edited English Wiktionary, Ancient Greek language section, not a historical print edition. Raw Wiktextract JSONL: dump 2026-09-02, extraction 2026-10-03; 68,196 top-level `grc` records. Pin compressed input SHA-256 `3dac8a09…5dbdb`, derived subset `65c795d9…7a12`, filter script and extractor pins in SOURCES. | [Kaikki's own raw-data page][kaikki] identifies the extraction, format and provenance. [WMF terms §7][wmf] give CC BY-SA 4.0 and GFDL for original contributions, with exceptions for imported/quoted material. Use the CC BY-SA route; do not assume every imported passage is also GFDL. Suitable for personal lookup and attributed, share-alike redistribution of covered definitions. Quoted translations need separate provenance checks. |
| Perseus LSJ | *A Greek-English Lexicon*, Liddell/Scott, revised by H. S. Jones with R. McKenzie, Clarendon Press, 1940; ninth-edition witness, not a claim to the later Supplement. [TEI header and source text][lsj1]. 27 TEI P4 files, Beta Code headwords, 111,895 main entries before excluding 311 unsupported keys. | [Component README][lsj-rights] explicitly says "freely distributed" under CC BY-SA 4.0; preserve its credit/availability statement and offer modifications to Perseus. [Repository README][perseus-rights] also contains general personal-use/varying-component-copyright wording; [licence text][perseus-license] is BY-SA 4.0. Explicit digital grant supports testing and conditional reuse, not an independent global print-rights determination. |
| Perseus Middle Liddell | *An Intermediate Greek-English Lexicon*, Liddell/Scott, Clarendon Press, 1889, [institutional witness][ml-web]. TEI P4 copied in `blinskey/middle-liddell@5db8d88a20066e182ca3fc6ee758c9f61ac60603`, [XML][ml-xml], 30,676 main entries before 18 unsupported keys are excluded. The pinned PerseusDL/lexica tree does **not** contain it. | Old print text and modern transcription are distinct. The XML availability statement requires non-commercial use, credit, preservation and offer-back. The [institutional page][ml-web] and [fork licence][ml-license] instead say CC BY-SA 3.0 US plus offer-back. Not cleared for this map's non-NC path or redistributed decks until reconciled. |
| DGE (CSIC) | *Diccionario Griego–Español*, modern Greek–Spanish scholarly dictionary, [online edition][dge]; not an English-gloss drop-in. [Digitisation credits][dge-credits] identify XML encoding by Juan Rodríguez Somolinos and Sabine Arnaud-Thuillier, technical work by Frédéric Glorieux. | [First-party licence][dge-license] states BY-NC-SA 3.0 Spain. Direct XML directory returned 401; XML existence is not an open downloadable release. Exclude this online witness; no dictionary pin, entry extraction or Book 1 coverage claim. No separate open grant for the underlying modern print text was established. |
| Bailly 2020 | Greek–French revision/digitisation of **1935** Bailly, not merely the 1895 first edition. [Project page][bailly-project] names the modern revisers and revised etymologies/toponymy; links a [StarDict adaptation][bailly-stardict], advertised v4.7, Nov. 2025 when inspected. | [Latin-dict's conversion page][bailly-conversion] describes conversion from that StarDict and licenses **its work** NC-ND 4.0. That is primary evidence for this derivative, not proof all Bailly witnesses have that licence. No explicit unrestricted grant was found on the inspected originating pages. Exclude the derivative, leave original digitisation uncleared. No full dictionary download/pin or measured Book 1 coverage. |

Pape and Passow are plausible German-language alternatives but were not assessed;
no licence or coverage is claimed. Autenrieth/Cunliffe are Homer-focused rather than
the first comparison for Xenophon's Attic prose. Logeion and Scaife are access platforms,
not blanket licences for every dictionary they display. Their fetched pages did not
establish a new, pinned, unrestricted lexicon here. This is a shortlist, not an
exhaustive survey or evidence that unassessed sources are unavailable.

## What the entries actually provide

These observations are from the exact input bytes in SOURCES, not inferred from the
size of a website. Native IDs below are meaningful **only with their source pin**.

### Wiktionary / raw Wiktextract

- `word`, `lang_code`, `pos`, optional `etymology_number` and `etymology_text` separate
  entries. Forms retain tags such as voice/tense/person/case; canonical/romanised forms
  must not all be treated as equivalent Greek headwords.
- `senses[]` contains `glosses`, often `raw_glosses`, `tags`, topics and examples.
  Multiple gloss strings can be **ancestor-to-child breadcrumbs**, not synonyms.
  Native `senseid` is optional, not a universal stable key. Preserve it where present;
  otherwise use pinned record ordinal plus sense ordinal/path, not gloss text alone.
- In the raw `grc` subset, lines 757–758 are two `λέγω` verb records with no etymology
  number: one has sense ID `grc:gather`, the other `grc:say`. Missing etymology numbers
  do not imply a merged lexeme or absence of all source discrimination.
- Lines 684–686 contain `δέω` etymologies 1 (bind), 2 (lack), 3 (alternative form).
  Etymology 2 includes genitive/infinitive construction information in `raw_glosses`.
  These are evidence for a crosswalk, not proof those numbers equal `morpheus:N`.
- Line 248, `βάρβαρος`, has gloss paths from "non-Greek-speaking; foreign" to
  "non-Greek peoples" to "Medes or Persians", with `in-plural` and dialect tags.
  An example has `ref` containing `Xenophon, Anabasis 1.2.14`, Greek text and English
  translation. The reference does not identify the translator or establish rights
  in that English wording. [Live entry][wikt-barbaros] is navigational, not the dump pin.
- Line 207, `ἀρχή`, is a noun entry with distinct glosses for beginning and authority.
  Source usage/inflection tags can constrain candidate readings, but do not determine
  which meaning an occurrence expresses.

### Perseus LSJ

- The [TEI header][lsj1] names Tufts/Perseus, Gregory Crane, supervisors Lisa
  Cerrato and David Smith, NEH funding, and a machine-readable release date of
  September 1995. Its revision history records later corrections and the 2014
  conversion of many bibliography IDs to CTS URNs; the repository commit, not
  the 1995 date, pins this particular digitisation.
- `<entryFree id key type="main">`, `<orth>`, homograph-suffixed Beta Code `key`,
  and `<sense id n level>` provide native entry and sense identity. Preserve both
  physical XML containment and `level`/`n`: hierarchy can be represented by ordered
  sibling sense elements, not just XML nesting.
- English cues occur in `<tr>` among running discussion, Greek examples and references;
  extracting every `<tr>` descendant as independent senses would duplicate/misassign
  evidence. `<gramGrp>/<gram type="voice|dialect">`, tense and other grammatical tags,
  `<lbl>`, and prose provide labels/construction evidence.
- **POS exists but is sparse, not a complete UPOS column.** The inspected files contain
  9,733 `<pos>` tags (for example `Adv.` and `Adj.`); a tag may describe a local use.
  The measurement below deliberately does not map them. Missing `<pos>` is unknown,
  not proof of a noun/verb identity or a licence to infer POS from an English gloss.
- [File 1][lsj1], `a)rxh/`, entry `n15893`, has `n15893.0` (`n="A"`, `level="1"`)
  glossed "beginning, origin" and later numbered/Roman-letter senses.
  [File 4][lsj4] has `de/w1` / `de/w2`, entries `n24292` / `n24293`, with sense IDs,
  tense paradigms, dialect and voice notes. [File 12][lsj12] has `le/gw1` / `le/gw2`;
  these are not a two-row substitute for Wiktionary's gather/say split.
- File 1, `a)yi/nqion` (wormwood), has a `<bibl n>` value
  `urn:cts:greekLit:tlg0032.tlg006.perseus-grc1:1:5:1`.
  `<author>`, `<biblScope>`, `<cit>/<quote>` and a containing sense can provide
  attributable Xenophon evidence. Some references are shorthand or not CTS URNs;
  matching work/book/section is not exact token correspondence or a verified sense.

### Middle Liddell

- The [header][ml-xml] credits Tufts/Perseus, Gregory Crane, supervision by Lisa
  Cerrato, William Merrill, Elli Mylonas and David Smith, and Annenberg CPB funding.
  Its history includes P4/XML conversion in 2004 and later corrections; the fork's
  commit and XML digest pin the inspected copy, not a claimed new Perseus release.
- [Pinned XML][ml-xml] has `<entry key id>`, `<form>/<orth>`, `<etym>`,
  `<sense level n id>`, `<trans>/<tr>` and `<usg>` labels. Its 1,011 `<pos>` tags are
  sparse too; it is incorrect to say the dictionary has no POS field at all.
- `a)rxh/`, entry `n5059`: `n5059.0` (`I`, level 2) is beginning; `n5059.2`
  (`II`, level 2) is power/command; lower-level numbered uses follow.
- `de/w1`, `n7542`, includes binding and a middle/passive subsection; `de/w2`,
  `n7543`, distinguishes lacking and `de/omai`, with genitive/infinitive notes.
- Xenophon is commonly a bare `<usg>Xen.</usg>` (6,831 such tags contain `Xen`);
  the measurement does not resolve these to a work, book or passage. No machine-readable
  Book 1 passage links were established, so **Book 1 citation coverage is unmeasured**,
  not zero. Do not assume an author-level mention supports this occurrence's meaning.

## OGA matching contract worth testing

1. Keep the full OGA identity `(grc, canonical lemma, UPOS, lexical discriminator)`
   and the original citation headword/morphology. A bare lemma remains separate from
   a numbered sibling; no accents/breathings/iota subscripts are discarded.
2. Derive dictionary candidate keys with `ancient-greek/1` in the real prototype.
   Convert Beta Code explicitly and report unsupported editorial notation; do not
   silently strip unknown characters and call the result an exact match.
3. Keep dictionary identity **separate**: `(source pin, file/record, entry ID/key,
   native sense ID/path)`. Wiktionary etymologies, LSJ entry digits, Middle Liddell
   digits and OGA `morpheus:N` are different schemes. Equal digits are unverified.
4. Use explicit, appropriately scoped POS and grammatical evidence as constraints.
   Map OGA morphology through documented rules; voice, case government and plural-only
   uses may narrow evidence, but are neither complete nor a semantic gold label.
   Do not infer construction facts from the production run's placeholder dependency
   skeleton: ADR 0099 records syntax as absent.
5. An explicit reviewed crosswalk can connect homographs when source evidence supports
   it. Otherwise return **separate unresolved candidates**, or abstain. Even a unique
   retrieved entry is only a headword candidate, not verified homograph equivalence.
6. A sense-local LSJ citation to the same chapter/section is a promising ranking feature
   to test. Inherited/shared references, variant editions and multiple target uses in
   one section must be accounted for. No semantic equivalence between lexica is implied.

## Measured Book 1 candidate coverage

[Measurement JSON](ancient-greek-lexica/summaries/book1-lookup-measurement.json)
and [tier JSON](ancient-greek-lexica/summaries/crosswalk-tiers.json) were independently
reproduced from the pinned inputs. **These are exploratory headword-retrieval figures,
not the production dictionary hit-rate report or sense coverage.**

The OGA file matches the compiled pin: 7,438,037 bytes, SHA-256
`c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61`.
Book 1 MISC references yield 10,572 native nodes, including 110 POS `-` ellipses:
10,462 printed tokens. The exploratory denominator keeps native `n/a/d/v`, includes
proper names, and excludes 178 exact-lemma εἰμί occurrences: **6,017 occurrences,
1,429 identities (1,408 bare; 21 numbered)**. It does not implement the production
proper-name classifier or all selection rules. Excluding `c` is consistent with
counting only the four vocabulary classes; it is not an assertion that all `c`
tokens map to the same UPOS. εἰμί remains VERB under the real import contract.

| Candidate retrieval measure | Kaikki raw `grc` | LSJ | Middle Liddell |
|---|---:|---:|---:|
| Any entry at headword, out of 1,429 identities | 1,185 (82.9%) | 1,291 (90.3%) | 1,094 (76.6%) |
| Explicit coarse POS-compatible candidate | 1,062 (74.3%) | Not mapped | Not mapped |
| Occurrences whose headword has any entry, out of 6,017 | 5,707 (94.9%) | 5,674 (94.3%) | 5,303 (88.1%) |
| Exactly one compatible record (Kaikki POS; others headword only) | 989 | 1,221 | 1,059 |
| Multiple compatible records | 73 | 70 | 35 |
| Some entry but none POS-compatible | 123 | Not mapped | Not mapped |
| No indexed entry | 244 | 138 | 335 |
| Identities whose candidate set has a Book 1 reference | 17 | 436 | Not measured |

Kaikki has 35 standard-form Anabasis example references, 17 for Book 1, across 17
headwords; these are string-pattern counts, not a complete bibliography parser.
LSJ has 580 matching Book 1 URN strings across all files, not 580 independently
verified senses. A homograph candidate carrying a citation does not mean every
OGA identity sharing its spelling has that sense. The 156 OGA identities with
multiple Kaikki headword rows are **not** a count of 156 distinct homographs.

Limitations: Python `str.lower()` and its Unicode tables approximate, not certify,
Go's pinned simple-lowercase Unicode 17 normalization. Regex-based XML scanning and
a limited Beta Code converter are research tools, not import adapters. Unsupported
keys are excluded (LSJ 311; Middle Liddell 18); 13 OGA identities had only excluded
LSJ keys. There may be other conversion/extraction errors. No production-equivalent
coverage percentage, card-candidate coverage, or contextual-sense coverage is claimed.

Earlier commits on this research branch used a deprecated Kaikki download, counted
ellipses as printed tokens, and compared dictionary digits. The tables above and
linked JSON supersede those preliminary counts; equal-digit counts retained in JSON
are explicitly `UNVERIFIED`, never matching decisions.

## Current Mouseion baseline and remaining evidence

At [base commit 6e02d180](https://github.com/justin-hayes/mouseion/tree/6e02d180b1015536127839e0a45953684c0e5a83):

- [Derivation](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/nlp/scripts/derive_dictionary_index.py#L309-L337)
  keeps only `glosses[0]` and example text, losing child breadcrumbs and structured
  citation/translator information. Greek rows retain etymology numbers and forms,
  but forms lose tags and there is no native source-entry identifier.
- [Greek lookup](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/internal/dictionary/greek_index.go#L50)
  loads only headword, POS and senses; it flattens matching entries. Content-hash
  [deduplication](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/internal/dictionary/index.go#L208-L267)
  does not preserve native entry/sense identity. The lexical request has no OGA
  discriminator or morphology field. Existing Lesk-style ordering is not validated WSD.
- The installed dump was **not inspected**. Metadata can record dump/extractor dates,
  but the default download URL is mutable; no installed snapshot is asserted here.
  `cmd/greekhitrate` reads current installation analyses, not inherently Book 1 only.

Before any new export behaviour: preserve attribution and source IDs; settle LSJ's
applicable territorial rights/extra wording if it is chosen; reconcile Middle Liddell
or leave it excluded; archive the hash-pinned Kaikki bytes; and replace exploratory
matching with the actual OGA rules. No new research ticket is needed just to repeat
these caveats: the named contextual-sense prototype already owns the next evaluation.
ADR 0064's older BY-SA 3.0 wording can be corrected separately; ADR 0079 does not
state a licence version and does not need that alleged correction.

[kaikki]: https://kaikki.org/dictionary/rawdata.html
[wmf]: https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use#7._Licensing_of_Content
[lsj-rights]: https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj/README.md
[perseus-rights]: https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/README.md
[perseus-license]: https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/license.md
[lsj1]: https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj/grc.lsj.perseus-eng1.xml
[lsj4]: https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj/grc.lsj.perseus-eng4.xml
[lsj12]: https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj/grc.lsj.perseus-eng12.xml
[ml-web]: https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0058
[ml-xml]: https://github.com/blinskey/middle-liddell/blob/5db8d88a20066e182ca3fc6ee758c9f61ac60603/Perseus_text_1999.04.0058.xml
[ml-license]: https://github.com/blinskey/middle-liddell/blob/5db8d88a20066e182ca3fc6ee758c9f61ac60603/LICENSE.md
[dge]: http://dge.cchs.csic.es/xdge/
[dge-license]: http://dge.cchs.csic.es/xdge/doc/licencia.html
[dge-credits]: http://dge.cchs.csic.es/xdge/doc/creditos.html
[bailly-project]: http://gerardgreco.free.fr/spip.php?article52
[bailly-stardict]: https://chaerephon.e-monsite.com/pages/litterature/grec-ancien/bailly2020.html
[bailly-conversion]: https://latin-dict.github.io/dictionaries/Bailly2020.html
[wikt-barbaros]: https://en.wiktionary.org/wiki/βάρβαρος#Ancient_Greek
