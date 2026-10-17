# French NLP Resources Assessment

**Research date:** 2026-10-17

**Ticket:** #2139 (child of Wayfinder map #2138)

**Scope:** Assess French (`fr`) NLP resources for the modern-language reading
pipeline: sentence segmentation, tokenization (including multi-word tokens and
elision), lemmatization, UPOS/morphology, dependency parsing, and the Go
consumers that turn analysis into prepared decks. Target variety is
contemporary standard written French. Regional and historical French is out of
scope and is documented below as a limit, not a promise.

## Executive Recommendation

Stanza's French default package is the lowest-risk addition to the existing
Stanza architecture, and it needs no processor change: the `fr` pipeline is the
same shape as the current Italian config, and Stanza adds the `mwt` processor
automatically for French. Adopt-or-not is a human decision (see open
questions); this note does not make it.

The recommendation is conditional on fixing one launch-critical gap that is
independent of the NLP model choice. The Go card-export and target-matching
code cannot match a target word that is preceded by an elided article or
pronoun written with a typographic apostrophe, and it cannot match one written
with an ASCII apostrophe either. Concretely, `targetIndex("Je vois l’homme ici.",
"homme")` returns `-1`, and so does the ASCII variant. Without a fix, a large
share of French nouns and verbs with elided forms (`l'homme`, `d'accord`,
`s'appuie`) will be silently dropped from prepared decks. This is a Go-side
change, not a model problem.

Other Go-side gaps are cosmetic for launch but must be decided before French is
advertised as supported: the GDEX quality gate is German-only, book-cover article
lists and the active-language display labels have no `fr` entry, and the
dictionary index builder has no `fr` source.

spaCy's `fr_core_news_*` models are a credible second option for a bake-off but
are weaker on the published numbers and carry the same LGPL-LR license family
as the Stanza treebank. UDPipe 2 and Trankit are not recommended as the default:
UDPipe's models are CC BY-NC-SA (non-commercial), and Trankit adds a large
XLM-R dependency with weaker maintenance signals.

## Existing Architecture Constraints

- The NLP service is authoritative for language capabilities
  ([ADR 0023](../adr/0023-nlp-capabilities.md)). There is no web-app language
  allowlist, so adding `fr` is a provisioning and capability change plus
  consumer fixes.
- Models are provisioned on a volume, not baked into the image
  ([ADR 0063](../adr/0063-stanza-models-on-volume.md)).
- Adding a language with multi-word tokens required an explicit processor
  decision for Greek ([ADR 0073](../adr/0073-modern-greek-language-support.md)).
  For French, that decision is simpler: the Stanza default package already
  includes `mwt`, and `add_mwt` adds it on both the runtime and download paths
  (read from Stanza 1.15.0 source; not run end to end here).
- The current NLP config has no `fr` entry (`nlp/src/mouseion_nlp/model_config.py`),
  so French would fall back to Stanza defaults. Provisioning
  (`nlp/src/mouseion_nlp/provision.py`) and serving
  (`nlp/src/mouseion_nlp/producer.py`) use one shared processor string, which is
  already sufficient for French through the automatic MWT addition.

Sources: [ADR 0023](../adr/0023-nlp-capabilities.md),
[ADR 0063](../adr/0063-stanza-models-on-volume.md),
[ADR 0073](../adr/0073-modern-greek-language-support.md),
[language support](../features/language-support.md).

## Verified Facts vs. Expectations

Each claim is tagged. **Verified** means read from a primary source or a file
in the repo, or reproduced with a throwaway probe. **Expected** means a
reasonable inference that has not been run or measured here.

### Stanza 1.15.0 French package (verified from source and resource manifest)

- Default French package: tokenize `combined_nocharlm`, mwt `combined`, lemma
  `combined_nocharlm`, pos and depparse `combined_charlm`, ner
  `wikinergold_charlm`, optional coref `udcoref_xlm-roberta-lora`.
- The combined French training data is GSD, ParisStories, Rhapsodie, and
  Sequoia (Stanza `combined_models` page). ParTUT is not in the default.
- Approximate download sizes (HEAD Content-Length): tokenize 0.67 MB, mwt
  0.54 MB, lemma 2.8 MB, pos 34.6 MB, depparse 148.8 MB, pretrained embeddings
  (conll17) 107 MB, charlm forward and backward 20 MB each. The optional
  camembert-large coref variant is about 1.35 GB and is not needed for this
  pipeline.
- Per-treebank UD 2.12 scores (Stanza performance page, v1.5.1). The combined
  model is **not** in that table, so these are per-treebank reference points,
  not the score of the default package:

| Treebank | Lemma acc. | LAS |
|---|---|---|
| GSD | 98.20 | 91.28 |
| Sequoia | 98.50 | 91.57 |
| ParTUT | 97.35 | 89.58 |
| Rhapsodie | 97.74 | 80.72 |
| ParisStories | 97.66 | 73.95 |

Expected: the combined model's lemma and UPOS accuracy will sit near the GSD
and Sequoia rows. The low LAS on Rhapsodie and ParisStories (spoken-register
and dialogue text) is a known limitation for learner reading of narrative prose,
not a reason to reject the model.

### Treebank and tokenization evidence (verified on UD r2.12 dev files)

- **Multi-word tokens.** `au` splits to `à` + `le`; `du` and `des` split to
  `de` + an article. In GSD dev there are 395 MWT `du` occurrences against 7
  single-token DET `du`. The pipeline's per-syntactic-word mapping in
  `producer.py` (`_map_sentence`) therefore handles French contractions as
  intended, and the lemma of the article part is `le`/`les`, not the surface
  form.
- **Elision.** `l'` is a DET token with lemma `le`. The elided word is kept as
  a separate token, which is why the surface text and the token boundaries
  disagree for matching purposes (see the Go findings below).
- **Pronominal clitics.** `s'` is a PRON with dependency `expl:pv` and lemma
  `soi`; the verb keeps its non-reflexive lemma (`appuyait` → `appuyer`).
- **Negation.** `ne`/`n'` is ADV, not PART.
- **Apostrophe form.** Treebank text uses ASCII `'` (1,453 tokens in GSD dev, 0
  U+2019). The UD tokenization page uses `’` in its examples. **Expected**
  (not sampled here): learner books, especially publisher EPUBs, may use `’`
  as well, which would make the treebank form a mismatch. **Expected:** the
  tokenizer outputs the apostrophe inside the token, so a typographic apostrophe
  in input may produce different token splits than the treebank; this is not
  measured.

### Go consumer behavior (verified by reading and throwaway probes, removed)

- `internal/selection`: AllowedPOS is NOUN/VERB/ADJ/ADV, which is language
  neutral and works for French once UPOS is correct.
- `internal/canonicalization`: `fr` falls to the generic
  `UnicodeCasefoldProfile` (NFC and casefold; accents preserved). This is the
  right behavior for French, where accents are meaningful (`marché` ≠ `marche`).
- `internal/cardexport`: GDEX runs only when `isGerman`. Non-German uses the
  text-only `scoreSentenceQuality` gate (word count, `targetIndex` whole-word
  match, sentence boundaries, noise).
- **Verified failure:** `targetIndex` returns `-1` for elided targets:
  - `"Je vois l’homme ici."`, target `homme` → `-1`
  - `"Je vois l'homme ici."` (ASCII), target `homme` → `-1`
  - `"Il est d’accord avec nous."`, target `accord` → `-1`

  The cause is that `FoldedWordSpan` and `isWordRune`
  (`internal/textmatch/match.go`) count both `'` and `’` as word characters, so
  an elided target (`l’homme`) is one token and does not match the bare word
  `homme` as a whole word.
- **Verified control:** `"Je vais au marché."`, target `marché`, matches at index
  11. Accented letters work.
- **Verified:** a constructed impersonal parse (`Il faut partir.`, `expl:subj`)
  would be rejected by a GDEX-style knock-out, which accepts only `nsubj`,
  `nsubj:pass`, and `csubj` as subjects, with the message "no finite verb and
  subject". This is German-only today, so it is not a live defect, but a
  French GDEX port must account for `expl:subj`.
- `internal/webapp/components.go:135`: book-cover article lists exist for `de`,
  `it`, and `el` only. No `fr` entry.
- `internal/webapp/active_language.go`: display labels for `de`, `it`, `el`
  only.
- `internal/enrichment/lexical.go:309`: context stopwords include Italian and
  German, with no French entries.
- `internal/dictionary/index.go:146`: the gender/article branch is `el`-only.
- `scripts/derive_dictionary_index.py`: `LANGUAGES = {"de","it","el"}`, so
  French has no dictionary index.

### Alternatives (verified published numbers; not run here)

- **spaCy `fr_core_news_{sm,md,lg}` 3.8.0.** Trained on UD Sequoia v2.8 plus
  WikiNER (CC BY 4.0). Held-out lemma accuracy: sm 0.908, md 0.912, lg 0.914.
  Dependency LAS: sm 0.836, md 0.859, lg 0.857. These are spaCy's own splits,
  so they are **not** directly comparable to the Stanza UD table above. License
  of the model packages is LGPL-LR.
- **UDPipe 2, UD 2.17 French models.** Distributed under CC BY-NC-SA, which is
  non-commercial. The table columns for French were not verified, so no numbers
  are used here. Not recommended as a default for that license reason alone.
- **Trankit.** Lists French as supported. Not evaluated in detail. Large
  XLM-R dependency and older constraints make it a poor default for this
  service, as already concluded for Greek.
- **Kaikki French dictionary** (for gloss enrichment): extracted 2026-10-03 from
  the English Wiktionary dump, English glosses. The Kaikki page states no
  license. Wiktionary text is CC BY-SA 4.0 and GFDL (English Wiktionary
  copyright page). The Kaikki extract's own license statement must be confirmed
  before use.

### Licenses (verified from repository README/metadata pages)

| Resource | License | Implication |
|---|---|---|
| Stanza code | Apache-2.0 | OK |
| Stanza UD-built models | "unclear" per Stanza performance page | Confirm before redistribution; self-hosted use is the current scenario |
| GSD | CC BY-SA 4.0 (annotations; text "as is") | Share-alike applies to annotation derivatives |
| Sequoia | LGPL-LR | Confirm applicability to model weights |
| Rhapsodie, ParisStories | CC BY-SA 4.0 | Share-alike |
| ParTUT | CC BY-NC-SA 4.0 | Non-commercial; not in Stanza's combined default, so not a concern for the default |
| spaCy fr models | LGPL-LR (models); WikiNER CC BY 4.0 | Confirm |
| UDPipe 2 UD 2.17 French | CC BY-NC-SA | Non-commercial |

These are summaries for planning, not legal advice. A human should confirm the
combined Stanza model license before any distribution beyond the home-lab
deployment (see open questions).

### Version gap (verified)

The UD French page (v2.17) states the modern treebanks moved to SUD-based
features in v2.17. Stanza's stated resource release is UD 2.12. **Expected:**
the UPOS and lemma conventions are stable across these versions, but dependency
labels for some constructions may differ. The current Go code does not depend on
dependency relation names except the GDEX subject check, which is German-only.

FTB is retired in UD and is not a source.

## Footprint

- Provisioning adds: tokenize, mwt, lemma, pos, depparse, charlm forward and
  backward, and the Stanza `combined` French pretrained embeddings. Roughly
  **0.3 GB** of Stanza models on top of the existing German and Italian volume
  (the depparse model at 149 MB and the pretrained embeddings at 107 MB are the
  largest). Verified by HEAD Content-Length; the runtime memory footprint was
  not measured.
- Optional camembert-large coref is about 1.35 GB and should stay out of the
  default provisioning.

## Bounded Validation Plan (no model downloads in this ticket)

1. Add a `fr` probe in a scratch environment (not the repo) that runs the default
   Stanza French pipeline over 20 to 30 sentences from a public-domain
   contemporary French text, and records token splits for `au`, `du`, `des`,
   `l'`, `d'`, `s'`, `n'`, with ASCII and typographic apostrophes.
2. Compare the lemma and UPOS output on the same sentences against an
   application-corpus sample of the learner's own French books, using a
   hand-labelled subset of about 200 tokens.
3. Run the same sample through `targetIndex` and the card-export gate, and
   count the drops by cause (elision, apostrophe form, GDEX-equivalent).
4. Only if the Stanza result is unacceptable, benchmark spaCy `fr_core_news_md`
   on the same sample.

Stop criteria: if elided targets are not fixed in Go, do not advertise French
as supported, because the deck output would be systematically incomplete.

## Limits

- Contemporary standard written French only. Regional varieties, Quebec and
  African French lexis, and historical French (pre-20th-century orthography and
  syntax) are not covered by any of the resources above.
- Spoken and dialogue text (Rhapsodie, ParisStories) scores much lower on LAS
  than written text. Novels with heavy dialogue will have weaker dependency
  parses.
- No accuracy was measured on Mouseion's own corpus. All accuracy figures are
  published numbers on different splits.

## Open Questions for the Parent

1. Should curly apostrophes (`’`) be normalized to ASCII `'` before analysis
   and matching, or should `targetIndex` and `isWordRune` treat elided forms as
   word-boundary splits? This affects every French book and is the largest
   launch risk.
2. Does a French GDEX-equivalent knock-out need the `expl:subj` relation
   accepted, and should it be a port or a separate French rule set?
3. The UD version gap (Stanza 2.12 vs UD 2.17 SUD features): is it acceptable
   for the first French release, or should an alternative with a newer release
   be benchmarked?
4. The combined Stanza French model's license status for redistribution, and
   the Kaikki extract's license statement, need a human decision.
5. Should French get a dictionary index (`scripts/derive_dictionary_index.py`,
   `internal/dictionary/index.go`) and book-cover article lists before launch,
   or can those wait?
6. Is the `fr` context-stopword list (`internal/enrichment/lexical.go`) needed
   for the first release?
