"""Evaluate HamleDT's identify_coap_members heuristic against gold _CO/_AP suffixes.

Usage: python -I coord_heuristic_eval.py AGDT_XML [AGDT_XML ...]

Simulates OGA-style input (suffixes deleted) on AGDT 2.1 trees. For each
COORD/APOS node, candidates are children whose relation is not AuxG/AuxX/AuxY
(as in Treex::Block::HamleDT::Harmonize::identify_coap_members at ufal/treex
f5889e47). If there are <=2 candidates all are members; otherwise members are
candidates with the most frequent label (ties -> earliest), plus nested
COORD/APOS/ExD. Gold: a child is a member if its relation carries _CO/_AP, or
it is AuxP/AuxC and a descendant through AuxP/AuxC chains carries it.
Prints counts only.
"""
import collections
import re
import sys
import xml.etree.ElementTree as ET

SUF = re.compile(r"(_CO|_AP)+$", re.I)


def base(rel):
    return SUF.sub("", rel or "")


def joker(rel):
    return rel.startswith("ExD") or rel in ("COORD", "APOS")


def main():
    st = collections.Counter()
    for path in sys.argv[1:]:
        root = ET.parse(path).getroot()
        for s in root.iter("sentence"):
            words = s.findall("word")
            kids = collections.defaultdict(list)
            for w in words:
                kids[w.get("head")].append(w)

            def gold(w, depth=0):
                if SUF.search(w.get("relation") or ""):
                    return True
                if depth < 10 and base(w.get("relation")) in ("AuxP", "AuxC"):
                    return any(gold(k, depth + 1) for k in kids[w.get("id")])
                return False

            for w in words:
                if base(w.get("relation")) not in ("COORD", "APOS"):
                    continue
                cand = [k for k in sorted(kids[w.get("id")], key=lambda x: int(x.get("id")))
                        if base(k.get("relation")) not in ("AuxG", "AuxX", "AuxY")]
                if len(cand) <= 2:
                    pred = {k.get("id") for k in cand}
                    bucket = "le2"
                else:
                    count, first = collections.Counter(), {}
                    for i, k in enumerate(cand):
                        r = base(k.get("relation"))
                        if joker(r):
                            continue
                        count[r] += 1
                        first.setdefault(r, i)
                    win = min(count, key=lambda r: (-count[r], first[r])) if count else ""
                    pred = {k.get("id") for k in cand
                            if base(k.get("relation")) == win or joker(base(k.get("relation")))}
                    bucket = "gt2"
                gold_ids = {k.get("id") for k in cand if gold(k)}
                for b in ("all", bucket):
                    st[b + "_coap"] += 1
                    st[b + "_exact"] += pred == gold_ids
                    st[b + "_gold"] += len(gold_ids)
                    st[b + "_pred"] += len(pred)
                    st[b + "_tp"] += len(pred & gold_ids)
    print(dict(st))
    for b in ("all", "le2", "gt2"):
        if st[b + "_coap"]:
            print(f"{b}: n={st[b+'_coap']} exact={st[b+'_exact']/st[b+'_coap']:.3f} "
                  f"member_P={st[b+'_tp']/max(1,st[b+'_pred']):.3f} "
                  f"member_R={st[b+'_tp']/max(1,st[b+'_gold']):.3f}")


if __name__ == "__main__":
    main()
