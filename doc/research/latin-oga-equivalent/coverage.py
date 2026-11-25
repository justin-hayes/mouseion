"""Classical coverage of the candidate resources, in Works and words (aggregates only).

usage: python3 -I coverage.py <ola_inventory.json> <lll_corpus.csv> <out.json> <shard_dir...>
A Work is author.work (the Edition suffix is dropped); where a resource holds
several Editions of a Work, the longest one is counted, so a Work is counted once
and its words are those of one Edition. A Work counts as "classical" per classes.py.
"""
import collections
import csv
import glob
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import classes  # noqa: E402


def tally(rows):
    """rows: iterable of (urn 'a.w.edition', words). Longest edition per Work."""
    best = {}
    eds = collections.Counter()
    for urn, w in rows:
        p = urn.split(".")
        if len(p) < 2:
            continue
        wk = ".".join(p[:2])
        eds[wk] += 1
        if wk not in best or w > best[wk]:
            best[wk] = w
    out = {}
    for k in ("core", "borderline", "out"):
        ws = [(wk, w) for wk, w in best.items() if classes.klass(wk) == k]
        out[k] = dict(works=len(ws), words=sum(w for _, w in ws),
                      multi_edition_works=sum(1 for wk, _ in ws if eds[wk] > 1))
    return out, best


def size_buckets(best):
    edges = [(0, 5000, "<5k"), (5000, 20000, "5-20k"), (20000, 60000, "20-60k"),
             (60000, 200000, "60-200k"), (200000, 10**12, ">=200k")]
    b = collections.Counter()
    for wk, w in best.items():
        if classes.klass(wk) == "core":
            for lo, hi, lab in edges:
                if lo <= w < hi:
                    b[lab] += 1
    return dict(b)


def main(inv, lllcsv, out, *shards):
    res = {}
    r = json.load(open(inv))
    ola, olabest = tally((u, x["words"]) for u, x in r.items())
    res["ola"] = dict(by_class=ola, core_size_buckets=size_buckets(olabest))
    # Scaife shards: one tagged.tsv per Work and edition; words = non-PUNCT, non-blank rows
    rows = []
    for sd in shards:
        for f in glob.glob(os.path.join(sd, "*", "*", "*.tagged.tsv")):
            urn = os.path.basename(f)[: -len(".tagged.tsv")]
            n = 0
            for line in open(f, encoding="utf8"):
                c = line.rstrip("\n").split("\t")
                if len(c) >= 4 and c[2].strip() and c[3] != "PUNCT":
                    n += 1
            rows.append((urn, n))
    sc, scbest = tally(rows)
    res["scaife"] = dict(by_class=sc, core_size_buckets=size_buckets(scbest))
    # latin-lemmatized-texts (word counts from its own corpus.csv; punctuation included)
    rows = []
    with open(lllcsv, encoding="utf8") as fh:
        for rec in csv.DictReader(fh):
            urn = rec["File"].split("urn:cts:latinLit:")[-1].rsplit(".xml", 1)[0]
            rows.append((urn, int(rec["Word-count"] or 0)))
    ll, llbest = tally(rows)
    res["lemmatized_texts"] = dict(by_class=ll, core_size_buckets=size_buckets(llbest))
    # overlap of classical core Works
    cs = lambda b: {wk for wk in b if classes.klass(wk) == "core"}  # noqa: E731
    o, s, l_ = cs(olabest), cs(scbest), cs(llbest)
    res["overlap_core_works"] = dict(ola=len(o), scaife=len(s), lemmatized_texts=len(l_),
                                     ola_and_scaife=len(o & s), ola_only_vs_scaife=len(o - s),
                                     scaife_only_vs_ola=len(s - o), all_three=len(o & s & l_),
                                     union=len(o | s | l_))
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    print(json.dumps(res, indent=1))


if __name__ == "__main__":
    main(*sys.argv[1:])
