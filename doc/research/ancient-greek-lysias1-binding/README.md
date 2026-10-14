# Lysias I binding artifacts (#2048)

Report: [`../ancient-greek-lysias1-binding.md`](../ancient-greek-lysias1-binding.md). Evidence: [`evidence-ledger.md`](evidence-ledger.md). Rights: [`NOTICE.md`](NOTICE.md).

## Contents

- `scripts/fetch_inputs.sh` — fetches the pinned AGDT v2.1, grc2 and two release witnesses from GitHub, and verifies the SHA-256 of the two primary files. Needs `git` and `sha256sum`. No credentials.
- `scripts/binding_probe.py` — builds the canonical running text and binds AGDT nodes. Stdlib only.
- `scripts/fold_and_release_checks.py` — fold mechanisms and cross-release stability. Stdlib only.
- `manifest/` — committed outputs: `occurrence_binding.tsv` (2,834 AGDT nodes), `canonical_tokens.tsv` (2,410 canonical word tokens), `sentences.tsv`, `discrepancies.tsv`, `summary.json`, `fold_mechanisms.json`, `release_stability.tsv`. ASCII only; keys are native IDs (`sNNNwNNN`, `cg2:wNNNN`); spans are half-open code-point offsets.

The scripts write no Greek text into `manifest/`. Per-occurrence surfaces go to an optional `detail/` directory, which is scratch and not committed.

## Reproduce from a clean state

```sh
W=doc/research/ancient-greek-lysias1-binding
D=$(mktemp -d "$HOME/.cache/scratch/greek-2048-binding/repro.XXXX")
sh $W/scripts/fetch_inputs.sh "$D/inputs"
python3 -I $W/scripts/binding_probe.py "$D/inputs" "$D/manifest" "$D/detail"
python3 -I $W/scripts/fold_and_release_checks.py "$D/inputs" "$D/manifest" "$D/detail"
diff -r "$D/manifest" $W/manifest && echo MANIFEST_IDENTICAL
```

The inputs are untrusted data. Run Python on them only with `python3 -I`, and pass paths as arguments.

Expected: the two `sha256sum` lines report `OK` for the primary files; the diff prints `MANIFEST_IDENTICAL`. There is no separate unit-test suite.

## Hash-fetch instructions for a reader

1. Clone or fetch `PerseusDL/treebank_data` at `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`.
2. Clone or fetch `PerseusDL/canonical-greekLit` at `01b725d835e6e733062ffd79e0efdbae1ba06e5c`.
3. Check `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` against `44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568`.
4. Check `data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml` against `21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a`.

`fetch_inputs.sh` does steps 1–4 for you.

## Non-goals and boundaries

Canonical POS and headword interpretation are out of scope (#2049). Card eligibility is not computed here (#2047). No annotation was created or corrected.
