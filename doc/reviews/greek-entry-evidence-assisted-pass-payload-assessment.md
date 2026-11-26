# Greek entry-carrying evidence-assisted pass payload assessment

Status: **Drafted, pending Justin's approval in the PR** · 2026-11-26 · the
`translation-v19-evidence-assisted-greek-entries` payload for Ancient Greek, for
personal use of the locally produced Study surface.

Ticket: #2774 (Record the Greek stacked-homograph pass assessment), spec #2759,
ADR 0139 §2, ADR 0138 §1 and §6, ADR 0133 ("Each language needs its own provider
payload assessment for each payload kind. Justin reviews each one before
anything is sent."). The Greek pass this follows is #2171; the closest prior
review is `latin-evidence-assisted-pass-payload-assessment.md` (#2766).

## Why an assessment is needed

ADR 0139 makes Greek Study look up every competing LSJ homograph entry, where the
Prepared deck takes exactly one. Study's pass therefore needs a contract in which
each candidate sense names its entry, so a cited sense is exactly one sense of one
entry (`λέγω` "lay" versus `λέγω` "say"). That contract has its own prompt
version and its own payload kind, `greek-entry-evidence-assisted-pass`. The
recorded Greek pass (#2171, `translation-v16-evidence-assisted-greek`) does not
cover it, and it is unchanged: the Prepared deck keeps sending it.

Until this record merges the gate refuses the payload and Greek Study sends
nothing.

## The payload

Transport: the configured OpenAI-compatible Chat Completions endpoint
(`POST {MOUSEION_LLM_BASE_URL}/chat/completions`, the same one every other pass
uses), with `response_format: json_object`. One request per word, sent only by the
Study job packages, never by a page request.

Prompt version: `translation-v19-evidence-assisted-greek-entries` (provider
version `<model>/translation-v19-evidence-assisted-greek-entries`, with a
reasoning suffix when one is configured).

The user message carries these fields, in this order (pinned in
`enrichment.GreekEntryEvidenceAssistedPassPayloadFields`). `passage_context` and
`candidate_senses` are omitted when empty:

| Field | Value |
| --- | --- |
| `item_id` | the constant per-request handle `i1`, never the stable item ID |
| `language` | `grc` |
| `target_language` | `en` |
| `canonical_lemma` | the word's lemma |
| `upos` | its part of speech |
| `target_word` | the word as it appears in the sentence |
| `example_sentence` | one sentence of a Book in the learner's library |
| `passage_context` | the English text of the Perseus reference translation sections the sentence touches (see Sources) |
| `candidate_senses` | the pinned LSJ senses of every homograph entry of the lemma, each `{evidence_id, entry, source, gloss}` |

`evidence_id` is a per-request handle (`e1`, `e2`, …); `entry` is a per-request
entry handle (`h1`, `h2`, …) shared by the senses of one LSJ entry and different
for each homograph; `source` is `lsj`; `gloss` is the sense's compact definition
text (ADR 0104), or its glosses when no sense of the entry has a definition.

The system prompt is `greekEntryEvidenceAssistedPassSystemPrompt`
(`internal/enrichment/llm_codec.go`). It tells the provider that `passage_context`
is background and not a translation of the sentence, that `candidate_senses` are
the pinned LSJ senses, each with an evidence handle and the handle of its entry
(homograph entries have different entry handles), and asks for one JSON object of
ten fields: `item_id`, `source_language`, `target_language`, `translation`,
`sentence_translation`, `sentence_translation_targets`, `gloss`, `evidence_ids`,
`context_only` and `unresolved_reason`. The `gloss` is one brief English cue for
the target's meaning in the sentence, with the supporting `evidence_ids`, which
may only be handles from `candidate_senses`. When no contextual meaning can be
defended it abstains: an empty `gloss`, empty `evidence_ids`, `context_only` and
an `unresolved_reason`. The decoder rejects an unknown or repeated handle, and
maps a cited handle back to exactly one entry and sense locally.

## Sources

Two third-party sources reach the provider through this payload. Both are public
reference texts, not learner data. Licences are as `doc/third-party-data.md`
records them.

- **LSJ**, Liddell-Scott-Jones, *A Greek-English Lexicon* (`PerseusDL/lexica@56061ca1`):
  the source of every `candidate_senses` gloss. CC BY-SA 4.0 (README: "licensed
  under a Creative Commons Attribution-ShareAlike 4.0 International License").
  Attribution is carried by `dictionary.AncientGreekLSJNotice` with the pin,
  wherever a matched entry is shown (Study's page does so); derived data remains
  share-alike.
- **The Perseus reference translation** that `passage_context` carries: the
  pinned English `perseus-engN` file of the Work the Book was imported from
  (`PerseusDL/canonical-greekLit@0189b9c8`; `internal/classical/layers.json`:
  Brownson's *Anabasis*, Fowler's, Lamb's and Shorey's Plato). CC BY-SA 4.0 by the
  repository README ("licensed under a Creative Commons Attribution-ShareAlike
  4.0 International License"); the files carry no licence element of their own.
  Print copyright is recorded as unverified for Brownson (1921–22) and for
  Shorey's *Republic* vol. 6 (1935), shipped on the Perseus licence by maintainer
  decision (#2418); the other translations are US public domain. Each file's
  `deckNotice` names translator, year, pin and licence and is shown with the
  passage. The Greek sentence is credited by `cardpolicy.GreekSourceNotice`
  (OGA v0.2.0, derived from Perseus `canonical-greekLit`, CC BY-SA 4.0).

This payload sends the same two sources to the provider that the #2171 pass
already sends; the licence position is unchanged. Sending them for processing is
a use inside the personal-use, locally produced Study surface; the attribution
above stays where the material is shown to the learner.

## What is not sent

No learner identity, username or account data. No Book title, author or
identifier. No whole Book: the sentence is one library sentence the #2171 pass
already sends for the same kind of word. No review history, grades or schedule.
No Known vocabulary or Learned meaning. No raw annotation or lexical
discriminator. No Wiktionary evidence.

**No native LSJ entry ID or sense ID is transmitted.** Verified from the bytes of
a real encoded request, not from reading the code:

- `TestGreekStudyPassRequestSendsOnlyShortHandles` (`internal/enrichment`) encodes
  a request over native IDs such as `n61607` and `n61607.0` and asserts the whole
  request body contains no native ID and no `entry#sense` evidence ID, that every
  sense carries only `evidence_id`, `entry`, `source` and `gloss`, with handles
  `e1…` and `h1`, `h2`.
- `TestGreekStudyPassRequestPayloadIsTheRecordedFieldSetInOrder` asserts the user
  message's fields are exactly the recorded list, in order.
- `TestGreekStudyPassRequestOfALargeEntryCarriesEverySenseWithinBudget` repeats
  the no-native-ID check over the large synthetic entry below.

## What is new compared with the Greek pass (#2171)

Three things, and only these:

1. **Every LSJ homograph.** The deck takes a lemma only when exactly one LSJ entry
   competes; Study takes every competing entry, in index order. A frequent lemma
   such as ἔχω, ἄν, λέγω or πᾶς, which the deck skips, is now sent with all of its
   entries. The competing entries are the same ones the deck counts (no phrase or
   redirect sub-entry, no capitalised proper name for a lowercase lemma).
2. **An entry handle on each sense.** Each sense carries `entry`, a per-request
   handle (`h1`, `h2`, …) assigned by order of first appearance. It carries no
   native entry ID. The #2171 senses carry no `entry`.
3. **Uncapped senses.** The deck freezes at most `DefaultMaxCandidateSenses` (8)
   candidate senses. Study sends every sense of every entry, as it did for one
   entry before ADR 0139, so a frequent lemma's later senses stay citable.

Unchanged: the transport, the nine payload fields and their order (identical to
#2171's and to Latin's), the ten-field answer shape, the passage source, the two
sources and their licences. The prompt differs from #2171's in naming the entry
handle and homograph entries alone. The #2171 record, its prompt
(`translation-v16-evidence-assisted-greek`) and the Prepared deck's bytes are not
touched (`TestGreekDeckPassContractIsUnchanged`, and the #2171 record is pinned
whole by `TestGreekEntryEvidenceAssistedPassAssessmentIsRecordedForGreekOnly`).

## Request size of an uncapped entry

Uncapped senses make long requests for frequent words. The data for real LSJ is
not in the repository's tests, so this is measured on a synthetic fixture
(`syntheticLargeGreekEntryRequest`) shaped like ἔχω: three homograph entries with
90, 36 and 14 senses (140 in all), definitions of 30 to 129 characters. Token
counts are estimated at four bytes per token; real Greek-script definitions and
the provider's tokenizer will differ, so treat them as an order of magnitude.

| Request | Senses | Encoded request | Approx. tokens |
| --- | --- | --- | --- |
| No senses (system prompt and fields only) | 0 | 2,222 bytes | ~560 |
| Typical, capped at 8 senses | 8 | 3,396 bytes | ~850 |
| Large synthetic ἔχω-like entry, uncapped | 140 | 23,946 bytes (23.4 KiB) | ~6,000 |

The user message alone is 19,583 bytes for 140 senses, about 140 bytes per sense;
the system prompt and request envelope are about 2.2 KiB. Cost grows linearly
with the number of senses. One such request is sent per word and only for a
handful of very frequent lemmas; most lemmas have a few senses and cost about as
much as the 8-sense row. The request stays far inside a modern model's context
window, and the answer is small (a Gloss, a translation and a few handles).
`TestGreekStudyPassRequestOfALargeEntryCarriesEverySenseWithinBudget` fails if
this fixture's request exceeds 32 KiB or if any sense is dropped. Justin: this is
the cost to weigh; ADR 0139 chose uncapped senses deliberately, and a cap would
need its own decision.

## Disposition

Pending Justin's approval in the PR. If approved: "approved for personal use of
the locally produced Study surface", for Ancient Greek only, with the provider,
prompt version (`translation-v19-evidence-assisted-greek-entries`) and field set
pinned as above, recorded in `greekEntryEvidenceAssistedPassAssessment`
(`internal/enrichment/provider_assessment.go`, ticket #2774) and proved by
`TestGreekEntryEvidenceAssistedPassAssessmentIsRecordedForGreekOnly`.

The gate compares the provider name, the prompt version and the field list the
codec reports with this record, so a change to any of them sends nothing until it
is re-recorded, and so does any other language. It does not hash the prompt text.

**Runtime effect once recorded: none yet.** Greek's Study switch stays off until
#2767, so no Greek Study material is prepared and nothing is sent. The Prepared
deck is unaffected: it still uses the #2171 payload.
