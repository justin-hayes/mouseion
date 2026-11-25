"""Cross-tabulate the modal Citation depth OLA's tokens carry (cts.json of
Research #2660, `modal_depth`) against the depth of the TEI Citation scheme
used here, for Works with a translation file.

Usage: python3 -I depth_crosstab.py alignment.json works.json cts.json OUT.json
"""
import json
import sys
from collections import Counter

al, wk, cts = (json.load(open(p)) for p in sys.argv[1:4])
out = sys.argv[4]
tiers = {r["work"]: r["tier"] for r in wk}
ct = Counter()
byk = {}
for k, v in cts.items():
    w = k.rsplit(".", 1)[0]
    if w in al and tiers.get(w, "-") != "-" and v.get("modal_depth") is not None:
        key = f"ola{v['modal_depth']}_tei{len(al[w]['source_units'])}"
        ct[key] += 1
        byk.setdefault(key, []).append(w)
json.dump({"crosstab": dict(sorted(ct.items())), "works": byk}, open(out, "w"), indent=1, sort_keys=True)
print(dict(sorted(ct.items())))
