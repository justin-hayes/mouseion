# Fluency agreement source-evidence payload assessment (Ancient Greek)

Status: **Drafted, pending Justin's approval in the PR** · 2026-11-26 · the
`fluency-agreement-source-evidence-v2` payload for Ancient Greek, for personal
use of the locally produced Study surface.

Ticket: #2765 (Record the Greek Fluency-agreement assessment), spec #2759,
ADR 0138 §4, ADR 0133 ("Each language needs its own provider payload assessment
for each payload kind. Justin reviews each one before anything is sent.").

## Why an assessment is needed

Study for a source-evidence language (ADR 0138) checks a Fluency context the
same way German does, but the check also weighs the dictionary senses each side
rests on. The request therefore sends two things the recorded German Fluency
payload (#2702) does not: the LSJ senses, with their definitions, that the
learner's Learned meaning cites, and those that the occurrence's own Gloss
cites, sent as definition strings alone. That makes it a new payload kind, `fluency-agreement-source-evidence`,
with a prompt contract of its own. The German `fluency-agreement-v2` contract
and its record are unchanged.

Each candidate context also gets the Greek evidence-assisted pass, which writes
its Gloss and citations. That pass is a separate request already covered by the
#2171 assessment and is not re-assessed here.

## The payload

Transport: the configured OpenAI-compatible Chat Completions endpoint, the same
one prepared decks and Study material use. One request per candidate occurrence,
sent only by the study job package, never by a page request.

The user message carries exactly these fields, in this order (pinned in
`enrichment.FluencyAgreementSourceEvidencePayloadFields`):

| Field | Value |
| --- | --- |
| `item_id` | the constant handle `f1` |
| `language` | `grc` |
| `target_language` | `en` |
| `canonical_lemma` | the word's lemma |
| `upos` | its part of speech |
| `target_word` | the word as it appears in the sentence |
| `example_sentence` | one sentence of a Book in the learner's library |
| `learned_meaning` | the word's Learned meaning (a short English Gloss) |
| `learned_senses` | an ordered array of the definitions of the LSJ senses the Learned meaning cites (empty when the dictionary has since moved to a new pin) |
| `context_senses` | an ordered array of the definitions of the LSJ senses the occurrence's Gloss cites |

The system prompt is `fluency-agreement-source-evidence-v2`
(`fluencyAgreementSourceEvidenceSystemPrompt` in
`internal/enrichment/fluency_agreement.go`). It asks one question: judging by
those definitions, does the word in this sentence have the
meaning in `learned_meaning`? The answer shape is the one `fluency-agreement-v2`
uses: `item_id`, a `reason` (`same-meaning`, `different-meaning`, `idiomatic`,
`not-this-lemma` or `undefendable`), a contextual `gloss`, a
`sentence_translation` and `sentence_translation_targets`. The answer cites no
sense. Only `same-meaning` is ever shown; the others are kept with their reason,
Gloss and translation as evidence of the word's other meanings. That data comes
from the provider's answer and is not sent anywhere.

## What is not sent

No learner identity, username or account data. No Book title, author or
identifier. No review history, grades, Fluency responses or schedule. No Known
or Reserved vocabulary. No passage context: the Perseus translation passage the
Study card shows is not sent by this request. No raw annotation or lexical
discriminator. The sentence is library text the Greek pass already sends for
the same word.

## What is new compared with the recorded payloads

Compared with the German Fluency payload (#2702): `learned_senses` and
`context_senses`. They are public LSJ definitions of the entries the word's
Gloss cited, not learner data; they reveal only which dictionary senses the
learner met.

Compared with the Greek evidence-assisted pass (#2171): the pass sends
candidate senses as short evidence handles and transmits no native LSJ ID.
This payload keeps that rule. **No dictionary sense ID, native or handle, is
transmitted**: each cited sense is sent as its definition text alone. The
answer cites no senses, so the provider has no use for an ID, and the study
worker keeps track of which senses it sent without the provider's help.

`learned_senses` may be an empty array. A Learned meaning recorded against an
older dictionary pin cannot be matched to today's senses, since the same sense
ID may name another sense now, so no definition is looked up or guessed and
nothing is sent for it. The prompt then tells the provider to judge against
`learned_meaning` alone. A pin change therefore never stops Fluency; only
`context_senses` must be non-empty, and an empty entry is never sent.

An earlier draft, `fluency-agreement-source-evidence-v1`, carried a `sense_id`
beside each definition. It was never recorded and never sent; Justin decided to
drop the IDs before recording, so `-v2` is the contract assessed here.

## Disposition

Pending Justin's approval in the PR. If approved, it is for Ancient Greek only,
with the provider, prompt contract (`fluency-agreement-source-evidence-v2`) and
field set pinned as above, recorded in
`fluencyAgreementSourceEvidenceGreekAssessment`
(`internal/enrichment/provider_assessment.go`) and proved by
`TestSourceEvidenceFluencyAgreementAssessmentIsRecordedForGreekOnly`.

A changed prompt, provider or field set fails closed again until it is
re-recorded, and so does any other language, including Latin. Recording this
assessment has no runtime effect until Greek's Study switch is turned on (#2767).
