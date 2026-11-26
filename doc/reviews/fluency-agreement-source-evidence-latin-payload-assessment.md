# Fluency agreement source-evidence payload assessment (Latin)

Status: **Drafted, pending Justin's approval in the PR** · 2026-11-26 · the
`fluency-agreement-source-evidence-v2` payload for Classical Latin, for personal
use of the locally produced Study surface.

Ticket: #2766 (Record the Latin pass and Fluency-agreement assessments), spec
#2759, ADR 0138 §4, ADR 0133 ("Each language needs its own provider payload
assessment for each payload kind. Justin reviews each one before anything is
sent."). It mirrors the Greek assessment in
`fluency-agreement-source-evidence-greek-payload-assessment.md` (#2765).

## Why an assessment is needed

Study for a source-evidence language (ADR 0138) checks a Fluency context the
same way German does, but the check also weighs the dictionary senses each side
rests on. The request therefore sends two things the recorded German Fluency
payload (#2702) does not: the Lewis & Short senses, with their definitions, that
the learner's Learned meaning cites, and those that the occurrence's own Gloss
cites, sent as definition strings alone. That is the payload kind
`fluency-agreement-source-evidence` with the prompt contract
`fluency-agreement-source-evidence-v2`. The contract and field set are the ones
the Greek record pins; the assessment is per language, so Latin needs its own
record. The German contract and record, and the Greek record, are unchanged.

Each candidate context also gets the Latin evidence-assisted pass, which writes
its Gloss and citations. That pass is a separate request, assessed in
`latin-evidence-assisted-pass-payload-assessment.md`, and is not re-assessed here.

## The payload

Transport: the configured OpenAI-compatible Chat Completions endpoint, the same
one prepared decks and Study material use. One request per candidate occurrence,
sent only by the study job package, never by a page request.

The user message carries exactly these fields, in this order (pinned in
`enrichment.FluencyAgreementSourceEvidencePayloadFields`):

| Field | Value |
| --- | --- |
| `item_id` | the constant handle `f1` |
| `language` | `la` |
| `target_language` | `en` |
| `canonical_lemma` | the word's lemma |
| `upos` | its part of speech |
| `target_word` | the word as it appears in the sentence |
| `example_sentence` | one sentence of a Book in the learner's library |
| `learned_meaning` | the word's Learned meaning (a short English Gloss) |
| `learned_senses` | an ordered array of the definitions of the Lewis & Short senses the Learned meaning cites (empty when the dictionary has since moved to a new pin) |
| `context_senses` | an ordered array of the definitions of the Lewis & Short senses the occurrence's Gloss cites |

The system prompt is `fluency-agreement-source-evidence-v2`
(`fluencyAgreementSourceEvidenceSystemPrompt` in
`internal/enrichment/fluency_agreement.go`). It asks one question: judging by
those definitions, does the word in this sentence have the meaning in
`learned_meaning`? The answer shape is the one `fluency-agreement-v2` uses:
`item_id`, a `reason` (`same-meaning`, `different-meaning`, `idiomatic`,
`not-this-lemma` or `undefendable`), a contextual `gloss`, a
`sentence_translation` and `sentence_translation_targets`. The answer cites no
sense. Only `same-meaning` is ever shown; the others are kept with their reason,
Gloss and translation as evidence of the word's other meanings. The stored Gloss and
translation are those of the occurrence's own pass; the agreement answer decides
only the reason. None of that is sent anywhere.

## Latin specifics

- **The definitions are Lewis & Short's** (`PerseusDL/lexica`,
  `lat.ls.perseus-eng1.xml`, CC BY-SA 4.0; `doc/third-party-data.md`). They are
  public dictionary text, credited by `dictionary.LatinLSNotice` where a matched
  entry is shown to the learner.
- **Cited senses can come from different homograph entries.** The Learned
  meaning may cite `dico` "dedicate" while the occurrence's Gloss cites `dico`
  "say". Both are sent as bare definitions; neither entry is named. When the two
  sides cite no entry in common under the same dictionary pin, the Study worker
  records the context as a different meaning without making this agreement
  request (the occurrence's own pass has already run), so this payload is sent
  only when they share an entry or the pins differ. The
  definitions alone are what lets the provider judge a same-entry, different-sense
  case.

## What is not sent

No learner identity, username or account data. No Book title, author or
identifier. No review history, grades, Fluency responses or schedule. No Known
or Reserved vocabulary. No passage context: the Perseus translation passage is
not sent by this request. No raw annotation or lexical discriminator. The
sentence is library text the Latin pass already sends for the same word.

## What is new compared with the recorded payloads

Compared with the German Fluency payload (#2702): `learned_senses` and
`context_senses`. They are public Lewis & Short definitions of the entries the
word's Gloss cited, not learner data; they reveal only which dictionary senses
the learner met.

Compared with the Latin evidence-assisted pass: the pass sends candidate senses
as short evidence and entry handles and transmits no native Lewis & Short ID.
This payload keeps that rule and goes further. **No dictionary entry or sense
ID, native or handle, is transmitted**: each cited sense is sent as its
definition text alone. The answer cites no senses, so the provider has no use
for an ID, and the study worker keeps track of which senses it sent without the
provider's help. The request encoder is shared with Greek and checked for this by
`TestSourceEvidenceFluencyAgreementRequestSendsBothSidesCitedSenses`.

`learned_senses` may be an empty array. A Learned meaning recorded against an
older dictionary pin cannot be matched to today's senses, since the same sense
ID may name another sense now, so no definition is looked up or guessed and
nothing is sent for it. The prompt then tells the provider to judge against
`learned_meaning` alone. A pin change therefore never stops Fluency; only
`context_senses` must be non-empty, and an empty entry is never sent.

## Disposition

Pending Justin's approval in the PR. If approved: "approved for personal use of
the locally produced Study surface", for Classical Latin only, with the provider,
prompt contract (`fluency-agreement-source-evidence-v2`) and field set pinned as
above, recorded in `fluencyAgreementSourceEvidenceLatinAssessment`
(`internal/enrichment/provider_assessment.go`) and proved by
`TestSourceEvidenceFluencyAgreementAssessmentsAreRecordedForGreekAndLatinOnly`.

The gate compares the provider name, the prompt contract and the field list
the codec reports with this record, so a change to any of them sends nothing
until it is re-recorded, and so does any other language. It does not hash the
prompt text. Recording this assessment has no
runtime effect until Latin's Study switch is turned on (#2768).
