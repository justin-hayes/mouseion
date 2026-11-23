"""Profile the AGDT-style syntax in an oga_src CoNLL-U file for UD conversion.

Usage: python -I oga_probe.py OGA_CONLLU

Ellipsis nodes are rows whose FORM is a "[n]" placeholder (POS "-").
Prints counts only (no source text).
"""
import collections
import re
import sys

ELL = re.compile(r"^\[\d+\]$")
AUX = {"AuxX", "AuxG", "AuxK", "AuxY", "AuxZ"}


def sentences(path):
    rows = []
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.rstrip("\n")
            if not line:
                if rows:
                    yield rows
                rows = []
            elif not line.startswith("#"):
                c = line.split("\t")
                rows.append({"id": int(c[0]), "form": c[1], "pos": c[3],
                             "head": int(c[6]), "rel": c[7]})
    if rows:
        yield rows


def main():
    st = collections.Counter()
    rels = collections.Counter()
    coord_kids = collections.Counter()
    head_on_ell_rel = collections.Counter()
    ell_rel = collections.Counter()
    ell_depth = collections.Counter()
    for rows in sentences(sys.argv[1]):
        st["sentences"] += 1
        byid = {r["id"]: r for r in rows}
        ells = {r["id"] for r in rows if ELL.match(r["form"])}
        kept = [r for r in rows if r["id"] not in ells]
        st["kept_tokens"] += len(kept)
        st["ellipsis_nodes"] += len(ells)
        if ells:
            st["sentences_with_ellipsis"] += 1
        for r in rows:
            rels[r["rel"]] += 1
        for e in ells:
            ell_rel[byid[e]["rel"]] += 1
            # chain: ellipsis node headed by another ellipsis node
            if byid[e]["head"] in ells:
                st["ellipsis_headed_by_ellipsis"] += 1
        for r in kept:
            if r["head"] in ells:
                st["kept_headed_by_ellipsis"] += 1
                head_on_ell_rel[r["rel"]] += 1
            if "_" in r["rel"]:
                st["kept_rel_with_suffix"] += 1
        # coordination member ambiguity without _CO: children of a COORD/APOS
        # head that are not delimiters are either conjuncts or shared modifiers.
        children = collections.defaultdict(list)
        for r in rows:
            children[r["head"]].append(r)
        for r in rows:
            if r["rel"] not in ("COORD", "APOS"):
                continue
            st[r["rel"] + "_heads"] += 1
            cand = [k for k in children[r["id"]] if k["rel"] not in AUX]
            labels = collections.Counter(k["rel"] for k in cand)
            if len(cand) <= 1:
                kind = "<=1 non-delimiter child"
            elif len(labels) == 1:
                kind = "all same label"
            elif len(cand) == 2:
                kind = "2 children, labels differ"
            else:
                kind = ">2 children, labels differ (member vs shared ambiguous)"
            coord_kids[(r["rel"], kind)] += 1
    print("STATS", dict(st))
    print("RELATIONS", rels.most_common())
    print("ELLIPSIS NODE RELATIONS", ell_rel.most_common())
    print("RELATIONS OF KEPT TOKENS HEADED BY ELLIPSIS", head_on_ell_rel.most_common())
    print("COORD/APOS CHILD PATTERNS")
    for k, n in sorted(coord_kids.items()):
        print(" ", k, n)


if __name__ == "__main__":
    main()
