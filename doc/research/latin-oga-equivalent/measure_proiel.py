"""Lemma and coarse-POS agreement of OLA v0.2.0 with UD Latin-PROIEL gold on Works OLA's
model did not train on (as far as its published description says).

usage: python3 -I measure_proiel.py <ola_conllu_dir> <proiel_dir> <out.json>
PROIEL is UD-scheme with its own lemma conventions, so this is an agreement figure,
not an accuracy: lemma is compared after stripping sense digits, case and u/v, i/j.
Attachment is not compared (UD and AGDT attach differently).
Aggregates only.
"""
import glob
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import Tok, align, norm_lemma, read_ola, _finish  # noqa: E402

WORKS = {
    "Commentarii belli Gallici": ("phi0448.phi001", r"Gall\. (\d+)\.(\d+)"),
    "Epistulae ad Atticum": ("phi0474.phi057", r"Book (\d+), letter (\d+)(\w*)"),
    "De officiis": ("phi0474.phi055", r"Book (\d+), section (\d+)"),
}
UPOS2AGDT = {"NOUN": "n", "PROPN": "n", "VERB": "v", "AUX": "v", "ADJ": "a", "ADV": "d",
             "ADP": "r", "PRON": "p", "NUM": "m", "CCONJ": "c", "SCONJ": "c", "PART": "d",
             "INTJ": "e", "PUNCT": "u", "X": "x", "DET": "p"}


def read_proiel(paths):
    blocks = []
    for p in paths:
        cur = None
        for line in open(p, encoding="utf8"):
            line = line.rstrip("\n")
            if line.startswith("# source"):
                cur = {"src": line.split("=", 1)[1].strip(), "toks": [], "id": 0}
                blocks.append(cur)
            elif line.startswith("# sent_id") and cur is not None:
                cur["id"] = int(line.split("=", 1)[1].strip())
            elif line and not line.startswith("#") and cur is not None:
                c = line.split("\t")
                if "-" in c[0] or "." in c[0]:
                    continue
                cur["toks"].append(c)
    return blocks


def pct(a, b):
    return round(100 * a / b, 2) if b else None


def main(odir, pdir, out):
    blocks = read_proiel(sorted(glob.glob(os.path.join(pdir, "*.conllu"))))
    res = {}
    for title, (wk, rx) in WORKS.items():
        sel = []
        for b in blocks:
            if b["src"].startswith(title):
                m = re.search(rx, b["src"])
                if m:
                    key = tuple(int(g) if g.isdigit() else g for g in m.groups())
                    sel.append((key, b["id"], b["toks"]))
        sel.sort(key=lambda x: (x[0], x[1]))
        gold = []
        for _, _, toks in sel:
            for c in toks:
                gold.append(Tok(c[1], c[2], UPOS2AGDT.get(c[3], "x"), {}, -1, c[7], 0, c[3] == "PUNCT"))
        _finish(gold)
        files = glob.glob(os.path.join(odir, f"*.ola.{wk}.*.tok01_*.conllu"))
        if not files:
            res[wk] = {"error": "no OLA file"}
            continue
        ola = read_ola(files[0])
        amap = align(gold, ola)
        n = lem = pos = posd = 0
        for gi, oi in amap.items():
            g, o = gold[gi], ola[oi]
            if g.punct:
                continue
            n += 1
            lem += norm_lemma(g.lemma) == norm_lemma(o.lemma)
            ok = g.pos == o.pos or (g.pos == "p" and o.pos == "a") or (g.pos == "a" and o.pos == "p")
            pos += g.pos == o.pos
            posd += ok
        res[wk] = dict(title=title, sentences=len(sel), gold_tokens=sum(1 for t in gold if not t.punct),
                       aligned=n, lemma=pct(lem, n), pos_exact=pct(pos, n), pos_det_lenient=pct(posd, n),
                       _counts=dict(n=n, lemma=lem, pos=pos, posd=posd))
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    for k, v in res.items():
        print(k, {a: b for a, b in v.items() if not a.startswith("_")})


if __name__ == "__main__":
    main(*sys.argv[1:4])
