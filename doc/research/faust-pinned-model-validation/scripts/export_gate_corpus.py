"""Export sampled Producer output as Go gate input (scratch only).

Single-group records use unit-relative (document-absolute) code-point offsets,
as cardexport.tokenAnchorSpan expects. Combined boundary runs are excluded
because their token offsets belong to the joined probe text, not the unit.
Usage: python3 -I export_gate_corpus.py <producer_results.json> <out.json> <extra.json> [<extra.json> ...]
Each extra file is a run_extra.py / run_prose_d.py style mapping of label -> sentences.
"""

import json
import sys

results = json.load(open(sys.argv[1], encoding="utf-8"))
out_path = sys.argv[2]
extras = []
for extra_path in sys.argv[3:]:
    extras.extend(json.load(open(extra_path, encoding="utf-8")).items())

cases = []
for record in results["records"]:
    if record["label"].startswith("combined_") or record.get("exception"):
        continue
    base = record.get("unit_start", 0)
    sentences = []
    for s in record["sentences"]:
        tokens = [
            {"surface": t["surface"], "raw_lemma": t["raw_lemma"], "canonical_lemma": t["canonical_lemma"],
             "upos": t["pos"], "dependency": t["dep"], "head": t["head"], "morphology": t["morph"],
             "start": base + t["span"][0], "end": base + t["span"][1]}
            for t in s["tokens"]
        ]
        sentences.append({"text": s["text"], "start": s["span_unit"][0], "tokens": tokens})
    cases.append({"label": record["label"], "group_ordinal": record.get("ordinal"), "language": "de", "sentences": sentences})

for label, value in extras:
    if label == "line_195_in_group_38":
        continue
    sentences = [
        {"text": s["text"], "start": s["start"],
         "tokens": [{"surface": t["surface"], "raw_lemma": t["raw_lemma"], "canonical_lemma": t["canonical_lemma"],
                     "upos": t["pos"], "dependency": t["dep"], "head": t["head"], "morphology": t["morph"],
                     "start": t["span"][0], "end": t["span"][1]} for t in s["token_detail"]]}
        for s in value["sentences"]
    ]
    cases.append({"label": "extra_" + label, "group_ordinal": None, "language": "de", "sentences": sentences})

json.dump(cases, open(out_path, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
print("cases", len(cases), "sentences", sum(len(c["sentences"]) for c in cases))
