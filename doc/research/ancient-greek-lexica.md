# Open Ancient Greek lexica for contextual word sense (Anabasis Book 1)

Research for #2133, feeding prototype #2130 under map #2127. This is a source
and licence survey plus a lookup-coverage measurement. It is not contextual-sense
evaluation, not gold annotation, and not a claim that any source is accurate
enough for contextual glosses. No production code or contract changed.

Inputs, pins, hashes, and rights are in [ancient-greek-lexica/SOURCES.md](ancient-greek-lexica/SOURCES.md)
and [ancient-greek-lexica/NOTICE.md](ancient-greek-lexica/NOTICE.md). Scripts are
in `ancient-greek-lexica/scripts/`; the measurement output is in
`ancient-greek-lexica/summaries/`.

## Recommendation

1. **Use LSJ (Perseus `pdllex/grc/lsj`) as the primary candidate for attributable,
   structured citation evidence.** It is the only candidate here that carries
   machine-readable Xenophon passage references (`urn:cts:greekLit:tlg0032.tlg006…:1:…`)
   that can be counted for Book 1 (410 of 1,428 identities, 585 citations). Its
   licence is CC BY-SA 4.0 per the repo README, but the underlying 1940 print's
   status is disclosed as unresolved below. Pin: `PerseusDL/lexica@56061ca1`.
2. **Use English Wiktionary via Kaikki/Wiktextract (`grc`) as the candidate for
   part-of-speech and structured gloss data**, not for homograph-safe sense
   identity. Licence CC BY-SA 4.0 / GFDL. Its Book 1 Xenophon examples are sparse
   (17 identities). Its homograph separation is weak (156 Book 1 headwords have
   more than one Kaikki entry, and most lack an etymology number). The download
   is not pinned upstream and must be mirrored with a hash.
3. **Do not use Middle Liddell (Perseus 1999.04.0058) in shipped decks until its
   licence conflict is resolved.** The Perseus availability statement restricts
   use to non-commercial purposes. The fork's `LICENSE.md` says CC BY-SA 3.0 US
   without that restriction. Its Xenophon references are unstructured ("Xen.") so
   Book 1 cannot be measured.
4. **Not assessed in this pass:** DGE, Bailly, Pape, Passow, Logeion (a wrapper
   over LSJ/Middle Liddell), and other open lexica. Autenrieth and Cunliffe are
   Homeric and Lewis & Short is Latin, so they are out of scope for Attic prose.
   Their licences are unverified here; none is claimed.

The Morpheus stem source is unchanged. OGA `morpheus:N` keys are not verified
against LSJ keys one by one. Digit overlap is not evidence of the same homograph
(see below).

## Source comparison

| Source | Pin | Format | Licence (as stated by the source) | Xenophon Book 1 structured? | Homograph key |
|---|---|---|---|---|---|
| LSJ, Perseus `pdllex/grc/lsj` | `PerseusDL/lexica@56061ca1` (`56061ca127f4a2844980baffc5f2b6d1332897b3`) | TEI P4 XML, 28 files, 111,895 main entries | CC BY-SA 4.0 (repo README/licence); Tufts "personal use" wording in the same README | Yes, URN `tlg0032.tlg006…:1:c:s` | beta-code key with trailing digit |
| Kaikki / Wiktextract `grc` | `kaikki.org/dictionary/Ancient Greek/kaikki.org-dictionary-AncientGreek.jsonl`, last-modified 2026-10-03 (not an upstream commit pin) | JSONL, 68,196 entries | CC BY-SA 4.0 / GFDL (Wiktionary); Kaikki data derived | Partial: `examples[].ref` text only, 33 Anabasis refs, 16 Book 1 | `etymology_number` (sparse, 1,345 entries) |
| Middle Liddell, Perseus 1999.04.0058 | `blinskey/middle-liddell@5db8d88a` (fork); `adiel-mittmann/middle-liddell@b492bdf2` (unlicensed copy) | TEI P4 XML, 30,440 entries | **Conflict**: TEI availability is non-commercial; fork `LICENSE.md` is CC BY-SA 3.0 US; adiel copy has no licence | No: `Xen.` without book/chapter | beta-code key with trailing digit |
| Morpheus (OGA input) | `PerseusDL/morpheus@b1b33c56` (from the earlier proof) | Stem files | CC BY-SA 3.0 US (from the earlier proof's NOTICE; not re-fetched this pass) | N/A | `morpheus:N` source |

## Measurement

Denominator: the pinned OGA file's own `Xen. Anab._1.*` labels, 10,572 printed
native nodes. 110 empty nodes (POS `-`) are excluded. Identities are counted as
`(canonical lemma, trailing digit or none, coarse class)`. The canonical form is
NFC, simple lowercase, final sigma folded to σ, NFC, with diacritics kept
(ADR 0099 identity rule). The class is a letter map (n/a/d/v → NOUN/ADJ/ADV/VERB),
and εἰμί is excluded by string match. Excluded letters are c, u, l, r, p, i, and
-. Proper names are not separated in the headline counts.

| Measure | Result |
|---|---|
| Book 1 printed tokens (vocabulary classes) | 10,572 matched; 6,014 occurrences after excluding εἰμί (ADR counts 6,017) |
| Identities (heuristic) | 1,428 (21 numbered, 1,407 plain). The count matches the proof's 1,428 but the rules are not identical |
| Kaikki headword has any entry | 1,185 / 1,428 = 83.0% (plain only 1,165 / 1,407 = 82.8%) |
| Kaikki headword has POS-compatible entry (heuristic class map) | 1,062 / 1,428 = 74.4% |
| LSJ headword has any entry (any homograph key) | 1,304 / 1,428 = 91.3% |
| LSJ plain key exactly present (no digit) | 1,195 / 1,407 = 84.9% |
| Middle Liddell any entry | 1,094 / 1,428 = 76.6% |
| Middle Liddell plain key exactly present | 1,033 / 1,407 = 73.4% |
| Token-weighted "any entry" (Kaikki / LSJ / ML) | 94.9% / 95.0% / 88.2% (6,014 occurrences) |
| Identities with Book 1 Xenophon Anabasis citations (Kaikki / LSJ / ML) | 17 / 410 / 0 (ML unmeasurable) |
| Capitalised-only (name-like) identities, any entry (Kaikki / LSJ) | 112 of 152 / 71 of 152 |

Reading these numbers: they measure headword and key existence, not sense
coverage. "Any entry" for a numbered OGA identity means any homograph of that
headword. That is a conflation, and the plain-only figures are the more
conservative ones. The token-weighted figures are high because frequent words are
well covered. They say nothing about sense resolution.

## Limitations and disclosed conflations

- **Heuristic identity and POS.** The class map is a reimplementation of the
  letter rule for this measurement, not the ADR 0099 `oga-agdt-upos/4` mapping. The
  identity count and the excluded-token total are close to the proof's figures but
  not identical. Treat them as heuristic.
- **Beta-code conversion.** The LSJ converter leaves 311 keys with unrecognised
  characters (`^`, `&`, `;`, `[`, `]`, `_`, `'`), which are editorial markers and
  entities. Middle Liddell has 18. Those keys may fail to match. The effect is
  not measured.
- **Homograph numbers are not comparable across sources.** OGA's digits come from
  Morpheus. Digit-equal matches in LSJ or Middle Liddell are coincidental in the
  checks made. Example: `λέγω3` is absent from LSJ digits 1 and 2 but present in ML
  digits 1-3. Numbered identities are therefore reported separately and are not a
  match criterion.
- **Kaikki homographs.** `λέγω` has two verb entries with no etymology number, so
  any sense-level claim for it is a conflation. Most Kaikki entries have no
  `etymology_number`.
- **Kaikki Book 1 examples.** Only 33 Anabasis refs in the whole file, 16 in
  Book 1 (17 identities). Of 36 with translation text, 35 have no attribution line
  and one cites H. G. Dakyns. The provenance of the unattributed English is not
  known and should not be used for deck English without checking.
- **Wiktionary-quoted translations.** Wiktionary example sentences can quote
  third-party translations under their own copyright. Kaikki inherits the
  Wiktionary licence, not the rights of quoted translators. This needs a rule before
  examples are shipped.
- **LSJ Book 1 citations are presence, not sense.** The 585 Book 1 citations are
  counted from URN text in `<bibl>` elements. They are not checked against the
  Marchant 1904 text, and LSJ sense numbering is not mapped to Kaikki or OGA
  identities.
- **Middle Liddell cannot be measured for Book 1** because its Xenophon refs have
  no book or chapter. The "0" for ML is "unmeasurable", not "no evidence".
- **Pins.** Kaikki is a live download, not an upstream commit. The audit reported
  that the production derivation uses a mutable `raw-wiktextract-data.jsonl.gz`,
  and that the installed dump's extraction date is not recorded. This pass does not
  establish which dump is installed.
- **Not run:** no contextual-sense evaluation; no human gold; no DGE/Bailly/Pape/
  Passow fetch.

## Executable truths (local audit at 6e02d180, reported by the parent)

The parent's local audit reported the following. I re-checked the three code
claims by grep and they hold at these lines. The rest is taken from the audit.

- `nlp/scripts/derive_dictionary_index.py:324` stores `glosses[0]` as the Gloss.
  Nested gloss breadcrumbs that end in more specific senses are dropped.
  (Verified by grep.)
- Examples keep text only. Structured author, ref, and translation are dropped.
  Topics, Tags, Phrase, Gender, Article, Plural, and IPA are retained.
- Greek rows (`:454-505`) keep source row boundaries, `etymology_number`, and NFC
  forms. The forms discard tags, and the SQL table has no source row ID.
- `internal/dictionary/greek_index.go:50` selects only `headword, upos,
  senses_json`. It does not select `etymology_number` or `forms`. The lookup at
  `:112-131` combines rows into flat senses by lemma and UPOS, and ignores the
  discriminator. (Line 50 verified by grep.)
- `internal/enrichment/lexical.go` has no discriminator or morphology in the
  request struct.
- `internal/dictionary/index.go:208-267` deduplicates by hash of (language, lemma,
  UPOS, Gloss, Phrase, Examples, Topics, Tags), not by source entry or etymology
  ID. Retained storage and flattened lookup identities are different things, and
  the current code does **not** provide homograph-safe sense provenance.
- `OrderSenses` (`lexical.go:87-173`) is a Lesk-style overlap and tie ordering, not
  source-based verified sense resolution. (Verified by grep.)
- `internal/dictionary/attribution.go` says CC BY-SA 4.0 / GFDL. That is an
  attribution string, not proof of licence.
- `internal/ancientgreek/ancientgreek.go:23-39` pins the OGA file at 7,438,037 bytes,
  SHA-256 `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61`,
  covering 1.1.1–1.10.19. This pass re-extracted and matched that size and hash from
  the Zenodo zip.
- `cmd/greekhitrate` reports against the installed Browse projection across all
  current analyses. It is not inherently a Book 1 denominator. This research's
  measured local-file coverage must not be confused with that report.

## Documentation conflicts

- ADR 0064 and ADR 0079 cite Wiktionary as CC BY-SA 3.0. The current Wiktionary
  statement and `attribution.go` say CC BY-SA 4.0 / GFDL. The ADRs look stale. Not
  changed here.
- The Perseus LSJ README says Tufts holds overall copyright and materials are
  "provided for the personal use of students, scholars, and the public", while
  also saying the repo is CC BY-SA 4.0. The parent should decide whether that
  requires per-component clearance before LSJ text or definitions are redistributed
  in decks.

## Follow-ups

1. Resolve the Middle Liddell licence conflict with Perseus, or exclude it.
2. Decide whether LSJ's Tufts personal-use wording blocks redistribution. Get
   clearance or an explicit statement.
3. Mirror a dated Kaikki/Wiktextract `grc` extract with a SHA-256 pin before any
   production use. Record the dump date and extractor commit.
4. Map OGA identities to LSJ and Kaikki with the ADR 0099 class rules, not this
   heuristic, and report sense-level coverage only after a source-level homograph
   rule is agreed.
5. Assess DGE and Bailly, and state their licences, before ruling them in or out.
6. Fix the stale CC BY-SA 3.0 references in ADR 0064 and ADR 0079 in a separate
   change.
