"""Agreement of the released OLA v0.2.0 output with LDT 2.1 gold (AGDT scheme).

usage: python3 -I measure_ola.py <ola_conllu_dir> <ldt_dir> <hf_dir> <out.json>
  ola_conllu_dir  OLA conllu/ directory (workspace/conllu.zip)
  ldt_dir         PerseusDL/treebank_data v2.1/Latin/texts, files renamed <phiA.phiW>.xml
  hf_dir          bowphs/latinpipe-evalatin conllus/{dev,test}.120.conllu (held-out sentence lists)

Three subsets per Work: all aligned tokens, the HF dev sentences, the HF test
sentences. The two HF sets are the model's own held-out data, so the test set
is the only one the OLA model never trained on. Punctuation is excluded from
every figure; head and relation scores use only tokens whose gold head aligns.
Aggregates only.
"""
import collections
import glob
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import align, norm_form, norm_lemma, read_ldt, read_ola  # noqa: E402


OLA_ID = {"phi1351.phi005": "phi1351.phi004"}


def hf_sets(path):
    """doc urn -> list of normalised form sequences, per held-out sentence."""
    out = collections.defaultdict(list)
    doc = None
    cur = []
    for line in open(path, encoding="utf8"):
        if line.startswith("# doc_id"):
            doc = line.split("=", 1)[1].strip()
        if line.startswith("#"):
            continue
        line = line.rstrip("\n")
        if not line:
            if cur:
                out[doc].append(tuple(cur))
            cur = []
            continue
        c = line.split("\t")
        if c[0].isdigit():
            f = norm_form(c[1])
            if f:
                cur.append(f)
    if cur:
        out[doc].append(tuple(cur))
    return out


def score(gold, ola, amap, sel):
    """sel: set of gold token indices to score."""
    n = lem = pos = full = hd = rel = hdn = 0
    feats = collections.Counter()
    featn = collections.Counter()
    for gi in sel:
        g = gold[gi]
        if g.punct or g.skip or gi not in amap:
            continue
        o = ola[amap[gi]]
        n += 1
        lem += norm_lemma(g.lemma) == norm_lemma(o.lemma)
        pos += g.pos == o.pos
        ok = g.pos == o.pos
        for k in set(g.feats) | set(o.feats):
            featn[k] += 1
            if g.feats.get(k) == o.feats.get(k):
                feats[k] += 1
            else:
                ok = False
        full += ok
        # head / relation: gold head must be aligned (or root)
        if g.head == -1:
            hok = o.head == -1
            have = True
        elif g.head in amap:
            hok = amap[g.head] == o.head
            have = True
        else:
            have = False
        if have:
            hdn += 1
            hd += hok
            rel += hok and g.rel == o.rel
    return dict(n=n, lemma=lem, upos=pos, alltags=full, head_n=hdn, uas=hd, las=rel,
                feat_n=dict(featn), feat_ok=dict(feats))


def merge(a, b):
    for k in ("n", "lemma", "upos", "alltags", "head_n", "uas", "las"):
        a[k] = a.get(k, 0) + b[k]
    for k in ("feat_n", "feat_ok"):
        d = a.setdefault(k, {})
        for kk, v in b[k].items():
            d[kk] = d.get(kk, 0) + v
    return a


def pct(a, b):
    return round(100 * a / b, 2) if b else None


def main(odir, ldir, hdir, out):
    dev = hf_sets(os.path.join(hdir, "dev.120.conllu"))
    test = hf_sets(os.path.join(hdir, "test.120.conllu"))

    def by_work(sets):
        d = collections.defaultdict(list)
        for doc, ss in sets.items():
            m = re.search(r"(phi\d+\.[a-z0-9]+)\.perseus-lat1", doc)
            if m:
                d[m.group(1)].extend(ss)
        return d

    dev, test = by_work(dev), by_work(test)
    res = {}
    for gp in sorted(glob.glob(os.path.join(ldir, "*.xml"))):
        wk = os.path.basename(gp)[:-4]
        # the OLA file carrying the same Work (all editions that exist)
        # LDT files a Work under a CTS id that Perseus gives to another Work:
        # phi1351.phi005 holds Historiae 1.1-1.21, which Perseus files as phi1351.phi004
        ow = OLA_ID.get(wk, wk)
        cands = glob.glob(os.path.join(odir, f"*.ola.{ow}.*.tok01_*.conllu"))
        if not cands:
            res[wk] = dict(error="no OLA file")
            continue
        gold = read_ldt(gp)
        for op in sorted(cands):
            ola = read_ola(op)
            amap = align(gold, ola)
            ed = re.search(r"\.ola\.[a-z0-9]+\.[a-z0-9]+\.([a-z0-9-]+)\.tok01", os.path.basename(op)).group(1)
            # sentence index -> forms for gold
            sents = collections.defaultdict(list)
            for t in gold:
                sents[t.sent].append(t.gid)
            fseq = {s: tuple(f for f in (norm_form(gold[i].form) for i in ids) if f) for s, ids in sents.items()}
            idx = collections.defaultdict(list)
            for s, q in fseq.items():
                idx[q].append(s)

            def pick(sets):
                chosen, found = set(), 0
                for q in sets.get(wk, []):
                    ss = idx.get(q)
                    if ss:
                        found += 1
                        chosen.update(sents[ss[0]])
                return chosen, found, len(sets.get(wk, []))

            tsel, tf, tn = pick(test)
            dsel, df, dn = pick(dev)
            allsel = set(range(len(gold)))
            r = dict(gold_tokens=len(gold), ola_tokens=len(ola),
                     aligned=len(amap), edition=ed,
                     test_sent=[tf, tn], dev_sent=[df, dn],
                     all=score(gold, ola, amap, allsel),
                     test=score(gold, ola, amap, tsel),
                     dev=score(gold, ola, amap, dsel))
            res[f"{wk}.{ed}"] = r
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    tot = {"all": {}, "test": {}, "dev": {}}
    best = {}
    for k, v in res.items():
        if "error" in v:
            continue
        wk = k.rsplit(".", 1)[0]
        if wk not in best or v["aligned"] > res[best[wk]]["aligned"]:
            best[wk] = k  # one edition per Work, so no Work is counted twice
    for k in best.values():
        v = res[k]
        for s in tot:
            merge(tot[s], v[s])
    for s, v in tot.items():
        print(s, "n", v.get("n"), "lemma", pct(v.get("lemma", 0), v.get("n", 0)),
              "upos", pct(v.get("upos", 0), v.get("n", 0)),
              "alltags", pct(v.get("alltags", 0), v.get("n", 0)),
              "uas", pct(v.get("uas", 0), v.get("head_n", 0)),
              "las", pct(v.get("las", 0), v.get("head_n", 0)))
    json.dump(tot, open(out.replace(".json", "_total.json"), "w"), indent=1, sort_keys=True)


if __name__ == "__main__":
    main(*sys.argv[1:5])
