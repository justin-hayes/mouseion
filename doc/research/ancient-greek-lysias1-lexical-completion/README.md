# Lysias I lexical evidence completion artifacts (#2049)

Companion folder of [`../ancient-greek-lysias1-lexical-completion.md`](../ancient-greek-lysias1-lexical-completion.md). Rights and attribution: [`NOTICE.md`](NOTICE.md).

## Layout

| Path | Content |
|---|---|
| `inputs/pinned-inputs.tsv` | Registry of the 20 primary-source files: role, repository, commit (not recorded for this pass), path, local path, sha256, declared rights, use. |
| `inputs/sha256sums.txt` | sha256 of the 20 files, relative to the sources directory. |
| `scripts/lexical_completion.py` | Reads the #2048 reconciliation manifest (route T1A2) and the sources; runs the named checks; writes `manifest/`. Stdlib only, deterministic, ASCII output. |
| `scripts/test_lexical_completion.py` | 16 tests: 11 synthetic (parsers, decision table, labels, input verification, ASCII output) and 5 source-backed (totals and blockers, proposal partition, no eligibility change, occurrence counts, byte-identical regeneration). Source-backed tests run only with `LEX_MANIFEST` and `LEX_SOURCES` set. |
| `manifest/occurrence_patches.tsv` | 235 rows, one per T1A2 vocabulary-blocked native node: before and after fields, proposal ID, basis, unsupported and unperformed points, finding labels, evidence checks, blockers. |
| `manifest/lemma_evidence.tsv` | 32 distinct lemmas among the 235: Smyth 2769 status, AGDT 4.1.12.1 status, UD named-in lists. |
| `manifest/name_checks.tsv` | Six name lemmas: Morpheus exact and near keys, by file and line. |
| `manifest/headword_checks.tsv` | Two headwords: H1, H2, H3 checks with file and line numbers. |
| `manifest/eimi_checks.tsv` | Two εἰμί nodes: postag tense and voice letters, relation, UD AUX and cop tests. |
| `manifest/summary.json` | Totals, rule and blocker counts, proposal and label counts, occurrence-level Smyth and AGDT counts, headword and name results. |

## Running

```sh
R=doc/research
python3 -I $R/ancient-greek-lysias1-lexical-completion/scripts/lexical_completion.py \
  $R/ancient-greek-lysias1-reconciliation/manifest <sources-dir> $R/ancient-greek-lysias1-lexical-completion/manifest
LEX_MANIFEST=$R/ancient-greek-lysias1-reconciliation/manifest LEX_SOURCES=<sources-dir> \
  python3 -I $R/ancient-greek-lysias1-lexical-completion/scripts/test_lexical_completion.py -v
```

`<sources-dir>` must hold the 20 files listed in `inputs/sha256sums.txt`. The script refuses to run on any mismatch. The source copies are not committed; the parent review retains them outside the repository (see `NOTICE.md`).

## Keys and statuses

- `node_key` = `sNNNwNNN` (native AGDT sentence and word ids), as in the reconciliation manifest.
- `*_before` fields are the 50f4ad6c mapping values, unchanged. `*_after` fields are the same values: nothing is adopted in this pass (`adoption_status = not_adopted (parent decision)`). `proposal_id` and `upos_proposed` record what a parent decision could adopt.
- Greek appears only as `U+XXXX` code points. Lemma and form are the AGDT v2.1 values; v1.6 lemma is shown only where it differs.
- Accent-variant matches (for example grave καὶ against Smyth's καί) are reported as `accent_variant`. They never feed a key, a rule, or a status.
