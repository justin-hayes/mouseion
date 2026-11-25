# Latin lemma conventions against dictionary headwords on the chosen pin

Research ticket: [Research: Latin lemma conventions against dictionary headwords on the chosen pin](https://github.com/justin-hayes/mouseion/issues/2665),
part of the classical Latin wayfinder
([map](https://github.com/justin-hayes/mouseion/issues/2658)). Measured 2026-11-25. Code at `a2317215`.

## Answer

**Recommend a new `latin` profile whose key is: NFC, strip combining marks, simple lowercase, then `v`→`u` and `j`→`i`.
Do not fold `ae`/`oe`, do not strip homograph digits inside the profile, and do not put orthographic aliases (assimilated prefixes, archaic
`-os`/`-om`) into identity.** On the 80 classical prose Works of at least 5,000 words (2,918,687 word tokens, 68,651 lemma types) the
published OLA lemma matches a Lewis & Short or Elementary headword for **93.12 % of tokens (33.4 % of types)** under that key. Lewis & Short alone gives 92.60 %. The
exact string already matches 92.72 % of tokens (union), so the normalisation is cheap insurance, not the main lever: it adds 0.40 points of tokens and 3.5 points of types.

The unmatched 6.88 % of tokens (200,874) is mostly **not a convention gap**. A hand-judged seeded sample of 120 unmatched tokens puts about 72 % in
lemmatiser errors in the published layer (an inflected form or a non-word as the lemma: `potest`, `pluribus`, `judicao`), 16 % in non-lexical strings
(praenomen abbreviations, numbers, Greek script), 7.5 % in spelling variants of a real headword, and 4 % in valid lemmas the dictionaries lack (proper names).
The head of the vocabulary is clean: the 1,000 most frequent lemma types match 98.2 % by type and 99.2 % by token, and the unmatched types are 58 % hapax.

The earlier dictionary ticket said its pick "would change if the lemma-conventions ticket shows the `j`/`v` keys lose too many corpus lemmas". **They do not.** OLA lemmas use
the same `v` and `j` convention as Lewis & Short. Lewis & Short stays the pinned dictionary.

## Sources, pins, and the hard constraint

| Layer | Pin | Verified | ADR 0111 class |
| --- | --- | --- | --- |
| OLA v0.2.0 CoNLL-U, one member per Work | Zenodo `10.5281/zenodo.15183688`, members fetched by HTTP range request from the archive; per-Work SHA-256 in [`ola-bysa-classical-pins.tsv`](https://github.com/justin-hayes/mouseion/blob/d6b4517dd1662b48562e2d9f2e6204f4ca360997/doc/research/latin-scheme-route/ola-bysa-classical-pins.tsv) | all 213 fetched files match the pinned SHA-256 | Analysis resource, restriction **unclear** (grilling decision); lemma strings only are committed here |
| Lewis & Short, `lat.ls.perseus-eng2.xml` | `PerseusDL/lexica@56061ca127f4a2844980baffc5f2b6d1332897b3`, SHA-256 `a21c3799…1836c` | digest matches the [dictionary ticket](https://github.com/justin-hayes/mouseion/blob/69fe59eaa07f73582d3ee66cb364fbe6d9999406/doc/research/latin-dictionary-candidates.md) | Source content, CC BY-SA 4.0, share-alike, admitted |
| Lewis, *Elementary Latin Dictionary*, `viaf2845558.viaf001.xml` | same commit, SHA-256 `16db938a…522cd` | digest matches | Source content, CC BY-SA 4.0, admitted |

No new third-party layer was introduced, so no new ADR 0111 row. Nothing in this research ran an analyser: the lemmas are the published OLA annotation, and the
only computation is string comparison against two dictionaries. Work classes and word counts come from the
[`ola-conllu-manifest.tsv`](https://github.com/justin-hayes/mouseion/blob/3c1318cfafc5f95b4868d071fa317f37ab8969c5/doc/research/latin-oga-equivalent/ola-conllu-manifest.tsv)
(SHA-256 `05c76d7c…2ec0`).

**Sets measured** (IDs in [`results/work_sets.json`](latin-lemma-conventions/results/work_sets.json); one Edition per Work, the longest):

| Set | Works | Word tokens | Lemma types |
| --- | ---: | ---: | ---: |
| `prose80`: classical prose, BY-SA text, at least 5,000 words (the headline set) | 80 | 2,918,687 | 68,651 |
| `prose_all`: all classical prose Works in the 205 | 134 | 3,044,801 | 70,140 |
| `verse61` / `verse_all`: verse of at least 5,000 words / all verse | 61 / 71 | 1,095,311 / 1,124,181 | 45,238 / 46,130 |
| `borderline`: Apuleius (3 Works) and Gellius, admitted by the grilling | 4 | 205,139 | 21,858 |

"Word tokens" excludes the punctuation tag (`u`). Prose versus verse is by author (Plautus, Terence, Lucretius, Catullus, the elegists, Virgil, Horace, Ovid,
Persius, Phaedrus, Lucan, Statius, Valerius Flaccus, Silius, Martial, Juvenal, Priapea, Seneca's tragedies are verse); the split reproduces the
earlier count (80 prose, 61 verse at 5,000 words). **Cato is not in the pool**: Perseus's `phi0478` is Quintus Cicero, and no Cato file is in the 205. The archaic evidence below
is therefore Plautus, Terence, Lucretius and Varro's *Res rusticae* (the only archaic-ish prose Work).

**How a match is counted.** A lemma type matches if its key equals the key of any dictionary headword, with the same normalisation on both sides. A headword is the entry's
`key` attribute with the trailing homograph digit removed, plus (unless stated) every other single-word `<orth>` form of the entry, with marks stripped.
Tokens are weighted by frequency, types are distinct lemma strings. A match says the lemma is a headword; it does not say the lemma is the right one for the token (see the sample below).

## Conventions, measured

### u/v, i/j, case, diacritics

| Property (`prose80`) | OLA lemmas | Lewis & Short keys | Elementary keys |
| --- | --- | --- | --- |
| consonantal `v` written `v` | 6,841 types, 180,012 tokens contain `v` | 4,271 of 50,027 base keys | 1,647 of 16,986 |
| consonantal `i` written `j` | 670 types, 27,942 tokens | 683 keys | **0** (uses `i`) |
| homograph numbering | none (28 lemma types end in a digit: footnote-style artefacts such as `et1`, `ut1`, `in1`) | trailing digit on the key, 4,266 entries in 2,649 groups | trailing digit, plus un-numbered siblings |
| diacritics | none, except accents in Plautus (below) | none on the key; macrons and breves in the printed `<orth>` | same |
| capitalised | 12,913 types, 141,019 tokens | 8,287 base keys | n/a |

OLA lemmas follow the dictionary's `v`/`j` convention, which came from the lemmatiser's training data, not from the surface text: the text has `v` in 176,129 tokens
and `j` in 8, yet lemmas carry `j` in 27,942 tokens (form `iam`, lemma `jam`). The convention is not perfectly consistent: 13 of the 80 Works also contain
the `u`-style lemmas (`uoco`, `seruus`) next to the `v`-style ones. **Per Work**, the share of tokens that match only after the u/v and i/j fold has a median of 0.06 %, a
maximum of 1.75 %, and 7 Works above 1 % (the three Seneca the Elder Works, Celsus, Valerius Maximus and two Suetonius Lives); no Work is above 3 %
([`results/uv_ij_per_work.json`](latin-lemma-conventions/results/uv_ij_per_work.json)).

Capitalisation is unreliable on the OLA side. 1,614 capitalised lemma types also occur in lowercase (39,000 tokens), for example `Graecus` (1,361) next to `graecus` (30), `Crassus`
(596) next to `crassus` (90). Some lemmas are all-uppercase heading text (`QVI`, `IN`, `SI`): 587 types, 12,910 tokens, mostly praenomen abbreviations but also headings.
Case folding repairs 1,625 groups (1,656 lemma types) on the corpus side. It costs 616 Lewis & Short base-key pairs (17 in Elementary) that differ only by case; the
proper/common ones are `Liber`/`liber`, `Cos`/`cos`, `Leo`/`leo`, `Mus`/`mus`, `Dis`/`dis`. By tokens this is negligible (`Liber` 5 tokens).

### Homograph numbering

OLA does **not** number homographs. `cum` is one lemma string with POS `r` (preposition, 11,028 tokens) and `c` (conjunction, 18,842); `ne` is one string with `d` (20,210) and `c` (6,690); `sum` has 108,431 tokens
and no `edo` collision, because the lemmatiser maps `est` to `sum` in 37,848 of 37,851 cases (`edo` appears 562 times). The cost falls on the dictionary side: 28.7 % of the tokens that match a Lewis & Short
headword (12.6 % of types) land on a base key with two or more entries after folding (20.5 % by exact base key alone). The commonest are `sum` (3 entries), `qui` (2), `in` (16: `in1` to `in16`), `cum` (2), `ne` (3), `sed` (3), `dico` (2), `quis` (3). Lewis & Short's `<pos>` element exists on only 16,911 of 51,645 entries, and its first part-of-speech word is unreliable (`sum1` reads "part."), so OLA's POS cannot select the entry. **Homograph choice belongs to the card, not to identity**: identity is one `cum`, the dictionary lookup returns all entries for the key, and the grilling on
card policy decides whether to show them all (as the Greek lookup does for a lemma without a POS match). Elementary is less ambiguous (23.6 % of matched tokens, 4.9 % of types).

### Enclitics

All three are **split into their own tokens**, with the lemma equal to the enclitic and the headword present in both dictionaries (`que`; `ne1`–`ne3`; `ve1`–`ve2`):

| Enclitic token | Tokens (`prose80`) | Per 1,000 tokens | Lemma / POS |
| --- | ---: | ---: | --- |
| `que` | 40,967 | 14.0 | `que` / `c` in every case |
| `ne` | 26,903 | 9.2 | `ne` / `d` 20,210, `c` 6,690; the negator, the conjunction and the enclitic share one string |
| `ve` | 823 | 0.3 | `ve` / `c` 806, `d` 12 |

Together 2.35 % of all tokens. The split is not complete. Unsplit words keep their `-que` when it is part of a headword (`atque`, `quoque`, `quisque`: 22,137 of 30,836 tokens ending in `que` keep the form
as lemma, and 33,698 of 34,929 tokens whose lemma ends in `que` match). The failures are specific: `itaque` is lemmatised `ita` (3,342 tokens), `namque` as `nam` (250), `uterque` as `uter` (169): 3,840 tokens, 0.13 %. For `-ne`, the 10,746 "lemma = form minus `ne`" cases are ablatives of `-io` and `-o` nouns (`ratione`→`ratio`, `sermone`→`sermo`, `Cicerone`→`Cicero`), not enclitics; no unsplit `-ne` failure was found ([`results/spot_checks.json`](latin-lemma-conventions/results/spot_checks.json)). Normalisation cannot repair `itaque`→`ita`; it is a lemma error that a lemma review must catch.

### Deponents and citation forms

Verb lemmas are first-person-present forms, the same as the headword, so principal parts need no stripping (Lewis & Short keeps them in the entry body and `<itype>`, not in the key).

| Verb lemma shape (`prose80`) | Types | Tokens | Tokens matched |
| --- | ---: | ---: | ---: |
| ends `-o` | 14,766 | 473,737 | 88.5 % |
| ends `-or` (deponent/passive) | 1,242 | 34,845 | 88.1 % |
| ends `-m` (`sum`, `inquam`) | 1,304 | 133,002 | 97.1 % |
| infinitive-shaped (`-re`, `-ri`) | 491 | 1,280 | 2.0 % |
| other endings | 8,474 | 36,769 | 23.1 % |

`Voice=d` is not a reliable deponent flag: it marks 7,424 tokens, of which 6,506 have `-or` lemmas, while 20,673 tokens tagged `Voice=p` carry matching `-or` lemmas. A deponent is recognised by the lemma
ending in `-or`, which matches the dictionary. The failure class is a deponent given as an active `-o` lemma (`censo`, `irasco`, `reminisco`, `progredo`), 3 % of the unmatched sample. A suffix rule (add `r`/`ior`) "rescues" 512 types and 4,808 tokens but
is a false-positive generator (`censo`→`censor`, a noun; `timo`→`timor`), so it is not recommended.

### Numerals, proper nouns, Greek

| Class (`prose80`) | Types | Tokens | Matched tokens | Notes |
| --- | ---: | ---: | ---: | --- |
| numeral words (POS `m`, alphabetic) | 356 | 22,984 | 94.3 % | `unus`, `duo`, `mille` match; the misses are comparatives and lemma errors |
| Roman-numeral lemmas (POS `m`) | 159 | 1,793 | mostly 0 | `IIII`, `XXX` are not headwords; of all 172 Roman-shaped lemma types, 11 (824 tokens) match a headword by accident (`C`, `M`, `DI` and the like) |
| digit lemmas | 573 | 5,042 | 0 | page and line numbers read as words (`1108`, 565 tokens, POS `v`); apparatus leakage |
| capitalised lemmas (proper-noun proxy, ASCII, no `.`) | 12,857 | 128,421 | 74.3 % (types 37.6 %) | the dictionaries lack many ethnonyms and gentilicia (`Hispania`, `Macedonia`, `Quinctius`, `Aetolus`, `Tarquinius`) |
| praenomen abbreviations (lemma has `.`) | 68 | 13,617 | 0 | `P.`, `M.`, `L.`, `C.`, `Q.`, `Cn.`: 6.8 % of the unmatched tokens |
| lowercase ASCII lemmas | 53,028 | 2,773,934 | 94.5 % (types 33.6 %) | |
| Greek script lemmas | 2,195 | 3,048 | 0 | 15 of the 80 Works; 220 types (336 tokens) are betacode-like (`kli=ma`) |
| Latin-script lemmas that look like Greek loans (`y`, `ph`, `ch`, `th`, `rh`, `z`, `k`) | 1,846 | 9,569 | 75.6 % | heuristic; includes ordinary words with `ch`/`th` |

Greek loans in Latin script are mostly fine. In Lewis & Short, Greek-derived headwords use `y` on the key, while the printed `<orth>` has Cyrillic `у` in 1,061 entries, which the index must map to `y` if it reads `<orth>`; keys are plain ASCII, so a
key-based index avoids this. Greek script and betacode lemmas are out of dictionary scope.

## Match rate under each candidate normalisation

Token rate / type rate, in percent. The ladder is cumulative; `+i/j` and `+u/v` are each applied alone on top of `+strip-diacritics`, and the last two rows combine them. Source:
[`results/match_rates.json`](latin-lemma-conventions/results/match_rates.json).

**`prose80`** (headline):

| Normalisation | L&S key only | L&S key + orth | Elementary key + orth | L&S + Elementary |
| --- | ---: | ---: | ---: | ---: |
| exact string | 91.27 / 27.8 | 92.12 / 28.9 | 87.25 / 18.2 | 92.72 / 29.9 |
| NFC + lowercase (**today's `la` fallback**, `unicode-casefold/1.2.0`) | 91.91 / 29.2 | 92.28 / 30.4 | 87.61 / 19.3 | 92.88 / 31.4 |
| + strip combining marks | 91.92 / 29.6 | 92.30 / 30.8 | 87.62 / 19.7 | 92.90 / 31.9 |
| + `j`→`i` only | 92.06 / 29.9 | 92.42 / 31.1 | 88.46 / 19.9 | 92.93 / 32.0 |
| + `v`→`u` only | 92.08 / 30.9 | 92.48 / 32.1 | 87.77 / 20.7 | 93.08 / 33.2 |
| **+ `v`→`u` and `j`→`i` (recommended)** | 92.23 / 31.2 | **92.60 / 32.5** | 88.60 / 20.9 | **93.12 / 33.4** |
| + `ae`,`oe`→`e` (rejected) | 92.27 / 31.6 | 92.65 / 32.9 | 88.64 / 21.2 | 93.16 / 33.8 |

Two things stand out. Using each entry's other `<orth>` forms (`quom`, `Juppiter`, `haud`/`haut`) is worth more than any fold (+0.37 points of tokens, +1.3 of types for L&S). And Elementary alone
is clearly weaker (88.6 %); the union adds 0.5 point over Lewis & Short, mostly where L&S has no entry, at the cost of a second dictionary to ship.

**Other sets** (L&S + Elementary, token / type): `prose_all` 93.14 / 33.1; `verse61` 92.91 / 43.5; `verse_all` 92.90 / 43.3; `borderline` 90.69 / 52.5. With Lewis & Short alone: `verse61` 92.51, `borderline` 90.32. By author
(L&S + Elementary, exact → recommended): Plautus 87.11 → 91.28; Terence 93.78 → 94.07; Lucretius 94.49 → 94.65; Varro *Res rusticae* 92.50 → 92.86; Caesar 93.25 → 93.47; Cicero 93.68 → 93.92;
Livy 92.00 → 92.21; Virgil 93.15 → 93.56; Apuleius 91.24 → 91.91; Gellius 89.11 → 89.81.

**Per Work** (`prose80`, recommended key, L&S + Elementary): minimum 86.08 %, 10th percentile 90.52 %, median 94.08 %, maximum 96.10 %. The lowest are `phi0474.phi049.perseus-lat1` (86.08), `phi1038.phi001.perseus-lat1` (89.21),
`phi1014.phi003.perseus-lat1` (89.48), `phi1348.abo016.perseus-lat2` (89.52), `phi0836.phi002.perseus-lat6` (89.57)
([`results/per_work_prose80_all.json`](latin-lemma-conventions/results/per_work_prose80_all.json)).

**Head of the vocabulary** (`prose80`, recommended key, L&S + Elementary; [`results/head_vocab.json`](latin-lemma-conventions/results/head_vocab.json)):

| Most frequent N lemma types | Types matched | Cumulative token coverage matched |
| ---: | ---: | ---: |
| 100 | 100.0 % | 100.0 % |
| 1,000 | 98.2 % | 99.2 % |
| 5,000 | 88.3 % | 97.5 % |
| 10,000 | 74.9 % | 96.2 % |
| 20,000 | 58.7 % | 94.8 % |

## The unmatched remainder

Under the recommended key and both dictionaries, 200,874 tokens (6.88 %) in 45,748 types are unmatched; 26,549 of those types are hapax, 30,825 occur in one Work, and only 3,610 have 10 or more tokens
(they hold 118,833 of the 200,874 tokens). The top 10 types cover 7.4 % of the unmatched tokens, the top 100 16.8 %, the top 1,000 39.4 %, the top 5,000 64.7 %. The ranked list is
[`results/unmatched_prose80_top300.tsv`](latin-lemma-conventions/results/unmatched_prose80_top300.tsv) (rank, lemma string, tokens, Works, dominant POS, cause class); the first 30:

`potest` 3,292; `P.` 2,170; `M.` 2,113; `L.` 2,057; `C.` 1,397; `Q.` 1,254; `Cn.` 838; `Hispania` 694; `1108` 565; `T.` 497; `pluribus` 494; `Quinctius` 472; `Macedonia` 463; `censo` 455; `Aetolus` 395; `p.` 393; `cuiusque` 369;
`potes` 354; `judicao` 351; `ceter` 345; `audo` 339; `milibus` 339; `optimum` 333; `A.` 331; `potuisse` 328; `vido` 309; `Campanus` 299; `Pompei` 292; `Romani` 291; `operis` 280.

**By programmatic cause class** (tokens): lowercase non-headword 95,696 (47.6 %); lemma equal to the only form seen 49,439 (24.6 %); capitalised, not a headword 31,651 (15.8 %); abbreviation 13,617 (6.8 %); digits 5,042; Greek script 3,045;
Roman numeral 1,367; hyphenated 608 (`con-sequor`, `in-pendeo`); other non-ASCII 332; single letter 77.

**By hand-judged sample** (seed 2665, 120 unmatched tokens drawn uniformly from `prose80`; [`results/sample_judged.tsv`](latin-lemma-conventions/results/sample_judged.tsv), lemma strings and token IDs only):

| Judgement | n of 120 | Share (Wilson 95 %) | About, of all tokens |
| --- | ---: | --- | ---: |
| lemmatiser error: an inflected form or a non-word as the lemma | 83 | 69 % (60–77) | 4.8 % |
| deponent lemma given as an active `-o` form | 4 | 3 % (1–8) | 0.2 % |
| non-lexical: abbreviation, number, Greek, fragment | 19 | 16 % (10–23) | 1.1 % |
| spelling variant of a headword (`inplere`, `adprobent`, `inmisit`, `hordeacius`) | 9 | 7.5 % (4–14) | 0.5 % |
| valid lemma the dictionaries lack (proper name) | 5 | 4 % (2–9) | 0.3 % |

A second sample of 100 matched tokens found 99 with a correct or acceptable lemma and one wrong (the single letter `c` read as a headword; Wilson for the error rate 0.2–5.4 %). Suppletion and AGDT-style choices
(`plures`→`multus`, `vos`→`tu`, `vero`→`verus`) look acceptable but are conventions a card must live with.

The spelling-variant class is rule-repairable only in part. Assimilated prefixes (`conparo`, `inpleo`, `adprobo`, `inbecillus`) account for 326 unmatched types and 2,618 tokens (0.09 % of all tokens) when the rewrite
(`inp`→`imp`, `conp`→`comp`, `adc`→`acc` and so on) produces a headword
([`results/features3.json`](latin-lemma-conventions/results/features3.json)). The u/v and i/j fold repairs 1,041 types and 6,444 tokens (0.22 %).

## Archaic orthography (Plautus, Terence, Lucretius, Varro)

| Author | Works | Tokens | exact | recommended key | Unmatched share | Rule-rescuable (archaic `-os`/`-om`, `-umus`, `lubet`, `-st`) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plautus | 20 | 165,057 | 87.11 | 91.28 | 8.7 % | 859 tokens (0.5 %) |
| Terence | 6 | 51,038 | 93.78 | 94.07 | 5.9 % | 52 tokens |
| Lucretius | 1 | 50,877 | 94.49 | 94.65 | 5.4 % | 92 tokens |
| Varro, *Res rusticae* | 1 | 36,043 | 92.50 | 92.86 | 7.1 % | 75 tokens |

The measurable archaic effect is **one thing in one author**: the Perseus text of Plautus carries acute accents, and 9,080 of its lemma tokens (5.5 %) are accented (`égo`, `quí`, `út`, `tú`). Stripping combining marks
lifts Plautus by 3.9 points of tokens and its type rate from 40.1 % to 53.9 %; it is also why the strip step lifts verse by 0.6 point and prose by 0.02. Terence, Lucretius and Varro have no non-ASCII lemmas and gain nothing.
Genuinely archaic spellings that survive in lemmas (`servos`, `tuos`, `divom`, `opust`, `necessest`, `quom`) are worth about 0.5 % of Plautus tokens and under 0.2 % elsewhere; the rest of the Plautine remainder is lemmatiser error on hard text. This is
a prose-only first slice, so **archaic orthography does not affect slice 1 in any measurable way**, apart from Varro and early Cicero. Because the pool holds no Cato file, the claim cannot be extended to Cato.

## What each normalisation merges and splits

Corpus side is `prose80` lemma types (68,651); dictionary side is Lewis & Short base keys (50,027) and Elementary base keys (16,986)
([`results/features3.json`](latin-lemma-conventions/results/features3.json), `features4.json`).

| Step | Merges on the corpus side | Merges on the dictionary side | Verdict |
| --- | --- | --- | --- |
| NFC + lowercase | 1,625 groups, 1,656 types: `IN`/`in`, `IS`/`Is`/`is`, `Ut`/`ut`, `Magnus`/`magnus`, `Graecus`/`graecus` | 616 L&S pairs, 17 Elementary: `Liber`/`liber`, `Cos`/`cos`, `Leo`/`leo`, `Mus`/`mus`, `Dis`/`dis` | keep: the corpus case is not reliable and the proper/common splits it loses carry few tokens |
| strip combining marks | `sum`/`súm`, `qui`/`quí`, `non`/`nón`, `ego`/`égo`: Plautine accents | none (keys are ASCII) | keep: 0.6 point on verse, 3.9 on Plautus |
| `j`→`i` | `is`/`js`, `iam`/`jam` | with `v`→`u`, 23 L&S groups beyond case, all variant spellings of one lexeme or letter pairs: `Iulus`/`Julus`, `Troia`/`Troja`, `Maius`/`majus`, `Iesus`/`Jesus`, `I`/`J` | keep: required to combine with Elementary (`j`-free) |
| `v`→`u` | `uoco`/`voco`, `seruus`/`servus`, `QVI`/`qui`, `CVM`/`cum` | (counted above) `Euander`/`Evander`, `navarchus`/`nauarchus`, `euoe`/`evoe`, `U`/`V`; Elementary has one such group | keep: same as above |
| `ae`,`oe`→`e` | `quae`/`que` (40,976 tokens), `nae`/`ne` (26,906), `aet`/`et` (74,693) | 242 more L&S groups beyond case, many distinct lexemes: `caedo`/`cedo`, `laena`/`lena`, `paene`/`pene`, `vae`/`ve`, `Troes`/`tres` | **reject**: it merges the relative pronoun with the enclitic and `ne` with `nae` |
| case-sensitive (no lowercase) | splits 1,614 capitalised/lowercase twins, 39,000 tokens | keeps the 616 proper/common pairs apart | reject: OLA's capitalisation is inconsistent |
| orthographic alias table (prefix assimilation, archaic `-os`/`-om`) | would merge `conparo`/`comparo` and the like | n/a | not identity: 0.09 % and 0.5 % of Plautus; a later lookup fallback, if wanted |
| strip the homograph digit | none (OLA has none) | joins 4,266 entries into 2,649 groups; 2,056 folded keys have two or more entries | do at index time, not in the profile; keep the digit as the entry reference |

The u/v and i/j folds have no dictionary-side cost: the 23 groups they create in L&S and the one in Elementary (`nauarchus`/`navarchus`) are variant spellings of a single lexeme or letter pairs, which they correctly unite. This is the evidence for folding.

## Fit with the existing profile shape

`internal/canonicalization` defines a `Profile` with `Name`, `Version`, `Language` and `Canonical(value string) string`
(`canonicalization.go`); the registry picks the active version per language, and `Lookup` keeps older versions for reproducibility. `ancient-greek/1`
(`ancient_greek.go`) is NFC, simple Unicode lowercase and final-sigma folding; accents are significant. It is a **dedicated-profile language**
(`dedicatedProfileLanguages`), so a missing or unknown version is an error, not a silent fallback. `la` has no profile: `For("la")` returns the language-neutral `unicode-casefold/1.2.0`
(NFC, whitespace collapse, full case folding), which is the second row of the ladder above. The dictionary index keys each source headword with the same profile at open time
(`loadGreekEntries`, `ancientGreekKey`, which admits only headwords with a letter), so a Latin index would apply `Canonical` to the L&S `key` and `<orth>` forms exactly as the Greek index does.

A `latin` profile in the same shape would be:

1. `norm.NFC`; decompose, drop `unicode.Mn`, recompose (the repository already has this step as `SearchKey` for Greek), plus ligature expansion (`æ`, `œ`; 8 occurrences in L&S `<orth>`) and Cyrillic `у`→`y` (1,061 L&S entries).
2. Simple Unicode lowercase (as Greek does), not full case folding.
3. `v`→`u`, `j`→`i`.
4. Registered as a dedicated-profile language like `grc`, with a reviewed fixture (the Greek profile has `ancient-greek-normalization/1`).

Admission is separate from the profile. A Latin index should reject lemma strings containing `.` or a digit (13,617 + 5,042 tokens, 9.3 % of the unmatched tokens; none of them matches a headword), the way the Greek index rejects headwords with no letter; otherwise a lowercased
`c` (the praenomen) reads as a headword.

## What would change this

- A gold lemma check. This research separates lemmatiser error from convention by a 120-token hand sample, not by a gold standard. If the CIRCSE Works (Germania and four Seneca tragedies) are used as the yardstick,
  the error share (about 5 % of all tokens) can be pinned down. It does not move the recommendation, which concerns how conventions agree, but it decides how much lemma review the product needs.
- A decision to ship Elementary. The folds recommended here are needed for L&S alone to gain 0.3 point; they are needed for the L&S + Elementary union because Elementary has no `j`.
- Cato or another early prose Work entering the pool. That is where an alias table could begin to pay.
- A decision to show the homograph entries by POS. OLA's POS does not select among L&S entries, so a POS-aware card would need a hand-built map for the few thousand ambiguous lemmas.

## Reproduction

Scripts and aggregate results are in [`latin-lemma-conventions/`](latin-lemma-conventions/). `common.py`, `dicts.py`, `measure.py`–`measure8.py`, `sample.py` read the CoNLL-U files fetched by `fetch_all.py`
(range-requests members of the Zenodo archive using `rangezip.py` from `research/latin-scheme-route` at `d6b4517d`) and the Perseus `lexica` checkout. They also read the two copied
inputs `manifest.tsv` (`latin-oga-equivalent` at `3c1318cf`) and `pins.tsv` (`latin-scheme-route` at `d6b4517d`). Run with `python3 -I`, `LEXICA=<checkout> python3 -I measure.py <conllu-dir> <outdir>`.
Only counts, IDs, digests, URLs and lemma strings are committed; no running text and no full annotations.
