# Sentence-study grammar signals: what the analyzer can trustworthily supply

**Research date:** 2026-10-13 · **Web sources accessed:** 2026-10-13
**Ticket:** [#1987](https://github.com/justin-hayes/mouseion/issues/1987) (child of map [#1983](https://github.com/justin-hayes/mouseion/issues/1983))
**Scope:** German (`de`), Italian (`it`), Modern Greek (`el`, not Ancient Greek). Candidate grammatical and structural signals that Mouseion could explain or filter on in Sentence study, judged by what Stanza output and UD guidelines actually supply, and how far accuracy evidence reaches.
**Status:** Research evidence only. Not a product decision, not an implementation plan, and not promoted to accepted repo documentation.

## Evidence labels

- **[V]** Verified from a primary source read during this research (Stanza source/manifest, UD guideline page, Stanza performance page).
- **[R]** Verified from this repository's code or committed evidence (file cited).
- **[I]** Inference or recommendation drawn from [V]/[R]. Not separately measured.
- **[U]** Unknown or not verified here. Each is listed under Follow-up unknowns.

## Summary

1. **Relation labels are the most trustworthy signal.** UD defines `acl:relcl`, `aux:pass`, `nsubj:pass`, `obl:agent`, `expl:pass`, `compound:prt` and related subtypes for each of the three languages [V]. Per-relation accuracy is not published for any of them [U]; only aggregate UAS/LAS is published [V].
2. **The deployed `de` and `it` models are Stanza "combined" models, and no published accuracy table covers them.** Stanza's performance table excludes combined models [V]. The German and Italian rows that exist are single-treebank proxies (GSD, HDT, ISDT) from Stanza v1.5.1 on UD 2.12, not the models Mouseion runs [V].
3. **Greek accuracy is the best-documented of the three** for the deployed configuration: GDT-based tagging and parsing, with a GreekBERT parser whose dev/test LAS is quoted in Stanza's source comments [V]. Those figures predate the pinned Stanza 1.15.0 and are not a per-relation measure [V][U].
4. **"Subordinate clause" is an overclaim as a single signal.** UD's `acl` covers finite and non-finite clauses [V], and the existing German scorer treats any `acl`/`advcl`/`ccomp`/`csubj` ancestor as "subordinate" [R]. Show the specific relation instead.
5. **"Passive" and "case" are not morphological facts in German.** German has no Voice feature and its nominal case is mostly marked on the article, so passive comes from relations and case from determiners, pronouns, and adjectives [V].
6. **Subject-based checks do not transfer to Italian.** Italian is pro-drop, so a finite verb without an overt subject is normal [V]. The current finite-verb-and-subject knock-out is German-only by ADR scope [R].
7. **Separable verbs are German-only and have one measured false positive** in the repository's own 10-event sample (`überwerden`), which that report flags as debatable [R].

## Scope and method

- Read the ticket, map #1983 (Notes and Not-yet-specified), AGENTS.md, the head of CONTEXT.md, `doc/features/dependency-parse-foundation.md`, `doc/features/language-support.md`, the Concordance and sentence-study sections of `doc/features/vocabulary-browse-concordance-and-custom-decks.md`, the relevant parts of ADR 0062 and 0073, `doc/research/modern-greek-nlp.md`, and `doc/evidence/german-separable-verb-precision.md`.
- Inspected the local pipeline: `nlp/src/mouseion_nlp/model_config.py`, `producer.py`, `precision.py`, and the Go persistence/UI types listed below.
- Downloaded the Stanza 1.15.0 resource manifest and read Stanza v1.15.0 source (`default_packages.py`, `pipeline/core.py`, `resources/common.py`) from GitHub tag `v1.15.0`. These are read-only downloads into scratch space. No Stanza model was loaded, no text was run through any analyzer, and no external NLP or LLM service was used. No private book text was touched.
- No new NLP implementation or experiment was run.

## What Mouseion stores and shows today

| Layer | Fact | Label |
| --- | --- | --- |
| `corpus_tokens` (baseline migration) | Per token: `surface`, `raw_lemma`, `canonical_lemma`, `upos`, `morphology jsonb`, `dependency`, `head`, offsets. `head` is the 0-based token ordinal; roots are self-headed with `dependency = 'root'`. | [R] `migrations/000001_initialize.up.sql` (corpus_tokens); `doc/features/dependency-parse-foundation.md` |
| Producer | Morphology is the Stanza `feats` string split into a key/value map. Multi-word-token words are flattened and share the source span. Heads map from Stanza word IDs to flattened ordinals. | [R] `producer.py` (`_morphology`, `_map_sentence`) |
| Separable verbs | German only. A `VERB` token gets its canonical lemma rebuilt from `compound:prt` dependents whose surface is in a closed prefix set. | [R] `producer.py` (`GERMAN_SEPARABLE_PREFIXES`, `_reattach_separable_verbs`) |
| Sentence study page | Shows surface, UPOS, raw and effective lemma, dependency label, and head surface for each token. Morphology is persisted but **not** shown. | [R] `internal/webapp/concordance.templ` (~line 145); `internal/domain/domain.go` (`SentenceStudyToken`, no morphology field) |
| Sentence-quality scorer | German-only, first pass, by ADR scope. Uses `acl`/`advcl`/`ccomp`/`csubj` as "subordinate" and requires a finite root with a subject. | [R] `internal/gdex/gdex.go`; `doc/adr/0062-derived-sentence-quality-scoring.md` |

Implication: the learner can already see raw relation labels and head tokens. Morphology and any derived grammatical claim are not yet exposed, so any new explanation would be a new claim and needs the evidence rules below.

## Pipeline configuration per language

| Lang | Mouseion config (`model_config.py`) | Stanza 1.15.0 manifest default | UD basis of the default | Label |
| --- | --- | --- | --- | --- |
| `de` | `tokenize,pos,lemma,depparse`, no package | tokenize `combined_nocharlm`, mwt `combined`, lemma `combined_nocharlm`, pos `combined_charlm`, depparse `combined_charlm` | "combined": UD German-GSD plus lemma data composed from German Wiktionary | [V] manifest; [V] combined-models page |
| `it` | `tokenize,pos,lemma,depparse`, no package | same processor pattern as `de` (mwt `combined`) | "combined": UD Italian ISDT, VIT, PoSTWITA, TWITTIRO, plus an MWT list | [V] manifest; [V] combined-models page |
| `el` | `tokenize,mwt,pos,lemma,depparse`; tokenize/mwt `gdt`, pos/lemma `gdt_nocharlm`, depparse `gdt_greek-bert` (external GreekBERT) | default_processors: tokenize `gdt_nocharlm`, mwt `gdt`, lemma/pos/depparse `gdt_nocharlm` | UD Greek-GDT (CC BY-NC-SA 3.0 per existing repo note) | [V] manifest and `model_config.py`; [R] `doc/research/modern-greek-nlp.md` |

Points that matter for claims:

- **MWT is added automatically for `de` and `it`, but the stored processor string does not show it.** Stanza's `maintain_processor_list` calls `add_mwt` when `tokenize` is listed and no `mwt` is (`pipeline/core.py` line 241; `resources/common.py` `add_mwt`) [V]. Whether that branch fires for the resolved `de`/`it` tokenizer depends on name resolution I did not execute [U]. The marker therefore records `processors` without `mwt` for `de`/`it` even if `mwt` runs. This is a documentation/marker discrepancy, not a correctness finding, until a runtime check says otherwise.
- **Italian default training data includes social-media treebanks** (PoSTWITA, TWITTIRO) [V]. That is a register risk for literary prose. The effect on Italian accuracy for book text is not measured [U].
- **The Greek parser is not Stanza's default.** Stanza's default Greek depparse is `gdt_nocharlm`; Mouseion uses `gdt_greek-bert` [V][R]. Stanza's source comment gives Greek-GDT LAS dev/test for no transformer (88.21 / 89.51) and for GreekBERT (92.46 / 92.68) [V]. The comment does not give a Stanza version or per-relation breakdown.
- The existing report `doc/evidence/german-separable-verb-precision.md` was measured on Stanza 1.14.0, while the pin is 1.15.0 [R]. Treat it as a historical sample.

## Accuracy evidence available

End-to-end scores from Stanza's performance page, **historical: Stanza v1.5.1 on UD 2.12** [V]. These are single-treebank proxies for the deployed `de`/`it` combined models, which the page excludes [V]. Greek GDT is the same treebank family as deployed, but the page does not say which Greek variant it reports [U].

| Treebank (proxy for) | UPOS | UFeats | Lemma | UAS | LAS | MLAS | BLEX |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| German GSD (`de` default, partial) | 95.61 | 89.78 | 97.23 | 85.80 | 81.80 | 62.81 | 74.04 |
| German HDT (`de` default, partial) | 98.30 | 92.57 | 98.04 | 95.59 | 93.56 | 78.81 | 87.18 |
| Italian ISDT (`it` default, partial) | 98.41 | 98.14 | 98.23 | 93.29 | 91.61 | 85.05 | 85.17 |
| Greek GDT (`el`, closest) | 97.71 | 94.72 | 96.05 | 91.22 | 88.87 | 77.99 | 79.08 |
| Greek GUD (not used) | 96.52 | 92.50 | 92.63 | 89.81 | 85.33 | 68.93 | 69.75 |

Source: [Stanza performance](https://stanfordnlp.github.io/stanza/performance.html) (accessed 2026-10-13). Columns are the table's UFeats, Lemmas, UAS, LAS, MLAS, and BLEX.

What this does and does not support:

- **[V]** Aggregate parse quality is in the high 80s to mid 90s LAS. Relation-level accuracy is not in the table.
- **[V]** UFeats is a bundle score across all features. It says nothing about any single feature such as Case or Mood.
- **[I]** For a sentence with 5 or more clausal levels, the chance of at least one wrong attachment is much higher than for a flat sentence. No measurement supports a number here [U].
- **[R]** The German separable-verb sample: 101 sentences, 2095 tokens, 10 reattachment events, 10 accepted particle observations, 2 skipped, 1 spot-check flagged debatable. Single public-domain author (Goethe's *Werther*). Stanza 1.14.0 [R].

## Cross-language notes from UD guidelines

These are the guideline facts behind the matrix. Each is [V] from the UD page cited.

- **`acl` contrasts with `advcl` and covers finite and non-finite clauses modifying a noun** [V: UD dependency-relation table `u/dep/all.html`]. `acl:relcl` is a language-particular subtype for relative clauses [V: same table; `de/dep/acl-relcl.html`; `it/dep/acl-relcl.html`]. German also defines `advcl:relcl` for relatives whose antecedent is a clause [V: `de/index.html`].
- **German:**
  - Features: Mood is `Ind`, `Imp`, `Sub`. Tense is only `Pres` or `Past`. `Aspect` and `Voice` are not used, because perfect and passive are periphrastic [V: `de/index.html`].
  - Case values are `Nom`, `Gen`, `Dat`, `Acc`, on NOUN/PROPN/PRON/ADJ/DET. Case on nouns is "extremely ambiguous", mostly signalled by the article [V].
  - Core `obj` is bare accusative only. Bare dative/genitive objects and prepositional objects are `obl:arg` [V].
  - Passive: `aux:pass` for the passive auxiliary; `nsubj:pass`/`csubj:pass` for the subject; `obl:agent` for the agent [V]. `werden` is AUX for both the future and the passive [V].
  - Reflexives: `expl:pv` for inherently reflexive verbs; `obj` for true reflexive/reciprocal `sich`; the German page notes that `sich` must be decided by context [V].
  - Separable prefix: `compound:prt`. In main clauses the particle is separate; in subordinate clauses the particle verb is one token [V: `de/dep/compound-prt.html`]. `sauber machen` may be `xcomp` rather than `compound:prt` [V].
  - Infinitival complements of control or phasal verbs are `xcomp`; finite complement clauses of accusative-object verbs are `ccomp` [V].
  - Negation: `nicht` is PART with `Polarity=Neg` [V].
  - Modal verbs (`dürfen`, `können`, `mögen`, `müssen`, `sollen`, `wollen`) are AUX [V].
- **Italian:**
  - Features: Mood is indicative, imperative, conditional, or subjunctive. Tense is past, present, future, or imperfect [V: `it/index.html`].
  - No Case feature is listed for Italian [V].
  - Core arguments are bare nominals; prepositional arguments are `obl` [V].
  - Passive: `aux:pass` (essere, venire); `nsubj:pass`; `obl:agent`; `expl:pass` for the reflexive clitic in reflexive passives [V]. Impersonal clitic use is `expl:impers` [V].
  - Clitics: `dobj`, `iobj`, `expl`, `expl:impers`, `expl:pass` [V]. Clitics attached to imperatives and non-finite forms are MWTs [V].
  - Modal verbs carry the language-specific POS `VM`, and auxiliaries `VA` [V].
  - **Italian is pro-drop: "subjects can be omitted"** [V].
  - `acl:relcl` also covers partitive relatives introduced by *tra cui* / *fra cui* where the verb is elided [V].
  - Parataxis subtypes exist for appositive, discourse, hashtag, insert, and nsubj-implicit sentences [V].
- **Greek (GDT, the deployed treebank):**
  - Finite indicative verbs have `Voice` ∈ {Act, Pass}, `Tense` ∈ {Past, Pres}, `Number`, and `Person`. Finite imperative verbs are second person only. `Aspect` ∈ {Imp, Perf} [V: `el/index.html`, Features-GDT].
  - Nonfinite forms: perfect-forming infinitive-like forms, passive `-μένος` participles, and active `-(ώ/ο)ντας` converbs [V].
  - Future is formed with the particle `θα`, an AUX. The GUD page assigns `θα` the feature `Tense=Fut` [V]. The GDT page lists `θα` as AUX but does not state the Tense value [U].
  - Subtypes used: `acl:relcl`, `csubj:pass`, `nsubj:pass`, `obl:agent` [V]. Nominal subjects are nominative; direct objects are accusative; indirect objects genitive; **all prepositional objects are `obl`** [V].
  - Case values: Nom, Gen, Acc, Voc; Dat only in fixed expressions [V: Features-GUD].
  - Negation: `δεν`, `μην` are PART [V].
  - Composite tenses: auxiliary and lexical verb both keep their tense and mood features [V: GUD].
  - PronType (including `Rel`) is documented for Greek pronouns and determiners [V: Features-GUD].
  - Greek pro-drop and the full Mood value set in GDT were not confirmed in the fetched text [U].
- **Multi-word tokens:** German MWT covers preposition+article contractions (`zum`) [V]. Italian MWT covers articulated prepositions and clitics on imperatives and non-finite forms [V]. Greek MWT covers `σε` + article contractions (`στον`) [V]. Italian `dell'` is not listed as an MWT example in the guideline text fetched here [U].

## Candidate matrix

Each row is a candidate signal. "Tier" is a recommendation about how it could be used in explanations or explicit filters, not a decision. Tiers are defined after the table.

| # | Candidate | Analyzer output supplied | de mapping | it mapping | el mapping | Coverage vs accuracy | False positives / absences | Evidence labels | Tier |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | Relative clause | `dependency` on the clause head; relative pronoun typically a dependent | `acl:relcl`; `advcl:relcl` for clausal antecedents | `acl:relcl` (includes partitive *tra cui*) | `acl:relcl` (GDT) | Label defined for all three [V]. Only aggregate LAS: GSD 81.8, HDT 93.6, ISDT 91.6, GDT 88.9 [V] | `acl` also covers non-relative and non-finite clauses: filter on the `acl:relcl` label, not `acl` [V]. Extraposed relatives are non-projective [V]. Italian elided-verb partitives [V] | Mapping [V]; accuracy aggregate only [V]; per-label [U] | A |
| 2 | Other embedded clauses (complement, adverbial, participial) | `ccomp`, `csubj`, `csubj:pass`, `advcl`, `acl`, `xcomp` | `ccomp` for finite complements of accusative-object verbs; `xcomp` for infinitival complements [V] | `ccomp`, `xcomp`, `csubj` [V] | `ccomp`/`csubj:pass` [V] (GDT) | Labels defined [V]; no per-label accuracy [U] | `acl` covers non-finite participial modifiers; the German scorer's single "subordinate" flag treats them as subordinate [R]. `ccomp` also attaches to the copula *è* when its argument is a clause, with the copula as root [V] | Mapping [V]; overclaim risk [R] | A (by specific label), C (as a single "subordinate" flag) |
| 3 | Separable verb | `compound:prt` dependent of a `VERB`; canonical lemma rebuilt from particle + verb | Yes: closed set of ~60 prefixes; main-clause particle separate [V][R] | No equivalent in the fetched guidelines [U] | No equivalent in the fetched guidelines [U] | One repo sample: 10/10 accepted, 1 debatable [R]; sample is one author | Homographs in the closed set (*über*, *um*, *unter*, *vor*, *durch*, *gegen*, *hinter*, *wieder*) flagged debatable [R]. One false positive in the sample: *Sie ward rot über und über* → `überwerden` [R, my reading of the sample]. Subordinate-clause fused verbs are one token and may not carry the prefix in Stanza's lemma [U] | Mapping [V]; accuracy tiny sample [R]; fused-form lemma [U] | A (display with the particle shown); B (as a lemma-level filter) |
| 4 | Passive construction | `aux:pass`, `nsubj:pass`/`csubj:pass`, `obl:agent`, `expl:pass`, `expl:impers` | `aux:pass` on the passive auxiliary (`werden`); `nsubj:pass`; `obl:agent`. No Voice feature [V] | `aux:pass` (essere/venire); `nsubj:pass`; `expl:pass` (reflexive passive `si`); `expl:impers` (impersonal `si`) [V] | `nsubj:pass`, `csubj:pass`, `obl:agent`; `Voice=Pass` on finite indicative and `-μένος` participles [V] | Labels defined [V]; no per-label accuracy [U] | `werden` is AUX for both future and passive, so the label decides [V]. German stative `sein` + participle is not addressed by the fetched guideline [U]. Greek `Voice=Pass` is morphological and may not match a passive construction (deponents) [I] | Mapping [V]; stative and deponent cases [U][I] | A (for relation-backed passive); C (Greek `Voice=Pass` alone) |
| 5 | Subject and direct object case | `Case` on NOUN/PROPN/PRON/ADJ/DET; `nsubj`, `obj`, `obl` | `Nom` subject; `obj` is bare accusative only; dative, genitive and prepositional objects are `obl:arg`/`obl` [V]. Noun case ambiguous; article carries it [V] | No Case feature; core arguments are bare nominals; prepositional arguments are `obl` [V]. Case-based explanation not applicable | `Nom`/`Acc` subject and object; `Gen` indirect object; prepositional objects are `obl` [V] | UFeats aggregate only: GSD 89.8, HDT 92.6, ISDT 98.1, GDT 94.7 [V] | "Accusative object" must not include prepositional or dative objects [V]. German noun case should come from determiner/pronoun/adjective forms, not the noun [I] | Mapping [V]; accuracy aggregate [V] | B (German `obj`-accusative with label shown; Greek subject/object; not Italian) |
| 6 | Tense and mood of the finite token | `Tense`, `Mood`, `VerbForm`, `Person`, `Number` | Mood `Ind`/`Imp`/`Sub`; Tense `Pres`/`Past` only. `Sub` with `Tense` distinguishes Konjunktiv I/II [V] | Mood indicative/imperative/conditional/subjunctive; Tense past/present/future/imperfect [V] | Tense `Pres`/`Past`; `Aspect` `Imp`/`Perf`; `Fut` on `θα` in GUD [V]; GDT `θα` Tense value [U] | UFeats aggregate only (see table above) [V] | Composite forms carry the tense on the auxiliary as well as the participle [V]. Tense on a token is not the tense of the clause [I] | Mapping [V]; accuracy aggregate [V] | B (finite token labels only); C (a learner-facing "future tense" claim from morphology alone) |
| 7 | Verb group (auxiliary + participle/infinitive) | `AUX` + `aux` dependents; `VerbForm=Part` / `Inf` | Perfect: `haben`/`sein` + Part; future: `werden` + Inf; passive: `werden` + Part with `aux:pass` [V]. Modals: AUX list [V] | Perfect: avere/essere + Part; passive: essere/venire + Part with `aux:pass`; progressive: stare + gerund; modals `VM` [V] | Perfect: *έχω* + perfect infinitive-like form; future: `θα` (AUX) + verb [V] | Labels defined [V]; no per-construction accuracy [U] | Perfect/future with `aux` and passive with `aux:pass`: the same auxiliary lemma can be either, so the relation label decides [V][I] | Mapping [V]; accuracy [U] | A (display as a verb group with the auxiliary shown) |
| 8 | Clause nesting depth | `head` chain to root; clausal relations on the path | Derived from the labels in rows 1–2 [I] | Derived [I] | Derived [I] | Depth depends on attachment accuracy, which is only published as aggregate UAS/LAS [V][U] | Depth counts `acl` and `advcl`, including non-relative/non-finite [R][V]. Parataxis is not nesting [V] | Derived [I]; accuracy [U] | C (as a number); A-prose only as "inside a relative clause attached to X" |
| 9 | Finite verb with subject (knock-out) | `VerbForm=Fin`, root `VERB`/`AUX`, child `nsubj`/`nsubj:pass`/`csubj` | Used by the German scorer [R] | Not applicable: pro-drop makes a missing subject normal [V] | Not verified [U] | Subject attachment accuracy is aggregate only [V][U] | Italian pro-drop would be falsely rejected by this rule [V][I] | Mapping [V]; scorer scope [R] | C (German-only, as scoped by ADR 0062) |
| 10 | Clitic and reflexive constructions | `expl:pv`, `obj` (reflexive), `expl:pass`, `expl:impers`, `dobj`, `iobj`, `expl` | `expl:pv` for inherently reflexive; `obj` for `sich` (context-dependent) [V] | `dobj`/`iobj` for reflexive clitics; `expl` for pronominal verbs; `expl:impers`/`expl:pass` [V]. Enclitic clitics are MWTs [V] | Not in fetched guidelines [U] | Labels defined [V]; no per-label accuracy [U] | `sich` is ambiguous between `obj` and `expl:pv` [V]. MWT clitics share one source span [R] | Mapping [V] | B (show the label with explanation); not a filter |
| 11 | Negation | `Polarity=Neg` on `nicht`; `PART` for `δεν`/`μην` | `nicht` PART with `Polarity=Neg` [V] | Not in fetched guidelines [U] | `δεν`, `μην` PART [V] | Labels defined [V]; no accuracy [U] | Greek `όχι` is PART only in limited contexts [V] | Mapping [V] | A (simple, low-risk) |

### Tier definitions

- **A.** Reasonable to show as an explanation, with the raw label visible, on the analyzer-attributed token or group.
- **B.** Reasonable as an explicit, labelled filter or explanation with a stated caveat about coverage. Not a learner-facing grammar claim on its own.
- **C.** Do not present as a learner-facing claim. Use only as internal or research signal, or leave out.

These tiers are recommendations for the map's follow-up design. They are not product decisions.

## Availability versus detection accuracy

- **Annotation availability** (what the analyzer emits) is [V] for every label above, because each language has a published UD guideline for it and the producer persists `dependency`, `head`, and `morphology`.
- **Detection accuracy** (whether the emitted label is right) is published only as aggregate UFeats, LAS, UAS, and MLAS on single-treebank proxies [V]. No per-relation or per-feature accuracy is published for the deployed `de`/`it` combined models, or for the deployed Greek configuration [U].
- **Local evidence** is one German sample [R]. It supports separable-verb reattachment on a narrow prefix set, and it shows at least one false-positive class (closed-set homographs).
- Therefore: the labels are trustworthy as analyzer attributions, but not as facts about the sentence. Any explanation should say that the claim is attributed to the analyzer.

## Recommendations for the map

1. **Use relation labels as the explanation spine.** Show the label, the head, and the dependent, for relatives (`acl:relcl`), passives (`aux:pass`, `nsubj:pass`, `expl:pass`), and German separable particles (`compound:prt`). Keep the raw label visible, as the existing Sentence study page already does.
2. **Replace a single "subordinate" flag with the specific clause label.** The current scorer's grouping of `acl` with `advcl`/`ccomp`/`csubj` [R] would mislabel participial modifiers as subordinate. Any future explanation should name the relation.
3. **Do not infer passive from Greek `Voice=Pass` alone.** Use the relation with it.
4. **Do not claim German noun case from the noun form.** Use the article, pronoun, or adjective. Do not map prepositional or dative objects to "accusative object".
5. **Do not carry subject-based checks into Italian or Greek.** Italian pro-drop makes them false negatives [V]. Greek pro-drop is unverified here [U].
6. **Treat separable verbs as a German, relation-backed, lemma-level feature.** Keep the prefix set closed, and surface the homograph risk to the learner or the reviewer. Do not present it as a grammar fact.
7. **Keep depth and any composite complexity out of learner-facing text.** The map already rules out composite scores; this research adds that attachment error compounds with depth [I].

## Limitations

- No Stanza model was run, and no Mouseion analysis was re-run. All accuracy statements are from published or repository evidence, not from this session.
- The performance page is historical (Stanza 1.5.1, UD 2.12). It does not cover the combined `de`/`it` models or Stanza 1.15.0 [V].
- The Greek GreekBERT numbers come from Stanza source comments, with no version, relation-level breakdown, or data split detail [V].
- UD guideline pages describe intended annotation. They do not give error rates. Some pages say "ongoing work" or are partial (for example, Greek GUD syntax) [V].
- The German separable-verb sample is small, one author, Stanza 1.14.0, and reviewed by one spot-check judgment [R].
- License context from the existing repo note stands: UD Greek-GDT is CC BY-NC-SA 3.0, which matters for redistribution, not for this research [R].

## Follow-up unknowns

1. **[U] Runtime processor set for `de` and `it`.** Load the pipeline locally and read the resolved processor list to confirm whether `mwt` runs without being listed. Local only, no external service.
2. **[U] Per-label accuracy on book text.** A hand-labelled sample of sentences from a learner's own already-analyzed local data, reviewed in place, would measure relative-clause, passive, and case labels per language. This would need a human reviewer and must not send text to any external service.
3. **[U] Lemma of fused German verbs in subordinate clauses.** Check whether Stanza's lemma includes the prefix when the particle is not a separate token.
4. **[U] German stative passive (`sein` + participle).** Find how the UD German treebanks label it, or check a sample.
5. **[U] Greek pro-drop and the full Greek GDT Mood/Tense inventory.** Check the GDT treebank documentation or a sample, including whether `θα` carries `Tense=Fut` in GDT.
6. **[U] Italian `dell'` and clitic tokenization.** Confirm how these expand in the deployed `it` model.
7. **[U] Greek clitic and reflexive labels.** Confirm the relation labels for Greek clitics in GDT.
8. **[U] Stanza version of the Greek GreekBERT numbers.** Confirm the source of the dev/test LAS in Stanza's comments.

## Primary sources (all accessed 2026-10-13)

- Stanza available models, [stanfordnlp.github.io/stanza/available_models.html](https://stanfordnlp.github.io/stanza/available_models.html)
- Stanza combined models, [stanfordnlp.github.io/stanza/combined_models.html](https://stanfordnlp.github.io/stanza/combined_models.html)
- Stanza model performance (v1.5.1 on UD 2.12), [stanfordnlp.github.io/stanza/performance.html](https://stanfordnlp.github.io/stanza/performance.html)
- Stanza 1.15.0 resource manifest, [raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.15.0.json](https://raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.15.0.json)
- Stanza v1.15.0 source: [`stanza/resources/default_packages.py`](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.15.0/stanza/resources/default_packages.py), [`stanza/pipeline/core.py`](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.15.0/stanza/pipeline/core.py), [`stanza/resources/common.py`](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.15.0/stanza/resources/common.py)
- UD dependency relations, [universaldependencies.org/u/dep/all.html](https://universaldependencies.org/u/dep/all.html)
- UD `acl:relcl`, German: [universaldependencies.org/de/dep/acl-relcl.html](https://universaldependencies.org/de/dep/acl-relcl.html); Italian: [universaldependencies.org/it/dep/acl-relcl.html](https://universaldependencies.org/it/dep/acl-relcl.html)
- UD `compound:prt`, German: [universaldependencies.org/de/dep/compound-prt.html](https://universaldependencies.org/de/dep/compound-prt.html)
- UD for German: [universaldependencies.org/de/index.html](https://universaldependencies.org/de/index.html)
- UD for Italian: [universaldependencies.org/it/index.html](https://universaldependencies.org/it/index.html)
- UD for Greek: [universaldependencies.org/el/index.html](https://universaldependencies.org/el/index.html)

## Repository evidence

- `migrations/000001_initialize.up.sql` (corpus_tokens)
- `nlp/src/mouseion_nlp/model_config.py`, `producer.py`, `precision.py`
- `internal/domain/domain.go` (`SentenceStudyToken`), `internal/webapp/concordance.templ`
- `internal/gdex/gdex.go`; `doc/adr/0062-derived-sentence-quality-scoring.md`
- `doc/features/dependency-parse-foundation.md`, `doc/features/language-support.md`, `doc/features/vocabulary-browse-concordance-and-custom-decks.md`
- `doc/adr/0060`, `0073`; `doc/research/modern-greek-nlp.md`; `doc/evidence/german-separable-verb-precision.md`
