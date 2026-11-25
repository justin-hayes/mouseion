"""Lemma and coarse-POS agreement of the Scaife LatinCy shards with UD Latin-PROIEL gold.

usage: python3 -I measure_scaife_proiel.py <proiel_dir> <out.json> <shard_dir...>
Same sample and conventions as measure_proiel.py; LatinCy trained on UD Latin-PROIEL,
so the Works are seen. Aggregates only.
"""
import glob
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import Tok, _finish, align, norm_lemma, read_scaife  # noqa: E402
from measure_proiel import UPOS2AGDT, WORKS, read_proiel  # noqa: E402


def pct(a, b):
    return round(100 * a / b, 2) if b else None


def main(pdir, out, *shards):
    blocks = read_proiel(sorted(glob.glob(os.path.join(pdir, "*.conllu"))))
    res = {}
    for title, (wk, rx) in WORKS.items():
        sel = []
        for b in blocks:
            if b["src"].startswith(title):
                m = re.search(rx, b["src"])
                if m:
                    sel.append((tuple(int(g) if g.isdigit() else g for g in m.groups()), b["id"], b["toks"]))
        sel.sort(key=lambda x: (x[0], x[1]))
        gold = _finish([Tok(c[1], c[2], UPOS2AGDT.get(c[3], "x"), {}, -1, c[7], 0, c[3] == "PUNCT")
                        for _, _, ts in sel for c in ts])
        files = []
        for sd in shards:
            a, w = wk.split(".")
            files += glob.glob(os.path.join(sd, a, w, f"{wk}.perseus-lat*.tagged.tsv"))
        if not files:
            res[wk] = {"error": "no Scaife file"}
            continue
        sc = read_scaife(sorted(files)[-1])
        for t in sc:
            t.pos = UPOS2AGDT.get(t.pos, "x")
        amap = align(gold, sc)
        n = lem = pos = 0
        for gi, si in amap.items():
            g, o = gold[gi], sc[si]
            if g.punct:
                continue
            n += 1
            lem += norm_lemma(g.lemma) == norm_lemma(o.lemma)
            pos += g.pos == o.pos or {g.pos, o.pos} == {"a", "p"}
        res[wk] = dict(aligned=n, gold=sum(1 for t in gold if not t.punct), lemma=pct(lem, n), pos_lenient=pct(pos, n))
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    for k, v in res.items():
        print(k, v)


if __name__ == "__main__":
    main(*sys.argv[1:])
