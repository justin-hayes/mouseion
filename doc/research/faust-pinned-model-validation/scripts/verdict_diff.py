"""Per-case instance-level verdict changes between baseline and newline-knockout (scratch only).

Uses the same ordered alignment as aggregate_gate.py. Prints, per case: the number of
sentence instances containing an internal LF, how many have targets, and how many flip
from rejected to accepted. Also the Was faellt / Valentin facts from the raw corpus.
Usage: python3 -I verdict_diff.py <corpus.json> <base.json> <knockout.json>
"""

import json
import sys
from collections import Counter, defaultdict

corpus = json.load(open(sys.argv[1], encoding="utf-8"))
base = json.load(open(sys.argv[2], encoding="utf-8"))
knock = json.load(open(sys.argv[3], encoding="utf-8"))
TARGET_POS = {"NOUN", "VERB", "ADJ", "ADV"}

instances, cursor = [], 0
for case in corpus:
    for idx, s in enumerate(case["sentences"]):
        targets = []
        for t in s["tokens"]:
            if t["upos"] in TARGET_POS and t["surface"] not in targets:
                targets.append(t["surface"])
        rec = {"case": case["label"], "index": idx, "text": s["text"], "lf": s["text"].count("\n"),
               "tokens": len(s["tokens"]), "targets": targets}
        if targets:
            rec["rows_base"] = base[cursor:cursor + len(targets)]
            rec["rows_knock"] = knock[cursor:cursor + len(targets)]
            cursor += len(targets)
        instances.append(rec)
assert cursor == len(base) == len(knock)

stats = defaultdict(Counter)
for inst in instances:
    c = stats[inst["case"]]
    c["instances"] += 1
    if inst["lf"] > 0:
        c["with_internal_LF"] += 1
    if not inst["targets"]:
        continue
    c["with_targets"] += 1
    b = any(r["accepted"] for r in inst["rows_base"])
    k = any(r["accepted"] for r in inst["rows_knock"])
    if inst["lf"] > 0:
        c["with_LF_and_targets"] += 1
    if b != k:
        c["flip_rejected_to_accepted" if (not b and k) else "flip_accepted_to_rejected"] += 1
        c["flip_has_LF"] += 1 if inst["lf"] > 0 else 0
    # decision-level flips
    for rb, rk in zip(inst["rows_base"], inst["rows_knock"]):
        if rb["accepted"] != rk["accepted"]:
            c["decision_flips"] += 1
        if rb["reasons"] != rk["reasons"]:
            c["decisions_reason_changed"] += 1

print("case, instances, with_internal_LF, with_targets, with_LF_and_targets, instance flips (+ / -), flips_with_LF, decision flips")
for case in dict.fromkeys(c["label"] for c in corpus):
    c = stats[case]
    print(f"{case:42s} {c['instances']:4d} {c['with_internal_LF']:4d} {c['with_targets']:4d} {c['with_LF_and_targets']:4d} "
          f"+{c['flip_rejected_to_accepted']} -{c['flip_accepted_to_rejected']} lf_flips={c['flip_has_LF']} dec_flips={c['decision_flips']} "
          f"reason_changed_decisions={c['decisions_reason_changed']}")
tot = Counter()
for c in stats.values():
    tot.update(c)
print("TOTAL", dict(tot))

print("\n== Was faellt facts (instances whose text contains 'Was fällt')")
for inst in instances:
    if "Was fällt" in inst["text"]:
        print(inst["case"], "lf", inst["lf"], "tokens", inst["tokens"], "targets", len(inst["targets"]))
        if inst.get("rows_base"):
            print("   base acc", sum(r["accepted"] for r in inst["rows_base"]), "knock acc", sum(r["accepted"] for r in inst["rows_knock"]),
                  "base reasons", sorted({x for r in inst["rows_base"] for x in r["reasons"]})[:6])
