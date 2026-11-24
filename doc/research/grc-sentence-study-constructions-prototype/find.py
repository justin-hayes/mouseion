"""PROTOTYPE (#2609): find real OGA sentences for the Sentence-study construction prototype.

Usage (from ~/.cache/scratch/grc-syntax): python3 -I proto-2609/find.py
Applies the #2608 shipping gate to the v2 detectors and prints candidate sentences.
"""
import os
import sys

sys.path.insert(0, os.path.expanduser("~/.cache/scratch/grc-syntax"))
sys.path.insert(0, os.path.expanduser("~/.cache/scratch/grc-syntax/v2"))
import detect2 as d  # noqa: E402

LABELLED = {("result", "natural"), ("indirect_statement", "infinitive"),
            ("articular_infinitive", "dat"), ("articular_infinitive", "gen")}


def shipped(n):
    out = []
    for name, fn in d.DETECTORS.items():
        r = fn(n)
        if not r:
            continue
        stratum, sub = r
        if name == "conditional" and stratum == "excluded":
            stratum = "hit"  # #2608: "whether" exclusion dropped
        if stratum != "hit":
            continue
        out.append((name, sub if (name, sub) in LABELLED else None))
    return out


def extent(n):
    return {x.i for x in d.subtree(n) if not x.art}


def occurrences(s):
    occ = []
    for n in s["nodes"]:
        for name, sub in shipped(n):
            occ.append({"con": name, "sub": sub, "anchor": n.i, "extent": extent(n)})
    return occ


def text(s, marks=None):
    return " ".join(n.form for n in s["nodes"] if not n.art)


def main():
    files = sorted(os.listdir("data"))
    want = sys.argv[1] if len(sys.argv) > 1 else "nested"
    shown = 0
    for f in files:
        for s in d.read(os.path.join("data", f)):
            real = [n for n in s["nodes"] if not n.art]
            if not (6 <= len(real) <= 26):
                continue
            occ = occurrences(s)
            src = s["meta"].get("source")
            if want == "nested" and len(occ) == 2:
                a, b = occ
                if (a["extent"] < b["extent"] or b["extent"] < a["extent"]):
                    print(src, s["meta"]["sent_id"], [(o["con"], o["sub"]) for o in occ], "|", text(s))
                    shown += 1
            if want == "ellipsis":
                els = [n for n in s["nodes"] if n.art and any(not k.art for k in n.kids) and n.rel != "PRED"]
                if els and len(real) <= 16:
                    print(src, s["meta"]["sent_id"], [(o["con"], o["sub"]) for o in occ], "|", text(s),
                          "| elided:", [(e.rel, e.head.form if e.head else None, [k.form for k in e.kids]) for e in els])
                    shown += 1
            if want == "none" and not occ and 10 <= len(real) <= 18:
                print(src, s["meta"]["sent_id"], "|", text(s))
                shown += 1
            if shown > 60:
                return


main()
