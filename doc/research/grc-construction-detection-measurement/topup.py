"""Replace sample items invalidated by the article-duplicate fix (deviation 1). Scratch only."""
import json, random, sys, os, collections, glob
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import detect
src = open("sample.py", encoding="utf-8").read()
ns = {"detect": detect}
for name in ("node_line", "text_of", "find"):
    start = src.index("def %s(" % name); end = src.index("\n\n\n", start)
    exec(src[start:end], ns)
S = json.load(open("work/sample.json", encoding="utf-8"))
dropped = set(json.load(open("work/dropped_items.json")))
cands = [json.loads(l) for l in open("work/cands.jsonl", encoding="utf-8")]
rng = random.Random(26071)
have = collections.Counter(d["author"] for d in S["items"] if d["construction"] == "articular_infinitive"
                           and d["sample"] == "loose" and d["item"] not in dropped)
chosen = {(d["work"], d["sent_id"], d["anchor"]) for d in S["items"]}
new = []
n = len(S["items"])
for au in ("Plato", "Xenophon"):
    pool = sorted([d for d in cands if d["construction"] == "articular_infinitive" and d["stratum"] == "loose"
                   and d["author"] == au and (d["work"], d["sent_id"], d["anchor"]) not in chosen],
                  key=lambda d: (d["work"], int(d["sent_id"]), d["anchor"]))
    for d in rng.sample(pool, 15 - have[au]):
        n += 1
        new.append(dict(d, sample="loose", item="I%03d" % n))
S["items"] += new
json.dump(S, open("work/sample.json", "w", encoding="utf-8"), ensure_ascii=False, indent=0)
trees = {}
for path in sorted(glob.glob("data/*.conllu")):
    work = path.rsplit("/", 1)[-1].split(".perseus")[0]
    for s in detect.read(path):
        trees[(work, s["meta"]["sent_id"])] = s
blocks = []
for d in new:
    s = trees[(d["work"], d["sent_id"])]; a = ns["find"](s, d["anchor"]); ext = set(detect.subtree(a))
    chain, h = [], a.head
    while h is not None and len(chain) < 3:
        chain.append(h); h = h.head
    L = ["### %s · %s · %s %s · sent %s · anchor token %d (%s)" % (d["item"], d["sample"], d["author"], d["ref"], d["sent_id"], a.i, a.form),
         "Detector: loose", "Text (anchor ⟦⟧, its subtree ‹›): " + ns["text_of"](s, a, ext), "Parse — head chain:"]
    L += [ns["node_line"](x) for x in reversed(chain)]
    L.append("Parse — anchor subtree (≤25 nodes, sentence order):")
    L += [ns["node_line"](x, "*" if x is a else "") for x in sorted(ext, key=lambda x: x.i)[:25]]
    blocks.append("\n".join(L))
open("sheets/articular_infinitive_topup.md", "w", encoding="utf-8").write("\n\n".join(blocks) + "\n")
print(have, [d["item"] for d in new])
