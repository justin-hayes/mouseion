# External LLM translation

Mouseion supports OpenAI-compatible Chat Completions endpoints through
`NewConfiguredLLMProvider` for non-prepared-deck enrichment and the default
prepared-deck standard path. Explicit Batch work uses the separate durable
OpenAI Batch adapter and therefore requires the official OpenAI API base URL.
The administrator controls the shared provider with:

- `MOUSEION_LLM_ENABLED`: set to `true` to enable external translation.
- `MOUSEION_LLM_API_KEY`: bearer credential (required when enabled).
- `MOUSEION_LLM_MODEL`: backend model name (required when enabled).
- `MOUSEION_LLM_BASE_URL`: optional API root; defaults to
  `https://api.openai.com/v1` and can point to a self-hosted compatible server.
- `MOUSEION_LLM_TIMEOUT`: optional positive Go duration; defaults to `30s`.
- `MOUSEION_LLM_REASONING_EFFORT`: optional `low`, `medium`, or `high` value;
  defaults to `low`.
- `MOUSEION_LLM_SUPPORTS_REASONING_EFFORT`: optional boolean for a custom
  endpoint/model that supports the OpenAI `reasoning_effort` request field.

Admin enablement is not user consent. Pass the user's current preference as
`EnrichmentConfig.UserOptIn`; external calls occur only when that flag and
`ExternalEnabled` are both true. Use `SentenceContext` to send the canonical
lemma, tested surface form, and one example sentence, or `LemmaOnly` to omit the
sentence and tested surface entirely.

Mouseion sends `reasoning_effort` only for recognized OpenAI reasoning models at
`api.openai.com`, or when `MOUSEION_LLM_SUPPORTS_REASONING_EFFORT=true` is set.
Unknown and self-hosted endpoints therefore retain the compatible request shape
unless explicitly opted in. Reasoning requests omit `temperature`, which some
reasoning models reject; other requests continue to send `temperature: 0`.
The prompt requests one concise, strict JSON object and does not use a universal
verbosity field.

The provider boundary accepts only language, canonical lemma, UPOS, tested
surface form, and the optional complete example sentence. A response may
include a complete sentence translation and the plain-text
`sentence_translation_target` phrase corresponding to the target. Results use
the immutable external cache; cache keys include the complete source sentence
hash, language, provider, and model/prompt version, and result provenance
records that version and the cache timestamp. Provider HTML is never trusted.
HTTP 408, 429, and 5xx responses are retryable by the enrichment service;
failures become warnings in the inline pipeline, while durable jobs may retry
them through River.

## Optional lemma suggestions during review

When the shared provider is enabled, `NewConfiguredLemmaSuggestionProvider`
supports an explicit, one-occurrence request from Reading's lemma review. It
sends only the study language, observed target, analyzer lemma and POS, one
sentence, and an available local lexical alternative with its source/version.
The dedicated `LemmaSuggestionRequest` is size-bounded and contains no owner,
Book, catalog, or reading-history fields. Suggestions are shown with provider and
prompt-version provenance and are never applied; only the learner's existing
manual decision flow can change an effective identity. A disabled or failing
provider does not affect manual review.
