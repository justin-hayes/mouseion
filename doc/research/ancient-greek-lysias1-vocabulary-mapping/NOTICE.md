# Notice: sources, licences, and what is committed

Research for justin-hayes/mouseion#2049. Mapping rules for Lysias I (AGDT `tlg0540.tlg001.perseus-grc1`).

## Committed here

| Path | Derived from | Licence / handling |
| --- | --- | --- |
| `manifest/lysias1-nodes.tsv`, `lysias1-identities.tsv`, `lysias1-components.tsv`, `fixtures/lysias1-expected.tsv` | AGDT Lysias I fields at `treebank_data@bf4334f0`: v2.1 file plus the v1.6 file at the same commit. Derived columns added. | Root `treebank_data` README declares CC BY-SA 3.0 US. These files are derivatives: keep attribution and share-alike. The v1 docs README (AGDT 1.7, CC BY-NC-SA 2.5) is cited only for its tagset table. Whether that licence applies to the v1.6 Lysias file is not settled here; see #2048. |
| `manifest/lysias1-lineage.json`, `annotation-practice.json` | Header fields and counts from the AGDT files | Same as above. Counts and names only. |
| `manifest/lysias1-rules.tsv`, `lysias1-evidence.tsv`, `lysias1-summary.json` | Derived | Repo. The evidence table quotes short locator strings from the sources below. |
| `manifest/ud-crosscheck.json` | Aggregate UPOS counts from UD_Ancient_Greek-Perseus `331ddef9`, joined to AGDT v2.1 | UD Perseus is CC BY-NC-SA 2.5. Only counts are committed: no UD text, token lines or per-token fields. |
| `scripts/`, `mapping-spec.md`, `report.md` | Written for this research | Repo |

## Read as evidence, not committed

These sources are fetched by `scripts/fetch_inputs.sh` at pinned commits and verified by sha256. They are cited by line. None is redistributed or executed.

- **Morpheus** (`perseids-tools/morpheus@ab6898ff`, MPL-2.0): stemlib `vbs.simp.ml`, `lsj.vbs`, `vbs.irreg`, `vbs.cmp.ml`, `vbs.cmp.lsj`, `nom.proper`, `lemlist`, `entitylist.txt`, and the C sources `prntanal.c`, `checkpreverb.c`, `lemma_voice.c`, `morphkeys.c`, `indexstems.c`. The analyzer was not built or run.
- **UD documentation** (`UniversalDependencies/docs@24add7d0`) and **UD validator data** (`UniversalDependencies/tools@77751a63`, `data/data.json`).
- **HamleDT/Treex** (`ufal/treex@df815f19`, GPL as declared in the files) and **Interset** (`dan-zeman/interset@f3568ffb`). Converter code was read, not imported or run.
- **AGDT** (`PerseusDL/treebank_data@bf4334f0`): AGDT guidelines (1.1 PDF, 2.0 and 2.5 markdown), READMEs, TAGSETS, and the Lysias I v1.6 and v2.0 files.
- **Canonical TEI and UD treebank:** `PerseusDL/canonical-greekLit@01b725d8` and `UD_Ancient_Greek-Perseus@331ddef9`.
- **Smyth, Greek Grammar §2769:** read once on the Perseus website on 2026-10-14. Not pinned and not used by code.

No provider, LLM, NLP service, paid or gated source, or account was used. No person was contacted. No restricted or interrupted access was retried.
