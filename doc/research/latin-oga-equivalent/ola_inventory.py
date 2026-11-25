"""Per-Work inventory of the OLA v0.2.0 CoNLL-U files (aggregates only).

usage: python3 -I ola_inventory.py <conllu-dir> <urn_cts.xml> <out.json>
Reads the files, writes counts per Work. No token text is written.
"""
import collections
import glob
import json
import os
import re
import sys
import xml.etree.ElementTree as ET

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import classes  # noqa: E402

AGDT_REL = set("SBJ AuxY PRED ATV ATR AuxX AuxC AuxZ OBJ ADV APOS PNOM COORD AuxK AuxP AtvV AuxV ExD OCOMP AuxG".split())
SINGLE_OK = {"a", "e", "o"}


def base_rel(r):
    r = re.sub(r"_(CO|AP)$", "", r)
    r = re.sub(r"_(CO|AP)$", "", r)
    return r


def main(cdir, urnxml, out):
    meta = {}
    for w in ET.parse(urnxml).getroot():
        d = {c.tag: (c.text or "") for c in w}
        meta.setdefault(d["urn_cts"], d)
    res = {}
    for f in sorted(glob.glob(os.path.join(cdir, "*.conllu"))):
        urn = re.search(r"\.ola\.(.+?)\.tok01_", os.path.basename(f)).group(1)
        toks = punct = sents = roots = badrel = single = multiroot_s = 0
        cur_roots = 0
        sentlens = []
        cur = 0
        for line in open(f, encoding="utf8"):
            if line.startswith("#"):
                continue
            line = line.rstrip("\n")
            if not line:
                if cur:
                    sents += 1
                    sentlens.append(cur)
                    if cur_roots != 1:
                        multiroot_s += 1
                cur = 0
                cur_roots = 0
                continue
            c = line.split("\t")
            toks += 1
            cur += 1
            if c[3] == "u":
                punct += 1
            else:
                if len(c[1]) == 1 and c[1].isalpha() and c[1].lower() not in SINGLE_OK:
                    single += 1
            if c[6] == "0":
                cur_roots += 1
                roots += 1
            if base_rel(c[7]) not in AGDT_REL:
                badrel += 1
        if cur:
            sents += 1
            sentlens.append(cur)
        m = meta.get(urn, {})
        res[urn] = dict(
            tokens=toks, punct=punct, words=toks - punct, sentences=sents,
            single_letter=single, badrel=badrel, nonone_root_sents=multiroot_s,
            class_=classes.klass(urn), author_id=classes.author_of(urn),
            author=m.get("author", ""), title=m.get("title_labels", ""),
            edition=urn.split(".", 2)[2] if urn.count(".") >= 2 else "",
            max_sent=max(sentlens) if sentlens else 0,
        )
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    print(len(res), "works", sum(r["tokens"] for r in res.values()), "tokens")


if __name__ == "__main__":
    main(*sys.argv[1:4])
