# Licence posture for OGA-derived syntax and constructions shown to learners

**Research date:** 2026-11-23 · **Ticket:**
[#2602](https://github.com/justin-hayes/mouseion/issues/2602) · **Map:**
[#2597](https://github.com/justin-hayes/mouseion/issues/2597)

**Scope:** What ADR 0111 and the deployer register permit for OGA `oga_src`
v0.2.0 dependency relations, heads and Mouseion-computed constructions shown in
Sentence study and Concordance. Whether the brief's rule "anything
learner-visible must be no stricter than CC BY-SA" matches ADR 0111. What the
Stanza `grc` lineage would require. What the register and `layers.json` would
need for a new interpretation version.

The sources are repository documents at `origin/main` `45ffbcfd`, the #2301
findings at `18e3a63d`, and one primary external file: the Stanza resource
index at the pin the register names. This is a product reading of ADR 0111, not
legal advice.

## Answer

| Use | Verdict | Conditions |
|---|---|---|
| (a) Stored OGA relations and heads shown in Sentence study | **Permitted with conditions** | The four ADR 0111 Analysis-resource conditions, which the current delivery already meets: fetched lazily at preparation, not shipped, personal use, listed. Keep repository fixtures synthetic. |
| (b) Construction labels | **Permitted with conditions** | Same as (a). The label, its short definition and plain-English relation names must be Mouseion's own wording. The Smyth link is an outbound link plus a § citation, and no Smyth text is copied (already settled on the map). |
| (c) Concordance parallels | **Permitted with conditions** | Same as (a) for the relation and construction predicates. The sentences shown are OGA Greek text, already admitted as share-alike Source content. |

- **Classification.** Relations, heads, ellipsis nodes and constructions
  computed from them are Analysis-resource output, not Source content.
  `CONTEXT.md` defines an Analysis resource as a layer "from which the learner
  receives only derived linguistic labels about a text: sentence boundaries,
  lemmas, parts of speech, morphology or **syntax**" (`CONTEXT.md:928-933`).
  ADR 0111 names the `oga_src` "AGDT-derived lemmas and parts of speech" as an
  Analysis resource and says the annotations "are acceptable whatever the AGDT
  lineage behind them turns out to be" (ADR 0111 lines 47-51 and 86-87).
  Showing a layer in the app does not change its class. Source content is
  defined by the third party's *own expression* reaching the learner
  (`CONTEXT.md:936-939`; ADR 0111 lines 141-142).
- **A construction label is Mouseion's own expression**, derived from
  Analysis-resource output. Mouseion's rule chooses the label, its name and
  its definition. No third party's prose reaches the learner unless Mouseion
  copies it, for example AGDT guideline wording or Smyth text.
- **Conflict: the brief's rule, read literally, is stricter than ADR 0111.**
  ADR 0111 applies "no more restrictive than share-alike" to Source content
  only (lines 62-69). It expressly allows NC, unclear and research-only
  Analysis resources (lines 71-87). The rule's earlier wording on the
  graded-reader map (#2299, #2303) was "any **text** the readers show is Source
  content and must be no stricter than CC BY-SA", which is ADR 0111's rule.
  Widened to "anything learner-visible", it would bar features that already
  ship (next bullet). **This is recorded for Justin and not resolved here.**
- **What the stricter rule would bar.** The OGA annotation layer's
  restriction is `unclear` (register row; `layers.json:64-70`). Its declared
  licence is CC BY-SA 4.0; the unresolved AGDT 1.7 and Gorman NC statements in
  its training data make it unclear. The stricter rule would therefore bar the
  OGA lemmas, parts of speech and morphology that Sentence study, Concordance,
  Vocabulary Browse and Greek decks already show. It would also bar the modern
  languages, which the brief may not have intended: every warmed Stanza package
  (`de`, `it`, `el`, `fr`) is NC through the conll17 embeddings (register rows
  1-2). Sentence study already shows those packages' dependency relation and
  head to learners (`internal/webapp/concordance.templ:334`). So the stricter
  rule fails the shipped product in all four modern languages as well as in
  Greek.
- **The licence posture does not block syntax.** The `absent` syntax state is
  ADR 0099's design choice. ADR 0111 says ADR 0099's "pinned source and
  `absent` syntax stand on their own merits" (lines 116-118). Storing syntax
  therefore needs an ADR that amends ADR 0099's syntax decision. It needs no
  new licence decision.
- **Stanza `grc` is NC on every package, confirmed at the register's pin.** Any
  route through it is eligible only under ADR 0111's four conditions. It is
  outside this map's scope (#2404 / ADR 0116).
- **Register and `layers.json`.** A new interpretation of the same pinned files
  needs **no new layer and no new register row**. Both already say "syntax".
  The changes are a catalog manifest bump plus an optional clarification of the
  row (see "Register changes").

## Evidence

### 1. ADR 0111's two rules

- Classification is per layer. The `oga_src` file holds Source content (the
  Greek text) and an Analysis resource (the annotations). ADR 0111 lines
  45-51:
  > the `oga_src` file carries Greek text (Source content) and AGDT-derived
  > lemmas and parts of speech (Analysis resource)
- The Source content rule (lines 62-69) admits public domain, CC0, CC BY,
  CC BY-SA or GFDL, and excludes NC, research-only, unlicensed and unclear
  content "in every language and on every delivery path".
- The Analysis-resource rule (lines 71-87) allows an NC, unclear or
  research-only layer when four conditions hold: (1) the deployer fetches it at
  deploy time or supplies it, and "a runtime in-app download would qualify on
  the same terms"; (2) it never ships in the repository, an image or a release
  artifact; (3) the deployment is personal, self-hosted and non-commercial;
  (4) it is listed in the register.
- The principle (lines 141-142): "The line that matters is whether a third
  party's expression reaches the learner". ADR 0111 has no rule keyed to
  learner visibility alone. Its only display-related rule covers decks (lines
  89-97): a deck "may carry Analysis-resource output, such as the `Lemma` and
  `POS` fields".

### 2. The domain terms put syntax on the Analysis-resource side

- `CONTEXT.md:928-933`, **Analysis resource**: "only derived linguistic labels
  about a text: sentence boundaries, lemmas, parts of speech, morphology or
  syntax".
- `CONTEXT.md:936-939`, **Source content**: "A third-party layer whose own
  expression reaches the learner, either shown in Mouseion or copied into an
  exported deck: texts, translations, dictionary definitions and graded
  readers." So "shown in Mouseion" matters only for expression.
- `CONTEXT.md:702-706`, **Sentence study**: "each word's analyzed morphology
  and, where the analysis has syntax, its dependency relation and head. It is
  where construction-level help belongs."

### 3. What learners already see

- Sentence study renders the analyzer lemma, effective lemma, morphology,
  dependency and head per token (`internal/webapp/concordance.templ:334`). The
  variant without syntax is at line 343. It shows the "Syntax unavailable"
  note only for `native_agdt` or `absent` runs
  (`internal/webapp/sentence_study.go:397-399`;
  `internal/webapp/concordance.templ:146`). So Stanza NC-derived relations and
  heads already reach learners for `de`, `it`, `el` and `fr`, and Greek
  learners already see OGA lemmas, parts of speech (`concordance.templ:204`)
  and morphology.
- Greek decks carry the OGA `Lemma` and `POS` fields with
  `AncientGreekSourceNotice` (register; `internal/cardexport/cardexport.go:1733`).
  The notice reads "Ancient Greek text and annotation: OGA v0.2.0 (CC BY-SA
  4.0) … Commentary and derived data remain share-alike under the same
  licence." (`internal/dictionary/attribution.go:11`).

### 4. The OGA annotation layer

- Register row (`doc/third-party-data.md`, "OGA `oga_src` v0.2.0
  annotations: lemmas, parts of speech, morphology **and syntax**"). It records
  Analysis resource, restriction `unclear`, Zenodo `cc-by-sa-4.0`, AGDT lineage
  **unverified**, and reaches a deck as "Output only (`Lemma`, `POS`)".
- `internal/classical/layers.json:64-70`: key `oga-src-v0.2.0-annotations`,
  `licenceClass: analysis-resource`, `restriction: unclear`, and the register
  row "AGDT-derived lemmas, parts of speech, morphology and syntax with
  unverified training lineage."
- `Layer.validateLicence` (`internal/classical/types.go:262-283`) accepts any
  restriction, `unclear` included, for an Analysis resource. Only Source
  content must be none, attribution or share-alike.
- Lineage (`doc/research/oga-src-agdt-lineage.md`): the parser was trained on
  the AGDT (version unstated), the Gorman Trees and the Pedalion Trees. The
  AGDT 1.7 texts are stated as CC BY-NC-SA 2.5 inside a repository stated as
  CC BY-SA 3.0 US. The Gorman README says NC-SA 4.0, but its licence file is
  BY-SA 4.0. Neither conflict is resolved. The relations and heads come from
  the same parser output as the lemmas, so they carry exactly the same
  lineage. Syntax has no separate or worse lineage.

### 5. The current delivery meets the four conditions

1. **Fetched, not shipped.** Bytes are "fetched lazily when a learner prepares
   a Book" (register delivery column). ADR 0111 condition 1 admits a runtime
   in-app download on the same terms.
2. **Not in the repository or release.** "the release ships no annotations"
   (register). The committed `internal/ancientgreek/testdata/oga-src-*.conllu`
   fixtures are synthetic sentences, not excerpts (lineage research §1). The
   catalog manifest ships only each Work's `opening` text, which is Source
   content (`internal/classical/types.go:86-89`), and no annotations.
3. **Personal use** is the deployer's responsibility (register, "What a
   deployer should know").
4. **Listed**: register row above.

A new interpretation that stores syntax changes none of these, provided that
any new test fixtures (for example the real AGDT relation inventory in place of
the current `DET`) stay synthetic.

### 6. ADR 0099's `absent` syntax is not a licence decision

ADR 0099 (lines 52-59) records syntax as `absent` because the `corpus_tokens`
`dependency`/`head` fields "are read as UD" and "a native AGDT head cannot be
stored faithfully in a 0-based kept-token index after ellipsis tokens are
dropped". Its context premise that "there is no acceptable Ancient Greek
model" (line 12) was withdrawn by ADR 0111 (lines 116-120). ADR 0099 already
foresees "a `native_agdt` syntax state and fields that are never read as UD"
(lines 90-92).

### 7. Stanza `grc` (#2301, re-checked at the register's pin)

- #2301's resolution and findings
  ([`18e3a63d`](https://github.com/justin-hayes/mouseion/blob/18e3a63d/doc/research/grc-non-nc-lemmatisation.md))
  found: UD Perseus `LICENSE.txt` CC BY-NC-SA 2.5, UD PROIEL CC BY-NC-SA 4.0,
  UD PTNK CC BY-SA 4.0 (Septuagint Koine), and every `grc` package using the
  conll17 pretrain (LINDAT `11234/1-1989`, CC BY-NC-SA 4.0). #2301 read
  `resources_1.10.0.json`.
- Re-checked here against the register's pin,
  `stanfordnlp/stanza-resources@569db9d` `resources_1.15.0.json` (SHA-256
  `ae9741f2…4463`). The `grc` entry has packages `default`, `default_fast`,
  `default_accurate`, `perseus`, `proiel` and `ptnk`. Its only `pretrain` is
  `conll17`. All six `depparse` models (`{perseus,proiel,ptnk}_nocharlm` and
  `{perseus,proiel,ptnk}_grc-pranaydeeps`) and all `pos` models declare
  `{"model": "pretrain", "package": "conll17"}`. The default package is
  `perseus_nocharlm` for every processor, so NC comes through both the
  treebank and the embeddings.
- `default_accurate` also uses a `grc-pranaydeeps` transformer (HF
  `pranaydeeps/Ancient-Greek-BERT`, sha `5e3e29ec`). Its HF API record has no
  licence field or licence tag. **Unverified.**
- Conditions for any route through Stanza `grc` (ADR 0111 lines 76-85):
  fetched by the deployer at deploy time (`nlp-init`) or supplied by the
  operator; never in the repository, an image or a release; personal,
  non-commercial deployment; a register row per layer. The register lists no
  Stanza `grc` row today, because `grc` is not warmed. Its parse would be UD,
  so it would also need the "UD-shaped input" path the map reserves. This map
  excludes it (#2404 / ADR 0116).

## Register changes for a new interpretation version

The layer is the same files at the same pins. Only Mouseion's reading of them
changes (for example a new `oga-agdt-feats/2` and a syntax interpretation).

- **No new register row and no new `layers.json` layer.** Both already name
  syntax (section 4). Under ADR 0111, adding a *layer* requires a row (lines
  108-110). An interpretation version is Mouseion's AGPL code, not a layer.
- **The catalog manifest changes, not the layers.** Each of the 22 definitions
  in `internal/classical/manifest.jsonl` records
  `"interpretationVersion":"oga-agdt-upos/5"` (`types.go:92-93`), and a change
  to a content-bearing field bumps `revision`. Moving to the new interpretation
  is the ADR 0116 Analysis-update path (already settled on the map), not a
  licence step.
- **Optional clarification of the register row.** The row's "Reaches a deck"
  column stays "Output only (`Lemma`, `POS`)", because cards are out of scope,
  and the deck notice does not change. The register has no column for in-app
  display. A one-line note that relations and heads are shown in Sentence
  study and used by Concordance is accurate but not required by ADR 0111.
- **A converter.** An AGDT→UD (or AGDT→construction) mapping written by
  Mouseion is AGPL code and needs no row. A third-party converter needs a row
  only if it carries data, such as mapping tables or a model. A code-only
  converter is a software dependency (register, "Not covered here").
- **The grammar link target.** An outbound link and a § number copy no
  expression, so they are not a layer under the ADR 0111 definitions and need
  no row. A row, as Source content, becomes necessary only if Mouseion copies
  Smyth text or another grammar's prose. The map already forbids that.
- **Map wording to update.** `CONTEXT.md:770-774` (**Imported analysis**) says
  consumers show "Syntax unavailable" for a `native_agdt` run, and
  `sentence_study.go:397-399` implements that. Showing native AGDT syntax
  changes this term. It is a domain change, not a licence one.

## Findings that contradict or sharpen the map's assumptions

1. **The brief's "learner-visible ≤ CC BY-SA" is stricter than ADR 0111** and,
   taken literally, would bar shipped Greek *and* modern-language features
   (section 3). It appears to be a widening of #2299's "any text the readers
   show". For Justin; not resolved.
2. **OGA syntax is already registered.** The register row and `layers.json`
   both say "syntax", so the "register rows and notices" item under "Not yet
   specified" shrinks to an optional clarification and the converter and
   grammar-link questions.
3. **Measurement tables on research branches.** The repository is public
   (`gh repo view`: `PUBLIC`). ADR 0111 condition 2 says an unclear Analysis
   resource "never ships in the repository". A pushed per-occurrence table that
   copies OGA relations and heads for about 150 occurrences arguably puts that
   output in the repository, even on a never-merged branch. A few-label quote
   is not the concern; a systematic copy is. The measurement ticket could keep
   OGA labels in scratch and commit only occurrence IDs, Smyth references and
   Mouseion's judgments. This is a reading of ADR 0111, not settled.

## Open or unverifiable

- Whether OGA annotations are, legally, a derivative of NC training data is
  outside this research (lineage research, open item 6). The posture does not
  depend on it.
- Whether CC BY-SA 4.0 (OGA's declared licence) calls for in-app attribution
  when derived syntax is shown. ADR 0111 requires notices only on decks, and
  the Catalog credits text layers through their register rows
  (`internal/classical/attribution.go`). No in-app credit names the
  annotations. Not required by ADR 0111; flagged only.
- The licence of `pranaydeeps/Ancient-Greek-BERT` is unverified (no HF licence
  field).
- The Gorman and AGDT 1.7 conflicts stay unresolved (#2306). No author was
  contacted.
