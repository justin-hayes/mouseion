"""Record Justin's review: python3 -I record.py SITTING START END [OVERRIDES_JSON]
Items START..END (1-based, sheet order) default to agreeing with the agent;
OVERRIDES_JSON maps sheet number -> {"true":..,"subtype":..,"smyth":..,"note":..}."""
import json, glob, sys, os
R = json.load(open("review/set.json"))
J = {}
for p in glob.glob("judgments/*.jsonl"):
    if "random" not in p:
        for l in open(p, encoding="utf-8"):
            if l.strip():
                d = json.loads(l); J[d["item"]] = d
sit, a, b = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
ov = json.loads(sys.argv[4]) if len(sys.argv) > 4 else {}
path = "review/justin.jsonl"
H = {}
if os.path.exists(path):
    for l in open(path, encoding="utf-8"):
        d = json.loads(l); H[d["item"]] = d
ids = R["sitting %s" % sit]
for k in range(a, b + 1):
    it = ids[k - 1]; j = J[it]
    rec = {"item": it, "true": j["true"], "subtype": j.get("subtype"), "smyth": j.get("smyth"),
           "error": j.get("error"), "note": "agrees with agent", "agreed": True}
    if str(k) in ov:
        rec.update(ov[str(k)]); rec["agreed"] = False
    H[it] = rec
with open(path, "w", encoding="utf-8") as f:
    for it in sorted(H):
        f.write(json.dumps(H[it], ensure_ascii=False) + "\n")
print(len(H), "reviewed")
