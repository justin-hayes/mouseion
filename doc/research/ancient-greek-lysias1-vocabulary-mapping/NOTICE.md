# Notice: sources, licences, and what is committed

Research for justin-hayes/mouseion#2049. Mapping rules for Lysias I (AGDT `tlg0540.tlg001.perseus-grc1`).

## Committed here

| Path | Derived from | Licence / handling |
| --- | --- | --- |
| `manifest/lysias1-nodes.tsv` | AGDT Lysias I node fields (form, lemma, postag, relation, head, artificial) plus derived columns | AGDT data is CC BY-SA 3.0 US (`PerseusDL/treebank_data` README). This file is a derivative; keep attribution and share-alike. |
| `manifest/lysias1-identities.tsv` | Aggregated from the node table | Same as above |
| `manifest/annotation-practice.json` | Native AGDT v2.1 fields for all 33 Greek texts (counts only) | CC BY-SA 3.0 US; counts only |
| `manifest/lysias1-rules.tsv`, `lysias1-summary.json` | Derived | Repo |
| `manifest/ud-crosscheck.json` | Aggregate UPOS counts from UD_Ancient_Greek-Perseus, joined to AGDT v2.1 (counts only) | UD Perseus is CC BY-NC-SA 2.5. Only counts are committed; no UD text, token lines, or per-token UD fields. |
| `scripts/` | Written for this research | Repo |

## Not committed

- Raw AGDT texts, the canonical-greekLit TEI, the UD `.conllu` files, and the guideline PDFs. `scripts/fetch_inputs.sh` fetches them at pinned commits and verifies them.

## Pinned inputs

- `PerseusDL/treebank_data` commit `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`
  - Lysias I `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml`, sha256 `44ecf3a2…a568`
  - AGDT 1.1 guideline PDF `v1/greek/docs/guidelines.pdf`, git blob `49972e555853abaf98d7ca61f049875b0c0a4779`
- `PerseusDL/canonical-greekLit` commit `01b725d835e6e733062ffd79e0efdbae1ba06e5c`, `tlg0540.tlg001.perseus-grc2.xml`, sha256 `21dbae42…d6a`
- `UniversalDependencies/UD_Ancient_Greek-Perseus` commit `331ddef91411d0e6549744ee889e05549e6da77d`

No provider, LLM, NLP, or paid service was used. No person was contacted.
