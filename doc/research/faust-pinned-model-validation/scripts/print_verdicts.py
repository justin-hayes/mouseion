"""Print counts-only gate verdicts for two selected sentences, read from gate JSON (scratch only).

Selected sentences (no sentence text is printed):
  - was_faellt: the sentence in case was_faellt_surrounding_group that contains "Was fällt"
    (must match exactly one sentence);
  - director_first: the first sentence (index 0) of case director_first_speech.
The per-target verdicts come from the gate output rows, matched by case and sentence text.

Usage: python3 -I print_verdicts.py <corpus.json> <base_gate.json> <knockout_gate.json>
"""

import json
import sys

corpus = json.load(open(sys.argv[1], encoding="utf-8"))
gates = {"baseline": json.load(open(sys.argv[2], encoding="utf-8")),
         "knockout": json.load(open(sys.argv[3], encoding="utf-8"))}
cases = {c["label"]: c for c in corpus}

selections = {}
was = [s for s in cases["was_faellt_surrounding_group"]["sentences"] if "Was fällt" in s["text"]]
if len(was) != 1:
    raise SystemExit(f"expected exactly one 'Was fällt' sentence, found {len(was)}")
selections["was_faellt"] = ("was_faellt_surrounding_group", was[0]["text"])
first = cases["director_first_speech"]["sentences"][0]["text"]
selections["director_first"] = ("director_first_speech", first)

for name, (label, text) in selections.items():
    lf = text.count("\n")
    print(f"{name}: case={label} chars={len(text)} internal_LF={lf}")
    for mode, rows in gates.items():
        hits = [r for r in rows if r["case"] == label and r["sentence"] == text]
        accepted = sum(1 for r in hits if r["accepted"])
        print(f"  {mode}: targets={len(hits)} accepted={accepted} rejected={len(hits) - accepted}")
