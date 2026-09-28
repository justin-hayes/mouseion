# Contextual Gloss quality sample

Status: **Collection and human review pending** · Prepared for #1278

This is the repeatable sample plan and review worksheet for comparing historical
sense-list/fallback card meanings with current contextual Glosses. It is not a
claim that model outputs have been run or that the examples below have been
validated by a language reviewer. Do not mark the feature acceptance criterion
complete until the results are populated and a human reviewer records a decision.

## Freeze the comparison

For each case, save one immutable record containing:

- Case ID, language, sentence, target surface form, canonical lemma, and POS.
- Exact historical card Gloss and whole-sentence translation, taken from the
  historical artifact (not reconstructed from today's dictionary index).
- Exact frozen current candidate payload, including evidence IDs, glosses,
  origins, match strengths, and index version. Record an explicit empty payload
  when evidence is absent.
- Exact current response and rendered Gloss, or the item-level unresolved,
  invalid, or ambiguous outcome. Keep the sentence translation alongside it.
- Provider/model identifier, contextual prompt version, request date, and any
  retry or manual exclusion. Do not include credentials, learner identifiers,
  or book metadata.
- Reviewer, review date, per-dimension judgments, notes, and disposition.

Use the same case sentence and target in both generations. The historical side
is a control, not ground truth; a re-preparation may refresh evidence, so preserve
both generations' candidate payloads. If no historical artifact exists for a
case, label it `new-control` and exclude it from before/after aggregate claims.
Do not regenerate a completed record in place: add a new sample revision and
record the reason.

## Case register

The sentence/target pairs below are candidate elicitation cases, not linguistic
gold labels. Confirm wording, target analysis, and intended reading with the
reviewer before treating a case as eligible. Cases may be replaced, but retain
the reason and preserve coverage of every rubric dimension across `de`, `it`,
and `el`.

| ID | Language | Candidate sentence · target | Focus to verify |
| --- | --- | --- | --- |
| de-polysemy | German (`de`) | `Die Bank schließt um sechs.` · `Bank` | Institution vs. bench; context selects one sense. |
| de-function | German (`de`) | `Er wartet auf den Bus.` · `auf` | Function-word meaning should be natural in English context, not a dictionary list. |
| de-idiom | German (`de`) | `Sie hat den Nagel auf den Kopf getroffen.` · `treffen` | Idiomatic meaning vs. literal physical hitting. |
| de-absent | German (`de`) | Reviewer-selected sentence/target with no relevant indexed English candidate | Context-only inference must be supported by the sentence or resolved as omitted. |
| it-polysemy | Italian (`it`) | `La chiave è rimasta nella serratura.` · `chiave` | Physical key vs. other senses; candidate ordering must not override context. |
| it-function | Italian (`it`) | `Ci conto.` · `ci` | Clitic's contextual contribution; reject unsupported specificity. |
| it-idiom | Italian (`it`) | `In bocca al lupo!` · `lupo` | Idiom-level meaning vs. literal `wolf`; reviewer confirms target suitability. |
| it-conflict | Italian (`it`) | `La pesca è matura.` · `pesca` | Fruit vs. fishing evidence; only the sentence-compatible meaning is acceptable. |
| el-polysemy | Modern Greek (`el`) | `Η τράπεζα είναι κλειστή σήμερα.` · `τράπεζα` | Financial institution vs. table/other senses. |
| el-function | Modern Greek (`el`) | `Το βιβλίο είναι στο τραπέζι.` · `σε` (surface `στο`) | Preposition/article contraction; concise English wording must fit the sentence. |
| el-idiom | Modern Greek (`el`) | `Με αυτή τη λύση, μου έλυσε τα χέρια.` · `λύνω` | Idiomatic “make it possible/easier” reading vs. literal untying. |
| el-missing-pos | Modern Greek (`el`) | Select an analyzed target with only a lemma-only candidate | Weaker missing-POS evidence must be identified and must not justify a claim the sentence does not support. |

Across the three languages, ensure the completed records also include: a target
with no supplied evidence; a candidate set that conflicts with sentence context;
a supported context-only inference; at least one missing-POS weaker-evidence
case; and invalid and ambiguous provider outcomes. These may be additional rows
or variants of the cases above. An invalid response should exercise the existing
offline decoder/validation contract; it is not a live-model wording assertion.

## Rubric

Score each applicable dimension `0` (fail), `1` (mixed), or `2` (pass), and cite
the sentence/evidence that supports the judgment. Use `N/A` only for dimensions
that genuinely do not apply.

| Dimension | 0 — fail | 1 — mixed | 2 — pass |
| --- | --- | --- | --- |
| Contextual correctness | Wrong sense or contradicts the sentence. | Plausible but underdetermined or imprecise. | Captures the target's contextual meaning. |
| Brevity | Unfocused, list-like, or needlessly long. | Understandable but can be materially shortened. | Concise; longer wording only prevents a misleading cue. |
| English wording | Unnatural, ungrammatical, or not a usable cue. | Understandable with a wording defect. | Idiomatic, clear English for a learner. |
| Evidence discipline | Unsupported claim or misuses conflicting evidence. | Evidence relation is unclear but not plainly false. | Cites fitting evidence, or appropriately marks context-only inference. |
| Outcome handling | Invalid/ambiguous result is published as a confident Gloss. | Safely omitted but reason/diagnostic is inadequate. | Invalid is rejected; unresolved is omitted with a useful reason. |

For each generation, record scores and a case disposition (`better`, `same`,
`worse`, `not comparable`, `invalid`, or `unresolved`). A score is not a
statistical quality claim: report counts and concrete regressions, not an
unqualified average. Treat any unsupported meaning, wrong contextual sense, or
published invalid/ambiguous result as an actionable regression regardless of
brevity or aggregate score.

## Review record

Populate one row per case only after inspecting its frozen payloads and outputs.
Attach the full case records in the PR or a repository-local fixture; do not
replace them with paraphrased judgments.

| Case ID | Historical Gloss / source | Current Gloss / outcome | Historical → current scores (correctness, brevity, English, evidence) | Disposition and reviewer notes |
| --- | --- | --- | --- | --- |
| _pending_ | _pending_ | _pending_ | _pending_ | _pending_ |

**Source identity:** historical artifact(s) _pending_; index version(s) _pending_;
provider/model _pending_; contextual prompt version _pending_; sample revision
_pending_.

**Limitations:** The sample is small and purposive, not representative of all
lemmas, genres, senses, or provider behavior. Model outputs may vary by provider
and time; conclusions apply only to the recorded source/prompt/provider
identities. A correctly shaped response is not proof of semantic correctness.

**Human review:** reviewer _pending_; date _pending_; decision _pending_; follow-up
regressions _pending_.

## Offline checks

Deterministic tests may verify frozen-input identity, valid evidence references,
context-only/unresolved consistency, rejection of malformed or unsupported
outcomes, and omission behavior. They must not call a live model or assert a
particular model wording. Semantic judgments and the completed before/after
review remain human work.
