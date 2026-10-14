# Lysias I AGDT vocabulary mapping specification

Versions:

- `lysias1-upos-rules/2026-10-14.2`
- `lysias1-headword-rules/2026-10-14.1`
- `lysias1-component-conventions/2026-10-14.1`
- Normalization profile: Mouseion `ancient-greek` v1 (#2033)

These are research proposals, not accepted mappings. The rule table with node counts and evidence IDs is `manifest/lysias1-rules.tsv`. Each evidence ID resolves in `manifest/lysias1-evidence.tsv`.

## Join keys

| Key | Meaning |
| --- | --- |
| `doc` | `tlg0540.tlg001.perseus-grc1@treebank_data:bf4334f0:v2.1`. Native IDs are scoped to this pinned file. |
| `node_key` | `<sentence id>:<word id>` |
| `binding_join_key` | `sNNNwNNN`, the format used by the #2048 binding manifests |

## Per-node fields (`manifest/lysias1-nodes.tsv`)

**Native evidence.** Kept unchanged:

- `form` (NFC; the raw form is in the components appendix where it matters)
- `native_lemma`
- `native_postag`
- `native_relation`
- `head`
- `head_relation`
- `artificial`
- `letter`
- `native_pos`

**UPOS.**

- `rule_id`.
- `upos_candidates`: one value when resolved.
- `upos_status`: `resolved | conditional | unresolved | n/a`.
- `eligibility`:
  - `eligible`: all candidates are in `AllowedPOS` {NOUN, VERB, ADJ, ADV} and the repo's `OccurrenceEligible` holds.
  - `conditional`: the candidates are mixed.
  - `non_vocab`: no candidate qualifies.
  - `unprinted`.

**Headword.**

- `lemma_notation`: `plain | morpheus_preverb`.
- `lemma_preverbs` and `lemma_base`.
- `citation_headword`.
- `headword_status`: `source_plain | decoded | conditional | unresolved`.
- `headword_rule`: H0–H4.
- `canonical_key`: #2033 applied to the headword, empty when the headword is unresolved.

**Discriminator.**

- `discriminator_scheme`: `morpheus-homograph`.
- `discriminator_value`: the v1.6 witness number.
- `discriminator_status`: `absent_in_source | stripped_in_v2.1_witness_v1.6 | conflated | n/a`. `absent` is distinct from `unknown`.

**Identity.**

- `identity` (`key|UPOS`) is set only when there is exactly one vocabulary candidate.
- `identity_candidates` lists every `key|UPOS`.
- `identity_status` is `resolved` only if UPOS, headword, eligibility and discriminator are all resolved. Otherwise it is `conditional`, or `unresolved` when the headword or UPOS is unresolved. It is `n/a` for non-vocabulary nodes.

**Scope and provenance.**

- `source_scope`: `running_text | rubric_label | rubric_punctuation | unprinted`.
- `component_convention`.
- `evidence_ids`.
- `flags`.

## Identity aggregates (`manifest/lysias1-identities.tsv`)

Each row is one candidate identity, with occurrences counted separately as resolved, conditional and unresolved.

- An identity is `resolved` if it has at least one resolved occurrence. Its conditional occurrences stay counted separately.
- An identity is `conditional` if it has only conditional occurrences, or if a v1.6 discriminator shows it merges distinct entries.
- An identity is `unresolved` if its headword is unresolved.

## Rules

See the TSV for full applicability and evidence.

### UPOS rules

- **Resolved.**
  - Letter rules: R01 n, R02 a, R03 d (non-particle), R06 v, R10 r, R11 l, R12 p, R13 i, R14 u.
  - Context rules:
    - R01R: rubric noun.
    - R01P: Morpheus `nom.proper` → PROPN.
    - R03K: καί + AuxZ → ADV.
    - R03Y: repeated-coordinator AuxY → CCONJ.
    - R04: AuxV → AUX.
    - R05C: εἰμί with an effective PNOM → AUX.
    - R07: COORD → CCONJ.
    - R08: AuxC over a verbal clause → SCONJ.
- **Conditional**, with candidate sets:
  - Names: R01C, R01M, R01A.
  - Particles: R03N, R03C, R03A, R03S, R03P.
  - R05X: εἰμί without a PNOM.
  - R08X: AuxC without a verbal clause.
- **Unresolved:** R09, a conjunction with any other relation.

**Effective dependents.** Look through COORD/APOS members (`_CO`/`_AP`) and AuxP/AuxC children, plus the non-member dependents shared by a COORD node whose member is the verb. This follows the Prague convention as implemented by HamleDT.

### Headword rules

- **H0:** the plain lemma becomes the headword unchanged. No accent folding, no repair.
- **H1:** for `P-L`, if L's Morpheus `:le:` entry carries `rpb:P` or `pb:P`, the headword is L.
- **H2:** if Morpheus `vbs.cmp` has a row `P-L# C` whose homograph number matches the v1.6 witness, the headword is C.
- **H3:** if exactly one `:le:` entry has `pb:P` plus L's own stem and class, that entry is the headword, with status conditional.
- **H4:** otherwise the headword is unresolved. Candidates are recorded and the raw lemma is kept.

### Discriminator rules

- **D0:** absent. The v1.6 lemma carries no number.
- **D1:** a number N is present in v1.6 and stripped in v2.1.
- **D2:** conflated. One v2.1 `key|UPOS` spans different v1.6 numbers, so the identity is conditional.

### Component conventions

- **C-ELISION:** v1.6 has U+02BC where v2.x has U+0313 at the same node. The mapping is supported.
- **C-CRASIS:** a consonant fragment `X-` followed by the host remainder. Attested practice, undocumented.
- **C-ENCLITIC:** `-X` placed before its host, observed only at s011. This contradicts the attested host-first order elsewhere.

## Not included

- No acute/grave folding.
- No dictionary-coverage gate.
- No proper-name policy: PROPN falls outside `AllowedPOS` as a repo fact.
- No crasis or elision expansion.
- No UD tokens or UD labels are used as Lysias evidence.
