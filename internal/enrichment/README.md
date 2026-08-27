# External LLM translation

Mouseion supports OpenAI-compatible Chat Completions endpoints through
`NewConfiguredLLMProvider`. The administrator controls the provider with:

- `MOUSEION_LLM_ENABLED`: set to `true` to enable external translation.
- `MOUSEION_LLM_API_KEY`: bearer credential (required when enabled).
- `MOUSEION_LLM_MODEL`: backend model name (required when enabled).
- `MOUSEION_LLM_BASE_URL`: optional API root; defaults to
  `https://api.openai.com/v1` and can point to a self-hosted compatible server.
- `MOUSEION_LLM_TIMEOUT`: optional positive Go duration; defaults to `30s`.

Admin enablement is not user consent. Pass the user's current preference as
`EnrichmentConfig.UserOptIn`; external calls occur only when that flag and
`ExternalEnabled` are both true. Use `SentenceContext` to send the canonical
lemma, tested surface form, and one example sentence, or `LemmaOnly` to omit the
sentence and tested surface entirely.

The provider boundary accepts only language, canonical lemma, UPOS, tested
surface form, and the optional example sentence. Results use the existing
immutable external cache; cache keys include language, provider, and
model/prompt version, and result provenance records that version and the cache
timestamp. HTTP 408, 429, and 5xx responses are retryable by the enrichment
service; failures become warnings in the inline pipeline, while durable jobs
may retry them through River.
