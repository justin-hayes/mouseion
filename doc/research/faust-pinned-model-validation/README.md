# Faust I pinned-model validation: reproduction source

Throwaway research asset for the #2147 break-encoding validation on issue #2151. It is
published only so the evidence can be re-run. It is not product code, not a decision record,
and it is not merged. No application, generated, or SQL file changes with it.

## What this asset is and is not

- **Is:** scripts that re-run (1) the real production `Producer` (`mouseion_nlp.producer`,
  pinned German Stanza config) over bounded sample groups from Goethe's *Faust I* (Project
  Gutenberg #21000), (2) a body-only segmentation of the same EPUB, (3) an offset rebase of the
  existing Producer results, (4) a paragraph-kept variant of Werther prose, (5) the Go
  `cardexport` gate with a scratch newline-clause ablation injected through `go -overlay`, and
  (6) tokenizer-input identity checks.
- **Is not:** the final per-line list detector, role filter, or end-to-end feature pipeline.
  The gate ablation removes one clause; it is not the accepted content-sensitive list
  detector. Gate acceptance on these samples is not a Book omission rate, and nothing here
  measures model accuracy against gold annotations. Raw outputs are not gold.
- **Does not commit** the EPUB, any extracted text, the segmentation JSON, the raw corpus,
  or raw Producer/gate outputs. Those are regenerated locally by the commands below. Only
  scripts, this README, and `MANIFEST.sha256` are in the repository. Excerpts used as
  selectors are short needles; no Project Gutenberg header or license text is included.

## Superseded input (kept on purpose)

`scripts/segment_epub.py` is the **original candidate segmentation**. It keeps every spine
item, including the cover wrapper and `<title>` text. It is retained only because
`run_producer.py` selects units by the indexes this segmentation produced, so it reproduces the
original `out1` run. **It is superseded.** The corrected body-only oracle for sample provenance
is `scripts/segment_body.py`, which mirrors the production unit selection in
`internal/epub/epub.go` (`readDocument` / `extractXHTML`).

## Pinned inputs

| Item | Pin |
| --- | --- |
| Source EPUB | `pg21000.epub`, SHA256 `93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01` |
| Repo base | commit `2993f171` (`docs(context): define source structure update (#2150) (#2179)`) |
| Python | 3.13.5 |
| stanza | 1.15.0 (`nlp/uv.lock`) |
| torch | 2.14.1+cpu (`nlp/uv.lock`, CPU index) |
| Go | go1.27.2 linux/amd64 |
| Stanza resource manifest source | `stanfordnlp/stanza-resources` commit `569db9d983a47931673d7d265361ca93af85d176`, `resources_1.15.0.json` |
| German model source | `stanfordnlp/stanza-de` commit `6b928a48d4c8d8096b9dc75c887da3652f8ef671` (the measured `v1.15.0` revision), `models/` |
| `nlp/uv.lock` | SHA256 `89fe4f40ec67a643a0f42aa6183d6b86fa5081bd5058bcf4fa4fdb3caa729e1a` |
| `nlp/src/mouseion_nlp/model_config.py` | SHA256 `b6187f65fc819b74300fb4f6f9637097ad6fc9269270245b177bddcdb35adfe7` |
| `nlp/src/mouseion_nlp/producer.py` | SHA256 `e8d9bdbcc93419ccd9e3e79c13ae1f65882ad30c7cd219d3e0ce964b90789ed1` |
| `HEAD:internal/cardexport/cardexport.go` (ablation target) | SHA256 `34e79d1a8ef50be64ac79fba74e47bcf400aba1ad4155cf554bda4f98071207f` |
| `nlp/testdata/german_prose_werther_excerpt.txt` | SHA256 `e066b0ee1c3fff0bfca6a7f4a45fedf0c9148e7bbea6106df19110aaf9865a1e` (repo file) |

Stanza artifacts, pinned in the `STANZA_RESOURCES_DIR` directory (downloaded only
by the explicit prerequisite commands below, never by a probe script):

```
447e419d87d0623f6ae5a84021472ea8024d6a4b6803566aa8a4f5a80ef598e4  de/backward_charlm/newswiki.pt
3e69de89d7ae43488f09ebd10ff29c1d305b0a0995d5427406a031623c721361  de/depparse/combined_charlm.pt
ba0ebfb852329cee8f13d9a0d0a30b4bdab7d95587029e78bd90c2a0dfe22248  de/forward_charlm/newswiki.pt
00ce0574363ef64626c8255751981a33ae904e6bbb52c585d234e136e9d2dc45  de/lemma/combined_nocharlm.pt
6ccb3c99f2bb099f0e21c5340d3206108b5a9d00f7194e6244a654d7bb3101c7  de/mwt/combined.pt
70ff1dda0c617a73688340a6a831d8df4fc388dedfdcf6a8cee0bb4f5922814c  de/pos/combined_charlm.pt
19a5cefbfdb93f3696ace424b904553f1d2525e7c1500bf58fb22f06f7ddceb7  de/pretrain/conll17.pt
1c1a399870a2dd771356e0e611de8323b42df439a44a66dc8023d8d456217de1  de/tokenize/combined_nocharlm.pt
ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463  resources.json
```

## Reproducibility limits

Raw file digests are **not** byte-reproducible in general. `elapsed_seconds`, Torch thread
counts, resource and provenance paths, and set-derived key order vary between runs. Use
`scripts/canonical_digest.py`, which drops volatile fields and sorts keys. The canonical digests
recorded below were produced by the verification run for this asset.

## Ordered commands

Run from a checkout of this branch. Each variable is set explicitly; nothing is guessed.

```sh
# 0. Environment (set once per shell)
set -eu
export REPO="$(git rev-parse --show-toplevel)"          # research branch checkout
export S="$REPO/doc/research/faust-pinned-model-validation/scripts"
export WORK="$HOME/.cache/scratch/faust-2151-repro"      # all outputs go here
export EPUB="$WORK/pg21000.epub"
export STANZA_RESOURCES_DIR="$WORK/stanza"
export PY="$WORK/venv/bin/python"
export OMP_NUM_THREADS=2
export GOTMPDIR="$HOME/.cache/gotmp"                   # go1.27.2 must already be on PATH
mkdir -p "$WORK" "$GOTMPDIR" "$STANZA_RESOURCES_DIR"

# Locked environment and Go module prerequisites (uv 0.13.0 on PATH).
UV_PROJECT_ENVIRONMENT="$WORK/venv" uv sync --project "$REPO/nlp" --python 3.13 --locked --no-build --group dev
"$PY" -c 'import stanza, torch; assert stanza.__version__ == "1.15.0"; assert torch.__version__ == "2.14.1+cpu"'
(cd "$REPO" && go mod download)
export GOPROXY=off

# Explicit network prerequisite: about 315 MiB of model downloads if absent.
# Existing verified bytes may be placed at these exact paths instead.
if ! test -f "$EPUB"; then
  curl --fail --location --retry 3 https://www.gutenberg.org/ebooks/21000.epub.noimages --output "$EPUB"
fi
if ! test -f "$STANZA_RESOURCES_DIR/resources.json"; then
  curl --fail --location --retry 3 https://raw.githubusercontent.com/stanfordnlp/stanza-resources/569db9d983a47931673d7d265361ca93af85d176/resources_1.15.0.json --output "$STANZA_RESOURCES_DIR/resources.json"
fi
while IFS= read -r model; do
  dest="$STANZA_RESOURCES_DIR/de/$model"
  mkdir -p "$(dirname "$dest")"
  if ! test -f "$dest"; then
    curl --fail --location --retry 3 "https://huggingface.co/stanfordnlp/stanza-de/resolve/6b928a48d4c8d8096b9dc75c887da3652f8ef671/models/$model" --output "$dest"
  fi
done <<'MODELS'
backward_charlm/newswiki.pt
depparse/combined_charlm.pt
forward_charlm/newswiki.pt
lemma/combined_nocharlm.pt
mwt/combined.pt
pos/combined_charlm.pt
pretrain/conll17.pt
tokenize/combined_nocharlm.pt
MODELS
```

The verification run reused the previously provisioned locked Python 3.13
environment and checksum-matching models; it did not repeat a cold environment
installation or model download. All probe commands below ran successfully from
the published scripts. The pinned manifest URL was fetched and its SHA-256
verified; the eight model source-revision LFS SHA-256 values were checked against
the downloaded runtime artifacts without downloading them a second time. A
partial/wrong download fails the following checksum gate: remove only that
task-local bad file and fetch again, never silently continue on a mismatch.

```sh
# 1. Verify inputs
echo "93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01  $EPUB" | sha256sum -c -
(cd "$STANZA_RESOURCES_DIR" && sha256sum -c) <<'EOF'
447e419d87d0623f6ae5a84021472ea8024d6a4b6803566aa8a4f5a80ef598e4  de/backward_charlm/newswiki.pt
3e69de89d7ae43488f09ebd10ff29c1d305b0a0995d5427406a031623c721361  de/depparse/combined_charlm.pt
ba0ebfb852329cee8f13d9a0d0a30b4bdab7d95587029e78bd90c2a0dfe22248  de/forward_charlm/newswiki.pt
00ce0574363ef64626c8255751981a33ae904e6bbb52c585d234e136e9d2dc45  de/lemma/combined_nocharlm.pt
6ccb3c99f2bb099f0e21c5340d3206108b5a9d00f7194e6244a654d7bb3101c7  de/mwt/combined.pt
70ff1dda0c617a73688340a6a831d8df4fc388dedfdcf6a8cee0bb4f5922814c  de/pos/combined_charlm.pt
19a5cefbfdb93f3696ace424b904553f1d2525e7c1500bf58fb22f06f7ddceb7  de/pretrain/conll17.pt
1c1a399870a2dd771356e0e611de8323b42df439a44a66dc8023d8d456217de1  de/tokenize/combined_nocharlm.pt
ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463  resources.json
EOF
(cd "$S/.." && sha256sum -c MANIFEST.sha256)

# 2. Superseded candidate segmentation, then the real Producer over its samples (~15 s)
"$PY" -I "$S/segment_epub.py" "$EPUB" "$WORK/seg_candidate.json"
"$PY" -I "$S/run_producer.py" "$WORK/seg_candidate.json" "$WORK/out1" "$REPO"

# 3. Corrected body-only segmentation and offset rebase (no model call)
"$PY" -I "$S/segment_body.py" "$EPUB" "$WORK/seg_body.json"
"$PY" -I "$S/rebase_check.py" "$WORK/seg_candidate.json" "$WORK/seg_body.json" "$WORK/out1/producer_results.json" "$WORK/out2"

# 4. Isolated-line extras and the paragraph-kept Werther variant D
"$PY" -I "$S/run_extra.py" "$WORK/seg_candidate.json" "$WORK/extra2.json" "$REPO"
"$PY" -I "$S/run_prose_d.py" "$REPO" "$WORK/out2/werther_paragraphs_kept.json"

# 5. Gate corpora (sample; sample + D)
"$PY" -I "$S/export_gate_corpus.py" "$WORK/out2/producer_results_rebased.json" "$WORK/corpus_sample.json" "$WORK/extra2.json"
"$PY" -I "$S/export_gate_corpus.py" "$WORK/out2/producer_results_rebased.json" "$WORK/corpus_sample_plusD.json" "$WORK/extra2.json" "$WORK/out2/werther_paragraphs_kept.json"

# 6. Scratch overlays: derives the knockout from HEAD's cardexport.go, checks it, writes to $WORK/gate
"$PY" -I "$S/make_overlays.py" "$REPO" "$WORK/gate"

# 7. Go gate runs (scratch test injected via -overlay; no repo file is changed)
cd "$REPO"
for c in sample sample_plusD; do for m in base knockout; do
  PROBE_CORPUS="$WORK/corpus_$c.json" PROBE_OUT="$WORK/gate/${c}_$m.json" \
    go test -count=1 -v -run TestProbeGate -overlay="$WORK/gate/overlay_$m.json" ./internal/cardexport/
done; done
cd - >/dev/null

# 8. Aggregates, verdict flips, selected verdicts
for x in sample_base sample_knockout sample_plusD_base sample_plusD_knockout; do
  c="$(echo "$x" | sed 's/_base$//;s/_knockout$//')"
  "$PY" -I "$S/aggregate_gate.py" "$WORK/corpus_$c.json" "$WORK/gate/$x.json" "$x"
done
"$PY" -I "$S/verdict_diff.py" "$WORK/corpus_sample.json" "$WORK/gate/sample_base.json" "$WORK/gate/sample_knockout.json"
"$PY" -I "$S/print_verdicts.py" "$WORK/corpus_sample.json" "$WORK/gate/sample_base.json" "$WORK/gate/sample_knockout.json"

# 9. Tokenizer-input evidence on the repo Werther excerpt
WERTHER="$REPO/nlp/testdata/german_prose_werther_excerpt.txt"
"$PY" -I "$S/tok_feat_identity.py" "$REPO" "$WERTHER"
"$PY" -I "$S/tok_variants.py" "$REPO" "$WERTHER"

# 10. Canonical digests (compare with the table below)
for f in "$WORK/out1/producer_results.json" "$WORK/out2/producer_results_rebased.json" "$WORK/out2/werther_paragraphs_kept.json" "$WORK/seg_candidate.json" "$WORK/seg_body.json"; do
  "$PY" -I "$S/canonical_digest.py" "$f"
done
```

## Expected results (verification run for this asset)

Canonical digests (`canonical_digest.py`) from the verification run for this asset. A
mismatch requires investigation of inputs, environment and scripts; it is not a
passing rerun:

```
bf674799ce59eb2f91419e230da512546bb00e972357998f463f969229a541a8  out1/producer_results.json
56ccbac336dbec7096879c2825893ea23577317dfea6d9a40414d39f4259788b  out2/producer_results_rebased.json
c8cbeaece303a2aa2435a1ecbc5f1a00339b6f67093afe4eabd685bc93baa461  out2/werther_paragraphs_kept.json
8ac39fd174f238549cebc6409b0fb135290a944e9555f1687d5997bdf160870b  seg_candidate.json
92a71280906005e35b55cdd99c3a80679088561281b0156fe41bfce2608c4f5b  seg_body.json
```

Raw SHA256 values seen in this run, for reference only (not stable): `seg_candidate.json`
`a77f69953c292342186b013e89d6a51ac092b4cabb095ba6f3e1fcb1b9a4f88e` in one run and
`989bd5295d6f1fc09515bd6c15cd1db4d64942c714677696e06a1d4310615d3e` in another, because `role_groups`
key order follows set iteration. Their canonical digest is the stable comparison.

Gate totals (instances: sentence instances / with targets; decisions: sentence×target):

| Corpus (instances / with targets) | Ablation | Accepted instances | Accepted decisions / total decisions |
| --- | --- | --- | --- |
| sample (155 / 153) | baseline | 9 | 41 / 1146 |
| sample (155 / 153) | knockout | 48 | 424 / 1146 |
| sample + D (254 / 252) | baseline | 36 | 300 / 1964 |
| sample + D (254 / 252) | knockout | 75 | 683 / 1964 |

Flips baseline → knockout (sample): 39 rejected→accepted instances in total. 23 are the
as-file Werther prose (hard-wrap line breaks, 4 → 27 of 99). 16 are literary sentences
(Director 5, Was fällt 5, Thule 5 across single-stanza cases, Ins-correction 1) whose
`<br>`-shaped contract breaks the newline clause currently rejects.

Selected verdicts (from the gate JSON, counts only): the Was fällt sentence (one sentence,
4 internal LF, 17 targets) goes from 0 to 17 accepted; the Director first speech sentence
(3 internal LF, 12 targets) goes from 0 to 12 accepted.

Werther paragraph-kept variant D: 27 of 99 accepted in both baseline and knockout, so the
newline clause does not change contract-shaped prose.

## Open items not settled here

- Per-line list detection, the role filter, and the full pipeline are not exercised.
- The `Was fällt euch an? Entzückung oder Schmerzen?` sentence is one Stanza sentence.
  `Valentin fällt.` is a speaker label with nested `fällt`, which the policy excludes
  while its earlier example called `fällt` eligible. The parent acceptance decision
  resolved these by respecting measured analyzer boundaries and the enclosing
  speaker role, not by changing the core rules; this asset supplies measurements,
  not an independent product decision.
- Lemma and dependency risks (`sag’`, `schwanken`, `fängt … an`, `hatt’`) were not
  separately re-reviewed during this reproduction run. The parent evidence report
  retains its earlier reviewed observations and accepted limitations.
