# Fluency agreement payload assessment (German)

Status: **pending Justin's review. Nothing is sent until it is recorded.**

Ticket: #2702 (Study: Fluency for mature words), spec #2691, ADR 0134 §7,
ADR 0133 ("Each language needs its own provider payload assessment for each
payload kind. Justin reviews each one before anything is sent.").

## Why an assessment is needed

Fluency asks the configured provider whether another occurrence of a mature
word uses the meaning the learner already knows for it. The request therefore
sends one thing no recorded payload sends: the **learned meaning**, the short
English Gloss the same provider gave the word when the learner first saw it.
That makes it a new payload kind, `fluency-agreement`. The existing German
contextual Gloss payload is not assessed (a payload that is not assessed does
not consult the gate), but this one is, because it carries a value derived from
the learner's own study record.

## The payload

Transport: the configured OpenAI-compatible Chat Completions endpoint, the same
one prepared decks and Study material use. One request per occurrence, sent
only by the study job package, never by a page request.

The user message carries exactly these fields, in this order (pinned in
`enrichment.FluencyAgreementPayloadFields`):

| Field | Value |
| --- | --- |
| `item_id` | the constant handle `f1` |
| `language` | `de` |
| `target_language` | `en` |
| `canonical_lemma` | the word's lemma |
| `upos` | its part of speech |
| `target_word` | the word as it appears in the sentence |
| `example_sentence` | one sentence of a Book in the learner's library |
| `learned_meaning` | the word's learned meaning (a short English Gloss) |

The system prompt is `fluency-agreement-v1` (`fluencyAgreementSystemPrompt` in
`internal/enrichment/fluency_agreement.go`). It asks one question: does the word
in this sentence have the meaning in `learned_meaning`? It returns
`uses_learned_meaning`, a contextual Gloss, a sentence translation and a target
alignment. Contexts that answer no are discarded and never shown.

## What is not sent

No learner identity, username or account data. No Book title, author or
identifier. No review history, grades, Fluency responses or schedule. No Known
or Reserved vocabulary. No passage context beyond the one sentence. No lexical
discriminator. The sentence is library text the existing German payload already
sends for the same word.

## What is new compared with the recorded German payload

Only `learned_meaning`. It is text the provider itself produced for the word,
echoed back; it reveals that the learner has this word in study, which the
existing Study material request already implies for the same word.

## Proposed disposition

Approve for personal use of the locally produced Study surface, for German
only, with the provider, prompt contract and field set pinned as above.

## How to record it

1. Review the payload above and `encodeFluencyAgreementRequest` in
   `internal/enrichment/fluency_agreement.go`.
2. In `internal/enrichment/provider_assessment.go`, set
   `fluencyAgreementAssessment.Recorded` to `true` and replace its `Disposition`
   with the recorded wording.
3. In `internal/enrichment/fluency_agreement_test.go`, replace
   `TestFluencyAgreementAssessmentFailsClosedUntilRecorded` with
   `assertRecordedAssessment(t, "#2702", FluencyAgreementPayload, "de", ...)`, as
   `TestIdiomCardAssessmentIsRecordedForItalianOnly` does for idiom cards.

Until step 2, the Fluency worker leaves each job waiting, transmits nothing, and
reads no occurrence beyond Today's own data. A changed prompt, provider or field
set fails closed again until it is re-recorded.
