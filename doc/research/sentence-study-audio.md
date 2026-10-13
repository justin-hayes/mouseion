# Sentence Study Audio Research

**Research date:** 2026-10-13 (sources fetched 2026-10-13 unless noted)

**Scope:** Candidate sentence-audio sources for German (`de`), Italian (`it`), and Modern Greek (`el`): ElevenLabs (named by the ticket), one cloud alternative (Google Cloud Text-to-Speech, Chirp 3: HD), and one locally runnable alternative (Piper). Coqui XTTS-v2 is included only as a Greek negative control. Feeds map #1983 and research ticket #1990.

**Status:** Research only. No accounts were created, no paid or credentialed API calls were made, no book text was uploaded, and no provider was selected. Every quality statement is a documented claim, not a listening result.

## Summary

- **Documented language coverage:** ElevenLabs (v4, v4 Turbo, v3, multilingual v2, Flash) lists Greek, German, and Italian. Google Chirp 3: HD lists `de-DE`, `it-IT`, and `el-GR`. Piper has voices for all three languages, but only three Greek voices. XTTS-v2 does not list Greek.
- **ElevenLabs terms are a qualified blocker, not an absolute one.** The non-EEA Terms of Service (Last Updated 2026-10-09) grant ElevenLabs a perpetual, irrevocable, sublicensable license to use submitted Input "to provide the Services ..., to improve the Services, and to develop new services and products." Enterprise customers have separate terms, a DPA, and Supplemental Terms that control in a conflict, and those were not reviewed. Under the self-serve terms, sending sentence text would conflict with ADR 0007. Enterprise terms might differ, but that is unverified.
- **Google Cloud TTS has narrower Greek controls.** Custom pronunciations and pause control are documented as unavailable for `el-GR`. Its privacy terms are better documented for logging and training, but Chirp 3: HD's specific data terms were not confirmed.
- **Piper is the only local path, with real license constraints.** The engine and its espeak-ng dependency are GPL-3.0. The Greek voice `el_GR-joy-medium` is CC BY-NC 4.0 with a named speaker. The other Greek voices are CC0.
- **Costs are low at sentence scale, but "free" and "sustainable" differ.** At 30,000 characters per month, ElevenLabs pay-as-you-go is about $1.20–$2.40/month in marginal cost, and a $22/month Creator subscription is the commercial-use minimum if the pay-as-you-go plan does not qualify. Google Chirp 3: HD is $0 marginal within its 0–1M character free usage limit.
- **Nothing here proves naturalness.** Native-speaker listening tests are required for every candidate, language, sentence-level replay, and clause-level replay.

## Language and model support

| Candidate | German (`de`) | Italian (`it`) | Modern Greek (`el`) | Notes |
| --- | --- | --- | --- | --- |
| ElevenLabs v4 / v4 Turbo | Yes | Yes | Yes | "90+ languages supported" on the pricing page [E3]. |
| ElevenLabs v3 / v3 Conversational | Yes | Yes | Yes | "70+ languages supported"; v3 has a 5,000-character limit [E3]. |
| ElevenLabs multilingual v2 | Yes | Yes | Yes (`el`) | Models page lists `el` in the v2 language set; pricing page says "29 languages", 10,000-character limit [E1, E3]. |
| ElevenLabs Flash | Yes | Yes | Yes | Models page: all v2 languages plus `hu`, `no`, `vi` [E1]. |
| Google Chirp 3: HD | Yes (`de-DE`) | Yes (`it-IT`) | Yes (`el-GR`) | Locale list on the Chirp 3: HD page [G1]. |
| Piper (`rhasspy/piper-voices`) | 10 voices | 4 voices | 3 voices | Counts from `voices.json` [P2]. See voice table. |
| Coqui XTTS-v2 | Yes | Yes | **No** | 17 listed languages; Greek not among them [X1]. Not a Greek candidate. |

ElevenLabs' documented `language_code` parameter is not supported for `multilingual_v2` models [E2]. Greek and German selection should be tested with real sentences, not by relying on a language code.

### Piper voices for the three languages

Counts and licenses are from `voices.json` [P2] and each voice's `MODEL_CARD` (fetched 2026-10-13).

| Language | Voice (quality) | Speakers | License per model card |
| --- | --- | --- | --- |
| de_DE | thorsten high / medium / low; thorsten_emotional-medium | 1 / 1 / 1 / 8 | CC0 (thorsten high verified) |
| de_DE | mls-medium | 236 | CC-BY 4.0 (verified) |
| de_DE | karlsson, kerstin, pavoque, ramona (low); eva_k (x_low) | 1 each | Not checked in this pass |
| it_IT | serena-high / medium | 1 | CC-BY 4.0 (high verified) |
| it_IT | paola-medium | 1 | "See URL" (unclear; not resolved) |
| it_IT | riccardo (x_low) | 1 | Not checked in this pass |
| el_GR | joy-medium | 1 | **CC BY-NC 4.0** (verified); dataset speaker named "Chara Kaltsou" [P2] |
| el_GR | rapunzelina-medium / low | 1 | CC0 (verified) |

## Candidate details

### ElevenLabs

- **API.** `POST /v1/text-to-speech/{voice_id}` returns audio; streaming and WebSocket variants are documented [E2, E9].
- **Pricing (USD, excludes tax, fetched 2026-10-13).** Per the pricing page: v4 $0.08 per 1K characters; v4 Turbo $0.04 per 1K; v3 $0.08; v3 Conversational $0.04; multilingual v2 $0.08 [E3]. Subscriptions: Starter $6/month ($1 first month), Creator $22/month ($11 first month, 275k v4 characters included) [E3]. Pricing page says "Pay as you go" is available; this report did not establish the Flash price from the page.
- **Commercial use.** Free Users may use the Services only for non-commercial purposes. Paid Users (a paid subscription plan) may use them commercially, subject to the Prohibited Use Policy [E5]. Whether pay-as-you-go counts as a paid subscription plan was not established.
- **Latency.** Documented as about 100 ms (v4 Turbo), about 280 ms (v3 Conversational), and about 75 ms (Flash), "excluding application & network latency" [E1, E3]. Not measured here.
- **Pronunciation.** Pronunciation dictionaries (PLS with IPA or CMU phonemes). IPA/CMU in non-English languages requires `eleven_v4` [E10].
- **Speed.** `voice_settings.speed` defaults to 1.0. No documented range [E2].
- **Clause continuity.** `previous_text`/`next_text` and request IDs are documented for prosody across split segments [E2].
- **Determinism.** `seed` is best-effort; outputs are nondeterministic [E2]. A cache must store generated audio rather than rely on regeneration.
- **Zero Retention Mode.** Enterprise only, API only. Web UI and playground traffic are not covered [E4].

### Google Cloud Text-to-Speech (Chirp 3: HD)

- **API.** Synchronous `synthesize_speech` and bidirectional `streaming_synthesize` [G1].
- **Pricing (verified on the pricing page, 2026-10-13).** Chirp 3: HD is US$30 per 1M characters after a 0–1M character free usage limit. Chirp 3 Instant Custom Voice is US$60 per 1M [G2]. This report did not confirm whether the free usage limit resets monthly.
- **Pronunciation and pacing.** Custom pronunciations are unavailable for `el-GR`, and pause control is unavailable for `el-GR` [G1]. SSML is not supported in streaming requests [G1]. Per-locale availability of the `<phoneme>` tag was not verified for `el-GR`.
- **Speed.** `speaking_rate` from 0.25 to 2.0 [G1].
- **Data logging.** "Google does not log any customer Cloud TTS text or audio data" [G3].
- **Training.** The service terms say Google will not use Customer Data to train AI/ML models without the customer's permission [G4]. Chirp 3: HD's specific data terms were not confirmed.
- **Possible retention.** For some Google Frontier Model services, prompts and output may be stored for up to 30 days [G4]. Applicability to Chirp 3: HD was not established.

### Piper (local)

- **Engine license.** `piper1-gpl` is GPL-3.0 (COPYING, verified). It embeds espeak-ng for phonemization [P1].
- **Dependency license.** espeak-ng is GPL-3.0 (COPYING, verified). The runtime is therefore GPL-3.0 as a whole.
- **Voice license.** Model weights are licensed per voice model card, separately from the engine. The repository-level label on the Hugging Face page does not override per-voice cards [P2].
- **Runtime.** `pip install piper-tts`; voices are ONNX checkpoints with JSON config. The Greek config uses espeak voice `el` [P1, P3].
- **Speed.** `length_scale` in the voice config [P3]. The Greek `joy-medium` config ships 0.847 [P2]. Quality at non-default values is untested.
- **Latency and network behavior.** Not measured. "Fast and local" is the project's own description [P1]. Network egress was not tested.

### Coqui XTTS-v2 (negative control)

- 17 listed languages include German and Italian but not Greek [X1].
- Licensed under the Coqui Public Model License (CPML) [X1]. The full license text was not read in this pass, so commercial and redistribution terms are unverified.

## Cost

Assumption: 300 newly generated sentences per month, average 100 characters each (about 30,000 characters per month). Each sentence is generated once and cached. Prices fetched 2026-10-13 and subject to change.

| Option | Subscription minimum | Marginal cost at 30,000 characters/month | Notes |
| --- | --- | --- | --- |
| ElevenLabs v4 pay-as-you-go | None documented | $2.40 | Commercial-use qualification unclear (see above). |
| ElevenLabs v4 Turbo | None documented | $1.20 | Turbo is for latency; the quality tradeoff is untested. |
| ElevenLabs Creator | $22/month ($11 first month) | Included in 275k v4 characters | Sustainable commercial minimum if pay-as-you-go does not qualify. |
| Google Chirp 3: HD | None documented | $0 within 0–1M free usage | Free usage is not a sustainable price; the post-free rate is $30 per 1M, which is $0.90 at 30,000 characters. |
| Piper | None | Not metered | Local compute unmeasured. |

Regeneration, retries, and clause-level clips multiply characters. Clause-level replay roughly multiplies the sentence text several times over and should be costed separately.

## Retention, training, storage, and redistribution

### ElevenLabs

- **Terms license over Input.** Under the non-EEA Terms, the user retains rights to Input and Output (4(c)). The user grants ElevenLabs a license to use "your Content to provide the Services ..., to improve the Services, and to develop new services and products" (4(d)). It is described as perpetual, irrevocable, royalty-free, worldwide, and sub-licensable [E5]. "Input" includes text, and "Content" means Input and Output [E5].
- **Enterprise.** Enterprise customers have separate terms, a DPA, and Supplemental Terms, which control in a conflict with the self-serve Terms [E5]. Those documents were not reviewed here, so enterprise scope is unverified.
- **Training default.** The data-use help article says ElevenLabs does not train on Enterprise customer data by default. Other users can opt out through the "Improve the models for everyone" toggle, and new data is not used once it is disabled [E7]. The retrieved text does not state the default for individual accounts, so that default needs confirmation from the live account settings.
- **Retention.** The Zero Retention option is enterprise and API only [E4]. Other data may be retained per the Privacy Policy [E6].
- **Storage.** The Privacy Policy says personal information is transferred to the United States for storage [E6]. EU and India residency hosts exist for enterprise [E2].
- **Redistribution.** The Terms do not establish whether generated audio can be cached and shared among learners. The Prohibited Use Policy and Service-Specific Terms were not fetched.

### Google Cloud

- **Logging.** No logging of customer text or audio [G3].
- **Training.** No training on Customer Data without permission [G4].
- **Storage.** Possible 30-day storage for certain Frontier Model services; applicability not established [G4].
- **Output ownership and redistribution.** Not checked in this pass.

### Piper

- No external service; retention depends on the local deployment. Output and redistribution depend on each voice's license [P2].

## Licensing

| Component | License | Notes |
| --- | --- | --- |
| ElevenLabs API | Terms of Service | Free Users: non-commercial only. Paid Users: commercial use permitted [E5]. |
| Google Cloud TTS | Google Cloud terms | Output rights not separately checked. |
| Piper engine (`piper1-gpl`) | GPL-3.0 | Verified COPYING [P1]. |
| espeak-ng (embedded) | GPL-3.0 | Verified COPYING [P1]. |
| Piper voices (de/it/el) | Per model card | `joy-medium` CC BY-NC 4.0 (verified); `rapunzelina` CC0 (verified). |
| XTTS-v2 | CPML | Full text not read; Greek unsupported [X1]. |

## Repository contract conflicts

- **ADR 0007 §4 (privacy).** Permits external providers to receive only the target lemma, tested surface form, and one example sentence, and only for translation or context selection. The full document, surrounding text, and reading history never leave the lab. Sending sentence text for audio is a different purpose and is outside what ADR 0007 currently permits. Changing that would require a new or amended accepted ADR, which needs human review under AGENTS.md. This is not a blanket incompatibility of every audio provider; a local engine like Piper does not send text externally.
- **ElevenLabs Terms §4(d)** would give ElevenLabs a sublicensable license over sentence text sent under the self-serve Terms. This conflicts with the ADR 0007 stance. Enterprise terms might change that, but they were not reviewed.
- **Cache design.** ADR 0007 requires immutable, versioned cache keys including provider and provider version. Any audio cache needs an equivalent key: text, language, voice, model, speed, and pronunciation dictionary version.

## Learning-constraint relevance

- **Listening-first.** ElevenLabs v4 and v3 emphasize expressive delivery. `eleven_multilingual_v2` is documented as the stable long-form option [E1, E3]. Chirp and Piper stability is untested.
- **Sentence versus clause replay.** ElevenLabs documents continuity parameters for split segments [E2]. Google SSML does not work in streaming [G1]. Piper's clause pauses are untested.
- **Shadowing.** ElevenLabs `speed` has no documented range; Google `speaking_rate` is 0.25–2.0 [G1]; Piper `length_scale` is available but its naturalness effect is unknown [P3]. Pre-generated, cached audio makes latency less important than predictable replay.
- **Greek pronunciation.** Stress and lemma reading matter for learners. ElevenLabs pronunciation dictionaries are available on `eleven_v4` [E10]. Google's custom pronunciations and pause control are unavailable for `el-GR` [G1]. Piper uses espeak-ng for Greek phonemization [P3].

## Documented claims versus untested quality

Documented, not tested here:
- Language availability per model and voice.
- ElevenLabs latency figures (excluding network).
- Pricing as of 2026-10-13.
- Retention, training, and logging policies as stated on the pages cited.
- Piper voice licenses and the GPL runtime.

Not tested or verified:
- Naturalness, accent, or intelligibility in German, Italian, or Modern Greek. Greek voice quality is the highest-risk unknown.
- Latency or throughput from this environment.
- Clause continuity and pausing for any candidate.
- Piper's network behavior and espeak-ng coverage of named or foreign words.
- Google Chirp 3: HD output ownership and redistribution rights.
- ElevenLabs Prohibited Use Policy, Service-Specific Terms, and enterprise terms.
- Whether Google's free usage limit resets monthly.

## Human listening tests required

These require a native speaker. Use public or synthetic test sentences for external candidates, not private book text.

1. Sentence-level naturalness and intelligibility for each candidate and language at normal speed.
2. Clause-level replay: whether split clauses sound continuous and whether the pause before each clause is usable for shadowing.
3. Speed behavior: whether slower speech keeps natural pitch and rhythm.
4. Greek lemma and stress accuracy on words a learner is likely to meet.
5. Blind A/B comparison across candidates for each language.

## Unresolved prerequisites

1. Listening tests in each language for at least one external candidate and Piper.
2. A human decision on whether any external text may be sent for audio, and under which privacy stance. Not decided here.
3. Confirmation of ElevenLabs enterprise terms, Service-Specific Terms, and Prohibited Use Policy before any caching or redistribution design.
4. Confirmation of Google Chirp 3: HD output and data terms.
5. A Greek voice quality check. Piper's Greek voices are few; `joy-medium` cannot be used commercially.
6. Measured latency and CPU real-time factor on target hardware for any local candidate.

## Sources

Primary sources. "Verified" means the claim was checked against the fetched page or file.

- [E1] ElevenLabs, Models. https://elevenlabs.io/docs/overview/models (verified)
- [E2] ElevenLabs, Create speech (TTS convert) API reference. https://elevenlabs.io/docs/api-reference/text-to-speech/convert
- [E3] ElevenLabs, API pricing. https://elevenlabs.io/pricing/api (verified)
- [E4] ElevenLabs, Zero Retention Mode (Enterprise). https://elevenlabs.io/docs/eleven-api/resources/zero-retention-mode (verified)
- [E5] ElevenLabs, Terms of Service (non-EEA), Last Updated 09 October 2026. https://elevenlabs.io/terms-of-use (verified §4(c), §4(d), commercial-use clause, Supplemental Terms)
- [E6] ElevenLabs, Privacy Policy. https://elevenlabs.io/privacy-policy
- [E7] ElevenLabs, "Is my data used to improve ElevenLabs' AI models?". https://elevenlabs.io/docs/help-center/legal/is-my-data-used-to-improve-eleven-labs-ai-models (verified)
- [E9] ElevenLabs, Latency optimization guide. https://elevenlabs.io/docs/eleven-api/guides/how-to/best-practices/latency-optimization
- [E10] ElevenLabs, Using pronunciation dictionaries. https://elevenlabs.io/docs/eleven-api/guides/how-to/text-to-speech/pronunciation-dictionaries.md
- [G1] Google Cloud, Chirp 3: HD voices. https://cloud.google.com/text-to-speech/docs/chirp3-hd (verified locale exclusions and streaming SSML limit)
- [G2] Google Cloud, Text-to-Speech pricing. https://cloud.google.com/text-to-speech/pricing (verified)
- [G3] Google Cloud, Text-to-Speech data logging. https://cloud.google.com/text-to-speech/docs/data-logging
- [G4] Google Cloud, Service Specific Terms. https://cloud.google.com/terms/service-terms
- [P1] Open Home Foundation, piper1-gpl README and COPYING. https://github.com/OHF-Voice/piper1-gpl (verified)
- [P2] Rhasspy, piper-voices. https://huggingface.co/rhasspy/piper-voices (voices.json and MODEL_CARD files verified for Greek, German thorsten/mls, Italian serena/paola)
- [P3] Piper source: `src/piper/config.py`, `src/piper/voice.py`, `src/piper/phonemize_espeak.py`. https://github.com/OHF-Voice/piper1-gpl/tree/main/src/piper
- [X1] Coqui, XTTS-v2 model card. https://huggingface.co/coqui/XTTS-v2 (verified language list)
- espeak-ng COPYING (GPL-3.0). https://github.com/espeak-ng/espeak-ng (verified)
- Repository: `doc/adr/0007-enrichment-providers-caching-privacy.md` §4; AGENTS.md "Human-review boundary".

Not fetched: Azure AI Speech, Gemini-TTS language list, Piper latency benchmarks, CPML full text, ElevenLabs Service-Specific Terms and Prohibited Use Policy, ElevenLabs enterprise terms. Azure was not evaluated; one cloud candidate is sufficient for this ticket.
