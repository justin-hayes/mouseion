"""Compare OGA v0.2.0 (oga_src) parser output with Gorman gold AGDT trees.

Usage: python3 -I compare_oga_gold.py [--ids] OGA.conllu GOLD1.xml [GOLD2.xml ...]

--ids lists the OGA document-wide token ids (MISC t_N) of construction
disagreements; no form, lemma, head or relation is printed.

Reports, over real (non-elliptic) tokens aligned by normalised word form:
  * UAS / LA / LAS, excluding punctuation;
  * per-gold-relation label recall and LAS, per-predicted-label precision;
  * the most frequent label confusions;
  * agreement for coordination members and for tokens headed by an
    elliptic node;
  * construction-level precision / recall of six AGDT-shaped detectors run on
    the OGA trees, against the same detectors run on the gold trees.

Gold relation suffixes (_CO, _AP, _ExD ...) are stripped, because OGA v0.2.0
deleted them before training (arXiv 2410.12055, section 3.1).
The script is ASCII-only: Greek lemmas are written as \\u escapes.
"""

import collections
import difflib
import sys
import unicodedata
import xml.etree.ElementTree as ET


def norm(s):
    s = unicodedata.normalize("NFD", s or "")
    s = "".join(c for c in s if not unicodedata.combining(c))
    s = s.lower().replace("\u03c2", "\u03c3")
    for ch in "\u2019\u02bc\u1fbd'\u1fbf\u0384\u00b4`":
        s = s.replace(ch, "")
    return "".join(c for c in s if not c.isdigit())


def strip_rel(r):
    r = r or ""
    return r.split("_")[0] if r else r


class Node:
    __slots__ = ("idx", "form", "lemma", "pos", "rel", "head", "art", "kids", "sent", "tid")

    def __init__(self, idx, form, lemma, pos, rel, head, art, sent):
        self.idx, self.form, self.lemma, self.pos = idx, form, lemma, pos
        self.rel, self.head, self.art, self.sent = rel, head, art, sent
        self.kids = []
        self.tid = ""


def finish(sent_nodes):
    by_id = {n.idx: n for n in sent_nodes}
    for n in sent_nodes:
        h = by_id.get(n.head)
        n.head = h  # Node or None (root)
        if h is not None:
            h.kids.append(n)


def read_oga(path):
    sents, cur, sid = [], [], 0
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if not line:
                if cur:
                    finish(cur)
                    sents.append(cur)
                    cur, sid = [], sid + 1
                continue
            if line.startswith("#"):
                continue
            c = line.split("\t")
            if "-" in c[0] or "." in c[0]:
                continue
            art = c[1].startswith("[") and c[1].endswith("]") and c[3] == "-"
            nd = Node(int(c[0]), c[1], c[2], c[4], c[7], int(c[6]), art, sid)
            nd.tid = c[9].split("|")[0] if len(c) > 9 else ""
            cur.append(nd)
    if cur:
        finish(cur)
        sents.append(cur)
    return sents


def read_gold(paths):
    sents, sid = [], 0
    for p in paths:
        root = ET.parse(p).getroot()
        for s in root.iter("sentence"):
            cur = []
            for w in s.iter("word"):
                art = w.get("artificial") is not None or w.get("insertion_id") is not None
                try:
                    head = int(w.get("head") or 0)
                except ValueError:
                    head = 0
                pos = (w.get("postag") or "").ljust(9, "-")
                cur.append(Node(int(w.get("id")), w.get("form") or "", w.get("lemma") or "",
                                pos, w.get("relation") or "", head, art, sid))
            finish(cur)
            sents.append(cur)
            sid += 1
    return sents


def real_tokens(sents):
    return [n for s in sents for n in s if not n.art]


def align(gold, oga):
    a = [norm(n.form) for n in gold]
    b = [norm(n.form) for n in oga]
    sm = difflib.SequenceMatcher(None, a, b, autojunk=False)
    g2o = {}
    for blk in sm.get_matching_blocks():
        for k in range(blk.size):
            g2o[id(gold[blk.a + k])] = oga[blk.b + k]
    return g2o


def is_punct(n):
    return n.pos[:1] == "u"


# ---------------------------------------------------------------- detectors
EI = {norm("\u03b5\u1f30")}
EAN = {norm("\u1f10\u03ac\u03bd"), norm("\u1f24\u03bd"), norm("\u1f04\u03bd")}  # ean, en, an (OGA lemma for contracted ean)
HINA = norm("\u1f35\u03bd\u03b1")
HOPOS = norm("\u1f45\u03c0\u03c9\u03c2")
HOS = norm("\u1f61\u03c2")
HOSTE = norm("\u1f65\u03c3\u03c4\u03b5")


def eff_kids(n, rel=None):
    """Children, looking through COORD (and AuxP-free) coordination groups."""
    out = []
    for k in n.kids:
        if strip_rel(k.rel) == "COORD":
            out.extend(eff_kids(k, rel))
        else:
            out.append(k)
    return [k for k in out if rel is None or strip_rel(k.rel) == rel]


def is_verb(n, mood=None):
    return n.pos[:1] == "v" and (mood is None or n.pos[4:5] in mood)


def det_gen_abs(n):
    if not (is_verb(n, "p") and n.pos[7:8] == "g" and strip_rel(n.rel) == "ADV"):
        return False
    subs = eff_kids(n, "SBJ")
    if not subs and n.head is not None and strip_rel(n.head.rel) == "COORD":
        subs = [k for k in n.head.kids if strip_rel(k.rel) == "SBJ"]
    return any(s.pos[7:8] == "g" for s in subs)


def det_acc_inf(n):
    return is_verb(n, "n") and any(s.pos[7:8] == "a" for s in eff_kids(n, "SBJ"))


def det_art_inf(n):
    return is_verb(n, "n") and any(k.pos[:1] == "l" for k in n.kids)


def clause_kids(n, moods=None):
    """ADV verb children of a subordinator. Without a mood filter, an elliptic
    ADV node also counts: OGA ellipsis nodes carry no POS ('-'), so a clause
    with an elided verb is only recognisable by its label."""
    return [k for k in eff_kids(n) if strip_rel(k.rel) == "ADV"
            and (is_verb(k, moods) or (moods is None and k.art))]


def lem(n):
    return norm(n.lemma)


def det_cond(n):
    return strip_rel(n.rel) == "AuxC" and lem(n) in EI | EAN and bool(clause_kids(n))


def det_purpose(n):
    return (strip_rel(n.rel) == "AuxC" and lem(n) in {HINA, HOPOS, HOS}
            and bool(clause_kids(n, "so")))


def det_result(n):
    return strip_rel(n.rel) == "AuxC" and lem(n) == HOSTE and bool(clause_kids(n))


DETECTORS = [
    ("genitive absolute", det_gen_abs),
    ("accusative + infinitive", det_acc_inf),
    ("articular infinitive", det_art_inf),
    ("conditional protasis (ei/ean)", det_cond),
    ("purpose clause (hina/hopos/hos + subj/opt)", det_purpose),
    ("result clause (hoste)", det_result),
]


def pct(a, b):
    return "n/a" if b == 0 else "%.3f (%d/%d)" % (a / b, a, b)


def main(argv):
    show_ids = "--ids" in argv
    argv = [a for a in argv if a != "--ids"]
    oga_s = read_oga(argv[1])
    gold_s = read_gold(argv[2:])
    gold, oga = real_tokens(gold_s), real_tokens(oga_s)
    g2o = align(gold, oga)
    o2g = {id(v): k for k, v in g2o.items()}
    gold_by_id = {id(n): n for n in gold}
    print("gold real tokens %d, OGA real tokens %d, aligned %d (%.1f%% of gold)"
          % (len(gold), len(oga), len(g2o), 100.0 * len(g2o) / max(1, len(gold))))

    tot = uas = la = las = 0
    ell_gold = ell_both = 0
    per_rel = collections.defaultdict(lambda: [0, 0, 0])  # n, label ok, las ok
    pred_rel = collections.defaultdict(lambda: [0, 0])  # n predicted, correct
    conf = collections.Counter()
    co = [0, 0]
    for g in gold:
        o = g2o.get(id(g))
        if o is None or is_punct(g):
            continue
        tot += 1
        grel, orel = strip_rel(g.rel), strip_rel(o.rel)
        if g.head is None:
            hok = o.head is None
        elif g.head.art:
            ell_gold += 1
            hok = o.head is not None and o.head.art
            ell_both += hok
        else:
            hok = o.head is not None and not o.head.art and g2o.get(id(g.head)) is o.head
        lok = grel == orel
        uas += hok
        la += lok
        las += hok and lok
        per_rel[grel][0] += 1
        per_rel[grel][1] += lok
        per_rel[grel][2] += hok and lok
        pred_rel[orel][0] += 1
        pred_rel[orel][1] += lok
        if not lok:
            conf[(grel, orel)] += 1
        if "_CO" in (g.rel or "") or grel == "COORD":
            co[0] += 1
            co[1] += hok and lok
    print("scored tokens (no punctuation) %d" % tot)
    print("UAS %s\nLA  %s\nLAS %s" % (pct(uas, tot), pct(la, tot), pct(las, tot)))
    print("coordination members + coordinators LAS %s" % pct(co[1], co[0]))
    print("tokens whose gold head is elliptic: %d; OGA head also elliptic: %s"
          % (ell_gold, pct(ell_both, ell_gold)))
    print("\nrelation | gold n | label recall | LAS | label precision")
    for r, (n, lok, lasok) in sorted(per_rel.items(), key=lambda x: -x[1][0]):
        if n < 20:
            continue
        pn, pc = pred_rel.get(r, [0, 0])
        print("%s | %d | %.3f | %.3f | %s" % (r, n, lok / n, lasok / n, pct(pc, pn)))
    print("\ntop label confusions (gold -> OGA)")
    for (a, b), c in conf.most_common(12):
        print("%s -> %s: %d" % (a, b, c))

    print("\nconstruction | gold | OGA | both | precision | recall")
    for name, fn in DETECTORS:
        gset = {id(g) for g in gold if id(g) in g2o and fn(g)}
        gun = sum(1 for g in gold if id(g) not in g2o and fn(g))
        oset = {o2g[id(o)] for o in oga if id(o) in o2g and fn(o)}
        both = len(gset & oset)
        print("%s | %d | %d | %d | %s | %s%s" % (
            name, len(gset), len(oset), both, pct(both, len(oset)), pct(both, len(gset)),
            "" if not gun else " (gold anchors unaligned: %d)" % gun))
        fp = [gold_by_id[i] for i in oset - gset]
        fn_ = [gold_by_id[i] for i in gset - oset]
        why = collections.Counter()
        for g in fp:
            o = g2o[id(g)]
            why["FP gold rel=%s pos=%s" % (strip_rel(g.rel), g.pos[:1] + g.pos[4] + g.pos[7])] += 1
        for g in fn_:
            o = g2o[id(g)]
            why["FN OGA rel=%s pos=%s" % (strip_rel(o.rel), o.pos[:1] + o.pos[4] + o.pos[7])] += 1
        for k, c in why.most_common(4):
            print("    %s: %d" % (k, c))
        if show_ids:
            print("    FP OGA ids: " + " ".join(sorted((g2o[id(g)].tid for g in fp), key=lambda t: int(t[2:]))))
            print("    FN OGA ids: " + " ".join(sorted((g2o[id(g)].tid for g in fn_), key=lambda t: int(t[2:]))))


if __name__ == "__main__":
    main(sys.argv)
