# ADR 0094: Source-independent annotation and provenance

Status: **Accepted; implementation pending** · Date: 2026-10-14 · Author: Justin + OpenCode

Recorded through the live decision in
[Choose a source-independent annotation and provenance contract](https://github.com/justin-hayes/mouseion/issues/2027).
This is an accepted contract, not authorization for production implementation,
schema changes, backfills, or deployment. Repository promotion requires human review.

## Context

Stanza and classical annotation producers differ in segmentation, scheme,
coverage, lexical identifiers, and review provenance. The reviewed
[scheme evidence](https://github.com/justin-hayes/mouseion/issues/2032#issuecomment-6161969802)
shows that inspected native and UD releases are not interchangeable: morphology,
secondary syntax, references, and covered passages can differ. The reviewed
[compatibility audit](https://github.com/justin-hayes/mouseion/issues/2025#issuecomment-6161582471)
also identifies current assumptions about mandatory dependencies, self-headed
roots, and exact-analysis corrections. A common contract must not manufacture
missing evidence to fit those assumptions.

## Decision

### Retain evidence separately from canonical interpretation

Retain each acquired artifact unchanged, with its origin and immutable revision.
Accept native or UD input through reviewed adapters; do not require paired
downloads. When both are acquired, retain separate lineage and verify their
correspondence rather than assume equivalent releases or token coverage.

Adapters retain source-native information linked to its sentences and nodes:
forms, lemma variants and lexical discriminators, POS, morphology, primary and
secondary syntax, review claims, and references where supplied. This includes
AGDT token `cite` and sentence `document_id`/`subdoc`, and PROIEL nested `slash`
elements. Unknown fields remain preserved in the artifact. Evidence is
retrievable for inspection and diagnostics subject to licensing and access;
this does not require a new learner-facing philological interface or a
first-class query for every native field.

The canonical interpretation is derived, not a replacement for native evidence.
Use canonical UPOS, named UD-compatible morphological features, and basic UD
dependency meanings only where documented, reviewed mappings support them.
Unsupported or ambiguous mappings remain unresolved. Native morphology is not
normalized FEATS; source lemma variants, lexical homograph distinctions, native
relations, and secondary edges remain distinct evidence.

Ancient Greek identities follow the accepted
[normalization decision](https://github.com/justin-hayes/mouseion/issues/2033#issuecomment-6163487386):
keep exact surfaces, decoded citation forms, canonical keys, and scheme-qualified
lexical homograph discriminators distinct. Producer decoding/mapping revisions
are separate from Mouseion-owned normalization profiles. This contract does not
change existing languages' identity rules or add learner POS editing.

### Exact text, nodes, and sentences

Use stable node/occurrence identifiers within an exact analysis. Offsets alone
cannot distinguish linguistic words sharing a surface span. References use
verified half-open Unicode-code-point spans against the exact extracted source
document text; shared or discontinuous spans are explicit. Preserve acquired
text and native coordinates separately from any extracted text or binding
transformation. Never guess component offsets from lemma or surface length.

Distinguish source spans, multiword-token containers, linguistic components, and
unprinted nodes. Empty nodes have no invented surface span and are not printed
vocabulary occurrences. Count each eligible, text-attested lexical occurrence
once; a multiword container adds no extra count, while its attested components
may count separately even when sharing a span. Punctuation and empty nodes do
not become vocabulary occurrences. Segmentation provenance matters because
different producers can yield different token totals.

Native identifiers are scoped to the source revision/document, not assumed
globally unique. Citation strings locate passages but establish neither node
identity nor text alignment. Preserve native segmentation and separately
declare the segmentation used by the analysis. Sentence text comes from the
exact bound text with verified spans and declared boundary completeness.
Annotation-carried text can be the authority under ADR 0093, but any spacing or
punctuation reconstruction is explicit and validated. Casual token joining or
invented words/punctuation cannot manufacture a complete sentence. Sentence
extent and card suitability are separate judgments.

### Usability is capability-specific

Vocabulary-ready publication for Reading requires verified coverage of the
Book's entire declared analysis scope, exact text correspondence, sentence/token
segmentation, and accounting for every lexical occurrence with a usable canonical
lemma/POS identity or an explicit, justified non-vocabulary classification.
Missing or ambiguous identities cannot silently disappear from counts, nor can
the scope shrink to available annotation fragments. Partial evidence remains
inspectable but does not make the whole Book vocabulary-ready.

A source-supported non-vocabulary classification is not an Occurrence exclusion.
Learner exclusions retain their contribution to the analyzable-token denominator
under ADR 0081. Neither exclusions nor invented classifications can hide missing
annotation to claim complete coverage. Existing language-specific vocabulary
eligibility is not replaced with eligibility for every graph node.

Morphology and dependencies are optional for the general contract. Explicitly
distinguish missing, unsupported, ambiguous, invalid, and usable information;
partial syntax remains partial. A dependent feature requires the validated
information it actually uses. In particular, unavailable syntax is not evidence
that a relation is absent, and vocabulary-ready does not mean card-ready.

Canonical basic dependencies use sentence-local node references. Actual roots
retain the self-headed `root` convention. A complete basic tree requires
resolved heads, one root, and no cycles or disconnected nodes. Retain empty
nodes and secondary syntax separately rather than flatten them into printed
words or silently discard them. A reduced graph derived by omitting nodes is a
separately versioned transformation, not evidence that the original parse was
complete. Never fabricate root relations to satisfy a validator.

Broken text correspondence, incomplete required scope, or unresolved required
lexical identities prevent vocabulary-ready publication. Invalid optional
morphology/syntax is retained with diagnostics but withheld from canonical
consumers; unaffected vocabulary may still qualify. Publication is atomic: a
failed candidate cannot replace valid current evidence or expose half-written
results.

### Provenance, versions, and competing interpretations

Distinguish human, human-reviewed automatic, automatic, and unknown provenance
by the documented annotation layer and scope: release, subset, sentence, or
token, with narrower exceptions where supplied. A syntax review does not prove
lemma/morphology review. Record conversion and mapping lineage separately.
Retain an upstream review designation as an attributed claim, not Mouseion's
scholarly certification. Learner corrections are separate owner-scoped facts.

Technical readiness and review status are independent. Valid automatic or
unknown-provenance evidence can satisfy the general contract, including Stanza
output. The human-reviewed Greek MVP has its own candidate-specific review
requirement; general admission does not certify that requirement.

Record exact source revisions/hashes, scheme and known version,
producer/model/configuration where applicable, adapter/mapping revision,
canonical-contract version, normalization profile, and binding transformations.
Unknown upstream metadata remains explicitly unknown; Mouseion-owned
transformations have known, reproducible versions. Missing metadata is not
permission to invent a producer or scheme version.

Each analysis declares which evidence supports its canonical fields. There is no
automatic preference merely because an input is native, UD, or reviewed.
Combining sources requires explicit versioned selection rules and verified
node/text correspondence; do not splice incompatible parses or infer lexical
equivalence from matching strings. Otherwise use one declared interpretation
and retain alternatives as evidence.

### Adapters and publication compatibility

Stanza and classical adapters satisfy the same interface. They own native
decoding and documented mappings, not publication, learner-state transitions,
or invented defaults. A shared validation module checks binding/alignment,
references, identities, and capability requirements. Consumers use its validated
interpretation rather than branch on producer names.

Changed evidence, mappings, configuration, or normalization produces a new
exact analysis, not an in-place reinterpretation. Preserve ADR 0040's single
current published analysis and ADR 0093's owner-scoped adoption. Preserve
existing corrections/exclusions, current-reading snapshots and reservations,
deck specifications, Known facts, and history. Corrections do not transfer
automatically to new analyses, even when spans or canonical output match.
Shared refreshes do not globally switch Books. Changing active-reading
vocabulary continues to require End, review, and Start with a new snapshot;
deck re-preparation creates new frozen specifications, not historical rewrites.

## Trade-offs and compatibility

UD-only lossy storage would erase evidence; requiring paired native/UD downloads
would exclude usable sources. Universal complete parses would force fabrication
or reject valid vocabulary evidence. Per-producer consumer branches would spread
scheme assumptions across the application. Separate evidence, explicit
capabilities, and one validation seam preserve fidelity at the cost of richer
identity, diagnostics, provenance, and adapter verification.

For the future contract, this amends ADR 0060's universal `depparse` requirement
and basic-only retained-evidence policy, while retaining actual self-headed
roots. It extends occurrence identity beyond offset-only matching without
migrating existing corrections. Existing protobuf, schema, Stanza processing,
publication behavior, and production analyses remain unchanged until separately
reviewed implementation. No field numbers, SQL shapes, or automatic backfills
are prescribed here.

Candidate-specific MVP verification, Greek sentence/card rules, acquisition
flow, corpus-count policy, operational adoption/retention, and the staged
implementation/migration plan remain with their existing map tickets. No
dataset, cross-edition alignment tolerance, reconstruction algorithm, new
correction workflow, storage layout, or Greek runtime support is delivered here.
