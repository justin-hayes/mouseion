"""Draw the seeded sample (protocol.md) and render judging sheets. Scratch only.

Usage: python3 -I sample.py   (run from ~/.cache/scratch/grc-syntax)
"""

import collections
import glob
import json
import os
import random
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import detect  # noqa: E402

SEED = 2607
HITS, LOOSE, ABST, PERC, RAND = 20, 15, 10, 40, 40
MIN_SUB = 10

rng = random.Random(SEED)
cands = [json.loads(l) for l in open("work/cands.jsonl", encoding="utf-8")]

# ---------------------------------------------------------------- sample
items = []


def take(pool, k, stratum):
    pool = sorted(pool, key=lambda d: (d["work"], int(d["sent_id"]), d["anchor"]))
    pick = rng.sample(pool, min(k, len(pool)))
    for d in pick:
        items.append(dict(d, sample=stratum))
    return pick


by = collections.defaultdict(list)
for d in cands:
    by[(d["construction"], d["stratum"], d["author"])].append(d)

for con in detect.DETECTORS:
    for au in ("Plato", "Xenophon"):
        take(by[(con, "hit", au)], HITS, "hit")
        if con == "indirect_statement":
            for sub in ("infinitive", "hoti-hos"):
                take([d for d in by[(con, "loose", au)] if d["subtype"] == sub], LOOSE, "loose")
        else:
            take(by[(con, "loose", au)], LOOSE, "loose")
        take(by[(con, "abstain", au)], ABST, "abstain")
    # subtype top-up
    subs = collections.Counter(d["subtype"] for d in items if d["construction"] == con and d["sample"] == "hit")
    allsubs = {d["subtype"] for d in cands if d["construction"] == con and d["stratum"] == "hit"}
    chosen = {(d["work"], d["sent_id"], d["anchor"]) for d in items if d["construction"] == con}
    for sub in sorted(allsubs, key=str):
        need = MIN_SUB - subs.get(sub, 0)
        if need > 0:
            pool = [d for d in cands if d["construction"] == con and d["stratum"] == "hit"
                    and d["subtype"] == sub and (d["work"], d["sent_id"], d["anchor"]) not in chosen]
            take(pool, need, "hit-topup")

perc = [d for d in cands if d["stratum"] == "excluded"]
take(perc, PERC, "excluded")

for i, d in enumerate(items):
    d["item"] = "I%03d" % (i + 1)

# random sentences
trees = {}
for path in sorted(glob.glob("data/*.conllu")):
    work = path.rsplit("/", 1)[-1].split(".perseus")[0]
    for s in detect.read(path):
        trees[(work, s["meta"]["sent_id"])] = s
rand = []
for au in ("Plato", "Xenophon"):
    pool = sorted(k for k, s in trees.items() if s["meta"]["source"].startswith(au)
                  and len(detect.real_order(s)) >= 4)
    for k in rng.sample(pool, RAND):
        rand.append({"work": k[0], "sent_id": k[1], "author": au,
                     "ref": trees[k]["meta"]["source"].split(", ", 1)[-1]})
for i, d in enumerate(rand):
    d["item"] = "S%03d" % (i + 1)

json.dump({"items": items, "random": rand}, open("work/sample.json", "w", encoding="utf-8"),
          ensure_ascii=False, indent=0)

# ---------------------------------------------------------------- render


def node_line(n, mark=""):
    h = n.head.i if n.head is not None else 0
    return "  %s%d %s  lemma=%s  pos=%s  rel=%s  head=%d" % (mark, n.i, n.form, n.lemma, n.pos, n.rel, h)


def text_of(s, anchor=None, extent=()):
    out = []
    for n in s["nodes"]:
        if n.art:
            continue
        w = n.form
        if n is anchor:
            w = "⟦%s⟧" % w
        elif n in extent:
            w = "‹%s›" % w
        out.append(w)
    return " ".join(out)


def find(s, tid):
    for n in s["nodes"]:
        if n.tid == tid:
            return n
    raise KeyError(tid)


os.makedirs("sheets", exist_ok=True)
sheets = collections.defaultdict(list)
for d in items:
    s = trees[(d["work"], d["sent_id"])]
    a = find(s, d["anchor"])
    ext = set(detect.subtree(a))
    chain, h = [], a.head
    while h is not None and len(chain) < 3:
        chain.append(h)
        h = h.head
    lines = ["### %s · %s · %s %s · sent %s · anchor token %d (%s)" % (
        d["item"], d["sample"], d["author"], d["ref"], d["sent_id"], a.i, a.form),
        "Detector: %s%s" % (d["stratum"], (", subtype " + d["subtype"]) if d["subtype"] else ""),
        "Text (anchor ⟦⟧, its subtree ‹›): " + text_of(s, a, ext), "Parse — head chain:"]
    lines += [node_line(x) for x in reversed(chain)]
    ev = []
    c = d["construction"]
    if c == "genitive_absolute":
        ev = detect.gen_subjects(a)
        g = detect.up(a)
        ev += [g] if g is not None else []
    elif c in ("conditional", "result", "purpose"):
        ev = detect.clause_kids(a)
    elif c == "indirect_statement":
        g = detect.up(a)
        ev = ([g] if g is not None else []) + detect.eff_kids(a, {"OBJ", "SBJ"})
    elif c == "articular_infinitive":
        ev = [k for k in a.kids if detect.neut_sg_article(k)]
    if ev:
        lines.append("Rule evidence (subjects / clause verbs / governing verb / article):")
        lines += [node_line(x) for x in ev]
    lines.append("Parse — anchor subtree (≤25 nodes, sentence order):")
    sub = sorted(ext, key=lambda x: x.i)[:25]
    lines += [node_line(x, "*" if x is a else "") for x in sub]
    sheets[d["construction"]].append("\n".join(lines))

for con, blocks in sheets.items():
    with open("sheets/%s.md" % con, "w", encoding="utf-8") as f:
        f.write("\n\n".join(blocks) + "\n")

for au in ("Plato", "Xenophon"):
    blocks = []
    for d in rand:
        if d["author"] != au:
            continue
        s = trees[(d["work"], d["sent_id"])]
        lines = ["### %s · %s %s · sent %s" % (d["item"], au, d["ref"], d["sent_id"]),
                 "Text: " + " ".join("%s_%d" % (n.form, n.i) for n in s["nodes"] if not n.art)]
        blocks.append("\n".join(lines))
    with open("sheets/random_%s.md" % au, "w", encoding="utf-8") as f:
        f.write("\n\n".join(blocks) + "\n")

print(collections.Counter((d["construction"], d["sample"]) for d in items))
print(len(items), "items,", len(rand), "random sentences")
