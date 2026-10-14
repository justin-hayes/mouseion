# Lysias I binding artifacts (#2048)

Report: [`../ancient-greek-lysias1-binding.md`](../ancient-greek-lysias1-binding.md). Evidence: [`evidence-ledger.md`](evidence-ledger.md). Rights and provenance: [`NOTICE.md`](NOTICE.md).

## Contents

- `scripts/fetch_inputs.sh`: fetches the pinned AGDT v2.1, grc2 and two release witnesses from GitHub and verifies the SHA-256 of the two primary files. Needs `git` and `sha256sum`. No credentials.
- `scripts/binding_probe.py`: builds the canonical text, tokens and rubric documents, binds AGDT nodes, and writes the manifest. Stdlib only.
- `scripts/test_binding_probe.py`: 14 focused tests on synthetic input. Stdlib only.
- `manifest/`: committed outputs. ASCII only.
  - `occurrence_binding.tsv`: one row per AGDT node (2,834), with separate `correspondence`, `surface`, `convention` and `occurrence_status` columns.
  - `canonical_tokens.tsv`: one row per canonical token (running and label words, running and label punctuation), with `coverage_status` and `node_keys`.
  - `rubric_bindings.tsv`: the 5 rubric nodes, with label document, TEI address and neighbour agreement.
  - `sentences.tsv`, `discrepancies.tsv`, `release_stability.tsv`, `summary.json`.

Revision 3 (text authority):

- `scripts/fetch_witnesses.sh`: run after `fetch_inputs.sh` on the same directory.
  - It exports the 18 historical canonical-greekLit blobs of the Lysias I Greek file into `history/`.
  - It fetches the Mnemosyne pages and the pinned Wikisource revisions into `witness/` and checks their SHA-256.
  - Needs `curl`. No credentials, and no cookies are kept.
- `scripts/lineage_collation.py`: compares the token sequences of every historical blob, the AGDT v1.6 and v2.1 printed sequences, and the revision current at the annotation date. Asserts git blob ids. Stdlib only.
- `scripts/witness_collation.py`:
  - Collates the full speech (grc2 with its labels) against Mnemosyne and Wikisource, under a declared comparison key.
  - Accounts for every word and punctuation token.
  - Joins the printed-page point checks.
  - Stdlib only. Imports `binding_probe.py`.
- `scripts/test_witness_collation.py`: 14 synthetic tests.
- `fixtures/hude_oct_page_checks.tsv`: 55 readings that I transcribed from Internet Archive page images of Hude's Oxford text. Each records the item id, leaf, line, and its code points. These are manual transcriptions, not OCR; diacritic-critical points were checked on IIIF crops.
- `collation/`: committed outputs. ASCII only; surfaces only as U+XXXX.
  - `lineage_*.tsv/json`, `agdt_vs_revisions.tsv`, `agdt_v16_vs_annotated_base.tsv`
  - `witness_word_ledger.tsv`, `witness_punct_ledger.tsv`, `witness_key_equalities.tsv`, `witness_named_checks.tsv`, `witness_point_checks.tsv`, `witness_structure.tsv`, `witness_summary.json`

Keys are native IDs (`sNNNwNNN` for AGDT, `cg2:wNNNN` / `cg2:pNNNN` for running tokens, `cg2:LNNwNNN` / `cg2:LNNpNNN` for label tokens). Spans are half-open code-point offsets in the named document.

The scripts write no Greek text into `manifest/`. The probe's optional `detail/` directory holds per-node surfaces for local review. It is scratch and not committed.

## Reproduce from a clean state

```sh
W=doc/research/ancient-greek-lysias1-binding
D=$(mktemp -d "$HOME/.cache/scratch/greek-2048-binding/repro.XXXX")
sh $W/scripts/fetch_inputs.sh "$D/inputs"
python3 -I $W/scripts/binding_probe.py "$D/inputs" "$D/manifest" "$D/detail"
python3 -I $W/scripts/test_binding_probe.py
diff -r "$D/manifest" $W/manifest && echo MANIFEST_IDENTICAL
```

The inputs are untrusted data. Run Python on them only with `python3 -I`, and pass paths as arguments. The probe asserts both primary hashes before it reads the inputs.

Expected: both `sha256sum` lines report `OK`, the test run reports `OK` for 14 tests, and the diff prints `MANIFEST_IDENTICAL`.

Revision 3, continuing in the same `$D`:

```sh
sh $W/scripts/fetch_witnesses.sh "$D/inputs"
python3 -I $W/scripts/lineage_collation.py "$D/inputs" "$D/collation"
python3 -I $W/scripts/witness_collation.py "$D/inputs" $W/manifest "$D/collation" $W/fixtures/hude_oct_page_checks.tsv
python3 -I $W/scripts/test_witness_collation.py
diff -r "$D/collation" $W/collation && echo COLLATION_IDENTICAL
```

Expected: every `sha256sum` line reports `OK`, 14 tests report `OK`, and the diff prints `COLLATION_IDENTICAL`.

The Mnemosyne and Wikisource pins are live web pages. If a page changes upstream, its hash check fails loudly, and the change is not silently absorbed. The Hude page images are not fetched by the scripts. The fixture records their item and leaf ids so the pages can be re-viewed at `https://archive.org/details/<item>/page/n<leaf>`.

## Hash-fetch instructions for a reader

1. Clone or fetch `PerseusDL/treebank_data` at `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`.
2. Clone or fetch `PerseusDL/canonical-greekLit` at `01b725d835e6e733062ffd79e0efdbae1ba06e5c`.
3. Check `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` against `44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568`.
4. Check `data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml` against `21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a`.

`fetch_inputs.sh` does steps 1 to 4.

## Non-goals and boundaries

Canonical POS and headword interpretation are out of scope (#2049). Card eligibility is not computed here (#2047). No annotation was created or corrected, and no text was repaired.
