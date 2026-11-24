# Prototype: constructions and syntax in Sentence study (#2609)

Throwaway UI prototype. **Never merged.** It answers how constructions and
per-word syntax appear in Sentence study, and how a construction's extent is
marked (the marking is reused as-is in Concordance's expanded construction row).

## Verdict (Justin, 2026-11-24)

**Variant C — one extract per construction** was chosen over A (numbered
brackets in the sentence; brackets collide with editorial brackets in Greek
editions) and B (tiered rules under the interlinear line; not reusable in
Concordance, which has no interlinear line). The full resolution is on #2609.

## What is here

- `gen.py` renders four real OGA v0.2.0 sentences (Phaedo 90; Anabasis 3.3.16,
  2.4.6, 1.1.3) into static pages shaped like the shipped Sentence study page,
  with `?variant=A|B|C`, `?focus=c<n>` (the focused-construction state) and
  `?open=relations`.
- `find.py` searched the pinned files for the scenario sentences.

The generated pages and screenshots are not committed: they contain source
text (the evidence-holds-IDs-only rule from the measurement tickets).

## Regenerating

Both scripts expect the scratch layout `~/.cache/scratch/grc-syntax/` with the
pinned OGA `oga_src` v0.2.0 CoNLL-U files in `data/`, `detect.py` from
`research/grc-construction-detection-measurement` and `v2/detect2.py` from the
remeasure tree on the same branch. `gen.py` also reads `study.html`, a Sentence
study page saved from `cmd/fixtureserver`, for the shell, and the pages link
`/static/app.css` (copy `internal/webapp/static/` beside them).

```sh
cd ~/.cache/scratch/grc-syntax
python3 -I proto-2609/gen.py
cd proto-2609 && python3 -m http.server 18610
```
