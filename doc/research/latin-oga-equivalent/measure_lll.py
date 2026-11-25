"""Lemma and coarse-POS agreement of lascivaroma/latin-lemmatized-texts (Pie-Extended LASLA model)
with LDT 2.1 and UD Latin-PROIEL gold, on the Works they share.

usage: python3 -I measure_lll.py <lll_xml_dir> <ldt_dir> <proiel_dir> <out.json>
  lll_xml_dir  directory with the 'urn:cts:latinLit:<id>.xml' files of release 0.1.2
The model was trained on LASLA, which holds these Works, so the figures are
seen-data agreement, not accuracy. Enclitic duplicates ({...}) are skipped, and
a token the gold splits (X-que) simply fails to align. Aggregates only.
"""
import glob
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import Tok, _finish, align, norm_lemma, read_ldt  # noqa: E402
from measure_proiel import UPOS2AGDT, WORKS, read_proiel  # noqa: E402

LASLA2AGDT = [("NOM", "n"), ("VER", "v"), ("ADJ", "a"), ("ADV", "d"), ("CON", "c"), ("PRE", "r"),
              ("PRO", "p"), ("INJ", "e"), ("PUNC", "u")]
W = re.compile(r'<w ([^>]*)>([^<]*)</w>')
ATTR = re.compile(r'(\w+)="([^"]*)"')


def read_lll(path):
    toks = []
    txt = open(path, encoding="utf8").read()
    for m in W.finditer(txt):
        a = dict(ATTR.findall(m.group(1)))
        form = m.group(2)
        if form.startswith("{"):
            continue
        pos = "x"
        for pre, ag in LASLA2AGDT:
            if a.get("pos", "").startswith(pre):
                pos = ag
                break
        if a.get("pos", "").startswith("ADJcar"):
            pos = "m"
        toks.append(Tok(form, a.get("lemma", ""), pos, {}, -1, "", 0, pos == "u"))
    return _finish(toks)


def pct(a, b):
    return round(100 * a / b, 2) if b else None


def cmp(gold, lll):
    amap = align(gold, lll)
    n = lem = pos = 0
    for gi, li in amap.items():
        g, o = gold[gi], lll[li]
        if g.punct or g.skip:
            continue
        n += 1
        lem += norm_lemma(g.lemma) == norm_lemma(o.lemma)
        pos += g.pos == o.pos or {g.pos, o.pos} == {"a", "p"}
    return dict(n=n, lemma=pct(lem, n), pos_lenient=pct(pos, n), gold=sum(1 for t in gold if not t.punct))


def main(ldir, gdir, pdir, out):
    res = {"ldt": {}, "proiel": {}}
    for gp in sorted(glob.glob(os.path.join(gdir, "*.xml"))):
        wk = os.path.basename(gp)[:-4]
        wk2 = {"phi1351.phi005": "phi1351.phi004"}.get(wk, wk)
        fs = sorted(glob.glob(os.path.join(ldir, f"*{wk2}.perseus-lat*.xml")))
        if not fs:
            res["ldt"][wk] = {"error": "no file"}
            continue
        res["ldt"][wk] = cmp(read_ldt(gp), read_lll(fs[-1]))
    blocks = read_proiel(sorted(glob.glob(os.path.join(pdir, "*.conllu"))))
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
        fs = sorted(glob.glob(os.path.join(ldir, f"*{wk}.perseus-lat*.xml")))
        res["proiel"][wk] = cmp(gold, read_lll(fs[-1])) if fs else {"error": "no file"}
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    for k in res:
        for w, v in res[k].items():
            print(k, w, v)


if __name__ == "__main__":
    main(*sys.argv[1:5])
