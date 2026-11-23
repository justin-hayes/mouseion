"""Align AGDT 2.1 XML trees with UD_Ancient_Greek-Perseus sentences.

Usage: python -I align_agdt_ud.py AGDT_DIR UD_CONLLU [UD_CONLLU ...]

UD sent_id is "<agdt file>@<sentence id>". Non-artificial AGDT words are
matched to UD tokens by order within the sentence; a sentence is counted only
when the token counts and every form match after NFC normalisation.
Prints counts only (no source text).
"""
import collections
import os
import sys
import unicodedata
import xml.etree.ElementTree as ET


def nfc(s):
    return unicodedata.normalize("NFC", s or "")


def read_ud(paths):
    sents = {}
    for path in paths:
        sid, toks = None, []
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                line = line.rstrip("\n")
                if line.startswith("# sent_id = "):
                    sid = line[len("# sent_id = "):]
                elif not line:
                    if sid is not None:
                        sents[sid] = toks
                    sid, toks = None, []
                elif not line.startswith("#"):
                    c = line.split("\t")
                    if "-" in c[0] or "." in c[0]:
                        continue
                    toks.append({"id": int(c[0]), "form": c[1], "upos": c[3],
                                 "feats": c[5], "head": int(c[6]), "deprel": c[7]})
    return sents


def split_rel(rel):
    rel = rel or ""
    member = False
    while True:
        for suf in ("_CO", "_AP"):
            if rel.upper().endswith(suf):
                rel = rel[: -len(suf)]
                member = True
                break
        else:
            break
    return rel, member


def main():
    agdt_dir, ud_paths = sys.argv[1], sys.argv[2:]
    ud = read_ud(ud_paths)
    by_file = collections.defaultdict(dict)
    for sid, toks in ud.items():
        f, n = sid.split("@")
        by_file[f][n] = toks
    cross = collections.Counter()          # (agdt base rel, member, ud deprel)
    head_on_ellipsis = collections.Counter()  # ud deprel of tokens whose AGDT head is artificial
    stats = collections.Counter()
    # UD relation of the node whose AGDT parent is AuxC, keyed by AGDT rel
    under_auxc = collections.Counter()
    for fname in sorted(os.listdir(agdt_dir)):
        if fname not in by_file:
            continue
        root = ET.parse(os.path.join(agdt_dir, fname)).getroot()
        for s in root.iter("sentence"):
            toks = by_file[fname].get(s.get("id"))
            if toks is None:
                continue
            stats["sent_candidates"] += 1
            words = s.findall("word")
            real = [w for w in words if w.get("artificial") is None]
            art_ids = {w.get("id") for w in words if w.get("artificial") is not None}
            if len(real) != len(toks) or any(
                nfc(w.get("form")) != nfc(t["form"]) for w, t in zip(real, toks)
            ):
                stats["sent_mismatch"] += 1
                continue
            stats["sent_aligned"] += 1
            stats["artificial_nodes"] += len(art_ids)
            byid = {w.get("id"): w for w in words}
            for w, t in zip(real, toks):
                stats["tokens"] += 1
                base, member = split_rel(w.get("relation"))
                if "ExD" in base:
                    base = "ExD-chain" if "_" in base else "ExD"
                cross[(base, member, t["deprel"])] += 1
                if w.get("head") in art_ids:
                    stats["tokens_head_artificial"] += 1
                    head_on_ellipsis[t["deprel"]] += 1
                parent = byid.get(w.get("head"))
                if parent is not None and split_rel(parent.get("relation"))[0] == "AuxC":
                    under_auxc[(base, t["deprel"])] += 1
    print("STATS", dict(stats))
    print("\nCROSSTAB agdt_rel member ud_deprel count")
    by_rel = collections.defaultdict(collections.Counter)
    for (r, m, d), n in cross.items():
        by_rel[(r, m)][d] += n
    for key in sorted(by_rel, key=lambda k: -sum(by_rel[k].values())):
        tot = sum(by_rel[key].values())
        top = ", ".join(f"{d}:{n}" for d, n in by_rel[key].most_common(6))
        print(f"{key[0]}\t{'member' if key[1] else '-'}\t{tot}\t{top}")
    print("\nUD deprel of tokens whose AGDT head is artificial")
    print(head_on_ellipsis.most_common(15))
    print("\nUnder AuxC: (agdt rel, ud deprel)")
    print(under_auxc.most_common(20))


if __name__ == "__main__":
    main()
