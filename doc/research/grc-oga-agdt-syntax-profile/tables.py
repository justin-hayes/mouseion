"""Render per-work Markdown tables from measure.py JSON output.

Usage: python3 -I tables.py OUT_JSON
ASCII-only.
"""

import json
import sys

WORKS = [
    ("tlg0032.tlg006.perseus-grc2", "Xen. Anab."),
    ("tlg0059.tlg002.perseus-grc2", "Pl. Ap."),
    ("tlg0059.tlg003.perseus-grc2", "Pl. Crito"),
    ("tlg0059.tlg004.perseus-grc2", "Pl. Phd."),
    ("tlg0059.tlg011.perseus-grc2", "Pl. Symp."),
    ("tlg0059.tlg024.perseus-grc2", "Pl. Meno"),
    ("tlg0059.tlg030.perseus-grc2", "Pl. Rep."),
]


def g(c, k):
    return c.get(k, 0)


def pct(a, b):
    return "%.1f%%" % (100.0 * a / b) if b else "-"


def table(head, rows):
    print("| " + " | ".join(head) + " |")
    print("|" + "|".join("---" for _ in head) + "|")
    for r in rows:
        print("| " + " | ".join(str(x) for x in r) + " |")
    print()


def main():
    d = json.load(open(sys.argv[1], encoding="ascii"))
    tot = {}
    names = [w for _, w in WORKS] + ["All"]

    def col(f):
        vals = [f(d[k]) for k, _ in WORKS]
        return vals

    print("### Size and tree integrity\n")
    rows = []
    for k, w in WORKS:
        r = d[k]
        rows.append([w, r["sentences"], r["tokens"], r["kept"], r["ellipsis"],
                     r["bad_head"], r["cycle_sentences"],
                     r["misc_head_mismatch"], r["id_gap_sentences"],
                     g(r["roots_per_sentence"], "1"),
                     r["sentences"] - g(r["roots_per_sentence"], "1"),
                     r["max_sentence_len"]])
    rows.append(["All"] + [sum(x[i] for x in rows) for i in range(1, 11)]
                + [max(x[11] for x in rows)])
    table(["Work", "Sentences", "Tokens", "Kept", "Ellipsis [0]",
           "Unresolved/self head", "Cycle sents", "MISC head mismatch",
           "ID gaps", "1-root sents", "Other-root sents", "Max sent len"],
          rows)

    print("### Root label and PRED placement\n")
    rows = []
    for k, w in WORKS:
        r = d[k]
        rr = r["root_rel"]
        rows.append([w, g(rr, "PRED"), g(rr, "AuxX"), g(rr, "COORD"),
                     g(rr, "ExD"), sum(rr.values()) - g(rr, "PRED")
                     - g(rr, "AuxX") - g(rr, "COORD") - g(rr, "ExD"),
                     g(r["pred_per_sentence"], "0"),
                     sum(v for kk, v in r["pred_per_sentence"].items()
                         if int(kk) > 1),
                     r["pred_nonroot"], g(r["pred_parent"], "COORD"),
                     g(r["auxk_parent"], "root")])
    rows.append(["All"] + [sum(x[i] for x in rows) for i in range(1, 11)])
    table(["Work", "Root=PRED", "Root=AuxX", "Root=COORD", "Root=ExD",
           "Root=other", "Sents 0 PRED", "Sents >1 PRED", "PRED not root",
           "...of which under COORD", "AuxK on virtual root"], rows)

    print("### Projectivity (arcs)\n")
    rows = []
    for k, w in WORKS:
        r = d[k]
        rows.append([w, pct(r["nonproj_arcs_full"], r["arcs_full"]),
                     pct(r["nonproj_sent_full"], r["sentences"]),
                     pct(r["nonproj_arcs_nopunct"], r["arcs_nopunct"]),
                     pct(r["nonproj_arcs_kept"], r["arcs_kept"])])
    a = lambda f: sum(f(d[k]) for k, _ in WORKS)
    rows.append(["All",
                 pct(a(lambda r: r["nonproj_arcs_full"]),
                     a(lambda r: r["arcs_full"])),
                 pct(a(lambda r: r["nonproj_sent_full"]),
                     a(lambda r: r["sentences"])),
                 pct(a(lambda r: r["nonproj_arcs_nopunct"]),
                     a(lambda r: r["arcs_nopunct"])),
                 pct(a(lambda r: r["nonproj_arcs_kept"]),
                     a(lambda r: r["arcs_kept"]))])
    table(["Work", "Non-proj arcs (all nodes)", "Sents with any",
           "Non-proj arcs ignoring punct", "Non-proj arcs, kept tokens after climb"],
          rows)

    print("### Ellipsis\n")
    rows = []
    for k, w in WORKS:
        r = d[k]
        ec = r["ellipsis_kept_children"]
        pr = r["ellipsis_promotion"]
        kr = r["kept_roots_after_climb"]
        rows.append([w, r["ellipsis"], pct(r["ellipsis"], r["tokens"]),
                     g(r["rel_ellipsis"], "PRED"), r["ellipsis_root"],
                     r["kept_under_ellipsis"],
                     pct(r["kept_under_ellipsis"], r["kept"]),
                     g(r["ellipsis_chain_depth"], "2"),
                     g(ec, "0"), g(ec, "1"), g(ec, "2") + g(ec, "3"),
                     sum(v for kk, v in kr.items() if kk != "1"),
                     g(pr, "unique"), g(pr, "tie"),
                     g(pr, "only-aux-children"),
                     g(r["ellipsis_position"], "last-before-final-punct")
                     + g(r["ellipsis_position"], "last")])
    rows.append(["All", sum(x[1] for x in rows),
                 pct(sum(x[1] for x in rows), a(lambda r: r["tokens"]))]
                + [sum(x[3] for x in rows), sum(x[4] for x in rows),
                   sum(x[5] for x in rows),
                   pct(sum(x[5] for x in rows), a(lambda r: r["kept"]))]
                + [sum(x[i] for x in rows) for i in range(7, 16)])
    table(["Work", "Ellipsis", "% tokens", "as PRED", "as root",
           "Kept tokens headed by ellipsis", "% kept", "2-deep chains",
           "Ell. 0 kept kids", "1 kid", ">=2 kids",
           "Sents >1 root after climb", "Promotion unique", "Promotion tie",
           "Only Aux kids", "At sentence end"], rows)

    print("### Relation inventory (kept tokens; ellipsis in brackets)\n")
    labels = set()
    for k, _ in WORKS:
        labels |= set(d[k]["rel_kept"]) | set(d[k]["rel_ellipsis"])
    order = sorted(labels, key=lambda l: -sum(
        g(d[k]["rel_kept"], l) + g(d[k]["rel_ellipsis"], l) for k, _ in WORKS))
    rows = []
    for l in order:
        row = [ascii(l)[1:-1]]
        tk = te = 0
        for k, _ in WORKS:
            x, y = g(d[k]["rel_kept"], l), g(d[k]["rel_ellipsis"], l)
            tk += x
            te += y
            row.append("%d" % x + (" [%d]" % y if y else ""))
        row.append("%d" % tk + (" [%d]" % te if te else ""))
        rows.append(row)
    table(["DEPREL"] + names, rows)

    print("### Suffixes and non-guideline labels\n")
    rows = []
    for k, w in WORKS:
        r = d[k]
        rows.append([w, g(r["suffix"], "_CO"), g(r["suffix"], "_AP"),
                     ascii(", ".join("%s %d" % kv for kv in
                                     sorted(r["nonagdt"].items())))[1:-1]
                     or "none"])
    table(["Work", "_CO", "_AP", "Labels outside AGDT 2.0 3.1-3.19"], rows)

    print("### COORD / APOS / AuxC / AuxP shapes (all works)\n")
    agg = {}
    for key in ["coord_upos", "coord_content_children", "coord_child_rel",
                "coord_conjunct_labels", "apos_upos", "apos_content_children",
                "auxc_upos", "auxc_content_children", "auxc_child_rel",
                "auxc_child_mood", "auxp_upos", "auxp_content_children",
                "auxp_child_rel", "auxp_child_case", "verb_mood",
                "verb_tense", "unmapped_today", "postag_voice",
                "postag_gender", "postag_case", "postag_degree",
                "upos_letters", "coord_mixed_patterns", "coord_resolved"]:
        c = {}
        for k, _ in WORKS:
            for kk, v in d[k][key].items():
                c[kk] = c.get(kk, 0) + v
        agg[key] = c
        print("- %s: %s" % (key, ", ".join(
            "%s %d" % (ascii(kk)[1:-1], v) for kk, v in
            sorted(c.items(), key=lambda kv: -kv[1]))))
    print("- coord_nested: %d" % a(lambda r: r["coord_nested"]))
    print()

    print("### Verb mood and tense per work (postag positions 5 and 4)\n")
    rows = []
    for k, w in WORKS:
        vm, vt = d[k]["verb_mood"], d[k]["verb_tense"]
        rows.append([w] + [g(vm, x) for x in "isomnp"]
                    + [g(vt, x) for x in "piflrat"])
    rows.append(["All"] + [sum(x[i] for x in rows) for i in range(1, 14)])
    table(["Work", "ind i", "subj s", "opt o", "imp m", "inf n", "ptcp p",
           "pres p", "impf i", "fut f", "plpf l", "perf r", "aor a",
           "futpf t"], rows)


if __name__ == "__main__":
    main()
