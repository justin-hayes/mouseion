"""Exact per-case / per-target gate aggregates from the raw gate JSON (scratch only).

The Go probe emits one row per (sentence, target) in corpus order, and a sentence with no
NOUN/VERB/ADJ/ADV surfaces emits no row. This script rebuilds the expected row sequence
from the corpus itself and asserts it matches the gate output row for row, so sentence
instances are never inferred from text equality.

Definitions:
  sentence instance  = one corpus sentence (per case, in order), duplicates kept
  target decision    = one (sentence instance, unique target surface) row
  has_targets        = sentence instance with >= 1 decision (the "153-style" denominator)
  accepted instance  = sentence instance with >= 1 accepted decision

Usage: python3 -I aggregate_gate.py <corpus.json> <gate_out.json> [label]
"""

import json
import sys
from collections import Counter, defaultdict

corpus_path, gate_path = sys.argv[1], sys.argv[2]
label = sys.argv[3] if len(sys.argv) > 3 else gate_path
corpus = json.load(open(corpus_path, encoding="utf-8"))
rows = json.load(open(gate_path, encoding="utf-8"))

TARGET_POS = {"NOUN", "VERB", "ADJ", "ADV"}
expected = []  # (case, sentence_index, sentence_text, target)
instances = []  # per corpus sentence: case, index, text, n_targets
for case in corpus:
    for s_index, s in enumerate(case["sentences"]):
        seen, targets = set(), []
        for t in s["tokens"]:
            if t["upos"] in TARGET_POS and t["surface"] not in seen:
                seen.add(t["surface"])
                targets.append(t["surface"])
        instances.append({"case": case["label"], "index": s_index, "text": s["text"], "n": len(targets)})
        for target in targets:
            expected.append((case["label"], s["text"], target))

actual = [(r["case"], r["sentence"], r["target"]) for r in rows]
if actual != expected:
    first = next((i for i, (a, e) in enumerate(zip(actual, expected)) if a != e), min(len(actual), len(expected)))
    raise SystemExit(f"row sequence mismatch at {first}: got {actual[first:first+1]} expected {expected[first:first+1]}")

by_instance = defaultdict(list)
cursor = 0
for inst in instances:
    if inst["n"] == 0:
        continue
    key = (inst["case"], inst["index"])
    by_instance[key] = rows[cursor:cursor + inst["n"]]
    cursor += inst["n"]
assert cursor == len(rows)

per_case = defaultdict(lambda: {"sentence_instances": 0, "with_targets": 0, "decisions": 0,
                                "accepted_instances": 0, "accepted_decisions": 0})
for inst in instances:
    c = per_case[inst["case"]]
    c["sentence_instances"] += 1
    rs = by_instance.get((inst["case"], inst["index"]), [])
    if rs:
        c["with_targets"] += 1
        c["decisions"] += len(rs)
        c["accepted_decisions"] += sum(1 for r in rs if r["accepted"])
        if any(r["accepted"] for r in rs):
            c["accepted_instances"] += 1

totals = Counter()
for c in per_case.values():
    totals.update(c)
reasons = Counter()
for key, rs in by_instance.items():
    if any(r["accepted"] for r in rs):
        continue
    for reason in sorted({x for r in rs for x in r["reasons"]}):
        reasons[reason] += 1

print(f"== {label}")
print(f"corpus cases {len(corpus)}; sentence instances {len(instances)}; with targets {totals['with_targets']}; "
      f"zero-target {len(instances) - totals['with_targets']}; decisions {len(rows)}")
print(f"accepted instances {totals['accepted_instances']}/{totals['with_targets']}; "
      f"accepted decisions {totals['accepted_decisions']}/{totals['decisions']}")
print("per case (with_targets/instances, accepted instances, decisions, accepted decisions):")
for case in dict.fromkeys(c["label"] for c in corpus):
    c = per_case[case]
    if c["sentence_instances"] == 0:
        continue
    print(f"  {case:42s} {c['with_targets']:3d}/{c['sentence_instances']:<3d} acc={c['accepted_instances']:3d} "
          f"dec={c['decisions']:4d} acc_dec={c['accepted_decisions']:4d}")
print("rejected-instance reasons (instances with zero accepted decisions, reason union):")
for reason, count in reasons.most_common():
    print(f"  {count:4d} {reason}")
import os

agg_dir = os.path.join(os.path.dirname(os.path.abspath(gate_path)), "aggregates")
os.makedirs(agg_dir, exist_ok=True)
json.dump({"label": label, "totals": dict(totals), "per_case": per_case, "rejected_reasons": dict(reasons)},
          open(os.path.join(agg_dir, label.replace(" ", "_").replace("/", "_") + ".json"), "w", encoding="utf-8"),
          ensure_ascii=False, indent=1)
