# Latin evidence-assisted pass payload assessment

Status: **Drafted, pending Justin's approval in the PR** · 2026-11-26 · the
`translation-v18-evidence-assisted-latin` payload for Classical Latin, for
personal use of the locally produced deck and Study surface.

Ticket: #2766 (Record the Latin pass and Fluency-agreement assessments), spec
#2759, ADR 0138 §1 and §6, ADR 0133 ("Each language needs its own provider
payload assessment for each payload kind. Justin reviews each one before
anything is sent."). The Greek pass this follows is #2171.

## Why an assessment is needed

Latin has its own pass contract. Its candidate senses come from Lewis & Short,
and each carries the handle of its dictionary entry beside its evidence handle,
because Lewis & Short stacks homographs (`dico` "dedicate" and `dico` "say") that
the Greek pass has no need to separate. A changed prompt and a changed sense
shape make this a payload the recorded Greek pass (#2171) does not cover. The
Greek contract `translation-v16-evidence-assisted-greek` and its record are
unchanged.

The pass serves two surfaces with one payload: the Latin Prepared deck and Study
material. Recording it therefore has two effects (see Disposition).

## The payload

Transport: the configured OpenAI-compatible Chat Completions endpoint
(`POST {MOUSEION_LLM_BASE_URL}/chat/completions`, the same one the Greek pass,
prepared decks and Study material use), with `response_format: json_object`.
One request per word, sent only by the prepared-deck and study job packages,
never by a page request.

Prompt version: `translation-v18-evidence-assisted-latin` (provider version
`<model>/translation-v18-evidence-assisted-latin`).

The user message carries these fields, in this order (pinned in
`enrichment.LatinEvidenceAssistedPassPayloadFields`). `passage_context` and
`candidate_senses` are omitted when empty:

| Field | Value |
| --- | --- |
| `item_id` | the constant per-request handle `i1`, never the stable item ID |
| `language` | `la` |
| `target_language` | `en` |
| `canonical_lemma` | the word's lemma |
| `upos` | its part of speech |
| `target_word` | the word as it appears in the sentence |
| `example_sentence` | one sentence of a Book in the learner's library |
| `passage_context` | the English text of the Perseus reference translation sections the sentence touches (see Sources) |
| `candidate_senses` | the pinned Lewis & Short senses of the lemma, each `{evidence_id, entry, source, gloss}` |

`evidence_id` is a per-request handle (`e1`, `e2`, …); `entry` is a per-request
entry handle (`h1`, `h2`, …) shared by the senses of one dictionary entry and
different for each stacked homograph; `source` is `ls`; `gloss` is the sense's
definition text.

The system prompt is `latinEvidenceAssistedPassSystemPrompt`
(`internal/enrichment/llm_codec.go`). It tells the provider that `passage_context`
is background and not a translation of the sentence, that `candidate_senses` are
the pinned Lewis & Short senses, each with an evidence handle and an entry
handle, and asks for one JSON object of ten fields: `item_id`, `source_language`,
`target_language`, `translation`, `sentence_translation`,
`sentence_translation_targets`, `gloss`, `evidence_ids`, `context_only` and
`unresolved_reason`. The `gloss` is one brief English cue for the target's
meaning in the sentence, with the supporting `evidence_ids`, which may only be
handles from `candidate_senses`. When no pinned evidence applies the answer may
carry a Gloss with an empty `evidence_ids` and `context_only` true; Study does not
use such a Gloss. When no contextual meaning can be
defended it abstains: an empty `gloss`, empty `evidence_ids`, `context_only` and
an `unresolved_reason`. The decoder rejects an unknown or repeated handle, and
maps a cited handle back to exactly one entry and sense locally.

## Sources

Two third-party sources reach the provider through this payload. Both are public
reference texts, not learner data.

- **Lewis & Short**, *A Latin Dictionary* (`PerseusDL/lexica`, file
  `lat.ls.perseus-eng1.xml`): the source of every `candidate_senses` gloss. CC BY-SA 4.0
  (`doc/third-party-data.md`: README "licensed under a Creative Commons
  Attribution-ShareAlike 4.0 International License"). Attribution is carried by
  `dictionary.LatinLSNotice` with the pin, wherever a matched entry is shown;
  derived data remains share-alike.
- **The Perseus reference translation** that `passage_context` carries: the
  pinned English `perseus-engN` file of the Work the Book was imported from
  (`PerseusDL/canonical-latinLit`; `internal/classical/layers.json`). CC BY-SA 4.0
  (the file's own `<licence>` element, or the repository README where the file
  has none; `doc/third-party-data.md`). Each file carries its own `deckNotice`
  naming translator, year, pin and licence, and the deck shows it with the
  passage. The source text of the sentence is credited by
  `cardpolicy.LatinSourceNotice` (OLA v0.2.0, derived from
  Perseus/canonical-latinLit, CC BY-SA 4.0).

Sending these texts to the provider for processing is a use inside the
personal-use, locally produced deck and Study surface; the attribution above
stays where the material is shown to the learner.

## What is not sent

No learner identity, username or account data. No Book title, author or
identifier. No whole Book: the sentence is one library sentence the Greek pass
already sends for the same kind of word. No review history, grades or schedule.
No Known or Reserved vocabulary. No raw annotation or lexical discriminator. No
Wiktionary evidence.

**No native Lewis & Short entry ID or sense ID is transmitted.** Verified from
the bytes of a real encoded request, not from reading the code:

- `TestLatinPassRequestSendsOnlyShortHandles` (`internal/enrichment`) encodes a
  request over native IDs such as `n40213` and `n40213.0` and asserts the whole
  request body contains no native ID, that the user message's fields are exactly
  the recorded list in order, and that every sense carries only `evidence_id`,
  `entry`, `source` and `gloss`, with handles `e1…` and `h1`, `h2`.
- `TestLatinEvidenceAssistedPassEncodedRequestCarriesNoNativeLewisShortID`
  (`internal/cardexport`) freezes a Latin deck over stacked `dico` homographs,
  encodes the work item it would send, and asserts neither native entry IDs nor
  the `entry#sense` evidence ID appear in the request.

## What is new compared with the Greek pass (#2171)

- **Dictionary.** Candidate senses are Lewis & Short (`source: ls`), not LSJ.
- **Entry handle.** Each sense carries `entry`, a per-request handle (`h1`,
  `h2`, …) so the provider can tell stacked homographs apart. It is assigned per
  request by order of first appearance and carries no native entry ID.
  Greek senses carry no `entry`.
- **Prompt.** `translation-v18-evidence-assisted-latin` names Lewis & Short and
  explains the entry handle. Otherwise it matches the Greek prompt, and the
  answer shape (ten fields) is unchanged.
- **Citation decoding.** A cited handle resolves locally to an entry and a
  sense, so a Study Gloss records which homograph it cites. This is local and
  is not sent anywhere.
- **Passage context.** The Perseus Latin translations are the passage source,
  not Brownson or the Greek translations.

The field list is the same nine fields as Greek's.

## Disposition

Pending Justin's approval in the PR. If approved: "approved for personal use of
the locally produced deck and Study surface", for Classical Latin only, with the
provider, prompt version (`translation-v18-evidence-assisted-latin`) and field
set pinned as above, recorded in `latinEvidenceAssistedPassAssessment`
(`internal/enrichment/provider_assessment.go`) and proved by
`TestLatinEvidenceAssistedPassAssessmentIsRecordedForLatinOnly`.

The gate compares the provider name, the prompt version and the field list
the codec reports with this record, so a change to any of them sends nothing
until it is re-recorded, and so does any other language. It does not hash the
prompt text.

**Runtime effect once recorded.** A Latin Prepared deck prepared while a
provider is configured starts sending this pass for every word as soon as the
record merges (`TestBatchPlannerPlansLatinDeckWithThePassUnderTheCompiledAssessments`
and `TestClassicalLatinDeckSendsThePassAndKeepsPassageContextWhenTheProviderFails`).
Latin Study material is not affected until Latin's Study switch is turned on
(#2768).


## Addendum, 2026-11-27: `translation-v21-evidence-assisted-latin-citation-form`

Status: **Addendum drafted, pending Justin's approval in the PR** · ticket #2781
(spec #2759). #2779 (PR #2784) moved the Latin pass contract from `translation-v18-evidence-assisted-latin` to
`translation-v21-evidence-assisted-latin-citation-form`. This addendum asks for the same disposition, for Classical Latin only, under the
new prompt version.

**What changed in the system prompt.** Two rules; nothing else in the prompt
changed. They are the modern contextual Gloss's rules, applied to this pass.

1. The old Gloss rule ("gloss must be one brief English cue for the target's
   meaning in this sentence, not a list of unrelated senses") is replaced by:

   > Gloss is the English equivalent of the supplied lemma in the sense the
   > representative sentence uses, in citation form: verbs as "to …", nouns
   > singular without an article, adjectives in base form. Never return a
   > definition-style paraphrase when a short equivalent exists, and never carry
   > the sentence's inflection, number or arguments. Keep the Gloss to one to
   > three words; close comma-separated synonyms are allowed only when one word
   > would mislead.

2. The old alignment rule ("identify only the smallest meaning-bearing words or
   phrases corresponding to the supplied target") is replaced by:

   > identify only the English words that render the target itself, never its
   > subject, object, complements or modifiers

   and one sentence is added after the multiple-excerpt rule:

   > Where the English renders the target with a function word (for example
   > "with" for habens), only that word is highlighted.

**Unchanged.** The field set is the same nine fields and the answer shape is the
same ten fields. The transport is the configured OpenAI-compatible Chat
Completions endpoint. `TestCitationFormPassFieldSetsAreUnchanged` proves the
field set, and `TestCitationFormPassPromptsStateTheGlossAndAlignmentRules` proves
the rules above are in the prompt. The example word in the added sentence is
fixed prompt text, not learner data.

**Nothing new is transmitted.** The request carries the same fields from the
same sources. No learner identity, Book title, whole Book, raw annotation,
lexical discriminator or native dictionary entry or sense ID is added. The only
new text is the fixed system-prompt wording quoted above.

**Disposition, if approved.** The disposition above, for Classical Latin only, with the
provider, the new prompt version (`translation-v21-evidence-assisted-latin-citation-form`) and the unchanged field set pinned in
`latinEvidenceAssistedPassAssessment` (`internal/enrichment/provider_assessment.go`, ticket #2781). The gate
refuses `translation-v18-evidence-assisted-latin`, which this addendum supersedes.

**Runtime effect once recorded.** Between #2784 and this record the gate refused v21, so Latin Prepared decks sent no pass. Merging restores it: a Latin Prepared deck prepared while a provider is configured sends the pass for every word again (`TestBatchPlannerPlansLatinDeckWithThePassUnderTheCompiledAssessments`). Latin Study is unaffected until its switch is turned on (#2768).
