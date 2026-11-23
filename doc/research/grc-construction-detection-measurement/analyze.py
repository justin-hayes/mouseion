"""Compute the protocol's rates from judgments; write the public table and the review set.

Usage: python3 -I analyze.py [--review]   (run from ~/.cache/scratch/grc-syntax)

Reads work/cands.jsonl, work/sample.json, judgments/*.jsonl and, if present,
review/justin.jsonl (Justin's judgments override the agent's). Writes:
  out/occurrences.tsv   public: IDs, verdicts, judgments, Smyth, error class
  out/results.md        public: per-construction figures
  review/set.json       the live-review set (with --review)
"""

import collections
import glob
import json
import math
import os
import random
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import detect  # noqa: E402

CONS = list(detect.DETECTORS)
AUTH = ("Plato", "Xenophon")
GREEK = re.compile(r"[Ͱ-Ͽἀ-῿]+")

cands = [json.loads(l) for l in open("work/cands.jsonl", encoding="utf-8")]
S = json.load(open("work/sample.json", encoding="utf-8"))
DROPPED = set(json.load(open("work/dropped_items.json"))) if os.path.exists("work/dropped_items.json") else set()
items = {d["item"]: d for d in S["items"] if d["item"] not in DROPPED}
rand = {d["item"]: d for d in S["random"]}

J = {}
for p in glob.glob("judgments/*.jsonl"):
    if p.endswith("random.jsonl"):
        continue
    for l in open(p, encoding="utf-8"):
        if l.strip():
            d = json.loads(l)
            J[d["item"]] = d
R = collections.defaultdict(list)
if os.path.exists("judgments/random.jsonl"):
    for l in open("judgments/random.jsonl", encoding="utf-8"):
        if l.strip():
            d = json.loads(l)
            R[d["item"]].append(d)
H = {}
if os.path.exists("review/justin.jsonl"):
    for l in open("review/justin.jsonl", encoding="utf-8"):
        if l.strip():
            d = json.loads(l)
            H[d["item"]] = d


def final(it):
    """Final judgment: Justin's where reviewed, else the agent's."""
    a = J.get(it)
    h = H.get(it)
    if h:
        m = dict(a or {})
        m.update({k: h.get(k) for k in ("true", "subtype", "smyth", "error", "note")})
        return m
    return a


def wilson(k, n):
    if n == 0:
        return (float("nan"),) * 3
    z, p = 1.96, k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    w = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return p, max(0, c - w), min(1, c + w)


def fmt(k, n):
    if n == 0:
        return "n/a"
    p, lo, hi = wilson(k, n)
    return "%.2f (%d/%d; %.2f–%.2f)" % (p, k, n, lo, hi)


# corpus counts
CNT = collections.Counter((d["construction"], d["stratum"], d["author"]) for d in cands)
CNTSUB = collections.Counter((d["construction"], d["stratum"], d["subtype"], d["author"]) for d in cands)
cand_ix = collections.defaultdict(list)
for d in cands:
    cand_ix[(d["work"], d["sent_id"])].append(d)

# random-sentence frame size
trees = {}
for path in sorted(glob.glob("data/*.conllu")):
    work = path.rsplit("/", 1)[-1].split(".perseus")[0]
    for s in detect.read(path):
        trees[(work, s["meta"]["sent_id"])] = s
FRAME = collections.Counter(s["meta"]["source"].split(",")[0] for s in trees.values()
                            if len(detect.real_order(s)) >= 4)


def sampled(con, au, strata, sub=None, detsub=None):
    out = []
    for it, d in items.items():
        if d["construction"] != con or d["author"] != au or d["sample"] not in strata:
            continue
        if detsub is not None and d["subtype"] != detsub:
            continue
        f = final(it)
        if f is None:
            continue
        out.append((d, f))
    return out


# classify random-sentence occurrences
def classify_random():
    rows = []
    for it, occs in R.items():
        meta = rand[it]
        s = trees[(meta["work"], meta["sent_id"])]
        byi = {n.i: n for n in s["nodes"]}
        for o in occs:
            if o.get("none"):
                continue
            n = byi.get(o["token"])
            if n is None:
                rows.append(dict(o, author=meta["author"], cover="bad-token", tid=None))
                continue
            near = {n.tid} | {k.tid for k in n.kids} | ({n.head.tid} if n.head is not None else set())
            cover = "outside"
            for c in cand_ix[(meta["work"], meta["sent_id"])]:
                if c["construction"] != o["construction"]:
                    continue
                if c["anchor"] == n.tid:
                    cover = c["stratum"]
                    break
                if c["anchor"] in near and cover == "outside":
                    cover = c["stratum"] + "-near"
            rows.append(dict(o, author=meta["author"], cover=cover, tid=n.tid,
                             work=meta["work"], ref=meta["ref"], sent_id=meta["sent_id"]))
    return rows


RROWS = classify_random()


def rates(con, ceiling=False):
    """Return per-author and pooled precision/recall/abstention figures."""
    out = {}
    pooled = collections.Counter()
    for au in AUTH:
        hs = sampled(con, au, {"hit"})
        yes = sum(1 for d, f in hs if f["true"] == "yes")
        no_parser = sum(1 for d, f in hs if f["true"] == "no" and (f.get("error") or "").startswith("parser"))
        n = sum(1 for d, f in hs if f["true"] in ("yes", "no"))
        if ceiling:
            n -= no_parser
        p = yes / n if n else 0
        Hc = CNT[(con, "hit", au)]
        tp = Hc * p
        miss = 0.0
        for stratum in ("loose", "abstain", "excluded"):
            subs = [None]
            if con == "indirect_statement" and stratum == "loose":
                subs = ["infinitive", "hoti-hos"]
            for sub in subs:
                ls = [(d, f) for d, f in sampled(con, au, {stratum}) if sub is None or d["subtype"] == sub]
                ln = sum(1 for d, f in ls if f["true"] in ("yes", "no"))
                ly = sum(1 for d, f in ls if f["true"] == "yes" and (not ceiling or not (f.get("error") or "").startswith("parser")))
                C = CNTSUB[(con, stratum, sub, au)] if sub else CNT[(con, stratum, au)]
                if ln:
                    miss += C * ly / ln
        outside = [r for r in RROWS if r["construction"] == con and r["author"] == au and r["cover"] == "outside"]
        k = len(outside) / max(1, len([i for i, d in rand.items() if d["author"] == au]))
        miss_out = FRAME[au] * k
        rec = tp / (tp + miss + miss_out) if tp + miss + miss_out else float("nan")
        A = CNT[(con, "abstain", au)] + CNT[(con, "excluded", au)]
        out[au] = dict(p=(yes, n), H=Hc, tp=tp, miss=miss, miss_out=miss_out, recall=rec,
                       abst=(A, Hc + A), outside=len(outside))
        pooled.update(dict(tp=tp, miss=miss, miss_out=miss_out, H=Hc, A=A, py=yes, pn=n))
    tp, mi, mo = pooled["tp"], pooled["miss"], pooled["miss_out"]
    out["pooled"] = dict(
        p_weighted=sum(out[a]["tp"] for a in AUTH) / max(1, pooled["H"]),
        p=(pooled["py"], pooled["pn"]),
        recall=tp / (tp + mi + mo) if tp + mi + mo else float("nan"),
        abst=(pooled["A"], pooled["H"] + pooled["A"]))
    return out


def subtype_rows(con):
    rows = []
    subs = sorted({d["subtype"] for d in items.values() if d["construction"] == con and d["sample"].startswith("hit")}, key=str)
    for s in subs:
        k = n = cons_ok = 0
        for it, d in items.items():
            if d["construction"] != con or not d["sample"].startswith("hit") or d["subtype"] != s:
                continue
            f = final(it)
            if not f or f["true"] not in ("yes", "no"):
                continue
            n += 1
            if f["true"] == "yes":
                cons_ok += 1
                if s is None or (f.get("subtype") or None) == s:
                    k += 1
        rows.append((s, k, n, cons_ok))
    return rows


def errors(con):
    c = collections.Counter()
    for it, d in items.items():
        if d["construction"] != con:
            continue
        f = final(it)
        if not f:
            continue
        wrong = (d["sample"].startswith("hit") and f["true"] == "no") or \
                (not d["sample"].startswith("hit") and f["true"] == "yes")
        if wrong:
            side = "false hit" if d["sample"].startswith("hit") else "miss (%s)" % d["sample"]
            c[(side, f.get("error") or "unclassified")] += 1
    return c


def write_outputs():
    os.makedirs("out", exist_ok=True)
    review = json.load(open("review/set.json")) if os.path.exists("review/set.json") else {}
    sit = {i: s for s, ids in review.items() for i in ids}
    cols = ["item", "work", "author", "citation", "sent_id", "anchor", "construction", "sample",
            "detector_verdict", "detector_subtype", "agent_true", "agent_subtype", "agent_smyth",
            "agent_error", "agent_note", "review_sitting", "justin_true", "justin_subtype",
            "justin_smyth", "justin_error", "justin_note"]
    with open("out/occurrences.tsv", "w", encoding="utf-8") as f:
        f.write("\t".join(cols) + "\n")
        for it in sorted(items):
            d, a, h = items[it], J.get(it, {}), H.get(it, {})
            row = [it, d["work"], d["author"], d["ref"], d["sent_id"], d["anchor"], d["construction"],
                   d["sample"], d["stratum"], d["subtype"] or "", a.get("true", ""), a.get("subtype") or "",
                   a.get("smyth") or "", a.get("error") or "", clean(a.get("note")), sit.get(it, ""),
                   h.get("true", ""), h.get("subtype") or "", h.get("smyth") or "", h.get("error") or "",
                   clean(h.get("note"))]
            f.write("\t".join(str(x).replace("\t", " ") for x in row) + "\n")
    with open("out/random_occurrences.tsv", "w", encoding="utf-8") as f:
        f.write("sentence\twork\tauthor\tcitation\tsent_id\tanchor\tconstruction\tsubtype\tsmyth\tdetector_coverage\tnote\n")
        for r in sorted(RROWS, key=lambda r: (r["item"], r.get("token", 0))):
            f.write("\t".join(str(x) for x in [r["item"], r.get("work", ""), r["author"], r.get("ref", ""),
                                               r.get("sent_id", ""), r.get("tid") or "", r["construction"],
                                               r.get("subtype") or "", r.get("smyth") or "", r["cover"],
                                               clean(r.get("note"))]) + "\n")


def clean(note):
    """Drop Greek from notes, except a stated governing-verb lemma (Mouseion's list review)."""
    if not note:
        return ""
    keep = re.findall(r"governing verb: ?([Ͱ-Ͽἀ-῿]+)", note)
    s = GREEK.sub("…", note)
    for k in keep:
        s = s.replace("governing verb: …", "governing verb: " + k, 1)
    return s


def build_review(seed=26072):
    rng = random.Random(seed)
    out = {"sitting 1": [], "sitting 2": []}
    for con in CONS:
        its = sorted(i for i, d in items.items() if d["construction"] == con and i in J)
        uns = [i for i in its if J[i]["true"] == "unsure"]
        dis = [i for i in its if i not in uns and (
            (items[i]["sample"].startswith("hit") and J[i]["true"] == "no") or
            (not items[i]["sample"].startswith("hit") and J[i]["true"] == "yes") or
            (items[i]["sample"].startswith("hit") and items[i]["subtype"] and J[i]["true"] == "yes"
             and (J[i].get("subtype") or None) != items[i]["subtype"]))]
        rng.shuffle(dis)
        dis = dis[:10]
        rest = [i for i in its if i not in uns and i not in dis]
        target = 26 if con == "genitive_absolute" or con == "indirect_statement" else 24
        k = max(5, target - len(uns) - len(dis))
        pick = uns + dis + rng.sample(rest, min(k, len(rest)))
        key = "sitting 1" if con in ("conditional", "articular_infinitive", "result") else "sitting 2"
        out[key] += sorted(pick)
        out.setdefault("random-part", []).extend(i for i in pick if i in rest)
    os.makedirs("review", exist_ok=True)
    json.dump(out, open("review/set.json", "w"), indent=1)
    return out


def report():
    L = ["# Results", ""]
    for con in CONS:
        r, rc = rates(con), rates(con, ceiling=True)
        L.append("## %s" % con)
        L.append("")
        L.append("| | Plato | Xenophon | Pooled |")
        L.append("|---|---|---|---|")
        L.append("| Corpus hits | %d | %d | %d |" % (r["Plato"]["H"], r["Xenophon"]["H"], r["Plato"]["H"] + r["Xenophon"]["H"]))
        L.append("| Precision (sampled) | %s | %s | %s |" % (fmt(*r["Plato"]["p"]), fmt(*r["Xenophon"]["p"]), fmt(*r["pooled"]["p"])))
        L.append("| Precision, hit-weighted | | | %.2f |" % r["pooled"]["p_weighted"])
        L.append("| Recall (estimated) | %.2f | %.2f | %.2f |" % (r["Plato"]["recall"], r["Xenophon"]["recall"], r["pooled"]["recall"]))
        L.append("| Misses outside the loose query (random sentences) | %d | %d | |" % (r["Plato"]["outside"], r["Xenophon"]["outside"]))
        L.append("| Abstention | %s | %s | %s |" % (fmt(*r["Plato"]["abst"]), fmt(*r["Xenophon"]["abst"]), fmt(*r["pooled"]["abst"])))
        L.append("| Detector ceiling (parser errors forgiven): precision / recall | %s / %.2f | %s / %.2f | %s / %.2f |" % (
            fmt(*rc["Plato"]["p"]), rc["Plato"]["recall"], fmt(*rc["Xenophon"]["p"]), rc["Xenophon"]["recall"],
            fmt(*rc["pooled"]["p"]), rc["pooled"]["recall"]))
        L.append("")
        st = subtype_rows(con)
        if any(s for s, *_ in st):
            L.append("Subtype precision (random hits plus top-up, pooled): " + "; ".join(
                "%s %s" % (s or "none", fmt(k, n)) for s, k, n, c in st))
            L.append("")
        e = errors(con)
        if e:
            L.append("Errors: " + "; ".join("%s / %s: %d" % (a, b, c) for (a, b), c in sorted(e.items())))
            L.append("")
    if H and os.path.exists("review/set.json"):
        rp = set(json.load(open("review/set.json"))["random-part"])
        L += ["## Agent-Justin agreement", "", "| Construction | Reviewed | Judgment agrees (all reviewed) | Judgment agrees (random part) | Subtype agrees (random part, yes items) |", "|---|---:|---|---|---|"]
        for con in CONS:
            rev = [i for i in H if i in items and items[i]["construction"] == con]
            ag = sum(1 for i in rev if H[i]["true"] == J[i]["true"])
            rr = [i for i in rev if i in rp]
            agr = sum(1 for i in rr if H[i]["true"] == J[i]["true"])
            ry = [i for i in rr if H[i]["true"] == "yes" and J[i]["true"] == "yes"]
            sa = sum(1 for i in ry if (H[i].get("subtype") or None) == (J[i].get("subtype") or None))
            L.append("| %s | %d | %s | %s | %s |" % (con, len(rev), fmt(ag, len(rev)), fmt(agr, len(rr)), fmt(sa, len(ry))))
        L.append("")
    open("out/results.md", "w", encoding="utf-8").write("\n".join(L) + "\n")
    print("\n".join(L))


if __name__ == "__main__":
    if "--review" in sys.argv:
        print({k: len(v) for k, v in build_review().items()})
    write_outputs()
    report()
