# ADR 0116: The NLP service analyses graded Ancient Greek readers

Status: **Accepted — direction, implementation pending** · Date: 2026-11-09 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided in
[#2304](https://github.com/justin-hayes/mouseion/issues/2304) under the graded
readers map [#2299](https://github.com/justin-hayes/mouseion/issues/2299).
Amends [ADR 0099](0099-ancient-greek-imported-analyses.md)'s "the import never
calls the NLP service" for graded readers only. The OGA import of
[ADR 0099](0099-ancient-greek-imported-analyses.md) and the Classical catalog
spec [#2401](https://github.com/justin-hayes/mouseion/issues/2401) are
unchanged.

## Context

A graded reader is a Classical Book whose Work is the reader itself, such as
Edwards, *Salamis in Easy Attic Greek* (1913). The Greek Learner Texts Project
digitisations are plain text under CC BY-SA 4.0. Unlike the `oga_src` file of
ADR 0099, they carry no annotation, so Mouseion must produce lemmas itself.

The accepted quality bar (#2303) is automatic lemmas, one per occurrence, with
an advisory Lemma review flag whenever a second source offers a specific
alternative lemma. The only analyser with a verified non-NC lineage, Morpheus,
gives candidate parses without choosing one, needs beta-code input, and would
mean building and pinning a new tool. [ADR 0111](0111-third-party-licence-posture.md)
made Stanza `grc` eligible as an NC Analysis resource when the deployer fetches
it. Stanza already runs in the NLP service, which `nlp-init` provisions. The
Mouseion server image is `distroless/static` and cannot run Stanza itself.

## Decision

**The NLP service analyses graded readers' Greek during preparation.** Reader
preparation sends the reader's paragraph text to the NLP service and publishes
the result as an ordinary analysis run. Heading lines are never sent. Each
request is one citation paragraph, so no sentence straddles a citation. The
OGA import keeps ADR 0099's rule: it never calls the NLP service. Once a reader
is prepared, Reading, Browse and decks work with the service stopped, as for an
EPUB.

**Two Stanza `grc` packages, one chosen lemma.** The `perseus` package
tokenises, segments, tags and parses, and chooses each occurrence's lemma. The
`proiel` package lemmatises the same tokens in pretokenised mode. After
canonical-lemma normalisation, a disagreement raises an advisory Lemma review
flag, with the `proiel` lemma as a Lemma suggestion, under the rules of #2303:
lemma ambiguity only, advisory, and only for identities that reach the
recurring floor. The analysis wire format gains an additive alternative-lemma
field on a token.

**Syntax is present.** Stanza's `grc` output is Universal Dependencies, so a
reader's run stores it as a modern-language run does. ADR 0099's `absent`
syntax stays for the OGA import.

**`grc` is a classical language, not a general one.** The NLP service
configures `grc` with its own setting and reports it in a separate classical
list in its capabilities. Only reader preparation requests it. EPUB and OPDS
analysis keep refusing `grc`, so a learner cannot analyse an arbitrary Greek
EPUB through this path. `grc` is in the default Compose deployment. A
deployment without it refuses reader preparation with an "analysis
unavailable" note.

**The interpretation is versioned.** A reader's interpretation version names
the paragraph parsing, the `perseus` + `proiel` selection and the comparison
rule, starting at `gltp-reader/1`. Changing any of them bumps it. The run also
records each model's version from the capabilities response. Stanza and its
resource manifest stay pinned by the repository, so a model change arrives with
a Mouseion release and a version bump.

**Attribution follows ADR 0111.** A reader deck carries the reader's own
Source-content notice and the generic analysis-models notice. It never carries
the OGA `AncientGreekSourceNotice`. The register lists the reader
digitisation, the two Stanza `grc` packages and their NC training lineage.

## Consequences

- The default deployment fetches the Stanza `grc` `perseus` and `proiel`
  packages, both NC through their UD treebanks and the `conll17` vectors.
  They are deployer-fetched and never shipped, as ADR 0111 requires.
- Preparing a reader needs the NLP service. Preparing an OGA work still does
  not.
- The proto, the NLP service's capabilities and the analysis consumers change
  additively. Modern-language analyses are unchanged.
- The pilot's acceptance evidence is one hand-reviewed chapter (Edwards 15)
  with its flag rate and missed-error rate. A poor miss rate is grounds to add
  Morpheus as the second source, not to block the pilot.
- The Classical catalog (#2401) keeps its scope. Reader preparation is a
  follow-on that needs one seam there: preparation chooses the adapter by
  source format.

## Alternatives considered

- **Morpheus-only analysis, published and pinned like `oga_src`.** Keeps
  ADR 0099 whole and is non-NC, but needs an arbitrary tie-break for the chosen
  lemma, a beta-code converter and a new tool. Rejected as too fragile.
- **Morpheus in the NLP image as the second source.** Gives principled
  candidate sets, but adds a C build, a converter and a third licence lineage.
  Kept as the fallback.
- **An operator-supplied Stanza-derived file.** Cannot ship under ADR 0111 and
  contradicts #2401's "no operator command".
- **A separate `grc` analyser container.** Cleaner isolation, one more service
  for one language.
- **`grc` opt-in, off by default.** Rejected: readers should work in a default
  deployment.

## Related

- [ADR 0099: Ancient Greek through imported analyses](0099-ancient-greek-imported-analyses.md)
- [ADR 0110: Bound Mouseion by philological reading competence](0110-philological-reading-competence-boundary.md)
- [ADR 0111: One licence posture for third-party data](0111-third-party-licence-posture.md)
