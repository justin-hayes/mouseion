# Attribution, rights and modification notice

This directory holds derived measurements and scripts only. It contains no
copies of the source texts or dictionary entries. The Kaikki, LSJ, Middle
Liddell, and OGA files were fetched into scratch space and are not committed.

## Sources and licences (as stated by each source)

- **Kaikki.org / Wiktextract (English Wiktionary, `grc`).** Text is derived from
  Wiktionary under CC BY-SA 4.0 and GFDL. Derived data remains share-alike. Quoted
  third-party translations in example sentences may carry separate copyright that
  Wiktionary's licence does not cover.
- **Perseus Digital Library, LSJ (`PerseusDL/lexica`, `pdllex/grc/lsj`).** The repo
  licence is CC BY-SA 4.0. The README says Tufts holds overall copyright and that
  materials are provided for personal use, with varying copyright status per
  component. Per-component status is unresolved here.
- **Perseus, Middle Liddell (`Perseus 1999.04.0058`, fork `blinskey/middle-liddell`).**
  The TEI availability statement restricts use to non-commercial purposes and
  requires offering Perseus modifications. The fork's `LICENSE.md` says CC BY-SA
  3.0 US without the non-commercial term. This conflict is unresolved, and
  Middle Liddell must not be used in shipped decks until it is.
- **Morpheus (`PerseusDL/morpheus`).** CC BY-SA 3.0 US, per the earlier OGA proof's
  NOTICE. Not re-fetched in this pass.
- **Open Greek Annotations (OGA) v0.2.0, `mgbilby/oga_src` (Zenodo DOI
  10.5281/zenodo.15199546).** CC BY-SA 4.0, traced to Perseus/canonical-greekLit
  (CC BY-SA 4.0). Only the file's identity, POS letters, and `Xen. Anab._1.*`
  labels were used.

## Modifications

- Measurement scripts (`scripts/`) are written for this research. They parse the
  sources read-only.
- `summaries/` contains aggregate counts and a hash list. They contain no
  headword-level excerpts from LSJ, Middle Liddell, or Kaikki text.

Per the parent's instruction, license conflicts are disclosed rather than
resolved. Nothing here is a licence clearance.
