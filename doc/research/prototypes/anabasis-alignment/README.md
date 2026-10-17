# Book 1 alignment prototype — human checkpoint pending

**Throwaway branch only. Do not merge or wire this into card preparation.**

Question: can pinned Brownson English be aligned to the imported OGA sentences
and selected vocabulary without an LLM, and how should uncertainty appear?

For [Prototype: aligning public-domain English to Book 1 without an LLM](https://github.com/justin-hayes/mouseion/issues/2129),
under [Ancient-language cards without an LLM: public-domain translations, alignment and word sense](https://github.com/justin-hayes/mouseion/issues/2127).
Source selection and licence evidence:
[Research: openly licensed English translations of Anabasis and existing alignments](https://github.com/justin-hayes/mouseion/issues/2128).

## Current status

**No human-checked gold set exists here yet. No alignment accuracy was measured.**
`review-candidates.json` has `is_gold: false`, `reviewer: null` and per-sentence
`human_review: null`. The proposals are agent-drafted discussion material,
not outputs of the comparison algorithms and not evidence of reliability.

Open **`review.html` directly in a browser**; no server, network or installation
is needed. The example selector is the guided walkthrough: ordinary spans,
short clauses, long sentences, a crowded section, a crossing, and an editorial
mismatch. The three buttons explore proposed-span, wider-passage and withheld
English presentation; word emphasis can be previewed separately. All state is
in memory. Nothing in the demo marks a record reviewed or exports gold.

Justin must correct/confirm every candidate span and all three selected target
correspondences per example in a live exchange. Allow discontinuous spans,
NULL (no English counterpart), or uncertain alternatives; do not force a link.
Ask whether each target has additional English realizations. Preserve source
wording and punctuation rather than rewriting a translation to fit.

Only after review: commit a **separate** `gold.json`, retaining candidate/history
provenance, reviewer, live-review reference, approved sentence spans, and complete
links for the selected targets. Exclude unresolved targets from sure-link scoring
and report them separately, rather than silently treating them as NULL. Link the
immutable asset from the issue for
[Prototype: word sense in context for Ancient Greek without an LLM](https://github.com/justin-hayes/mouseion/issues/2130).
Word links are **not** dictionary-sense labels.

## Proposed sample

Eight purposively chosen sentences, three eligible vocabulary targets each.
Word counts exclude punctuation, after the import's ellipsis removal.

| Source sentence | Sections | Printed words | Section size (sentences touching) | Purpose |
| --- | --- | --- | --- | --- |
| 1 | 1.1.1 | 12 | 2 | Opening; discontinuous verb realization |
| 3 | 1.1.2 | 6 | 3 | Short Greek sentence; English semicolon |
| 15 | 1.1.6 | 22 | 4 | Verbal phrases; several lexical anchors |
| 177 | 1.3.13 | 31 | 1 | Entire longer section |
| 238 | 1.4.8 | 8 | 10 | Greek sentence inside a larger English sentence |
| 103 | 1.2.17–18 | 39 | 4 / 4 | Section-crossing span; neighbouring clause excluded |
| 428 | 1.7.15 | 15 | 2 | Brownson's following canals aside has no OGA counterpart |
| 536 | 1.9.17 | 22 | 2 | Existing Brownson-attributed Ugarit record overlaps |

Counts and exact token identities are in JSON; the table is only a reading aid.
This sample covers useful failure modes, not Book 1's distribution. It omits
many long sentences, section sizes 5–7, and most vocabulary. Showing proposals
biases the review. Any eventual result is a small diagnostic, not a calibrated
production reliability estimate or a held-out test. Add an independently
annotated sample before making a stronger claim.

## Verified baseline

Ran the **real import seam** with a capture-only writer: no database, NLP,
provider, queue or production state. It returned 607 sentences, 10,462 tokens,
110 dropped ellipsis tokens. The preparation script checked all 607 reconstructed
sentence texts and all 10,462 retained token surfaces, raw lemmas and codepoint
offsets against that output, then retained source sentence/token IDs and all
touched section references in the sidecar.

- 205 section keys match the pinned Brownson Book 1 sections.
- First-section assignment histogram: 1:32, 2:56, 3:51, 4:39, 5:16, 6:6,
  7:4, 10:1. Thus 146/205 sections own 2–4 sentences.
- Counting every touched section instead gives 1:31, 2:56, 3:51, 4:40,
  5:16, 6:6, 7:4, 10:1. The first-section shortcut hides overlaps.
- Source sentences 103, 155 and 445 cross 1.2.17–18, 1.3.5–6 and 1.8.5–6.
- The 10-sentence section is 1.4.8.

This checks executable import correspondence, **not semantic alignment** and
not a persisted installation's existing database rows.

## Reproduce preparation

Python standard library; the Go probe uses the repository's pinned Go toolchain.
From the repository root, download into disk-backed scratch, checking hashes:

```sh
python3 - <<'PY'
import hashlib, json
from pathlib import Path
from urllib.request import urlopen
root = Path('doc/research/prototypes/anabasis-alignment')
scratch = Path.home()/'.cache/scratch/anabasis-alignment-2129'
scratch.mkdir(parents=True, exist_ok=True)
for name, pin in json.loads((root/'review-candidates.json').read_text())['sources'].items():
    data = urlopen(pin['url']).read()
    assert hashlib.sha256(data).hexdigest() == pin['sha256']
    (scratch/name).write_bytes(data)
PY
GOTMPDIR=$HOME/.cache/gotmp go run doc/research/prototypes/anabasis-alignment/import_probe.go \
  "$HOME/.cache/scratch/anabasis-alignment-2129/oga.conllu" \
  "$HOME/.cache/scratch/anabasis-alignment-2129/import.json"
python3 doc/research/prototypes/anabasis-alignment/prepare_review.py \
  "$HOME/.cache/scratch/anabasis-alignment-2129"
```

`import_probe.go` has a build-ignore directive; ordinary application builds do
not include it. Its stdout is counts only. Private import text goes to a local
0600 file. Do not commit that full-work source/output. Delete scratch at the end
of the investigation. Generated review assets are committed on this throwaway
branch; regenerate rather than manually editing them.

English extraction omits TEI notes/heads, retains note tails and bracketed main
text, adds a space for paragraph milestones, then collapses whitespace. All
English spans index that **extracted section string**, not raw XML byte offsets.
Greek offsets follow the actual import reconstruction. JSON documents the fixed
tokenisation and zero-based half-open codepoint convention.

## Comparison to run only after live review

1. Punctuation splitting, length-based allocation and anchors: compare separately
   and as a combined monotonic heuristic; permit many-to-many/partial spans and
   retain adjacent-section windows. Uncertainty is not a sentence translation.
2. Small from-scratch statistical aligner (IBM Model 1 / fast_align-style
   baseline): start with the 205 pinned section pairs and report that these are
   coarse/noisy training units. Reserve reviewed sections from training and report
   vocabulary support; do not invent a learning curve or adequate-data claim.
   Books 2–7 are not automatically verified parallel training data.
3. OGA-lemma/dictionary-gloss anchors: use an explicitly pinned, non-NC-cleared
   dictionary; distinguish actual gloss evidence from hand-authored seed pairs.
4. Usable existing links: inspect the Brownson-attributed Ugarit record at
   1.9.17–19 under the source research's provenance qualifications. No links from
   Ugarit are currently copied into these candidates. Report partial coverage
   and unavailable/uncleared strategies as such, not as zero precision.

No learned/pretrained weights are used in this checkpoint. The pretrained Ugarit
model remains uncleared because identified inputs have NC/conflicting notices;
the map allows treebanks only when applicable licences are confirmed non-NC.

Report exact sentence-span set accuracy (including multiple sections), token-link
micro precision/recall **only for the selected target occurrences**, coverage,
abstentions and concrete failures. An abstention has no predicted links and does
not erase gold links from recall; precision with no predictions is undefined.
Keep sure, possible and unresolved links separate. Broader-passage presentation
does not count as an exact sentence success. With eight sentences, show counts
alongside fractions; do not infer Book 1-wide reliability.

This is a comparison plan, not completed evidence. Final strategy/fallback
acceptance requires another live exchange after the evidence is available.

## Checkpoint validation

- Real no-database import probe and complete sidecar correspondence: passed.
- Generated JSON/HTML regeneration is byte-identical: passed.
- Standalone Chromium check of all eight examples, all three presentation modes,
  emphasis suppression under uncertainty, and 390px mobile overflow: passed.
- `go test ./...`, complete Go lint, explicit Ruff check of the preparation
  script, and `gofmt -d` of the build-ignored probe: passed.
- Full `make lint` reached/passed Go lint but cannot run Python lint because
  this worktree has no `.venv/bin/ruff`. `make coverage-new` stops at missing
  `.venv/bin/python`/pytest. No production source or configuration changed.

## Attribution

Greek text/annotations: OGA v0.2.0, packaged as `oga_src`, derived from Perseus;
CC BY-SA 4.0. English: Xenophon, translated by Carleton L. Brownson (1921–22),
digitisation Perseus/Tufts, CC BY-SA 4.0. Pins and attribution are embedded in
the JSON and standalone HTML. Extracted text and proposed correspondences are
adapted data shared under [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/);
code remains under the repository licence. These are proposals by the assistant
for Justin, not scholarly or human-approved annotations.
