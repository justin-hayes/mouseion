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

## Hash-fetch instructions for a reader

1. Clone or fetch `PerseusDL/treebank_data` at `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`.
2. Clone or fetch `PerseusDL/canonical-greekLit` at `01b725d835e6e733062ffd79e0efdbae1ba06e5c`.
3. Check `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` against `44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568`.
4. Check `data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml` against `21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a`.

`fetch_inputs.sh` does steps 1 to 4.

## Non-goals and boundaries

Canonical POS and headword interpretation are out of scope (#2049). Card eligibility is not computed here (#2047). No annotation was created or corrected, and no text was repaired.
