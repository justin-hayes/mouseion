"""Draw the #2615 sample (v2/protocol.md) and render judging sheets. Scratch only.

Usage: python3 -I v2/sample.py   (run from ~/.cache/scratch/grc-syntax)
"""

import collections
import glob
import json
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(1, os.path.dirname(HERE))
import detect2 as d2  # noqa: E402

src1 = open(os.path.join(os.path.dirname(HERE), "sample.py"), encoding="utf-8").read()
ns = {"detect": d2}
for name in ("node_line", "text_of", "find"):
    a = src1.index("def %s(" % name)
    exec(src1[a:src1.index("\n\n\n", a)], ns)

SEED = 2615
HITS, MIN_SUB, LOOSE, COND_EXCL = 120, 60, 60, 40
rng = random.Random(SEED)
cands = [json.loads(l) for l in open("v2/work/cands.jsonl", encoding="utf-8")]
key = lambda d: (d["work"], int(d["sent_id"]), d["anchor"])  # noqa: E731
items = []
chosen = set()


def take(pool, k, sample):
    pool = sorted((d for d in pool if (d["construction"],) + key(d) not in chosen), key=key)
    for d in rng.sample(pool, max(0, min(k, len(pool)))):
        chosen.add((d["construction"],) + key(d))
        items.append(dict(d, sample=sample))


def pool(con, stratum, sub=Ellipsis):
    return [d for d in cands if d["construction"] == con and d["stratum"] == stratum
            and (sub is Ellipsis or d["subtype"] == sub)]


for con in d2.DETECTORS:
    take(pool(con, "hit"), HITS, "hit")
    if con in ("articular_infinitive", "result", "indirect_statement"):
        subs = sorted({d["subtype"] for d in pool(con, "hit") if d["subtype"]})
        for sub in subs:
            have = sum(1 for d in items if d["construction"] == con and d["subtype"] == sub)
            take(pool(con, "hit", sub), MIN_SUB - have, "hit-topup")
for con, sub in (("conditional", Ellipsis), ("purpose", Ellipsis),
                 ("indirect_statement", "infinitive"), ("indirect_statement", "hoti-hos")):
    take(pool(con, "loose", sub), LOOSE, "loose")
take(pool("conditional", "excluded"), COND_EXCL, "excluded")
take(pool("genitive_absolute", "excluded"), 100, "excluded")
for i, d in enumerate(items):
    d["item"] = "R%03d" % (i + 1)

# link to #2607 items and Justin's judgments there
old = {(d["construction"],) + key(d): d["item"] for d in json.load(open("work/sample.json", encoding="utf-8"))["items"]}
for d in items:
    d["prior_item"] = old.get((d["construction"],) + key(d))
json.dump({"items": items}, open("v2/work/sample.json", "w", encoding="utf-8"), ensure_ascii=False, indent=0)

trees = {}
for path in sorted(glob.glob("data/*.conllu")):
    work = path.rsplit("/", 1)[-1].split(".perseus")[0]
    for s in d2.read(path):
        trees[(work, s["meta"]["sent_id"])] = s


def block(d):
    s = trees[(d["work"], d["sent_id"])]
    a = ns["find"](s, d["anchor"])
    ext = set(d2.subtree(a))
    chain, h = [], a.head
    while h is not None and len(chain) < 3:
        chain.append(h)
        h = h.head
    L = ["### %s · %s · %s %s · sent %s · anchor token %d (%s)" % (
        d["item"], d["sample"], d["author"], d["ref"], d["sent_id"], a.i, a.form),
        "Detector: %s%s" % (d["stratum"], (", subtype " + d["subtype"]) if d["subtype"] else ""),
        "Text (anchor ⟦⟧, its subtree ‹›): " + ns["text_of"](s, a, ext), "Parse — head chain:"]
    L += [ns["node_line"](x) for x in reversed(chain)]
    c, ev = d["construction"], []
    if c == "genitive_absolute":
        ev = d2.gen_subjects(a) + ([d2.up(a)] if d2.up(a) is not None else [])
    elif c in ("conditional", "result", "purpose"):
        ev = d2.clause_kids(a) + ([d2.up(a)] if c == "conditional" and d2.up(a) is not None else [])
    elif c == "indirect_statement":
        ev = ([d2.up(a)] if d2.up(a) is not None else []) + d2.eff_kids(a, {"OBJ", "SBJ"})
    elif c == "articular_infinitive":
        ev = [k for k in a.kids if d2.neut_sg_article(k)]
    if ev:
        L.append("Rule evidence (subjects / clause verbs / governing verb / article):")
        L += [ns["node_line"](x) for x in ev]
    L.append("Parse — anchor subtree (≤25 nodes, sentence order):")
    L += [ns["node_line"](x, "*" if x is a else "") for x in sorted(ext, key=lambda x: x.i)[:25]]
    return "\n".join(L)


os.makedirs("v2/sheets", exist_ok=True)
for f in glob.glob("v2/sheets/*.md"):
    os.remove(f)
by = collections.defaultdict(list)
for d in items:
    by[d["construction"]].append(d)
batches = []
for con, ds in by.items():
    for k in range(0, len(ds), 60):
        name = "%s_%d" % (con, k // 60 + 1)
        with open("v2/sheets/%s.md" % name, "w", encoding="utf-8") as f:
            f.write("\n\n".join(block(d) for d in ds[k:k + 60]) + "\n")
        batches.append((name, len(ds[k:k + 60])))
print(collections.Counter((d["construction"], d["sample"]) for d in items))
print(len(items), "items;", sum(1 for d in items if d["prior_item"]), "also in #2607")
print(batches)
