"""Agreement of the Scaife LatinCy tagging shards with UD Latin-Perseus gold (UD scheme).

usage: python3 -I measure_scaife.py <ud_perseus_dir> <shard_dir...> <out.json>
  ud_perseus_dir  directory holding la_perseus-ud-train.conllu and -test.conllu
  shard_dir       checkouts of scaife-viewer/tagging-shard-03 and -04

The UD test split is the one the LatinCy pipeline did not train on; the train
split is reported only to show how much seen data inflates the figure. Scaife
tags each citation chunk on its own, so a sentence that crosses a chunk
boundary (every verse line) loses its head; that shows up in UAS/LAS here.
Punctuation excluded. Aggregates only.
"""
import collections
import glob
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import Tok, align, norm_lemma, read_scaife, _finish  # noqa: E402


def read_ud_split(path):
    """Return {work: [(sentence number, [token rows])...]} from sent_id 'phi.phi.ed.tb.xml@N'."""
    out = collections.defaultdict(list)
    sid = None
    rows = []
    for line in open(path, encoding="utf8"):
        line = line.rstrip("\n")
        if line.startswith("# sent_id"):
            sid = line.split("=", 1)[1].strip()
        elif not line:
            if rows and sid:
                m = re.match(r"(phi\d+\.[a-z0-9]+|tlg\d+\.[a-z0-9]+)\.perseus-lat\d\.tb\.xml@(\d+)", sid)
                if m:
                    out[m.group(1)].append((int(m.group(2)), rows))
            rows = []
            sid = None if not line.startswith("# sent_id") else sid
        elif not line.startswith("#"):
            c = line.split("\t")
            if "-" in c[0] or "." in c[0]:
                continue
            rows.append(c)
    return out


def gold_tokens(sents):
    toks = []
    for _, rows in sorted(sents, key=lambda x: x[0]):
        base = len(toks)
        for c in rows:
            feats = dict(kv.partition("=")[::2] for kv in c[5].split("|")) if c[5] != "_" else {}
            h = int(c[6])
            toks.append(Tok(c[1], c[2], c[3], feats, base + h - 1 if h else -1, c[7], 0, c[3] == "PUNCT"))
    return _finish(toks)


def pct(a, b):
    return round(100 * a / b, 2) if b else None


def main(udir, *rest):
    out = rest[-1]
    shards = rest[:-1]
    res = {}
    for split in ("test", "train"):
        gold = read_ud_split(os.path.join(udir, f"la_perseus-ud-{split}.conllu"))
        agg = collections.Counter()
        per = {}
        for wk, sents in sorted(gold.items()):
            files = []
            for sd in shards:
                files += glob.glob(os.path.join(sd, wk.split(".")[0], wk.split(".")[1], f"{wk}.perseus-lat*.tagged.tsv"))
            if not files:
                per[wk] = {"error": "no Scaife file"}
                continue
            g = gold_tokens(sents)
            sc = read_scaife(sorted(files)[-1])
            amap = align(g, sc)
            n = lem = pos = hn = uas = las = fn = fk = 0
            for gi, si in amap.items():
                a, b = g[gi], sc[si]
                if a.punct:
                    continue
                n += 1
                lem += norm_lemma(a.lemma) == norm_lemma(b.lemma)
                pos += a.pos == b.pos
                fn += 1
                fa = {k: v for k, v in a.feats.items() if k != "Degree"}
                fb = {k: v for k, v in b.feats.items() if k != "Degree"}
                fk += fa == fb
                if a.head == -1:
                    ok, have = b.head == -1, True
                elif a.head in amap:
                    ok, have = amap[a.head] == b.head, True
                else:
                    ok, have = False, False
                if have:
                    hn += 1
                    uas += ok
                    las += ok and a.rel.split(":")[0] == b.rel.split(":")[0]
            per[wk] = dict(gold_tokens=sum(1 for t in g if not t.punct), aligned=n, n=n, lemma=lem,
                           upos=pos, feats_exact=fk, head_n=hn, uas=uas, las=las)
            for k, v in per[wk].items():
                if k not in ("gold_tokens",):
                    agg[k] += v
            agg["gold_tokens"] += per[wk]["gold_tokens"]
        res[split] = dict(per_work=per, total=dict(agg),
                          pct=dict(lemma=pct(agg["lemma"], agg["n"]), upos=pct(agg["upos"], agg["n"]),
                                   feats_exact=pct(agg["feats_exact"], agg["n"]),
                                   uas=pct(agg["uas"], agg["head_n"]), las=pct(agg["las"], agg["head_n"]),
                                   aligned=pct(agg["n"], agg["gold_tokens"])))
        print(split, dict(agg), res[split]["pct"])
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)


if __name__ == "__main__":
    main(*sys.argv[1:])
