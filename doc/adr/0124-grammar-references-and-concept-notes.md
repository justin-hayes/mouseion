# ADR 0124: Grammar references and Mouseion-authored concept notes

Status: **Accepted — implementation pending** · Date: 2026-11-24 · Author: Justin + OpenCode

Decided by Justin in
[Grilling: grammar link target and Mouseion-authored concept notes](https://github.com/justin-hayes/mouseion/issues/2605#issuecomment-7098653869),
under [Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597).
Applies [ADR 0111](0111-third-party-licence-posture.md) to grammar citations and
amends no earlier ADR. The feature is specified in
[Ancient Greek syntax and constructions](../features/ancient-greek-syntax-and-constructions.md).

## Context

When Sentence study names a construction, the learner needs to know what it is
and where to read more. While charting, Justin settled the shape: a short
Mouseion-authored definition, a Smyth § citation and an outbound link, with no
reproduced Smyth text.

The [grammar-reference research](https://github.com/justin-hayes/mouseion/issues/2601#issuecomment-7098264270)
found:

- Perseus Hopper serves Smyth's *A Greek Grammar for Colleges* in the **1920**
  edition. Every § from 1 to 3048 resolves from the § number alone.
- Most scholarship cites Messing's 1956 revision, which is still in copyright and
  renumbered some sections. A § recalled from it can open the wrong 1920 section.
- Perseus's licence instruments conflict (an open-source page says CC
  ShareAlike 3.0; the copyright page says personal use only and prefers links
  to copying). Linking is unaffected; copying text is ruled out.
- Hopper answers bare HTTP clients with 403 or a cookie redirect, so automated
  link checks would be flaky. Scaife has no grammar URNs.
- For Latin, Allen & Greenough at Dickinson College Commentaries (CC BY-SA,
  version unstated) has a per-§ URL; Perseus also has it.

## Decision

### One grammar reference per language, held as data

- A **grammar reference** record has an id, a citation label, an edition and one
  URL template filled from an integer §. The records are compiled data kept in
  one place.
- **Ancient Greek:** Smyth 1920 on Perseus Hopper,
  `https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={§}`.
  The on-host `text?doc=Smyth+grammar+{§}&fromdoc=Perseus:text:1999.04.0007`
  form and Alpheios `#s{§}` anchors are documented swaps.
- **Latin** (shape only; nothing is compiled now): Allen & Greenough at DCC,
  `https://dcc.dickinson.edu/grammar/latin/category-search?field_gl_section_number_value={§}`,
  cited "A&G §N", with Perseus as the documented swap.
- There is **no runtime fallback** and **no runtime or CI link checking**. If a
  host changes, one data row is edited.

### Citations

- The citation text, "Smyth 1920 §N" or "Smyth 1920 §§N–M", is always shown as
  plain text. The link is an addition to it; a range links to its first §.
- Each concept note has exactly one citation (a start § and optional end §).
- Every § is checked against the 1920 text, never the 1956 numbering. A
  construction without a citation does not ship.

### Concept notes

- A **concept note** is a learner-facing name, a definition of one or two
  plain-English sentences, and one citation. It has no example sentence: the
  learner's own sentence is the example.
- The wording is Mouseion's own. It is not written with Smyth's or the AGDT
  guidelines' prose as a template, and it describes the grammar concept, not the
  detection rule.
- Notes are **compiled reviewed data**: a Go literal keyed by stable
  construction id, following the provider-payload-assessment precedent. Not in
  the database, migrations or runtime config.
- The agent drafts notes on the implementation issue; Justin reviews and
  approves them there. Each note records its review ticket. A unit test asserts
  that every construction id the detector can emit has a reviewed note with a
  citation.
- Concept notes are **per language**. The Latin ablative absolute would be its
  own note, not shared with the Greek genitive absolute.
- The same review rule covers the other authored data the construction layer
  needs: the detector verb lists and the plain-English relation names. Relation
  names are labels only, with no definitions or links in the first slice.

### Notes are presentation, not evidence

Stored construction occurrences carry the construction id only. Name,
definition and citation are looked up at render. Editing a note needs no
Analysis update and no rebuild, and never touches frozen evidence
([ADR 0123](0123-constructions-are-a-derived-reading.md)).

### Licence and link behaviour

- A § citation and an outbound link reproduce nothing, so there is **no
  register row and no notice**.
- A row would be needed if Mouseion showed any Smyth or A&G wording (headings
  included), or shipped a § → URL map derived from a host's files.
- The link opens externally with `rel="noopener noreferrer"`, so the grammar
  host never receives tailnet URLs.

## Alternatives considered

- **Runtime or CI link checking with a fallback host:** rejected. Hopper refuses
  bots, the app is tailnet-only, and one data row is cheaper to edit.
- **Citing Smyth without a link:** rejected; the link is the cheap path to the
  full treatment.
- **Example sentences in notes:** rejected; the learner's sentence is the
  example, and authored Greek examples would need their own review.
- **Notes in the database or runtime config:** rejected; they need review, not
  operator editing, and a test can enforce coverage.
- **Shared cross-language notes:** rejected; Greek and Latin constructions
  differ in detail and cite different grammars.

## Consequences

- The first slice ships six reviewed concept notes and one Greek grammar-reference
  record.
- A Hopper change breaks links until a row is edited; the citation text stays
  correct.
- Latin can add a record and its own notes without changing the shape.

## Related

- [ADR 0111](0111-third-party-licence-posture.md): the licence posture applied
  here.
- [ADR 0123](0123-constructions-are-a-derived-reading.md): constructions carry
  ids only.
- [ADR 0110](0110-philological-reading-competence-boundary.md): reference-grammar
  links as direction.
