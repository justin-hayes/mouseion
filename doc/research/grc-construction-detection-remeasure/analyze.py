"""Compute the #2615 rates (v2/protocol.md); build the review set and sheets.

Usage: python3 -I v2/analyze.py [--review]   (run from ~/.cache/scratch/grc-syntax)

Reads v2/work/{cands,sample}.jsonl, v2/judgments/*.jsonl, Justin's #2607 judgments
(review/justin.jsonl, by prior item) and his #2615 review (v2/review/justin.jsonl).
Writes v2/out/occurrences.tsv and v2/out/results.md (public: IDs only), and with
--review v2/review/{set.json,unsure.md,blind.md} (scratch: Greek).
"""

import collections
import glob
import json
import math
import os
import random
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(1, os.path.dirname(HERE))
import detect2 as d2  # noqa: E402

CONS = list(d2.DETECTORS)
AMENDED = ("conditional", "purpose", "indirect_statement")
CARRIED_RECALL = {"articular_infinitive": 0.98, "result": 0.91, "genitive_absolute": 0.74}
GREEK = re.compile(r"[Ͱ-Ͽἀ-῿]+")


def jl(path):
    return [json.loads(l) for l in open(path, encoding="utf-8") if l.strip()] if os.path.exists(path) else []


cands = jl("v2/work/cands.jsonl")
items = {d["item"]: d for d in json.load(open("v2/work/sample.json", encoding="utf-8"))["items"]}
J = {d["item"]: d for p in glob.glob("v2/judgments/*.jsonl") for d in jl(p)}
H1 = {d["item"]: d for d in jl("review/justin.jsonl")}          # #2607 review, keyed by old item
J1 = {d["item"]: d for p in glob.glob("judgments/*.jsonl") if "random" not in p for d in jl(p)}
H2 = {d["item"]: d for d in jl("v2/review/justin.jsonl")}       # this review
REVIEW = json.load(open("v2/review/set.json")) if os.path.exists("v2/review/set.json") else {}


def justin(it):
    if it in H2:
        return H2[it], "2615"
    p = items[it].get("prior_item")
    if p and p in H1:
        return H1[p], "2607"
    return None, None


def final(it):
    a = J.get(it)
    h, _ = justin(it)
    if h:
        m = dict(a or {})
        m.update({k: h.get(k) for k in ("true", "subtype", "smyth", "error", "note")})
        return m
    return a


def wilson(k, n):
    if n == 0:
        return float("nan"), float("nan"), float("nan")
    z, p = 1.96, k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    w = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return p, max(0.0, c - w), min(1.0, c + w)


def fmt(k, n):
    if not n:
        return "n/a"
    p, lo, hi = wilson(k, n)
    return "%.2f (%d/%d; %.2f–%.2f)" % (p, k, n, lo, hi)


def verdict(k, n):
    if not n:
        return "n/a"
    p, lo, _ = wilson(k, n)
    return "**clears**" if lo >= 0.90 else ("straddles" if p >= 0.90 else "fails")


CNT = collections.Counter((d["construction"], d["stratum"]) for d in cands)
CNTSUB = collections.Counter((d["construction"], d["stratum"], d["subtype"]) for d in cands)
CNTAU = collections.Counter((d["construction"], d["stratum"], d["author"]) for d in cands)


def judged(con, samples, sub=Ellipsis, au=None):
    out = []
    for it, d in items.items():
        if d["construction"] != con or d["sample"] not in samples:
            continue
        if sub is not Ellipsis and d["subtype"] != sub:
            continue
        if au and d["author"] != au:
            continue
        f = final(it)
        if f and f["true"] in ("yes", "no"):
            out.append((d, f))
    return out


def yes(rows, ceiling=False):
    k = sum(1 for d, f in rows if f["true"] == "yes")
    n = len(rows)
    if ceiling:
        n -= sum(1 for d, f in rows if f["true"] == "no" and (f.get("error") or "").startswith("parser"))
    return k, n


def subtype_prec(con, sub):
    rows = judged(con, {"hit", "hit-topup"}, sub)
    k = sum(1 for d, f in rows if f["true"] == "yes" and (sub is None or (f.get("subtype") or None) == sub))
    return k, len(rows), sum(1 for d, f in rows if f["true"] == "yes")


# ---- random sentences from #2607, re-classified against the amended candidates
def outside_misses():
    S1 = {d["item"]: d for d in json.load(open("work/sample.json", encoding="utf-8"))["random"]}
    trees = {}
    for path in sorted(glob.glob("data/*.conllu")):
        w = path.rsplit("/", 1)[-1].split(".perseus")[0]
        for s in d2.read(path):
            trees[(w, s["meta"]["sent_id"])] = s
    ix = collections.defaultdict(list)
    for c in cands:
        ix[(c["work"], c["sent_id"])].append(c)
    frame = sum(1 for s in trees.values() if len(d2.real_order(s)) >= 4)
    out = collections.Counter()
    occ = collections.Counter()
    for o in jl("judgments/random.jsonl"):
        if o.get("none"):
            continue
        m = S1[o["item"]]
        s = trees[(m["work"], m["sent_id"])]
        n = next((x for x in s["nodes"] if x.i == o["token"]), None)
        if n is None:
            continue
        if o["construction"] == "purpose" and n.lemma != "ἵνα" and not any(
                k.lemma == "ἵνα" for k in [n.head] + n.kids if k is not None):
            continue  # ὅπως/ὡς purpose: now out of scope
        occ[o["construction"]] += 1
        near = {n.tid} | {k.tid for k in n.kids} | ({n.head.tid} if n.head is not None else set())
        if not any(c["construction"] == o["construction"] and c["anchor"] in near for c in ix[(m["work"], m["sent_id"])]):
            out[o["construction"]] += 1
    return out, occ, frame, len(S1)


OUT, OCC, FRAME, NRAND = outside_misses()


def recall(con, ceiling=False):
    """Pooled estimate: tp / (tp + loose misses + excluded misses + outside misses)."""
    if con == "indirect_statement":
        tp = 0.0
        for sub in ("infinitive", "hoti-hos"):
            k, n, ok = subtype_prec(con, sub)
            rows = judged(con, {"hit", "hit-topup"}, sub)
            kk, nn = yes(rows, ceiling)
            tp += CNTSUB[(con, "hit", sub)] * (kk / nn if nn else 0)
    else:
        k, n = yes(judged(con, {"hit"}), ceiling)
        tp = CNT[(con, "hit")] * (k / n if n else 0)
    miss = 0.0
    parts = []
    strata = [("loose", Ellipsis)] if con != "indirect_statement" else [("loose", "infinitive"), ("loose", "hoti-hos")]
    if con == "conditional":
        strata.append(("excluded", Ellipsis))
    for st, sub in strata:
        rows = judged(con, {st}, sub)
        if ceiling:
            rows = [(d, f) for d, f in rows if not (f["true"] == "yes" and (f.get("error") or "").startswith("parser"))]
        ky = sum(1 for d, f in rows if f["true"] == "yes")
        C = CNT[(con, st)] if sub is Ellipsis else CNTSUB[(con, st, sub)]
        if rows:
            miss += C * ky / len(rows)
        parts.append((st if sub is Ellipsis else "%s %s" % (st, sub), C, ky, len(rows)))
    mo = FRAME * OUT[con] / NRAND
    return tp / (tp + miss + mo) if tp + miss + mo else float("nan"), parts, mo


def errors(con):
    c = collections.Counter()
    for it, d in items.items():
        if d["construction"] != con:
            continue
        f = final(it)
        if not f or f["true"] not in ("yes", "no"):
            continue
        hit = d["sample"].startswith("hit")
        if (hit and f["true"] == "no") or (not hit and f["true"] == "yes"):
            c[("false hit" if hit else "miss (%s)" % d["sample"], f.get("error") or "unclassified")] += 1
    return c


def clean(note):
    if not note:
        return ""
    keep = re.findall(r"governing verb: ?([Ͱ-Ͽἀ-῿]+)", note)
    s = GREEK.sub("…", note)
    for k in keep:
        s = s.replace("governing verb: …", "governing verb: " + k, 1)
    return s


def write_tsv():
    os.makedirs("v2/out", exist_ok=True)
    where = {i: k for k, ids in REVIEW.items() if k in ("unsure", "blind") for i in ids}
    cols = ["item", "work", "author", "citation", "sent_id", "anchor", "construction", "sample",
            "detector_verdict", "detector_subtype", "prior_item_2607", "agent_true", "agent_subtype",
            "agent_smyth", "agent_error", "agent_note", "justin_review", "justin_true", "justin_subtype",
            "justin_smyth", "justin_error", "justin_note"]
    with open("v2/out/occurrences.tsv", "w", encoding="utf-8") as f:
        f.write("\t".join(cols) + "\n")
        for it in sorted(items):
            d, a = items[it], J.get(it, {})
            h, src = justin(it)
            h = h or {}
            rev = ("2615-%s-%s" % (where.get(it, ""), h.get("source", "justin"))) if src == "2615" else ("2607" if src else "")
            row = [it, d["work"], d["author"], d["ref"], d["sent_id"], d["anchor"], d["construction"],
                   d["sample"], d["stratum"], d["subtype"] or "", d.get("prior_item") or "",
                   a.get("true", ""), a.get("subtype") or "", a.get("smyth") or "", a.get("error") or "",
                   clean(a.get("note")), rev, h.get("true", ""), h.get("subtype") or "",
                   h.get("smyth") or "", h.get("error") or "", clean(h.get("note")) if src == "2615" else ""]
            f.write("\t".join(str(x).replace("\t", " ") for x in row) + "\n")


def report():
    L = ["# Results (generated by `analyze.py`)", ""]
    L += ["## Precision against the 0.90 bar", "",
          "| Construction | Corpus hits | Precision, pooled random hits (95% CI) | Plato / Xenophon | Ceiling | Verdict |",
          "|---|---:|---|---|---|---|"]
    for con in CONS:
        rows = judged(con, {"hit"})
        k, n = yes(rows)
        kc, nc = yes(rows, True)
        pa = " / ".join(fmt(*yes(judged(con, {"hit"}, au=au))) for au in ("Plato", "Xenophon"))
        L.append("| %s | %d | %s | %s | %s | %s |" % (con, CNT[(con, "hit")], fmt(k, n), pa, fmt(kc, nc), verdict(k, n)))
    L += ["", "## Subtype precision (random hits plus top-up)", "",
          "| Construction | Subtype | Corpus hits | Subtype correct | Construction correct | Verdict |", "|---|---|---:|---|---|---|"]
    for con in ("articular_infinitive", "result", "indirect_statement"):
        for sub in sorted({d["subtype"] for d in items.values() if d["construction"] == con and d["sample"].startswith("hit")}, key=str):
            k, n, ok = subtype_prec(con, sub)
            L.append("| %s | %s | %d | %s | %s | %s |" % (con, sub or "none", CNTSUB[(con, "hit", sub)], fmt(k, n), fmt(ok, n),
                                                       verdict(k, n) if sub else "(abstention)"))
    L += ["", "## Recall", "", "| Construction | Recall | Ceiling | Miss strata (pool, true / judged) | Outside misses (random sentences) |", "|---|---|---|---|---|"]
    for con in CONS:
        if con in AMENDED:
            r, parts, mo = recall(con)
            rc, _, _ = recall(con, True)
            L.append("| %s | %.2f | %.2f | %s | %d of %d occurrences (≈%.0f corpus) |" % (
                con, r, rc, "; ".join("%s %d, %d/%d" % p for p in parts), OUT[con], OCC[con], mo))
        else:
            L.append("| %s | %.2f (carried from #2607) | | | |" % (con, CARRIED_RECALL[con]))
    L.append("")
    ex = judged("conditional", {"excluded"})
    L.append("Conditional \"whether\" exclusion: %d corpus candidates; rightly excluded %s." % (
        CNT[("conditional", "excluded")], fmt(sum(1 for d, f in ex if f["true"] == "no"), len(ex))))
    k, n = yes(judged("genitive_absolute", {"hit"}))
    pe = judged("genitive_absolute", {"excluded"})
    rb = [(d, f) for d, f in pe if d["subtype"] == "B-readmit"]
    H = CNT[("genitive_absolute", "hit")]
    pb = (H * k / n + sum(1 for d, f in rb if f["true"] == "yes")) / (H + CNTSUB[("genitive_absolute", "excluded", "B-readmit")]) if n else float("nan")
    L.append("Genitive absolute perception exclusion: %d candidates, rightly excluded %s. Variant B readmits %d (%d judged true), so variant B precision ≈ %.3f against variant A %.3f." % (
        len(pe), fmt(sum(1 for d, f in pe if f["true"] == "no"), len(pe)), len(rb),
        sum(1 for d, f in rb if f["true"] == "yes"), pb, k / n if n else float("nan")))
    L += ["", "## Errors", ""]
    for con in CONS:
        e = errors(con)
        L.append("- **%s:** %s" % (con, "; ".join("%s / %s: %d" % (a, b, c) for (a, b), c in sorted(e.items())) or "none"))
    unsure = collections.Counter(items[i]["construction"] for i in J if J[i]["true"] == "unsure")
    left = collections.Counter(items[i]["construction"] for i in items if (final(i) or {}).get("true") == "unsure")
    L += ["", "Agent unsure: %s. Still unsure after review (dropped): %s." % (dict(unsure), dict(left)), ""]
    # agreement
    L += ["## Agreement", "", "| Construction | Blind spot-check (Justin vs agent) | Justin #2607 vs agent now (re-drawn items) | Agent now vs agent #2607 |", "|---|---|---|---|"]
    blind = set(REVIEW.get("blind", []))
    tb = [0, 0]
    for con in CONS:
        b = [i for i in blind if items[i]["construction"] == con and i in H2 and H2[i]["true"] in ("yes", "no", "unsure")]
        bk = sum(1 for i in b if H2[i]["true"] == J[i]["true"])
        tb[0] += bk
        tb[1] += len(b)
        pj = [i for i, d in items.items() if d["construction"] == con and d.get("prior_item") in H1 and i in J]
        pk = sum(1 for i in pj if H1[items[i]["prior_item"]]["true"] == J[i]["true"])
        pa = [i for i, d in items.items() if d["construction"] == con and d.get("prior_item") in J1 and i in J]
        ak = sum(1 for i in pa if J1[items[i]["prior_item"]]["true"] == J[i]["true"])
        L.append("| %s | %s | %s | %s |" % (con, fmt(bk, len(b)), fmt(pk, len(pj)), fmt(ak, len(pa))))
    L.append("| **all** (Justin, plus 3 reference-settled items) | %s | | |" % fmt(*tb))
    L.append("")
    open("v2/out/results.md", "w", encoding="utf-8").write("\n".join(L) + "\n")
    print("\n".join(L))


def build_review(seed=26151):
    rng = random.Random(seed)
    unsure = sorted(i for i in J if J[i]["true"] == "unsure")
    blind = []
    for con in CONS:
        pool = sorted(i for i, d in items.items() if d["construction"] == con and i in J
                      and J[i]["true"] != "unsure" and not justin(i)[0])
        blind += rng.sample(pool, 6)
    rng.shuffle(blind)
    os.makedirs("v2/review", exist_ok=True)
    json.dump({"unsure": unsure, "blind": blind}, open("v2/review/set.json", "w"), indent=1)
    trees = {}
    for path in sorted(glob.glob("data/*.conllu")):
        w = path.rsplit("/", 1)[-1].split(".perseus")[0]
        for s in d2.read(path):
            trees[(w, s["meta"]["sent_id"])] = s

    def text(d):
        s = trees[(d["work"], d["sent_id"])]
        rs = d2.real_order(s)
        a = next(n for n in s["nodes"] if n.tid == d["anchor"])
        ext = set(d2.subtree(a))
        k = rs.index(a) if a in rs else 0
        lo, hi = max(0, k - 18), min(len(rs), k + 19)
        w = [("⟦%s⟧" % n.form) if n is a else (("‹%s›" % n.form) if n in ext else n.form) for n in rs[lo:hi]]
        return ("… " if lo else "") + " ".join(w) + (" …" if hi < len(rs) else ""), a

    out = []
    for k, it in enumerate(unsure, 1):
        d = items[it]
        t, a = text(d)
        out.append("**U%d. %s** · %s · %s %s · detector: %s%s\n%s\nAgent (unsure): %s" % (
            k, it, d["construction"], d["author"], d["ref"], d["stratum"], (" " + d["subtype"]) if d["subtype"] else "",
            t, J[it].get("note")))
    open("v2/review/unsure.md", "w", encoding="utf-8").write("\n\n".join(out) + "\n")
    out = []
    for k, it in enumerate(blind, 1):
        d = items[it]
        t, a = text(d)
        q = {"articular_infinitive": " (subtype: article case)", "result": " (subtype: natural / actual)",
             "indirect_statement": " (subtype: infinitive / ὅτι-ὡς)"}.get(d["construction"], "")
        out.append("**B%d. %s** · is there a true **%s**%s anchored at ⟦%s⟧ (or the clause or infinitive it belongs to)? · %s %s\n%s" % (
            k, it, d["construction"], q, a.form, d["author"], d["ref"], t))
    open("v2/review/blind.md", "w", encoding="utf-8").write("\n\n".join(out) + "\n")
    return len(unsure), len(blind)


if __name__ == "__main__":
    if "--review" in sys.argv:
        print(build_review())
        REVIEW = json.load(open("v2/review/set.json"))
    write_tsv()
    report()
