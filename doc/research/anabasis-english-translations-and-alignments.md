# Anabasis Book 1: English translations and existing alignments

Research date: 2026-10-17. Throwaway research asset, not an accepted product decision.

For [Research: openly licensed English translations of Anabasis and existing alignments](https://github.com/justin-hayes/mouseion/issues/2128), under [Ancient-language cards without an LLM: public-domain translations, alignment and word sense](https://github.com/justin-hayes/mouseion/issues/2127).

**Investigation constraint clarification (Justin, 2026-10-17):** “I'm fine with treebanks and treebank inputs if we can confirm the license is not NC.” Treebank provenance alone is therefore **not** an exclusion for this investigation. The model and the identified input datasets need separate non-NC checks; an unresolved input licence is not affirmative clearance. This clarifies the investigation only, not the production import's accepted source contract.

## Answer

- **Use Brownson's Perseus `perseus-eng2` as the English input to the prototypes**, with the exact commit and digest below. Its TEI explicitly licenses the digitisation CC BY-SA 4.0; its 205 Book 1 section keys match the pinned OGA citations. This is **section correspondence, not sentence or word alignment**. [B][O]
- **Dakyns is the secondary candidate**, but correct the bibliographic starting point: *Anabasis* appeared in volume 1 of *The Works of Xenophon* in **1890**; 1897 belongs to volume 3. Gutenberg supplies the translation, but its exact print exemplar and a complete section mapping are not established. [D][DA]
- **Existing Book 1 word alignments do exist on Ugarit.** Three verified public records cover 1.5.12, 1.9.17–19 and 1.9.24. These are partial, contributor-created resources, not a verified complete Book 1 corpus or Justin's gold set. Their rights/provenance qualifications are below. [U1][U2][U3]
- **Do not adopt a pretrained aligner or a nominally “Anabasis” dataset on name/licence alone.** Ugarit's identified training sources carry NC notices (PROIEL and UD Perseus) or conflicting notices (Gorman); another alignment repository has misleading Anabasis filenames and synthetic/model-oriented material. Neither is a cleared prototype input. Treebank provenance alone is not the exclusion: see the licence audit in §4. [M][MP][TP][TU][GR][GL][N]
- **Textual mismatch matters even with identical section numbers:** Brownson includes bracketed English passages translating Greek that OGA omits, notably 1.7.15 and 1.8.6. Removing XML notes alone does not remove these passages. [B][G][O]

“Verified” below means inspected primary metadata/content or a reproducible measurement of the pinned files. Recommendations and legal assessments are identified as assessments, not human acceptance or legal advice. No provider/deck behaviour is changed.

## 1. Reproducible pins

### Greek baseline and Brownson

The compiled Mouseion definition at commit `a1a150f7fbe858414a16cde0d9da222dbc9a9d80` identifies **Marchant 1904**, range **1.1.1–1.10.19**, and the following complete-work file. The downloaded upstream bytes matched the compiled size and SHA-256. [DEF][O]

| Resource | Immutable pin / machine-readable file |
| --- | --- |
| `oga_src` v0.2.0, DOI 10.5281/zenodo.15199546 | Git commit `223d2258745ecf35e7659191080f93640fe900c1`; `data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu`; 7,438,037 bytes; SHA-256 `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61`. CoNLL-U dialect, with per-token MISC citations. [O][OZ] |
| Perseus `canonical-greekLit` | Commit `0189b9c8d8d1f148133f2f0af309b4ee90ea4332`. Use this commit, not `master`. [PC] |
| Brownson English | `data/tlg0032/tlg006/tlg0032.tlg006.perseus-eng2.xml`; git blob `72204fb46cf113a62773b9f1d003e8fcb460bd6d`; 1,101,217 bytes; SHA-256 `f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7`. TEI P5 XML. [B] |
| Marchant Greek comparison witness | Same directory, `tlg0032.tlg006.perseus-grc2.xml`; git blob `4978889462c3a72bda911051b708e423fc579db3`; 887,224 bytes; SHA-256 `5bf59352cab3dec763ecff9f4061385b8307146d2501432a5f5b178d6f31a4c2`. TEI P5 XML. This is **not** a substitute for OGA's pinned tokens. [G] |
| Perseus CTS metadata | Same directory, `__cts__.xml`; SHA-256 `bca057cc8842d7cbc05aecbbd812681abfeb8d0f763c085e363cec6dbe08cedf`. English URN `urn:cts:greekLit:tlg0032.tlg006.perseus-eng2`. [CTS] |

`oga_src` describes itself as a repackaging of OGA v0.2.0, with citation references extracted from OGA's CTS annotation, and licenses its release CC BY-SA 4.0. The baseline is this specific Anabasis file, not permission to ingest every upstream corpus mentioned by OGA. [OZ]

### Dakyns

The verified download is [Gutenberg #1170 UTF-8 text][DT]: **558,580 bytes**, SHA-256 `48785a24a81d7aacd7a59593d2846ed2208e30242f5fe2e7f0a596282d710b87`. Its header says release 1998-01-01, most recently updated 2024-10-29; the catalogue currently says last update 2024-05-16. **Use the byte digest, not either date, as the version check.** The URL is mutable; this is a fail-on-drift content pin, not an immutable hosted snapshot. HTML and EPUB are also offered, but only this TXT is recommended as the fallback pin. [D][DT]

## 2. Translation candidates and rights

### Brownson — preferred

**Bibliography.** The TEI source description identifies Carleton L. Brownson as translator, *Xenophon in Seven Volumes*, volumes 2–3, Harvard University Press / William Heinemann, **1921–1922**, Loeb Classical Library. The header also records scanning at St. Olaf in 1990 and the Perseus 2.0 release in 1996. These are the digitisation's bibliographic assertions; no fresh print-versus-digital collation was performed. [B]

**Original translation versus digital source.** Assessment for the US: the published 1921–22 translation is public domain by age, applying the maximum 95-year pre-1978 term and calendar-year termination in 17 USC §§304–305. Its maximum term therefore ended no later than 2017-12-31. This does not establish worldwide status or the status of later revised translations. [B][US]

Separately, **this digitisation** has an explicit `availability/licence` pointing to **CC BY-SA 4.0**, corroborated by the repository's licence. Do not replace that notice with “public domain” merely because the underlying translation is old. Conversely, the CC licence does not create exclusive rights in otherwise public-domain material: it licenses only rights the licensor actually holds. [B][PL][CC]

**Personal use:** CC BY-SA §2 permits reproducing and adapting, including privately; §3's attribution/share-alike conditions are triggered by sharing. A self-use deck export is not automatically publication. **Redistributed decks:** conservative recommendation is to attribute Xenophon, Brownson and Perseus/Tufts; identify the exact source/version and licence URL; retain supplied notices; describe extraction, note removal and alignment changes; license copyrightable adaptations under CC BY-SA 4.0 or an allowed compatible licence; impose no conflicting restrictions/DRM. This is permitted redistribution with conditions, not a non-commercial-only permission. Whether particular alignment/selection work constitutes adaptation, and the boundary between adapted material and a collection, are legal questions: do not assert that the app's code or every independent deck component becomes BY-SA. [CC] (§§1–4)

**Coverage and granularity.** Measurements on [B] and [G]: Book 1 has 10 chapters and **205 nonempty sections**, keys 1.1.1–1.10.19 in identical order. Brownson has **80 XML notes** and **zero `<s>` sentence elements** in that book. Its CTS declaration is book/chapter/section, not sentence/word. Exclude note content while preserving each note's XML tail. These facts establish a citation join, not a one-to-one translation for any OGA sentence. [B][G][CTS]; reproducible probe in §5.

**Edition mismatch.** Brownson is a separate translation edition, not an English rendering guaranteed to follow every OGA editorial choice. Concrete counterexamples are in §3. No full semantic collation of Brownson against Marchant's printed text or OGA was done. [B][G][O]

### Dakyns — fallback with coarser anchors

**Bibliography.** Gutenberg identifies Henry Graham Dakyns (1838–1911), and the text credits John Bickers and David Widger as producers. The library record for the printed *Works* explicitly places *Anabasis* in **volume 1, 1890**, with volume 3 parts dated 1897. This corrects the ticket's date; it does **not** prove which printing supplied Gutenberg's transcription. That exact exemplar, and Dakyns' base Greek edition, remain unverified. The original 1890 translation necessarily predates Marchant 1904; treating them as the same edition would be unjustified. [D][DT][DA]

**Rights.** Gutenberg labels this individual book “Public domain in the USA.” The US age-based assessment above also supports the underlying 1890 translation. Gutenberg's digitisation is **not presented as CC BY-SA**: its embedded Project Gutenberg licence distinguishes public-domain text from its trademark and compilation rights. [D][DT][US][PG]

**Personal use:** the embedded licence permits use under its terms. **Redistributed deck excerpts:** for a US-unrestricted work and US user, §1.C permits copying/adaptation/distribution without PG's trademark conditions if all PG references are removed. Alternatively, keeping it associated with PG invokes §1.E, including the prominent notice and full licence; do not assume that a tiny attribution alone satisfies that route. PG's explanatory policy distinguishes bibliographic acknowledgements from trademark branding. Recommendation: retain the download's complete provenance/licence in the research cache and select/document a compliant export route before redistribution; do not simply paste the branded ebook header into every card. Non-US use still needs local-law review (§1.D). [DT] (embedded licence §§1.C–1.E), [PG]

**Book 1 coverage.** The TXT contains Book I, chapter headings I–X, followed by Book II. It has sparse marginal numerals rather than a complete section markup. For example, chapter I's 1, 5 and 9 agree with the corresponding Greek passage anchors on inspection, but a full 205-section mapping was **not** established. Treat it as chapter text with sparse candidate section anchors, not as already section-aligned. Notes use parenthesised numbers; braces contain the preparer's Greek transliterations with diacritics lost. [DT][G]

**Textual differences.** Dakyns has the canal aside in the main translation and a footnote reporting the manuscripts' bareheaded-Persians phrase, unlike OGA's omitted Greek in 1.7.15 / 1.8.6. This is another reason not to use translation punctuation as a proxy for OGA segmentation. No exhaustive Dakyns-to-OGA collation was done. [DT] (chapter VII canal passage and chapter VIII note (2)), [O], [G]

Other translations were not exhaustively surveyed. Gutenberg's *The Retreat of the Ten Thousand* (Witt / Younghusband) is **not recommended**: its translator explicitly reports rearranging sentences/paragraphs and modifying, omitting or adding phrases. It is a retelling rather than the needed edition-faithful parallel witness. [W]

## 3. Greek correspondence and actual editorial differences

The baseline token MISC fields use `ref=Xen. Anab._book.chapter.section`. Independently counted on the digest-checked [O]:

| Measurement | Result |
| --- | --- |
| Sentences whose tokens fall within Book 1 | **607** |
| Distinct section refs | **205**, exactly the section-key set/order in [B] and [G] |
| Printed tokens retained / ellipsis tokens | **10,462 / 110**; the 110 have POS `-` |
| Sentences touching multiple sections | **3**: 1.2.17–18, 1.3.5–6, 1.8.5–6 |
| Sections touched by exactly one sentence | **31** |
| Sections owning exactly one sentence if assigned only by its first token | **32** — this reproduces the parent ticket's figure, but hides the overlaps |

These are source-file measurements, not a claim to have rerun a database import. The importer drops ellipsis nodes and reconstructs punctuation spacing; it uses source citations for the cut, then emits reconstructed sentence/token offsets. For the prototype, derive a sidecar from the exact source that retains **all** touched citations plus source sentence/token identity and checks reconstructed-text correspondence; do not assume the persisted sentence already carries a complete CTS mapping. [IMP]

**Character-aware check:** extracting Greek main text from [G] with `<del>` excluded, `<choice>` selecting `<corr>`, standalone `<sic>` retained, and `<add>` retained yields the **same NFC Greek-letter sequence in all 205 sections** as the printed tokens in [O], after lowercasing and final-sigma normalisation. Accents, breathings and iota subscripts are **retained** in this test. It ignores spaces, punctuation, elision marks and token boundaries; it is **not byte equality, a complete token binding or an independent annotation oracle**. OGA remains the sole annotation input. §5 gives the executable test. [G][O]

Specific observed editorial treatment:

- OGA omits the eight `<del>` spans at **1.2.12, 1.4.7, 1.7.13, 1.7.15, 1.8.6, 1.8.7, 1.8.16, 1.10.1**. [G][O]
- At **1.3.20**, Perseus encodes `<choice><sic>ἦν</sic><corr>ἢν</corr></choice>`; OGA has **ἢν**. An accent-stripping comparison would miss this distinction. [G][O]
- At **1.7.15**, Brownson's main `<p>` includes “[Here also are the canals, which flow from the Tigris river …]”. At **1.8.6** it includes “[In fact, it is said of the Persians in general …]”. Those English spans translate Greek `<del>` material omitted from OGA, and they are **not XML `<note>` elements**. Keep/omit policies therefore need explicit passage-level handling, never a blanket “strip all brackets” rule. [B][G][O]

This establishes the useful Marchant-derived textual relationship and its extraction qualifications. It does not prove that Brownson translated Marchant's exact readings throughout, nor that every English sentence has a Greek counterpart of the same extent.

## 4. Existing alignments: discovery is not clearance

### Ugarit Book 1 contributor records — real, partial word links

These public primary pages were fetched, not inferred from search snippets:

| Record | Claimed extent; creator/date; English attribution | Machine-readable evidence / status |
| --- | --- | --- |
| [U1] `text.php?id=34674` | **1.9.17–19**; Jacob Schwartz; created 2022-12-23; explicitly “Translated by Carleton L. Brownson” | HTML token IDs and shared `data-ref` link-group IDs; tokenised Greek and English. |
| [U2] `text.php?id=39624` | **1.5.12**; Nicole Kiernan; created/modified 2023-11-20 | Same HTML representation; no translator identified in the inspected record. |
| [U3] `text.php?id=34532` | **1.9.24**; Melanie Sun; modified 2022-12-22 | Same HTML; description “Based on Perseus Translation”. |

The pages and site's About footer display **CC BY-SA 4.0**. That is a primary site-level notice, not independent proof that each contributor had authority to license every embedded translation. Brownson's underlying text has the assessment above; the uncredited English in [U2] is **not cleared** for deck redistribution. A citation title also is not a source-edition/version declaration. [U1][U2][U3][UA][CC]

**Use assessment:** these are useful candidate links to inspect during the prototype; no complete Book 1 coverage or human-reviewed accuracy is established. Do not ingest them as a blanket cleared corpus, treat them as an approved Book 1 gold set, or silently expand their licence to other records. Private copying under an applicable CC grant and redistribution with attribution/share-alike are possible, but the contributor/translation provenance must be settled first. **Recommendation remains to build the primary prototype from Brownson + OGA and obtain Justin's own live-reviewed sample.**

**Formats/pins:** the linked `downloadTp.php?id=…` returns UTF-8 **TSV `GRC / ENG / Frequency`**, an aggregated phrase-pair lexicon. It is **not occurrence-level alignment**: use the HTML token IDs/link groups for occurrence identity. No immutable release was found for these pages; IDs and displayed edit dates are locators, not immutable versions. These fetched byte hashes are fail-on-drift pins, not author-issued archival versions:

| ID | HTML SHA-256 | Translation-pair TSV SHA-256 |
| --- | --- | --- |
| 34674 | `e5d891142483c7243fbf1869896cb4668eed8ceb9f57ca8c75d62025a36e0995` | `132fc2fb76c5b18a6599b9e1c1cec6d877889125e1d9b5bd63781849f575e213` |
| 39624 | `ee4d8ec5eb87a2d9a8adbaad8a3785cb085a6fdc45e724f123e71ba552b6484a` | `2cb84c3debf6e471973a9e110ec2c9dbeb49e0ea83a938cc86e3b70a17d51d1a` |
| 34532 | `c66e2338512a744c5a3ab106e314722d8e0fb47655174cca9f8b5c984755dcaa` | `127d33d5921560f321d4b707dad4c2a20386f0c9efb1b6de7e03af7dc7fb0c0f` |

URLs for these hashes use host **`www.ugarit.ialigner.com`**. The wider site's public records were not exhaustively enumerated. No API/release establishing full Book 1 coverage was found in this bounded search.

### Ugarit Grc–Eng gold standard — different works

The creators' [data paper][UGP] identifies expert manual word alignments from **Iliad, Crito and Cyropaedia**, **not Anabasis**, and describes JSON / NAACL sure-and-possible links and parallel-text files. The pinned [repository][UG] at commit `71ecd92479ae7025a6d3a3b340edf14ffe148281` has **274** JSON entries in `grc-eng/grc-eng-goldstandards.json`, with keys `grc`, `eng`, `alignment` (blob `bd3e3f264b4d09b5ea2eded3ae0f1abf5a7d8a30`). These are sentence-pair/token indices, not an Anabasis CTS mapping. [UG][UGP]

The repository says **CC BY-SA**, with a 4.0 licence file; [Zenodo 7362097][UGZ] and the data paper say **CC BY 4.0**. These are differing distribution notices, not permission to erase the pinned repository's SA condition. Conservative assessment: follow BY-SA for that repository distribution, attribute the annotators and underlying text producers, and verify underlying excerpt editions if redistributing. This resource is **method/evaluation background**, not Book 1 card content or Justin's gold. It does not itself label dictionary senses; word links cannot be scored as contextual-sense truth without additional labels. [UG][UGZ][UGP][CC]

### Pretrained Ugarit model — non-NC input condition not established

**Follow-up audit, 2026-10-17:** Justin allows treebanks and treebank inputs if their applicable licences can be confirmed non-NC. The model remains **not cleared**, but because of the input notices below, **not because treebanks are categorically prohibited**.

`UGARIT/grc-alignment`, revision `d1a6280cd4bd1911fee8751cd4fa64f4f5a16abf`, is an XLM-RoBERTa-based word aligner whose model card declares **CC BY 4.0**. Its card reports monolingual and parallel training, including Perseus Xenophon, but does not mention the treebanks. **The authors' paper §4.2 (p.104) explicitly names PROIEL, PERSEUS and Gorman treebanking datasets** alongside Perseus Digital Library and First1KGreek as sources for the 12-million-token masked-language-model training phase. Footnote 10 points to Universal Dependencies and footnote 11 to Gorman's site. The paper gives no exact training-data versions. [M][MP]

| Identified input | Verified primary licence notice | Assessment under the clarified condition |
| --- | --- | --- |
| PROIEL | UD Ancient Greek PROIEL `LICENSE.txt` at `8d53a43f6a7ddb380e02f61daa3744f4e43296b3` says **Attribution-NonCommercial-ShareAlike**. Its name says 3.0 while its URL says 4.0; both are NC. Upstream `proiel-treebank` README at `8e388967a1335ed12335ddc655fe46993ee7d57a` independently says **BY-NC-SA 3.0 US**. [TP][TPU] | **Not non-NC**, regardless of whether upstream or UD packaging was used. |
| PERSEUS via UD | UD Ancient Greek Perseus `LICENSE.txt` at `09cdc6f4d19f5fd31cc92ab7918e77c3b5551ea6` explicitly says **BY-NC-SA 2.5**. [TU] | **Not non-NC** under the distribution the paper links to. Exact training revision remains undocumented. |
| Gorman | README at `7e28185ff4a8eebb3a325f2c95ad13717d9db8e1` says **BY-NC-SA 4.0**, but `TREEBANK LICENSE` at `b690fad297c556d74f7dd6738e7c0619e349fcc5` is **BY-SA 4.0**, with no NonCommercial term. [GR][GL] | **Unresolved conflict**, not affirmative clearance. Obtain an authoritative clarification before using this dataset under the condition. |

**Important distinction:** original AGDT/Perseus `treebank_data` has a root README declaring **BY-SA 3.0 US**, not NC, at `1afdc9c2017a00f0022f08712c671acdfcf2dd1f`. It should not be blanket-labelled NC. That notice does not remove the separate UD conversion's NC notice or establish which exact bytes Ugarit used. Original AGDT is consequently a potential non-NC treebank lead for separate source-level review, **not a way to retroactively clear this checkpoint**. [TA][TU][MP]

**Conclusion:** the published input notices prevent confirming Justin's non-NC condition. Do not use this checkpoint in the recommended prototype without further evidence resolving the NC/provenance issue. This is **not** a legal conclusion that training was unlawful, that NC necessarily attaches to trained weights, or that the authors' model grant is invalid. Those are separate legal questions. The model card's CC BY grant and the input datasets' licence notices answer different questions. The paper's own publication licence is not a licence for its training data. [M][MP][TP][TPU][TU][GR][GL]

**Audit limits:** no weights or treebank annotation content were downloaded or used as input, repair witness or oracle. Only paper/licence/provenance metadata were inspected. Exact training revisions were not established, and this follow-up did not attempt a complete rights audit of First1KGreek, DFHG or every Ugarit contributor record: the identified NC treebank notices already prevent clearance. No positively documented alternative non-NC-trained Ugarit checkpoint was established.

The same paper reports 32,500 Perseus Greek–English sentence pairs, but the inspected model card/paper do **not** provide an independently pinned Anabasis Book 1 sentence-pair dataset with a source-specific rights manifest. Do not equate that training-data description with an available reusable corpus. [M][MP]

### Other Perseus-derived, treebank-linked and aligned-text searches

- **Alpheios:** its [enhanced-texts index][A] lists no Xenophon. This is a bounded negative for that index, not proof no Alpheios user ever aligned Anabasis. Under Justin's clarified investigation constraint, any newly discovered treebank-linked resource is eligible for review if its applicable data licence can be confirmed non-NC; discoverability alone is not clearance. [A]
- **Scaife ATLAS:** the full [text-alignment index][S], fetched on the research date, lists no Anabasis; it includes Iliad/Odyssey alignments and other works. The pinned canonical-greekLit Anabasis directory has the Greek/English editions and CTS metadata, not a sentence/word-link file. [S][PC]
- **Dynamic Lexicon:** the project's own [README][DL] describes 104 Greek–English documents, about 210,000 sentence pairs, Moore sentence alignment and Giza++ word alignment. Its pinned repository `c72abbb0ef391f7d3b3154013702cb1fab29f465` is a web application; the inspected tree provides no Anabasis alignment dump or corpus-specific licence/version. The linked live `dynamiclexicon.com` currently serves unrelated dog-name pages. Thus the historical corpus is a lead, **not a verified downloadable/licensed Book 1 candidate**. [DL][DLTREE][DLSITE]
- **Peter Nadel's `word-level-alignment`:** pinned commit `a2e5135fa428f1d7c2513d3f2a39ca85d3cffac0`, repository licence Apache-2.0. Despite the folder name `anabasis_sent_align_grc`, its four files are `0003-001`, `0016-001`, `0032-001` and `0032-007` — no `0032-006` Anabasis witness. The inspected `0032-001` opens with Hellenica's Thymochares passage; `0032-007` opens with Cyropaedia. `synthdata_anabasis_1219_data.jsonl` has 500 instruction/input/output rows, and its first output concerns Sicily, not Anabasis' opening. Repository title and code are explicitly LLM-alignment-oriented. **No reliable Book 1 extent, original-translation identity or human-gold provenance established; not recommended**, regardless of the permissive code/repo licence. No pickles/notebooks were executed. [N]
- **Current Perseus teaching work:** Gregory Crane's [first-party 2026 account][CR] describes manual sentence alignment, subsequent LLM assistance and GLAUx sentence identifiers in its learning materials. It supplies no cleared, pinned replacement here; treebank-linked/LLM-generated material must not be silently relabelled as a deterministic human-translation source. [CR][SPEC]

These searches found real partial alignments but **no verified, cleared, complete OGA-compatible Book 1 sentence/word corpus**. This is a bounded research conclusion, not a universal absence claim.

## 5. Reproducible core probe

Python standard library only. Save the three pinned [B], [G], [O] files under the names shown. Download from their immutable links below; verify **all three SHA-256 values** in §1 before running. This reads source text locally and prints only counts/refs. Greek normalisation here is a comparison diagnostic, **not a change to the application's vocabulary profile**.

```python
import collections
import hashlib
from pathlib import Path
import unicodedata as U
import xml.etree.ElementTree as E

names = {
    'e': ('tlg0032.tlg006.perseus-eng2.xml',
          'f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7'),
    'g': ('tlg0032.tlg006.perseus-grc2.xml',
          '5bf59352cab3dec763ecff9f4061385b8307146d2501432a5f5b178d6f31a4c2'),
    'o': ('tlg0032.tlg006.perseus-grc2_oga020_src.conllu',
          'c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61'),
}
data = {}
for key, (name, digest) in names.items():
    data[key] = Path(name).read_bytes()
    assert hashlib.sha256(data[key]).hexdigest() == digest
T = '{http://www.tei-c.org/ns/1.0}'
def book(raw):
    return next(d for d in E.fromstring(raw).iter(T+'div')
                if d.get('subtype') == 'book' and d.get('n') == '1')
def sections(b):
    return {f"1.{c.get('n')}.{s.get('n')}": s
            for c in b.findall(T+'div') for s in c.findall(T+'div')}
def text(x, greek=False):
    tag = x.tag.removeprefix(T)
    if tag in {'note', 'head'} or (greek and tag == 'del'):
        return ''
    if greek and tag == 'choice':
        corr = x.find(T+'corr')
        assert corr is not None
        return text(corr, greek=True)
    return (x.text or '') + ''.join(
        text(c, greek) + (c.tail or '') for c in x)
eb, gb = book(data['e']), book(data['g'])
eng, grc = sections(eb), sections(gb)
assert list(eng) == list(grc) and len(eng) == 205
assert all(text(s).strip() for s in eng.values())
assert len(list(eb.iter(T+'note'))) == 80
assert not list(eb.iter(T+'s'))

byref = collections.defaultdict(list)
touch, first = collections.Counter(), collections.Counter()
sentences = kept = ellipsis = 0
cross = []
for block in data['o'].decode().strip().split('\n\n'):
    ts = [l.split('\t') for l in block.splitlines()
          if l and not l.startswith('#')]
    if not ts:
        continue
    refs = [t[9].rsplit('|ref=', 1)[1].rsplit('_', 1)[1] for t in ts]
    if not any(r.startswith('1.') for r in refs):
        continue
    assert all(r.startswith('1.') for r in refs)
    sentences += 1
    first[refs[0]] += 1
    touch.update(set(refs))
    if len(set(refs)) > 1:
        cross.append(list(dict.fromkeys(refs)))
    for t, ref in zip(ts, refs):
        if t[3] in {'-', 'x'}:
            ellipsis += 1
        else:
            kept += 1
            byref[ref].append(t[1])
def letters(s):
    return ''.join(c for c in U.normalize('NFC', s).lower().replace('ς', 'σ')
                   if c.isalpha() and U.name(c, '').startswith('GREEK'))
assert list(byref) == list(grc)
assert all(letters(text(grc[r], greek=True)) == letters(''.join(byref[r]))
           for r in grc)
assert (sentences, kept, ellipsis) == (607, 10462, 110)
print('sentences/kept/ellipsis:', sentences, kept, ellipsis)
print('cross-section sentences:', cross)
print('one-sentence sections, touching/first-only:',
      sum(n == 1 for n in touch.values()), sum(n == 1 for n in first.values()))
print('Greek-letter-equal sections:', len(grc))
print('sections containing del:',
      [r for r, s in grc.items() if list(s.iter(T+'del'))])
```

Expected: 607 / 10,462 / 110; the three cross-section ranges above; 31 / 32; 205 Greek-letter-equal sections; the eight deletion-containing sections above. No human accuracy evaluation is implied.

## 6. Handoff and remaining uncertainty

For [Prototype: aligning public-domain English to Book 1 without an LLM](https://github.com/justin-hayes/mouseion/issues/2129):

1. Start from **[B] + [O] at the full pins in §1**. Use all citation keys touched by a Greek sentence, not its first key alone. Extract Brownson without notes, but explicitly inspect the bracketed/deleted-material mismatches in §3.
2. Maintain source English spans; do not rewrite English to force a one-to-one sentence fit. Record many-to-many/unaligned cases. Under the clarified investigation constraint, use no NC-licensed or licence-uncleared resource as a segmentation repair witness or evaluation oracle. The production import contract remains unchanged. [SPEC]
3. Ugarit's Book 1 records are partial candidate comparisons subject to provenance clearance. Its expert gold is a methodological reference from other works, not a substitute for the ticket's **live Justin-reviewed Book 1 sample**. [U1][U2][U3][UGP][PROTO]
4. If statistical alignment needs more parallel text, the pinned Brownson TEI contains all seven books, but **only Book 1 coverage/correspondence was checked here**. Wider training extraction and every tool/model licence need their own checks; historical corpus/model descriptions are not downloadable data licences. [B][MP]

For [Prototype: word sense in context for Ancient Greek without an LLM](https://github.com/justin-hayes/mouseion/issues/2130): use the same English/Greek pins and the eventual shared human-reviewed sample; treat translated spans as contextual evidence, **not dictionary-sense labels**. Do not infer that a word's English counterpart is necessarily an adequate short Gloss. Lexicon selection is the separate [Research: openly licensed Ancient Greek lexica for contextual word sense](https://github.com/justin-hayes/mouseion/issues/2133), not resolved here. [SENSE][UGP]

Unverified points that must not be smuggled into later claims:

- Exact Gutenberg print exemplar/base Greek edition, complete Dakyns section mapping, and exhaustive semantic comparison of either translation against OGA.
- Worldwide public-domain status, and legal classification of particular adapted deck/database components. The assessments here distinguish US underlying text from the digital source's stated terms.
- Full public-Ugarit enumeration, contributor authority/translation identity for uncredited records, immutable archival releases for individual records, or their accuracy against Justin's intended Gloss/translation standard.
- A complete, licensed, immutable Anabasis sentence/word dataset from the historical Dynamic Lexicon or reported model-training corpora.
- Byte/token/punctuation equality between current Perseus Greek and OGA: §3 deliberately makes the narrower Greek-letter comparison only.

These limits do not block a Brownson + OGA prototype. They do block claiming a ready-made reliable card translation, complete existing alignment, or accepted replacement of the provider.

## Primary sources

[B]: https://github.com/PerseusDL/canonical-greekLit/blob/0189b9c8d8d1f148133f2f0af309b4ee90ea4332/data/tlg0032/tlg006/tlg0032.tlg006.perseus-eng2.xml
[G]: https://github.com/PerseusDL/canonical-greekLit/blob/0189b9c8d8d1f148133f2f0af309b4ee90ea4332/data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml
[CTS]: https://github.com/PerseusDL/canonical-greekLit/blob/0189b9c8d8d1f148133f2f0af309b4ee90ea4332/data/tlg0032/tlg006/__cts__.xml
[PC]: https://github.com/PerseusDL/canonical-greekLit/tree/0189b9c8d8d1f148133f2f0af309b4ee90ea4332/data/tlg0032/tlg006
[PL]: https://github.com/PerseusDL/canonical-greekLit/blob/0189b9c8d8d1f148133f2f0af309b4ee90ea4332/license.md
[O]: https://raw.githubusercontent.com/mgbilby/oga_src/223d2258745ecf35e7659191080f93640fe900c1/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu
[OZ]: https://zenodo.org/records/15199546
[DEF]: https://github.com/justin-hayes/mouseion/blob/a1a150f7fbe858414a16cde0d9da222dbc9a9d80/internal/ancientgreek/ancientgreek.go#L23-L38
[IMP]: https://github.com/justin-hayes/mouseion/blob/a1a150f7fbe858414a16cde0d9da222dbc9a9d80/internal/ancientgreek/ogasrc.go
[SPEC]: https://github.com/justin-hayes/mouseion/issues/2098
[PROTO]: https://github.com/justin-hayes/mouseion/issues/2129
[SENSE]: https://github.com/justin-hayes/mouseion/issues/2130
[US]: https://www.copyright.gov/title17/92chap3.html
[CC]: https://creativecommons.org/licenses/by-sa/4.0/legalcode.en
[D]: https://www.gutenberg.org/ebooks/1170
[DT]: https://www.gutenberg.org/cache/epub/1170/pg1170.txt
[DA]: https://archive.org/metadata/worksxenophon00dakygoog
[PG]: https://www.gutenberg.org/policy/license.html
[W]: https://www.gutenberg.org/cache/epub/71894/pg71894-images.html
[U1]: https://www.ugarit.ialigner.com/text.php?id=34674
[U2]: https://www.ugarit.ialigner.com/text.php?id=39624
[U3]: https://www.ugarit.ialigner.com/text.php?id=34532
[UA]: https://ugarit.ialigner.com/about.php
[UG]: https://github.com/UgaritAlignment/Alignment-Gold-Standards/tree/71ecd92479ae7025a6d3a3b340edf14ffe148281
[UGZ]: https://zenodo.org/records/7362097
[UGP]: https://openhumanitiesdata.metajnl.com/articles/10.5334/johd.131
[M]: https://huggingface.co/UGARIT/grc-alignment/blob/d1a6280cd4bd1911fee8751cd4fa64f4f5a16abf/README.md
[MP]: https://aclanthology.org/2022.lt4hala-1.14.pdf
[A]: https://alpheios.net/pages/texts/
[S]: https://atlas.perseus.tufts.edu/text-alignments/
[DL]: https://github.com/OpenGreekAndLatin/GRC-LAT-Dynamic-lexicon/blob/c72abbb0ef391f7d3b3154013702cb1fab29f465/readme.md
[DLTREE]: https://github.com/OpenGreekAndLatin/GRC-LAT-Dynamic-lexicon/tree/c72abbb0ef391f7d3b3154013702cb1fab29f465
[DLSITE]: https://www.dynamiclexicon.com/
[N]: https://github.com/pnadelofficial/word-level-alignment/tree/a2e5135fa428f1d7c2513d3f2a39ca85d3cffac0
[CR]: https://sites.tufts.edu/perseusupdates/2026/03/10/towards-corpus-based-learning-exercises-with-ancient-greek/
[TP]: https://github.com/UniversalDependencies/UD_Ancient_Greek-PROIEL/blob/8d53a43f6a7ddb380e02f61daa3744f4e43296b3/LICENSE.txt
[TPU]: https://github.com/proiel/proiel-treebank/blob/8e388967a1335ed12335ddc655fe46993ee7d57a/README.md
[TU]: https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/09cdc6f4d19f5fd31cc92ab7918e77c3b5551ea6/LICENSE.txt
[TA]: https://github.com/PerseusDL/treebank_data/blob/1afdc9c2017a00f0022f08712c671acdfcf2dd1f/README.md
[GR]: https://github.com/vgorman1/Greek-Dependency-Trees/blob/7e28185ff4a8eebb3a325f2c95ad13717d9db8e1/README.md
[GL]: https://github.com/vgorman1/Greek-Dependency-Trees/blob/b690fad297c556d74f7dd6738e7c0619e349fcc5/TREEBANK%20LICENSE
