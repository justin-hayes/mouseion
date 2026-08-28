# OpenAI Batch prepared-deck validation report

Fixture: `<name>`<br>
Manifest digest: `<sha256>`<br>
Provider: `<provider>`<br>
Provider version/API version: `<version>`<br>
Model: `<model>`<br>
Prompt version: `<prompt-version>`<br>
Operator/date: `<operator> / <UTC date>`

This report must keep measured real-provider evidence separate from synthetic
replay evidence. Synthetic or stub results demonstrate deterministic code
paths only; they do not establish provider quality, production behavior, or a
paid-cost claim. Record hypotheses under the hypotheses heading, never as
measured results.

## Frozen inputs

- Manifest: `internal/batchvalidation/testdata/manifest.json`
- Request fixture (exact provider JSONL): `internal/batchvalidation/testdata/requests.jsonl`
- Fixed response fixture: `internal/batchvalidation/testdata/responses.json`
- Selection, prompt version, model, cache identity, and validator: unchanged
  from the frozen fixture and shared `TranslationCodec`.

## Synthetic/stub evidence (not cutover evidence)

Run without `-real-provider`. Record success, partial failure, expiry,
cancellation, restart, and ambiguous-submission scenarios, including the
expected fail-closed/manual-recovery result.

| Measure | Synchronous | Batch |
|---|---:|---:|
| queue / completion / total latency |  |  |
| token usage / estimated cost |  |  |
| cache-hit ratio |  |  |
| provider / retry / error / expiry counts |  |  |
| parse / validation failures |  |  |
| target alignment / translation-quality review |  |  |
| completeness / omissions |  |  |
| duplicate / miscorrelated outcomes |  |  |
| artifact bytes stable for fixed responses |  |  |

Artifact SHA-256 (APKG / TSV): `<sync> / <sync>`; `<batch> / <batch>`

## Measured real-provider evidence

Run only with the explicit operator command and paid-call acknowledgement.
There is no deployed flag or runtime transport selector.

| Measure | Synchronous | Batch |
|---|---:|---:|
| queue / completion / total latency |  |  |
| token usage / estimated cost |  |  |
| cache-hit ratio |  |  |
| provider / retry / error / expiry counts |  |  |
| parse / validation failures |  |  |
| target alignment / translation-quality review |  |  |
| completeness / omissions |  |  |
| duplicate / miscorrelated outcomes |  |  |
| artifact bytes stable for fixed responses |  |  |

## Cutover gates

Each gate requires real-provider evidence on the agreed frozen manifests.

| Gate | Pass condition | Evidence/result | Pass/fail |
|---|---|---|---|
| quality | translation review and target alignment are no worse than synchronous |  |  |
| correctness | semantic request parity and zero unexplained correlation, parse, or validation failures |  |  |
| privacy | no source/owner metadata, credentials, prompts, responses, or raw provider errors in durable/log artifacts |  |  |
| durability | partial failure, expiry, cancellation, restart, and ambiguity recover as documented |  |  |
| cost | approved budget after retries and cache effects |  |  |
| latency | queue, completion, and total latency fit the offline workflow |  |  |
| operational recovery | observation, cancellation, and fail-closed manual recovery rehearsed |  |  |

## Operator recovery record

- Observation/status history and Batch ID handling: `<record privacy-safe metadata>`
- Cancellation: local publication is stopped first; provider cancellation is
  best effort; late results are fenced and ignored.
- Ambiguous submission: search recent provider Batches by opaque metadata and
  exact input identity. If creation cannot be proven, do not resubmit
  automatically; fail closed for operator/manual recovery.
- Restart: resume polling/reconciliation from durable state; do not submit a
  second Batch for the same generation.

## Review and rollback decision

- ADR-0031 review/acceptance: `accepted`
- Endpoint retirement for custom OpenAI-compatible providers: `yes`
- Operational defaults (two generations, 5,000 requests, 30-second polling,
  seven-day provider-file expiry): `approved`
- Cutover recommendation for issue #354: `GO` (the issue #353 human gate
  approved every required cutover gate)
- Hypotheses/follow-ups (not evidence):

Rollback for a future cutover is a code revert before synchronous support is
removed. This validation harness intentionally does not add a runtime
selector; CI/startup never performs paid provider calls.
