# GLAUx *Anabasis* Book 1 import proof: companion files

Findings: [`../ancient-greek-glaux-anabasis1-import-proof.md`](../ancient-greek-glaux-anabasis1-import-proof.md). Rights: [`NOTICE.md`](NOTICE.md).

## Reproduce

```sh
doc/research/ancient-greek-glaux-anabasis1-import-proof/scripts/reproduce.sh [INPUT_DIR]
```

This one command:

1. fetches the 27 pinned inputs in `inputs/pinned-inputs.tsv` and verifies their sha256;
2. runs `scripts/import_proof.py`;
3. runs Mouseion's Go selection code (`_selectionprobe`) on the resulting analysis;
4. checks that the Go output is byte-identical to the Python model;
5. diffs every file in `manifest/` against the regeneration;
6. runs the 25 tests.

It needs Python 3.13, Go and network access for the first fetch. Pass `--update` as the second argument to rewrite `manifest/`. `INPUT_DIR` defaults to `~/.cache/scratch/greek-glaux-anabasis1-import-proof/inputs`.

Run only the synthetic tests (no inputs needed):

```sh
cd doc/research/ancient-greek-glaux-anabasis1-import-proof && python3 -I -m unittest discover -s scripts -p 'test_*.py'
```

## Files

| Path | Content |
| --- | --- |
| `scripts/import_proof.py` | Extraction, binding, mapping, segmentation, selection model and card-input stages. Each stage carries its own version constant. |
| `scripts/test_import_proof.py` | 22 synthetic tests, plus 3 full-input tests: regeneration, accounting and Go parity. |
| `_selectionprobe/main.go` | Calls `selection.Project`, `selection.NewEligibility(nil, nil)` and `AllowsBookDeckCandidate`. The underscore keeps it out of `./...`. |
| `inputs/pinned-inputs.tsv` | Input path, role, URL, revision, sha256 and licence evidence. |
| `fixtures/marchant-point-checks.tsv` | Expected OCR excerpts at editorially marked places. |
| `manifest/book1.txt` | Exact extracted text. All spans are half-open code-point offsets into this text without its trailing newline. |
| `manifest/nodes.tsv` | All 10,645 native Book 1 nodes: native fields; binding kind, rule, status and span; UPOS rule and status; headword; canonical key; vocabulary status with reason; complete sentence; flags. |
| `manifest/sentences.tsv` | 382 complete sentences: span, boundary, units, code points, cap status, native segments, editorial markup, text. |
| `manifest/identities.tsv` | 1,294 identities: frequency, selection reason, observed source forms. |
| `manifest/selection_model.tsv`, `selection_go.tsv` | Python model and real Go output. They are identical. |
| `manifest/card_inputs.tsv` | Each of the 373 selected identities: chosen sentence and target span, or omission reason; earlier rejections; flags. |
| `manifest/review_set.tsv` | Attributed source examples, each chosen by a stated rule. |
| `manifest/text_revisions.tsv` | GLAUx token stream compared with all 15 Perseus revisions of the file. |
| `manifest/marchant_point_checks.tsv` | Point-check results. |
| `manifest/crosswalk_*.tsv` | UD Perseus UPOS counts per AGDT letter, and per conjunction or verb lemma occurring in Book 1. |
| `manifest/sections.tsv`, `editorial_regions.tsv` | Section spans and paragraph starts; `del`, `add` and `sic` spans. |
| `manifest/summary.json`, `ledger.json`, `catalog_example.json` | Counts and sensitivity; separate verdicts; the discovery/acquisition example. |
