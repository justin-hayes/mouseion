# ADR 0063: Stanza model provisioning on a Docker volume instead of the image

Status: **Accepted** · Date: 2026-09-13 · Author: Justin + opencode

Builds on **ADR 0023** (the NLP service owns language capabilities).

## Context

The standard NLP deployment bakes Stanza models into the image at build time:
`nlp/Dockerfile` runs `stanza.download(..., processors='tokenize,pos,lemma')`
for `de` and `it` into `$STANZA_RESOURCES_DIR=/opt/stanza_resources`. This
couples the model bundle (which languages, which processors, which Stanza
version) to the image lifecycle:

- The bake is **incomplete**: the runtime pipeline always loads
  `tokenize,pos,lemma,depparse` and conditionally `ner`, but the image
  provisions only the first three. Every fresh container silently re-downloads
  `depparse` (and `ner`) into an ephemeral layer on first warmup, despite the
  cache being described as immutable.
- Adding a language or refreshing models requires a full image rebuild that
  re-installs CPU torch and re-downloads the models (`nlp/Dockerfile:10-13`).
- The NLP service is the deployment owner of language capabilities
  (ADR 0023); the deployment should be able to change that posture without a
  rebuild.

The home-lab deployment is single-host with outbound egress. A self-hosted
S3-like object store may join the lab later; the chosen mechanism should not
paint the deployment into a corner.

## Decision

- **Models live on a named Docker volume** mounted at
  `/opt/stanza_resources`, not in the image. The image keeps
  `STANZA_RESOURCES_DIR=/opt/stanza_resources` as its only model-location knob.
- **A one-shot init container provisions the volume** before the NLP service
  starts. It reuses the nlp image, mounts the volume, runs an explicit
  `stanza.download(..., processors='tokenize,pos,lemma,depparse,ner')` for each
  configured language, and exits; the NLP service gates on
  `service_completed_successfully`. The build-time bake is removed.
- **The provisioned bundle is the full runtime processor set including `ner`**,
  regardless of whether the runtime NER toggle is enabled. NER is a code-level
  default-off toggle today, so relying on warmup to provision models would
  never fetch NER resources.
- **Refresh is automatic via a marker** written into the volume recording the
  Stanza version and the configured language set. When the Stanza pin changes,
  the init wipes and re-provisions the resources; when the language set grows,
  it downloads only the missing languages.
- **`MOUSEION_NLP_WARM_LANGUAGES` stays the single knob** driving both
  provisioning and warmup: provision exactly what you warm and advertise.
- **No model-source abstraction yet.** The init's download source is Stanza's
  CDN today; when a self-hosted object store lands, the source is swapped in
  place and the volume remains the local working cache.

## Consequences

- Adding a language becomes an environment change (`compose.yaml` + the env
  var), not an image rebuild; model refresh rides the pinned Stanza version.
- First provisioning happens at runtime, so first startup downloads the full
  bundle (slower once) and the deployment now has a hard egress dependency for
  initial provisioning; the serving container previously ran offline.
- Fresh containers no longer re-download `depparse`/`ner` into an ephemeral
  layer.
- The deployment gains one init container and one named volume; the volume is
  re-downloadable, so it needs no backup.
- The Stanza bump path is explicit and automatic: marker mismatch triggers
  re-provisioning, so stale-version models cannot silently serve.

## Alternatives considered

- **Keep the bake but complete the processor set.** Rejected: still couples
  language/model changes to image rebuilds and pays the image churn (torch
  re-install, model re-download) every time.
- **Let the server's existing warmup download into the volume.** Rejected:
  warmup provisions only what the runtime pipeline enables, so NER (a
  code-level default-off toggle) would never be provisioned; it also mixes
  provisioning failures into the serving path and delays readiness.
- **Manual refresh (documented volume wipe on Stanza bumps).** Rejected:
  stale models would silently serve until someone remembers; the marker is a
  few lines and testable.
- **Separate provisioning and warmup environment variables.** Deferred:
  provisioning exactly what you advertise is coherent with ADR 0023 today;
  split only if models-on-disk ever diverge from advertised capabilities.
- **Introduce the model-source abstraction now (S3/MinIO seam).** Rejected:
  single host, single source — YAGNI. The swap is contained to the init's
  source step, so nothing built here is thrown away.

## References

- [ADR 0023: NLP service owns language capabilities](0023-nlp-capabilities.md)
- [Language Support feature](../features/language-support.md)
- `nlp/Dockerfile` (bake step being removed), `compose.yaml` (NLP service),
  `nlp/src/mouseion_nlp/server.py` (warmup and `STANZA_RESOURCES_DIR`).