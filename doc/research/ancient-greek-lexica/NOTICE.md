# Attribution, rights and modification notice

This directory holds derived counts, scripts, and hashes. It contains no copies
of the source texts or dictionary entries. The Kaikki, LSJ, Middle Liddell, and
OGA files were fetched into scratch space and are not committed. Entry fragments
in the research file are short quotations used to show structure.

Nothing here is a licence clearance for the app or for any deck. Each row gives the
licence the source states, where it was read, and the status for this research.
Print-edition status and digital-text rights are separate rows.

## Source by source

### Wiktionary text, via Kaikki/Wiktextract (`grc`)

- Wiktionary text: the WMF Terms of Use, section "Licensing of Content"
  (`https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use`), says contributed text
  is licensed under CC BY-SA 4.0 and GFDL (unversioned), and "Reusers may comply with
  either license or both." Attribution by hyperlink or URL to the page history.
- Kaikki raw-data page (`https://kaikki.org/dictionary/rawdata.html`): gives no licence
  for the extract. It asks academic users to cite Tatu Ylonen's Wiktextract work.
- Wiktextract code: MIT (`tatuylonen/wiktextract` LICENSE), with a carve-out for files
  under `tests/`, which are Wiktionary-licensed.
- Quoted third-party translations: 36 Anabasis example objects in the `grc` extract
  contain "Anabasis" (35 in the `Xenophon, Anabasis` form, one in a different form).
  Of the 35, 27 carry a real translation, 6 carry a placeholder ("please add an English
  translation"), and 2 name a translator in the text (Dakyns; Carleton L. Brownson). Book 1
  has 17 examples: 13 real translations, 2 placeholders, 1 Brownson-attributed. The rights
  in quoted translations are not established by the Wiktionary licence. Excluded from
  shipped output until checked.
- Status for this research: candidate for lookup and attribution. Quoted translations
  excluded.

### LSJ, Perseus `pdllex/grc/lsj` (commit `56061ca1`)

- Print edition: Liddell, Scott, Jones and McKenzie, *A Greek-English Lexicon*, Oxford:
  Clarendon Press, 1940 (ninth edition). The TEI header of
  `grc.lsj.perseus-eng1.xml` gives ISBN 0198642261.
- Component licence: `lsj/README.md` in the pinned repo is a component-specific statement.
  It says the text is "provided under a CC BY-SA license by Perseus Digital Library,
  http://www.perseus.tufts.edu, with funding from The National Endowment for the
  Humanities", and asks that the data be cited as "Data accessed from
  https://github.com/PerseusDL/lexica/ [date of access]". The repo-level `README.md`
  also says CC BY-SA 4.0 and must be offered back as modifications.
- Tufts wording: the repo-level README says "Tufts University holds the overall copyright
  to the Perseus Digital Library; the materials therein … are provided for the personal use
  of students, scholars, and the public." That is general wording and is not reconciled
  with the component statement.
- Print-edition public-domain status, separate from the digitisation:
  - Henry Stuart Jones (1867–1939) is confirmed (`en.wikipedia.org/wiki/Henry_Stuart_Jones`).
  - Roderick McKenzie's death date was not confirmed. The search result for "Roderick
    McKenzie" that turned up died 1940 and is a sociologist with a different career. Do
    not conflate the two. The LSJ co-author's dates are unverified.
  - The EU term is life plus 70 years from the death of the last-dying co-author. Jones
    died in 1939, so the term runs at least to 2009. The print is public domain in the EU
    from 2026 only if McKenzie died before 1956. The US position depends on renewal
    records not checked here.
  - Result: unresolved in both jurisdictions.
- Status for this research: candidate for personal and research use. Excluded from shipped
  decks until the print status and the Tufts clause are settled.

### Middle Liddell, Perseus `1999.04.0058` (*An Intermediate Greek-English Lexicon*)

- Print edition: Liddell and Scott, Oxford: Clarendon Press, 1889. ISBN 0199102066 and
  HathiTrust `hvd.32044041126962` (per the MODS record).
- Authors' dates: Henry George Liddell (1811–1898) is confirmed from
  `en.wikipedia.org/wiki/Henry_George_Liddell`. Robert Scott (1811–1887) is confirmed
  by the Rochester Cathedral dean page (`rochestercathedral.org/forc/deanscott`) and
  the MODS authority record.
- Print status: the last-dying co-author is Liddell (1898), so the EU term ran to the
  end of 1968 and the print has been in the public domain there since 1969. In the US,
  an 1889 publication is public domain. The print is not the issue.
- Digital text, TEI header (`dl/ml/ml-blinskey.xml`, fork `blinskey/middle-liddell@5db8d88a`):
  `<availability status="free">`, with conditions to credit Perseus, keep the statement
  intact, use it "for non-commercial purposes only", and "offer Perseus any modifications
  you make."
- Digital text, Perseus hopper page (`https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0058`):
  "licensed under a Creative Commons Attribution-ShareAlike 3.0 United States License",
  with the Perseus-modifications restriction.
- Fork `LICENSE.md` (`blinskey/middle-liddell@5db8d88a`): CC BY-SA 3.0 US with the
  Perseus-modifications restriction. The second copy (`adiel-mittmann/middle-liddell@b492bdf2`)
  has no LICENSE file in its root listing.
- Conflict: the TEI header says non-commercial. The hopper and fork say CC BY-SA 3.0 US.
  Not resolved here.
- Status for this research: excluded from shipped decks until Perseus says which
  statement governs the XML. If it is CC BY-SA 3.0 US, adaptations may be relicensed under
  BY-SA 4.0 (see deck scope, point 3), but that does not resolve the conflict.

### DGE, Diccionario Griego-Español (CSIC)

- Licence (DGE's licence page, `http://dge.cchs.csic.es/xdge/doc/licencia.html`, fetched
  2026-10-17): "distribuidos bajo la licencia Creative Commons: 'Reconocimiento - No
  Comercial - Compartir Igual; 3.0 España'". The non-commercial term applies.
- Machine-readable access: `http://dge.cchs.csic.es/xdge/xml/` returned HTTP 401. No open
  XML download was verified. The HTTPS host failed to connect from this environment.
- Status: excluded. Non-commercial, and no verified open machine-readable copy.

### Bailly (Dictionnaire grec-français, 1895; 2020 digital edition)

- Print edition: Anatole Bailly, Hachette, first edition 1895. Bailly's dates are
  16 December 1833 – 12 December 1911 (`en.wikipedia.org/wiki/Anatole_Bailly` and
  `fr.wikipedia.org/wiki/Anatole_Bailly`). The earlier draft's 1916 death year is wrong
  and is withdrawn.
- Digital derivative: `https://latin-dict.github.io/dictionaries/Bailly2020.html` says
  "licensed under a Attribution-NonCommercial-NoDerivatives 4.0 International License."
  The Gréco author page gives no licence line in the fetched text.
- Status: excluded. The derivative is NC and ND, whatever the print status.

### Scaife / OpenGreekAndLatin 19th-century lexica

- `https://scaife.perseus.org/library/` fetched as a shell page with no lexicon list.
  `github.com/OpenGreekAndLatin/lexica` returned 404 through the GitHub API.
- Status: not assessed. No claim is made about these texts.

### Logeion

- `https://logeion.uchicago.edu/` returned a JavaScript shell. Ownership and licence were
  not retrieved.
- Status: not assessed.

### Open Greek and Latin OGA `oga_src` v0.2.0 (Zenodo 10.5281/zenodo.15199546)

- `license.md` in the zip: the OGA corpus and this repository's CoNLL-U files are archived
  under CC BY-SA 4.0. The README says the data derives from Perseus/canonical-greekLit and
  First1KGreek and follows the AGDT annotation scheme.
- The Anabasis file's SHA-256 is pinned in `internal/ancientgreek/ancientgreek.go:23-39`.
- Status: candidate for the identity and POS import. The `ref=Xen. Anab._…` MISC labels
  are OGA's own labels, not a separate source.

### Pape, Passow, Autenrieth, Cunliffe, Lewis & Short

- Not assessed in this pass. Autenrieth and Cunliffe are Homeric, and Lewis & Short is
  Latin. Passow (1786–1833) is confirmed by HathiTrust authority data, but the later
  editors' dates were not, and Pape's dates were not checked. No licence is claimed.

## Deck licence scope

These statements apply to the data in the research and to output derived from it. They
are not a clearance for the app.

1. **Personal use.** Reading material in one's own account is not redistribution, and
   attribution is not a condition of private use. Non-commercial statements (Middle
   Liddell TEI, DGE, Bailly 2020) still bar commercial use. Those sources are excluded
   from shipping.
2. **Redistributed excerpts in decks.** An excerpt that is shipped or exported must
   credit the author or editor names, the title, the source URI, the licence name and
   a link to the licence text, and state that it was changed. For CC BY-SA 4.0 material,
   adaptations must be licensed under CC BY-SA 4.0 or a later version. For Wiktionary
   text, the WMF terms allow CC BY-SA 4.0 or GFDL, with a hyperlink or URL credit.
3. **BY-SA 3.0 to 4.0.** Creative Commons' compatibility list says adaptations of
   BY-SA 3.0 material may be licensed under BY-SA 4.0. The reverse is not allowed. This
   applies only to adaptations. Unadapted excerpts keep their original licence.
4. **Data versus the app.** The app's own LICENSE at `6e02d180` is GNU AGPL v3
   (`git show 6e02d180:LICENSE`). It covers the code and not the dictionary data.
   `internal/dictionary/attribution.go` says CC BY-SA 4.0 / GFDL for Wiktionary data and
   CC BY-SA 4.0 for OGA. Those are attribution strings, not a clearance.
5. **Unresolved status is excluded.** LSJ print and Tufts wording, Middle Liddell digital
   text, Scaife and Logeion are not used in shipped decks.

## Modifications

- The scripts in `scripts/` are written for this research and read sources read-only.
- `summaries/` contains counts, tier counts and hashes only. It holds no headword-level
  excerpts from LSJ, Middle Liddell, or Kaikki text.
- The research file quotes short fragments (single glosses and references) to show
  structure. These are not redistributed beyond the research file.
