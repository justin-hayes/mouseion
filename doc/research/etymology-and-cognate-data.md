# Etymology and cognate data for `de`, `it`, `el`, `la` and `grc`

**Research date:** 2026-11-28

**Question ([#2831](https://github.com/justin-hayes/mouseion/issues/2831)):**
What etymology and cognate data could link Mouseion's study languages to each
other and to their ancestors? For each candidate: which relations it gives, its
coverage per language, its licence and attribution (ADR 0111), how it is
distributed and updated, and how reliably its entries match Mouseion's
vocabulary identities (canonical lemma + POS). Does the dictionary data
Mouseion uses today carry any etymology?

**Method.** Licence statements and dataset documentation were read from the
primary pages linked below. The coverage figures were measured, not taken from
papers:

- The English-Wiktionary Wiktextract dump that `make dictionary-index` already
  reads (`raw-wiktextract-data.jsonl.gz`, downloaded 2026-10-18, SHA-256
  `3dac8a09…dbdb`) was scanned for every entry in the five languages.
- Kaikki's native German, Italian and Greek Wiktionary extracts (downloaded
  2026-11-28: `de` `39ea5598…`, `it` `9f35d5ac…`, `el` `ddaf58fe…`) were scanned
  for their etymology fields.
- The 2,000 most frequent NOUN/VERB/ADJ/ADV `(canonical_lemma, upos)`
  identities per language in this deployment's `corpus_tokens` were matched
  against the English-Wiktionary entries. This is a read-only query on the
  local database. That Corpus has no Modern Greek Books, so `el` has no
  Corpus-matched figure.

The scripts and outputs were scratch work and are not kept.

## Answer

- **Wiktionary through Wiktextract is the only source that covers all five
  languages with typed relations.** Use the English-edition dump Mouseion
  already downloads. Its `etymology_templates` name the relation (inherited,
  borrowed, learned borrowing, calque, derived, cognate, doublet) and the
  source language and term. `descendants` gives the reverse direction (Latin
  → Italian, Ancient → Modern Greek). It is CC BY-SA 4.0 / GFDL: the same
  terms, register row and deck notice as the Wiktionary layer already in use.
  No new delivery path is needed, because the operator-built index can store it.
- **Coverage follows the history of the languages.** Italian ↔ Latin is dense:
  77% of the top 500 Italian lemmas link to Latin, and 76% of the top 500 Latin
  lemmas list an Italian descendant. Ancient → Modern Greek is good: 44% of
  the top 500 `grc` lemmas link to Modern Greek. Greek ↔ Latin is mostly
  cognacy (about 20% each way). German links to any other study language for
  only 9–13% of its lemmas, almost all of them Latin or Greek loans, or cognates
  through Proto-Indo-European.
- **No other candidate is usable as a primary source.** EtymDB 2.1 is a 2019
  Wiktionary snapshot (CC BY-SA 4.0). CogNet is CC BY-NC-SA 4.0, which ADR 0111
  rules out for Source content. IE-CoR (CC BY 4.0) covers only 170 basic
  meanings. Wikidata lexemes (CC0) have almost no etymology for these
  languages. The native Wiktionaries give prose with untyped links. Lewis &
  Short has 1879-era free-text `<etym>` notes.
- **Matching works for well-lemmatised text and fails where the lemmatiser or
  the identity does.** 96–99% of the top-2000 German and Italian identities find
  a Wiktionary entry with the same POS. The figure is 74–91% for Latin and
  68–84% for Ancient Greek. The misses are lemmatiser errors, deponent and
  spelling-variant conventions, and POS-inventory differences. Macron
  stripping makes 3–5% of matched identities ambiguous between homographs
  (`volo` "fly"/"want", `populus` "people"/"poplar"). Those cases must show
  every candidate or be dropped, never guessed.
- **Mouseion carries no etymology today.** The index keeps only Wiktionary's
  `etymology_number`, for Ancient Greek row identity, and strips LSJ's
  etymology notes. Lewis & Short's `<etym>` is not stored.

## What Mouseion's dictionary data carries today

| Layer | Etymology in the source | What the index keeps |
|---|---|---|
| Kaikki/Wiktextract, English Wiktionary (`de`, `it`, `el`, `fr`, `grc`), built by `nlp/scripts/derive_dictionary_index.py` from `KAIKKI_DOWNLOAD_URL = https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz` | `etymology_text`, `etymology_templates`, `etymology_number` and `descendants` on every record (see the samples below) | Only `etymology_number`, as a column of `greek_entries` (`grc`), so that homographs stay separate rows (`greek_etymology_number`, `collect_greek_entries`). The `entries` table for `de`/`it`/`el`/`fr` has no etymology column. |
| LSJ (`PerseusDL/lexica@56061ca1`) | A closing "(Cf. Skt. …, Lat. …)" note on many entries | Removed on purpose: `LSJ_ETYMOLOGY_NOTE` and `LSJ_COGNATE_LANGUAGES` keep cognates out of glosses (ADR 0104 records that "Etymology cognates and stray punctuation were also taken as glosses", fixed in #2204). |
| Lewis & Short (`lat.ls.perseus-eng1.xml` at the same pin, SHA-256 `ccbd2f79…`) | 21,802 `<etym>` elements in 51,596 `<entryFree>` entries (counted in the pinned file) | Not stored: `ls_entries` has `entry_id, headword, orth, senses_json, homograph`. |

Latin is not built from Kaikki today. Its only dictionary layer is Lewis & Short
(`LANGUAGES = {"de", "it", "el", "fr"}` plus the `grc` and L&S paths).

The repository has also stated the direction before deciding anything: ADR 0110
admits "Latin vocabulary inherited or borrowed into French and Italian", "the
continuity from Ancient to Modern Greek" and "Greek and Latin influence on
German", and leaves the data sources to be decided when a feature is specified.
`doc/features/study.md` lists "Cognates, etymology and recommendations" as out
of scope after the Study cutover.

## Candidates

### 1. Wiktionary, English edition, through Wiktextract (recommended)

**Distribution and updates.** Kaikki's raw-data page says the data "is updated
regularly (usually at least once a week)". At the time of reading, the current
file came from "the enwiktionary dump dated 2026-10-01", extracted with
wiktextract `19f9a30` (24.8 GB JSONL, 2.9 GB gzip)
([kaikki.org/dictionary/rawdata.html](https://kaikki.org/dictionary/rawdata.html)).
The post-processed per-language files are marked deprecated there. Mouseion
already reads the raw file (`doc/features/dictionary-gloss-enrichment.md`).

**Fields.** The wiktextract README
([tatuylonen/wiktextract](https://github.com/tatuylonen/wiktextract), `master`
at `dffd2bc9`) defines:

- `etymology_text`: "etymology section as cleaned text"
- `etymology_templates`: "templates and their arguments and expansions"
- `etymology_number`: "for words with multiple numbered etymologies … the
  number of the etymology under which this entry appeared"
- `descendants`: "descendants of the word"

**Licence.** Wiktionary: "The original texts of Wiktionary entries are
dual-licensed … under both the Creative Commons Attribution-ShareAlike 4.0
International License … and the GNU Free Documentation License"
([Wiktionary:Copyrights](https://en.wiktionary.org/wiki/Wiktionary:Copyrights)).
Wiktextract's code is MIT. This is the existing register row in
`doc/third-party-data.md` ("Wiktionary entries, extracted by Wiktextract and
published by Kaikki.org"), which is Source content, share-alike, and reaches a
deck with `dictionary.AttributionNotice`. Etymology from the same dump fits that
row: the register would only widen the row's field list and add `la`.

**Relations.** English Wiktionary encodes relations as templates. The ones
that matter, with the arguments read from the dump:

| Template | Relation | Source language, term |
|---|---|---|
| `inh`, `inh+` | inherited from | `2`, `3` |
| `bor`, `bor+`, `lbor`, `slbor`, `ubor` | borrowed / learned / semi-learned / unadapted borrowing | `2`, `3` |
| `cal`, `sl` | calque, semantic loan | `2`, `3` |
| `der`, `der+`, `uder` | derived from (unspecified route) | `2`, `3` |
| `cog`, `ncog` | cognate with (`ncog`: non-cognate comparison) | `1`, `2` |
| `doublet` | doublet of | `1`, `2` |
| `af`, `compound`, `prefix`, `suffix`, `confix` | word formation within the language | — |
| `etymon`, `ety` | a newer machine-readable chain, such as `gmh:philosophīe<ety:bor<la:philosophia>>` | `3` |

The expansions of the `etymon`/`ety` wrappers appear in the dump as the
ordinary `inh`/`der`/`bor` templates they call (see `Philosophie` below). The
counts below therefore read those, and are lower bounds where a chain is only
in an `etymon` argument.

**Sample records** (from the 2026-10 dump, abbreviated):

```json
{"lang_code":"it","word":"acqua","pos":"noun","etymology_number":"1",
 "etymology_text":"Inherited from Latin aqua.",
 "etymology_templates":[{"name":"inh","args":{"1":"it","2":"la","3":"aqua"}}]}

{"lang_code":"el","word":"πατέρας","pos":"noun",
 "etymology_text":"Inherited from Byzantine Greek πατέρας (patéras), from Ancient Greek πατήρ (patḗr) through the accusative singular πατέρα (patéra), from Proto-Hellenic *patḗr, from Proto-Indo-European *ph₂tḗr. Doublet of πατήρ (patír).",
 "etymology_templates":[{"name":"inh","args":{"1":"el","2":"gkm","3":"πατέρας"}},
                        {"name":"inh","args":{"1":"el","2":"grc","3":"πατήρ"}}, …]}

{"lang_code":"grc","word":"πατήρ","pos":"noun",
 "descendants":[{"lang_code":"el","word":"πατήρ"},{"lang_code":"el","word":"πατέρας"}, …]}

{"lang_code":"la","word":"pater","pos":"noun",
 "descendants":[{"lang":"Italo-Romance","descendants":[ … {"lang_code":"roa-oit","word":"patre",
   "descendants":[{"lang_code":"it","word":"padre", …}]}]}, …]}

{"lang_code":"de","word":"Philosophie","pos":"noun",
 "etymology_text":"From Middle High German philosophīe, from Latin philosophia, which see. …",
 "etymology_templates":[{"name":"etymon","args":{"1":"de","2":":inh","3":"gmh:philosophīe<ety:bor<la:philosophia>>"}},
                        {"name":"inh","args":{"1":"de","2":"gmh","3":"philosophīe"}},
                        {"name":"der","args":{"1":"de","2":"la","3":"philosophia"}}]}

{"lang_code":"grc","word":"ὕδωρ","pos":"noun",
 "etymology_templates":[ … {"name":"cog","args":{"1":"la","2":"unda"}}, …]}
```

**Coverage across the whole dump.** These are lemma entries (records with at
least one sense that is not `form_of`/`alt_of`) with POS noun, verb, adj or
adv:

| Lang | Entries | With etymology text | Main relations to other study languages (entries) | Entries listing descendants in a study language |
|---|---|---|---|---|
| `de` | 82,413 | 82% | derived/borrowed from `la` 2,748; from `grc` 737; cognate `la` 154, `grc` 98 | — |
| `it` | 142,540 | 51% | inherited `la` 3,294 (+891 Vulgar, 644 Late Latin); borrowed `la` 3,212; derived `la` 6,119; learned borrowing `la` 337; derived `grc` 2,251 | `de` 260, `el` 181 |
| `el` | 26,423 | 49% | inherited `grc` 2,899 (+611 Koine, 1,007 Byzantine `gkm`); learned borrowing `grc` 1,116; derived `grc` 1,710; borrowed `it` 246 | — |
| `la` | 37,672 | 77% | borrowed/derived from `grc` 2,340; cognate `grc` 984, `de` 113 | `it` 10,703, `de` 1,968, `el` 246 |
| `grc` | 16,353 | 92% | cognate `la` 1,035, `de` 51 | `el` 4,313, `la` 2,511, `it` 866, `de` 403 |

Italian's 51% and Modern Greek's 49% are low because both editions have many
minor derived entries. Frequent words do much better, as the next table shows.

**Coverage of the vocabulary a learner meets.** These are the top `(lemma,
upos)` identities by token count in this deployment's Corpus. "Matched" means
a Wiktionary entry with the same key and POS. "Links to" counts a forward
relation or a descendant in that language.

| Lang | Band | Matched (key+POS) | Etymology text | Links to another study language | Main pair |
|---|---|---|---|---|---|
| `de` | top 500 | 98% | 98% | 9% | `la` 8%, `grc` 2% |
| `de` | 501–2000 | 96% | 92% | 13% | `la` 11%, `grc` 3% |
| `it` | top 500 | 97% | 98% | 78% | `la` 77%, `grc` 6% |
| `it` | 501–2000 | 95% | 92% | 64% | `la` 61% |
| `la` | top 500 | 91% | 95% | 79% | `it` 76%, `grc` 20%, `de` 15% |
| `la` | 501–2000 | 74% | 76% | 56% | `it` 54% |
| `grc` | top 500 | 84% | 86% | 52% | `el` 44%, `la` 21% |
| `grc` | 501–2000 | 68% | 71% | 41% | `el` 35%, `la` 15% |

Most links resolve to an existing Wiktionary entry in the target language
after that language's key is applied. For `it`→`la`, 349 of 383 do in the top
500. For `grc`→`el`, 212 of 222 do.

**Links where both ends are in this Corpus's top 2,000:** `it`→`la` 441,
`la`→`it` 414, `grc`→`la` 90, `la`→`grc` 87, `de`→`la` 57, `la`→`de` 36,
`de`→`it` 13, `it`→`de` 13, `it`→`grc` 12, `de`→`grc` 11. Some examples:

- `it`→`la`: `dire` inh `dico`, `avere` inh `habeo`, `mano` inh `manus`, `cosa`
  inh `causa`
- `grc`↔`la`: `εἰμί` cog `sum`, `φέρω` cog `fero`, `μέγας` cog `magnus`, `νύξ`
  cog `nox`
- `de`→`la`/`grc`: `schreiben` der `scribo`, `Familie` bor `familia`, `Engel`
  der `ἄγγελος`, `Meer` cog `mare`
- `la`→`de`: `corpus` desc `Körper`, `ordo` desc `Orden`

**Quality notes found in the samples.**

- **Cognates reach back to Proto-Indo-European.** Links such as `Frau` cog
  `prōvincia`, `genug` cog `nanciscor` and `rede` cog `ratio` are correct but
  opaque to a learner. A cognate is weaker evidence of a learnable connection
  than an inheritance or a loan.
- **`der` names a source, not a route.** `it` `filosofia` has `der` `la`
  `philosophia` and `der` `grc` `φιλοσοφία`, and the immediate step is only in
  the `etymon` template (`:lbor`).
- **Descendant trees run through intermediate languages.** `la` `pater` →
  Old Italian `patre` → `it` `padre`. Keep the path, or at least record the
  link as indirect.

### 2. Native Wiktionary editions through Wiktextract

German, Italian and Greek editions are extracted by Kaikki
([rawdata page](https://kaikki.org/dictionary/rawdata.html), "work in
progress"), under the same CC BY-SA 4.0 / GFDL terms. Their etymology is prose
with untyped links (`etymology_texts`/`etymology_text`, `etymology_links` as
`[display, target]`), for example:

```json
{"word":"padre","lang_code":"it","etymology_texts":["dal latino pater, -is"],
 "etymology_links":[["latino","latino"],["pater","pater"],["-is","patris"]]}

{"word":"Philosophie","lang_code":"de","etymology_texts":["im 15. Jahrhundert von lateinisch philosophia ^(→ la) entlehnt, das auf altgriechisch φιλοσοφία … ^(→ grc) „Weisheitsliebe“ zurückgeht"],
 "etymology_links":[["philosophia","philosophia#philosophia (Latein)"],["φιλοσοφία","φιλοσοφία#φιλοσοφία (Altgriechisch)"], …]}

{"word":"πατέρας","lang_code":"el","etymology_text":"πατέρας < (κληρονομημένο) μεσαιωνική ελληνική πατέρας < αρχαία ελληνική πατήρ …",
 "etymology_links":[["κληρονομημένο","κληρονομημένος"],["μεσαιωνική ελληνική","μεσαιωνικά ελληνικά"],["πατήρ","πατήρ#Αρχαία ελληνικά (grc)"], …]}
```

The target language can be recovered from the link anchors (`(Latein)`,
`(grc)`), but the relation cannot. Only the Greek edition marks
`κληρονομημένο` (inherited) in the text. Prose in the study language is
attractive for an immersion display. It is not a source of typed relations,
and it would need a second dump per language. Use it only for display text, if
ever.

### 3. EtymDB 2.1

"An etymological database extracted from the Wiktionary"
([clefourrier/EtymDB](https://github.com/clefourrier/EtymDB), last commit
`878e5a55`, 2022-01-04). `DATA_LICENSE` is CC BY-SA 4.0 and the scripts are
LGPL-3.0. Its `DATA_STATEMENT.md` says the authors "parsed the 2019/10/20
Wiktionary datadump", and lists Latin (69,224 lexemes), Italian (31,295) and
German (27,009) among the best-represented languages. It is a frozen, seven
years older extraction of the same source, made with regular expressions.
Candidate 1 gives the same relations, current, from a file Mouseion already
downloads. **Not recommended.**

### 4. CogNet v2

"8.1 million cognates in 338 languages … automatically constructed from
wordnets and dictionaries", with a README stating "available under the
Attribution-NonCommercial-ShareAlike 4.0 International (CC BY-NC-SA 4.0)
License" ([kbatsuren/CogNet](https://github.com/kbatsuren/CogNet), `1601560b`,
2023-06-15). Shown text would be Source content, and the register's rule is
that Source content "must permit redistribution and be no more restrictive than
share-alike". **Excluded by ADR 0111.** It is also concept-aligned (WordNet
synsets), not etymological, so it does not give inheritance or borrowing.

### 5. IE-CoR (Indo-European Cognate Relationships)

The CLDF dataset [lexibank/iecor](https://github.com/lexibank/iecor) (`master`
at `dd1ebc99`) "is licensed under a https://creativecommons.org/licenses/by/4.0/
license". It has 160 varieties, 170 concepts, 25,731 lexemes and 4,981 cognate
sets. Its `languages.csv` includes German, Italian, Latin, "Greek: Ancient" and
"Greek: Modern Std". It is expert-curated and highly reliable, but limited to
170 basic meanings (Heggarty-2020-200). It could at most check a few hundred
basic words. **Not a primary source.**

### 6. Wikidata lexemes

All Lexeme-namespace data is "released into the public domain under Creative
Commons Zero" ([Wikidata:Licensing](https://www.wikidata.org/wiki/Wikidata:Licensing)).
Counts from the Wikidata Query Service on 2026-11-28 (lexemes / lexemes with
`P5191` "derived from lexeme" / with `P5238` "combines lexemes"):

| Lang | Lexemes | P5191 | P5238 |
|---|---|---|---|
| `de` (Q188) | 241,996 | 1,997 | 8,477 |
| `it` (Q652) | 65,131 | 1,827 | 412 |
| `el` (Q36510) | 43,333 | 58 | 2 |
| `la` (Q397) | 56,455 | 280 | 130 |
| `grc` (Q35497) | 11,628 | 35 | 9 |

The licence is the cleanest, but cross-language etymology is close to absent.
**Not usable now.**

### 7. Lewis & Short `<etym>` and LSJ "(Cf. …)" notes

Both are already supplied by the operator, under CC BY-SA 4.0. L&S `<etym>` is
19th-century free text, for example for `pater`: "Sanscr. root pā, to nourish,
protect; Lat. pasco; hence, Zend, patar, protector; Gr. pathr; Sanscr pitri;
Engl. father; Germ. Vater". Greek is in Beta Code, and no relation is typed.
It is often internal word formation (`in-ploro`). LSJ's notes are mainly
comparanda. Neither links to Italian or Modern Greek, and both reflect
pre-modern scholarship. Display text at most. **Not a relation source.**

### Not pursued

Proprietary etymological dictionaries (DWDS/Pfeifer, Babiniotis, Beekes, de
Vaan, Etymonline) carry no open licence that Source content could use. The
Etymological WordNet page (de Melo, ICSI) did not respond. It is an older
Wiktionary derivative too, so it would add nothing over candidate 1.

## Lemma-matching risk

Mouseion's identities are `(canonical_lemma, upos)` in canonical profiles
(`internal/canonicalization`). These rules apply to Wiktionary entries:

- **`de`, `it`, `el`:** Go `strings.ToLower` plus the German post-1996 table.
  Use `lower()`, not Unicode casefolding: casefolding maps `ß` to `ss`, so
  `groß`, `Fuß` and `Straße` would miss. Capitalised nouns match after
  lowercasing. Residual misses are lemmatiser output (`zweit`, `dritt`,
  `vgl/VERB`) and Wiktionary's POS for ordinals (`erst`, `letzt`).
- **`la` (`latin/1`):** strips macrons and breves, lowercases, and folds `v`→`u`
  and `j`→`i`. Wiktionary headwords have no macrons, but template terms do
  (`fīlius`, `prōvincia`). Applying the profile to both sides makes them meet,
  and the `it`→`la` links then resolve at 91%. Misses at ranks 501–2000 (26%)
  are mostly LatinPipe output: errors (`proficimcor`, `intellexgo`,
  `potuisse`, `itineribus`), active lemmas for deponents (`uto`, `reuerto`,
  `proficisco`), unassimilated spellings (`conloco`, `conloquium`, `expecto`
  against Wiktionary `colloco`, `exspecto`), and adverbs lemmatised to their
  adjective with ADV (`longus/ADV`, `magnus/ADV`).
- **`grc` (`ancient-greek/1`):** NFC and lowercase. Accents and breathings stay
  significant, and final sigma is folded to `σ`. Wiktionary headwords match
  once the same folding is applied. Misses (16–32%) come from three causes. OGA
  tags function words and numerals as ADV/ADJ where Wiktionary uses
  particle/conjunction/pronoun/numeral (`δέ`, `καί`, `οὗτος`, `πέντε`).
  Comparatives are form-of entries in Wiktionary (`βελτίων`, `ἀμείνων`). Some
  are lemmatiser errors (`σύνλέγω`, `ἐνεἰμί`). A fallback that ignores POS
  rescues the function words, but they rarely need etymology.
- **Homographs.** 3–5% of matched top-2000 identities match more than one
  Wiktionary entry with the same POS. Examples: `la` `volo` ("fly"/"want"),
  `populus`, `dico` (macrons folded); `grc` `λέγω` ("say"/"lay"), `δέω`
  ("bind"/"lack"); `de` `Tag`, `Teil`; `it` `ora`, `fine`. Separate etymology
  sections are separate records (`etymology_number`), and the Ancient Greek
  index already keeps them as separate rows. Mouseion's identity cannot choose
  between them, so an etymology display must show each candidate or show none.
- **POS fallback adds errors.** Borrowing a different-POS entry's etymology
  attached the noun `Fest`'s cognate `festa` to the adverb `fest`. Match on key
  and POS, and treat a match on the key alone as weaker evidence, as the
  Ancient Greek gloss lookup already does.
- **Link targets need the target language's profile.** A template term is the
  target language's spelling. It must be keyed with that language's profile
  before it is compared with that language's Corpus: macron stripping for
  Latin, final-sigma folding for Ancient Greek. Language variants also need an
  explicit decision. Vulgar/Late/Medieval Latin (`la-vul`, `la-lat`, `la-med`),
  Koine (`grc-koi`) and Byzantine Greek (`gkm`) have their own codes. They are
  not `la`/`grc` and do not match a Classical Book's vocabulary unless the
  spec says so.
- **Modern Greek is unmeasured against a real Corpus.** This deployment has no
  `el` Books. The dump shows 49% of `el` lemma entries with etymology and
  2,899 with typed `grc` inheritance. Measure against an `el` Corpus before
  relying on a figure.

## Implications for the first cross-language slice

- The cheapest slice that clears the licence bar is to keep etymology from the
  dump the index build already reads. Keep the typed relations (`inh`, `bor`,
  `lbor`, `cal`, `der`, `cog`) and `descendants` that point into another study
  language, plus `etymology_text`. Link the result through the existing profile
  keys. This is the same register row and deck notice, with `la` added to the
  Kaikki row.
- Italian ↔ Latin and Ancient → Modern Greek have enough density to be
  noticeable on most frequent words. German ↔ Latin/Greek is sparse, around
  10%, and mostly loanwords. Cognates are plentiful but often obscure. A
  learner-facing default probably favours inheritance and borrowing over
  cognacy. That is a product decision for the spec.
- Homographs and lemmatiser noise mean any display needs an "ambiguous" state.
  Latin and Ancient Greek coverage depends as much on OLA/OGA lemma quality as
  on Wiktionary.
