# Lysias I binding and segmentation completion (#2048)

Companion folder of [`../ancient-greek-lysias1-binding-completion.md`](../ancient-greek-lysias1-binding-completion.md). It builds on the reconciliation at `7ac01256` ([report](../ancient-greek-lysias1-reconciliation.md), [folder](../ancient-greek-lysias1-reconciliation/README.md)), which it reads only through its committed manifests. Rights and attribution: [`NOTICE.md`](NOTICE.md).

## Layout

| Path | Content |
|---|---|
| `scripts/fetch_tokenizer.sh` | Exports `lib/llt/tokenizer.rb` and `lib/llt/tokenizer/greek.rb` at the two pinned llt-tokenizer commits and writes sha256 sums. |
| `scripts/producer_probe.py` | Re-runs the Greek path of the historical tokenizer (`split_krasis`, enclitic map, PUNCTUATION) over the grc1 and grc2 running text and aligns the output with the AGDT v1.6 printed nodes. Rules are parsed from the Ruby source, not retyped. |
| `scripts/test_producer_probe.py` | Unit tests for the rules, plus a full re-run that must reproduce `manifest/producer_summary.json` when the pinned inputs are present. |
| `scripts/completion_ledger.py` | Overlays the completion proposals on the baseline `route_nodes.tsv` and runs machine checks. Adds blocker labels and two proposal columns only; it never changes readiness, text status or the locked fields. |
| `scripts/test_completion_ledger.py` | Unit tests for the overlay and checks, and a run against the committed manifest. |
| `manifest/producer_summary.json` | Counts per (text, tokenizer revision): sim tokens, equal tokens, non-equal ops, enclitic insertions. |
| `manifest/producer_alignment_*.tsv` | Full alignment against native AGDT v1.6 for each (text, revision). Forms are code-point escaped. |
| `manifest/producer_enclitic_insertions_*.tsv` | Each enclitic the probe inserted, with its index in the producer token stream. |
| `manifest/completion_route_nodes.tsv` | The baseline route table (6 × 2,834 rows), same columns, plus `completion_proposal` and `completion_change`. |
| `manifest/completion_changes.tsv` | The 30 changed rows: 24 sentence-11 nodes and 6 `s125w015` rows, each with its added blocker. |
| `manifest/completion_producer_ops.tsv` | Each non-equal producer op (grc1 2014 at `c9b60a34`), mapped to its baseline T1A2 status. |
| `manifest/completion_sentences.tsv` | 134 sentences with the number of changed rows and added blocker labels per sentence. |
| `manifest/completion_checks.json` | Machine checks, their results, and sha256 of the baseline inputs. |

## Blocker labels added here (research-local)

- `BP-PRODUCER-IDENTITY`: the sentence-11 `μήτε` component order rests on a producer rule whose identity is not established.
- `BD-S125-ROUTE`: the printed route for `s125w015` (smooth vs rough breathing) needs a decision. The label is not a verdict.

## Keys

Node keys are `sNNNwNNN`, as in the reconciliation. Canonical keys are `cg2:wNNNN`. Forms in the producer manifests are `U+XXXX` sequences separated by spaces, with `|` between forms.

## Reproduce

```sh
D=$HOME/.cache/scratch/greek-lysias1-binding-completion   # new or existing scratch folder
sh ../ancient-greek-lysias1-reconciliation/scripts/fetch_inputs.sh "$D/inputs"   # verifies the pinned hashes
sh scripts/fetch_tokenizer.sh "$D/tok-new"                                       # new or empty dir; sha256 printed
python3 -I scripts/producer_probe.py "$D/inputs" "$D/tok-new" manifest         # regenerates producer_*
python3 scripts/completion_ledger.py                                             # regenerates completion_*
cd scripts && python3 -m unittest test_producer_probe.py test_completion_ledger.py
```

`fetch_tokenizer.sh` clones the tokenizer repository from `TOKENIZER_REPO` (default: the GitHub repository). The commits are checked by `cat-file`, so a clone with the same history is sufficient.

The probe's recorded summary test skips when `$D/inputs` or `$D/tok` is absent. `MOUSEION_LYSIAS_INPUTS` and `MOUSEION_TOKENIZER_DIR` override the scratch locations.
