"""Profile the AGDT syntax in pinned oga_src v0.2.0 CoNLL-U files.

Usage: python3 -I measure.py OUT_JSON FILE.conllu [FILE.conllu ...]

Reads only the files named on the command line. Writes a JSON summary
(ASCII-escaped) and prints Markdown tables to stdout. Prints no source
text: only counts, labels, POS letters, sent_ids and citation refs.
The script is ASCII-only.
"""

import collections
import json
import sys

AGDT_LABELS = {
    "PRED", "SBJ", "OBJ", "ATR", "ADV", "ATV", "AtvV", "PNOM", "OCOMP",
    "COORD", "APOS", "MWE", "AuxP", "AuxC", "AuxX", "AuxG", "AuxK", "AuxY",
    "AuxZ", "ExD",
}
AUX = {"AuxX", "AuxG", "AuxK", "AuxY", "AuxZ"}
# Mouseion oga-agdt-feats/1 mapping (internal/ancientgreek/ogasrc.go).
MAPPED = {
    3: set("pifla"),   # tense
    4: set("isom"),    # mood
}
SLOT = {1: "person", 2: "number", 3: "tense", 4: "mood", 5: "voice",
        6: "gender", 7: "case", 8: "degree"}


def read(path):
    sents = []
    cur = None
    for line in open(path, encoding="utf-8"):
        line = line.rstrip("\n")
        if line.startswith("#"):
            if line.startswith("# sent_id = "):
                cur["sent_id"] = line[len("# sent_id = "):]
            elif line.startswith("# source = "):
                cur = {"source": line[len("# source = "):], "toks": []}
                sents.append(cur)
            continue
        if not line:
            continue
        c = line.split("\t")
        misc = c[9].split("|")
        cur["toks"].append({
            "id": int(c[0]), "form": c[1], "lemma": c[2], "upos": c[3],
            "postag": c[4], "feats": c[5], "head": int(c[6]),
            "rel": c[7], "self": misc[0], "href": misc[1],
            "ref": misc[2] if len(misc) > 2 else "",
        })
    return sents


def nonproj_arcs(heads, ignore=()):
    """heads: dict node->head over linear positions (keys sorted = order).
    Returns number of non-projective arcs. Nodes in ignore are neither
    checked as dependents nor counted as intervening nodes."""
    order = sorted(heads)
    pos = {n: i + 1 for i, n in enumerate(order)}
    pos[0] = 0
    children = collections.defaultdict(list)
    for d, h in heads.items():
        children[h].append(d)

    def desc(n):
        out, stack = set(), [n]
        while stack:
            x = stack.pop()
            for ch in children.get(x, []):
                if ch not in out:
                    out.add(ch)
                    stack.append(ch)
        return out

    cache = {}
    bad = 0
    for d, h in heads.items():
        if d in ignore:
            continue
        lo, hi = sorted((pos[h], pos[d]))
        if h not in cache:
            cache[h] = desc(h)
        for n in order:
            if lo < pos[n] < hi and n not in cache[h] and n not in ignore:
                bad += 1
                break
    return bad


def has_cycle(heads):
    for start in heads:
        seen = set()
        x = start
        while x != 0:
            if x in seen:
                return True
            seen.add(x)
            x = heads.get(x, 0)
    return False


def profile(path):
    sents = read(path)
    C = collections.Counter
    r = {"sentences": len(sents), "tokens": 0, "kept": 0, "ellipsis": 0}
    rel = C()
    rel_ell = C()
    suffix = C()
    nonagdt = C()
    roots = C()
    root_rel = C()
    pred_count = C()
    pred_nonroot = 0
    bad_head = 0
    misc_mismatch = 0
    id_gap = 0
    cycles = 0
    auxk_parent = C()
    np_full = np_kept = arcs_full = arcs_kept = 0
    sent_np_full = sent_np_kept = 0
    kept_under_ell = 0
    ell_children = C()        # number of kept children per ellipsis node
    ell_kids_any = C()        # number of children (any) per ellipsis node
    ell_root = 0
    ell_chain = C()
    ell_pos = C()
    ell_postag = C()
    reatt_roots = C()          # kept roots per sentence after climb rule
    reatt_noroot = 0
    coord_kids = C()
    coord_upos = C()
    coord_nconj = C()
    coord_nested = 0
    coord_conj_lab_agree = C()
    apos_upos = C()
    apos_kids = C()
    auxc_kids = C()
    auxc_kid_mood = C()
    auxc_upos = C()
    auxc_nonaux = C()
    auxc_lemma = C()
    auxp_kids = C()
    auxp_nonaux = C()
    auxp_kid_case = C()
    auxp_upos = C()
    mood = C()
    tense = C()
    verb_mood = C()
    unmapped = C()
    upos_vs_postag = 0
    feats_has = C()
    pred_parent = C()
    coord_mixed = C()
    np_nopunct = arcs_nopunct = 0
    promo = C()
    slen = C()
    coord_resolved = C()
    slot_vals = collections.defaultdict(C)
    upos_vals = C()
    for s in sents:
        toks = s["toks"]
        n = len(toks)
        r["tokens"] += n
        byid = {t["id"]: t for t in toks}
        ids = [t["id"] for t in toks]
        if ids != list(range(1, n + 1)):
            id_gap += 1
        heads = {t["id"]: t["head"] for t in toks}
        kids = collections.defaultdict(list)
        for t in toks:
            kids[t["head"]].append(t)
        ell = {t["id"] for t in toks if t["upos"] == "-"}
        r["ellipsis"] += len(ell)
        r["kept"] += n - len(ell)
        nroot = 0
        npred = 0
        for t in toks:
            lab = t["rel"]
            base = lab
            for suf in ("_CO", "_AP"):
                if suf in base:
                    suffix[suf] += 1
                    base = base.replace(suf, "")
            (rel_ell if t["id"] in ell else rel)[lab] += 1
            if base not in AGDT_LABELS:
                nonagdt[lab] += 1
            h = t["head"]
            if h != 0 and h not in byid or h == t["id"]:
                bad_head += 1
            # MISC head reference must name the head's document id.
            want = "0" if h == 0 else byid[h]["self"] if h in byid else "?"
            if t["href"] != want:
                misc_mismatch += 1
            if h == 0:
                nroot += 1
                root_rel[lab] += 1
            if base == "PRED":
                npred += 1
                if h != 0:
                    pred_nonroot += 1
                    pred_parent[byid[h]["rel"]] += 1
            upos_vals[t["upos"]] += 1
            for i in range(1, 9):
                slot_vals[SLOT[i]][t["postag"][i]] += 1
            if lab == "AuxK":
                auxk_parent["root" if h == 0 else "rel:" + byid[h]["rel"]] += 1
            pt = t["postag"]
            if t["upos"] != pt[0]:
                upos_vs_postag += 1
            if t["upos"] == "v":
                verb_mood[pt[4]] += 1
                tense[pt[3]] += 1
            mood[pt[4]] += 1
            for i, allowed in MAPPED.items():
                if pt[i] != "-" and pt[i] not in allowed:
                    unmapped[SLOT[i] + "=" + pt[i]] += 1
            for kv in t["feats"].split("|"):
                if kv != "_":
                    feats_has[kv.split("=")[0]] += 1
        roots[nroot] += 1
        pred_count[npred] += 1
        if has_cycle(heads):
            cycles += 1
            continue
        a = nonproj_arcs(heads)
        np_full += a
        punct = {t["id"] for t in toks if t["upos"] == "u"}
        np_nopunct += nonproj_arcs(heads, punct)
        arcs_nopunct += n - len(punct)
        slen[min(n // 25 * 25, 200)] += 1
        r["max_sentence_len"] = max(r.get("max_sentence_len", 0), n)
        arcs_full += n
        sent_np_full += a > 0
        # Ellipsis diagnostics and climb re-attachment over kept tokens.
        for e in ell:
            et = byid[e]
            ell_postag[et["postag"]] += 1
            ck = [k for k in kids[e] if k["id"] not in ell]
            ell_children[min(len(ck), 3)] += 1
            ell_kids_any[min(len(kids[e]), 3)] += 1
            if et["head"] == 0:
                ell_root += 1
            # UD-style promotion: best-ranked kept non-Aux child.
            rank = ["PRED", "SBJ", "OBJ", "PNOM", "OCOMP", "COORD", "APOS",
                    "AuxC", "AuxP", "ADV", "ATV", "AtvV", "ATR", "ExD"]
            cand = [k for k in ck if k["rel"] in rank]
            if not ck:
                promo["no-kept-child"] += 1
            elif not cand:
                promo["only-aux-children"] += 1
            else:
                best = min(rank.index(k["rel"]) for k in cand)
                nb = sum(1 for k in cand if rank.index(k["rel"]) == best)
                promo["unique" if nb == 1 else "tie"] += 1
            ell_pos["last" if e == n else "last-before-final-punct"
                    if e == n - 1 and toks[-1]["upos"] == "u" else "inner"] += 1
        kept_heads = {}
        for t in toks:
            if t["id"] in ell:
                continue
            h = t["head"]
            depth = 0
            if h in ell:
                kept_under_ell += 1
            while h in ell:
                depth += 1
                h = byid[h]["head"]
            if depth:
                ell_chain[depth] += 1
            kept_heads[t["id"]] = h
        kr = sum(1 for h in kept_heads.values() if h == 0)
        reatt_roots[min(kr, 4)] += 1
        if kept_heads and kr == 0:
            reatt_noroot += 1
        if kept_heads:
            a = nonproj_arcs(kept_heads)
            np_kept += a
            arcs_kept += len(kept_heads)
            sent_np_kept += a > 0
        # Recover conjunct function without _CO: a COORD/APOS child that is
        # itself COORD/APOS takes the effective label of its own members.
        memo = {}

        def eff(t, depth=0):
            if t["rel"] not in ("COORD", "APOS") or depth > 50:
                return t["rel"]
            if t["id"] in memo:
                return memo[t["id"]]
            labs = C(eff(k, depth + 1) for k in kids[t["id"]]
                     if k["rel"] not in AUX)
            memo[t["id"]] = labs.most_common(1)[0][0] if labs else "none"
            return memo[t["id"]]

        for t in toks:
            if t["rel"] != "COORD":
                continue
            labs = C(eff(k) for k in kids[t["id"]] if k["rel"] not in AUX)
            if sum(labs.values()) < 2:
                coord_resolved["fewer-than-2-members"] += 1
            elif len(labs) == 1:
                coord_resolved["uniform"] += 1
            elif labs.most_common(1)[0][1] >= 2 and (
                    len(labs.most_common(2)) < 2
                    or labs.most_common(2)[1][1] < labs.most_common(1)[0][1]):
                coord_resolved["majority+others(shared-dependent?)"] += 1
            else:
                coord_resolved["ambiguous"] += 1
        # COORD / APOS / AuxC / AuxP shapes.
        for t in toks:
            lab = t["rel"]
            ch = kids[t["id"]]
            content = [k for k in ch if k["rel"] not in AUX]
            if lab == "COORD":
                coord_upos[t["upos"]] += 1
                coord_nconj[min(len(content), 4)] += 1
                for k in content:
                    coord_kids[k["rel"]] += 1
                if t["head"] in byid and byid[t["head"]]["rel"] == "COORD":
                    coord_nested += 1
                labs = {k["rel"] for k in content}
                coord_conj_lab_agree["same" if len(labs) <= 1 else "mixed"] += 1
                if len(labs) > 1:
                    lc = C(k["rel"] for k in content)
                    coord_mixed["+".join("%s*%d" % kv for kv in
                                         sorted(lc.items()))] += 1
            elif lab == "APOS":
                apos_upos[t["upos"]] += 1
                apos_kids[min(len(content), 3)] += 1
            elif lab == "AuxC":
                auxc_upos[t["upos"]] += 1
                auxc_nonaux[min(len(content), 3)] += 1
                auxc_lemma[t["lemma"]] += 1
                for k in content:
                    auxc_kids[k["rel"]] += 1
                    m = k["postag"][4] if k["upos"] == "v" else (
                        "pos:" + k["upos"])
                    auxc_kid_mood[m] += 1
            elif lab == "AuxP":
                auxp_upos[t["upos"]] += 1
                auxp_nonaux[min(len(content), 3)] += 1
                for k in content:
                    auxp_kids[k["rel"]] += 1
                    auxp_kid_case[k["postag"][7]] += 1
    r.update({
        "rel_kept": rel, "rel_ellipsis": rel_ell, "suffix": suffix,
        "nonagdt": nonagdt, "roots_per_sentence": roots,
        "root_rel": root_rel, "pred_per_sentence": pred_count,
        "pred_nonroot": pred_nonroot, "bad_head": bad_head,
        "misc_head_mismatch": misc_mismatch, "id_gap_sentences": id_gap,
        "cycle_sentences": cycles, "auxk_parent": auxk_parent,
        "nonproj_arcs_full": np_full, "arcs_full": arcs_full,
        "nonproj_sent_full": sent_np_full,
        "nonproj_arcs_kept": np_kept, "arcs_kept": arcs_kept,
        "nonproj_sent_kept": sent_np_kept,
        "kept_under_ellipsis": kept_under_ell,
        "ellipsis_kept_children": ell_children,
        "ellipsis_any_children": ell_kids_any,
        "ellipsis_root": ell_root, "ellipsis_chain_depth": ell_chain,
        "ellipsis_position": ell_pos, "ellipsis_postag": ell_postag,
        "kept_roots_after_climb": reatt_roots,
        "no_kept_root_after_climb": reatt_noroot,
        "coord_upos": coord_upos, "coord_content_children": coord_nconj,
        "coord_child_rel": coord_kids, "coord_nested": coord_nested,
        "coord_conjunct_labels": coord_conj_lab_agree,
        "apos_upos": apos_upos, "apos_content_children": apos_kids,
        "auxc_upos": auxc_upos, "auxc_content_children": auxc_nonaux,
        "auxc_child_rel": auxc_kids, "auxc_child_mood": auxc_kid_mood,
        "auxc_top_lemmas": C(dict(auxc_lemma.most_common(8))),
        "auxp_upos": auxp_upos, "auxp_content_children": auxp_nonaux,
        "auxp_child_rel": auxp_kids, "auxp_child_case": auxp_kid_case,
        "mood_all": mood, "verb_mood": verb_mood, "verb_tense": tense,
        "unmapped_today": unmapped, "upos_ne_postag0": upos_vs_postag,
        "feats_keys": feats_has, "pred_parent": pred_parent,
        "coord_mixed_patterns": C(dict(coord_mixed.most_common(12))),
        "coord_mixed_total_patterns": len(coord_mixed),
        "nonproj_arcs_nopunct": np_nopunct, "arcs_nopunct": arcs_nopunct,
        "coord_resolved": coord_resolved,
        "ellipsis_promotion": promo, "sentence_len_bucket25": slen, "upos_letters": upos_vals,
    })
    for k, v in slot_vals.items():
        r["postag_" + k] = v
    r.update({
    })
    return r


def fmt(c):
    return ", ".join("%s %d" % (k, v) for k, v in
                     sorted(c.items(), key=lambda kv: (-kv[1], str(kv[0]))))


def main():
    out = sys.argv[1]
    res = {}
    for p in sys.argv[2:]:
        name = p.rsplit("/", 1)[-1].split("_oga020")[0]
        res[name] = profile(p)
    with open(out, "w", encoding="ascii") as f:
        json.dump(res, f, indent=1, ensure_ascii=True, sort_keys=True)
    for name, r in res.items():
        print("## " + name)
        for k, v in r.items():
            if isinstance(v, collections.Counter):
                print("- %s: %s" % (k, ascii(fmt(v))[1:-1]))
            else:
                print("- %s: %s" % (k, v))
        print()


if __name__ == "__main__":
    main()
