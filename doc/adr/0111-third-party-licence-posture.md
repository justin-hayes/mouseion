# ADR 0111: One licence posture for third-party data in every language

Status: **Accepted — posture; implemented** · Date: 2026-10-26 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided in
[#2308](https://github.com/justin-hayes/mouseion/issues/2308). The deployer
register ([`doc/third-party-data.md`](../third-party-data.md)), provisioning
pointer and deck notice it requires were implemented in
[#2312](https://github.com/justin-hayes/mouseion/issues/2312). Amends the context of
[ADR 0099](0099-ancient-greek-imported-analyses.md) and restates the rights
footing of [ADR 0073](0073-modern-greek-language-support.md) and
[ADR 0102](0102-french-language-support.md) as a general rule.

## Context

Two licence rules coexisted. Ancient Greek admitted no non-commercial (NC) input
of any kind, even downloaded at deploy time (#2089, #2128 and ADR 0099's "no
acceptable Ancient Greek model"). The modern languages tolerated NC training
data because models are downloaded at deploy time, never redistributed, and used
for personal self-hosted purposes (ADR 0073, ADR 0102). No principle explained
the difference; ADR 0110 lists the reversal between #2089 and #2128 as a symptom
of the Greek MVP's drift.

The provenance research for
[#2307](https://github.com/justin-hayes/mouseion/issues/2307)
([findings at `b4568038`](https://github.com/justin-hayes/mouseion/blob/b4568038956f50069f225cfab3005e92cc553f8c/doc/research/third-party-licence-provenance.md))
found that the NC term is not confined to the Italian and Greek treebanks. The
conll17 word embeddings (LINDAT `11234/1-1989`, CC BY-NC-SA 4.0) sit under the
`pos` and `depparse` processors of every warmed Stanza package, German included.
A strict no-NC rule therefore fails `de`, `it`, `el` and `fr` alike.

A follow-up feasibility check for this decision (UD `LICENSE.txt` and
`stats.xml` files, spaCy, UDPipe, LatinCy and odyCy model cards, fetched
2026-10-26) found no off-the-shelf non-NC pipeline for any study language,
except a French spaCy candidate with unverified inputs. UDPipe models are
CC BY-NC-SA throughout. Retraining Stanza on non-NC treebanks with fastText
vectors (CC BY-SA 3.0) in place of conll17 is possible, but the non-NC data is
thin where it matters: about 114k modern Italian tokens against ISDT and VIT's
538k, 25k Modern Greek tokens (GUD) against GDT's 62k, and 39k Ancient Greek
tokens (PTNK). Stanza's own historical numbers put GUD-only Greek at lemma 92.6
against GDT's 96.1. Mouseion's vocabulary, coverage and cards are lemma-driven.

## Decision

One posture applies to every study language, including Latin when it arrives.
It classifies each third-party **layer**, not each artifact, as an **Analysis
resource** or **Source content** (`CONTEXT.md`). One artifact can hold both:
the `oga_src` file carries Greek text (Source content) and AGDT-derived lemmas
and parts of speech (Analysis resource); a Kaikki entry's definitions are
Source content, while Stanza's Wiktionary-composed German lemma table is an
Analysis resource.

### Restriction kinds

Terms are read from the licence instrument actually in force, not from README
prose. "Research only" and "academic use" are their own restriction kind,
stricter than NC, because personal language study is not research. UD Italian
ISDT, whose `LICENSE.txt` is CC BY-NC-SA 3.0, is NC despite its README's
"research purposes" wording. A layer whose only terms are research or academic
use counts as unclear.

### Source content

Source content must permit redistribution and be no more restrictive than
share-alike: public domain, CC0, CC BY, CC BY-SA or GFDL. NC, research-only,
unlicensed and unclear Source content is excluded in every language and on
every delivery path. Today this admits Wiktionary through Kaikki/Wiktextract,
LSJ, the `oga_src` text, the Brownson translation and the GLTP graded readers,
and excludes the Exocortex digitisation, LGPSI and EasyGreekReaders.

### Analysis resources

An NC, unclear or research-only Analysis resource is allowed when all of these
hold:

1. the deployer fetches it at deploy time (`nlp-init`) or the operator supplies
   it; a runtime in-app download would qualify on the same terms, though none
   exists;
2. it never ships in the repository, an image or any published release
   artifact, so models are never baked into a published container image;
3. the deployment serves the deployer's personal, self-hosted, non-commercial
   purposes, which is the deployer's responsibility;
4. it is listed in the deployer register.

Stanza `grc` and Stanza `la` are therefore eligible on licence grounds; whether
they are good enough is a separate question. The `oga_src` annotations are
acceptable whatever the AGDT lineage behind them turns out to be.

### Exported decks

A deck may carry Analysis-resource output, such as the `Lemma` and `POS`
fields. It carries Source content only under the rule above, with the existing
attribution notices. A deck whose analysis came from the NLP service also
carries a generic notice that its annotations come from deployer-provisioned
models with their own terms, pointing to the register; an imported Ancient
Greek deck already carries `AncientGreekSourceNotice` instead. Mouseion takes no
responsibility for a learner's choice to share a deck.

### Deployer register

`doc/third-party-data.md` lists every third-party layer: its kind, terms,
restriction kind, delivery path, whether it reaches a deck, and any NC or
unverified flag. It states that the default warm set `de,it,el,fr` provisions
NC-derived models and that the AGPL covers Mouseion's code, not these
resources. The README licence section and `doc/operations.md` link to it, and
provisioning logs a one-line pointer. There is no acknowledgement gate.

Adding a third-party layer classifies it and records it in the register in the
same change, with a deck notice when it is Source content that reaches decks.
Reviewers check the register; no CI check enforces it.

## Consequences

- No shipped model, analysis or deck changes. The four warmed languages keep
  their Stanza packages.
- ADR 0099's premise that no Ancient Greek model is acceptable no longer holds.
  Its import path, pinned source and `absent` syntax stand on their own merits.
  The OGA interpretation tables (`oga-agdt-upos/4`, `oga-agdt-feats/1`) stay as
  written from tagset documentation; later interpretations need not follow the
  #2089 no-treebank rule.
- ADR 0073's and ADR 0102's rights sections are instances of this posture.
  ADR 0102's statement that redistribution and public or commercial use are
  unsupported stands: Mouseion offers no configuration free of NC resources,
  and a commercial deployer must replace them, guided by the register.
- Unverified Analysis-resource terms (the `newswiki` character models, the
  Tiger-sourced sentences in UD German-GSD, GreekBERT's training corpora) do not
  block anything; the register marks them unverified.
- LLM provider output, such as Glosses and translations, is outside this
  posture and governed by each provider's terms. It remains an open question.
- This is a product posture, not legal advice. Its weakest assumption is that
  another person's self-hosted deployment is non-commercial; the register is
  what carries that responsibility to the deployer.

## Alternatives considered

- **No NC anywhere.** Rejected: no off-the-shelf pipeline qualifies, so
  Mouseion would train, evaluate, host and version its own models for every
  language, and accept clearly weaker lemmas for Italian, Modern Greek and
  Ancient Greek. Revisit if a non-NC pipeline of comparable quality appears.
- **A split rule by language.** Rejected: no principle distinguishes Ancient
  Greek from the modern languages. The line that matters is whether a third
  party's expression reaches the learner, and that holds in every language.
- **A commercial-safe configuration.** Rejected for now; it is the cost of the
  first alternative under another name.
