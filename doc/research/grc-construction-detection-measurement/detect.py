"""Prototype first-slice construction detectors over native AGDT (OGA oga_src v0.2.0).

Usage: python3 -I detect.py OUT.jsonl FILE.conllu [...]

Writes one JSON line per candidate: hits, abstentions, perception exclusions and
loose-query (non-hit) candidates, keyed by work, citation, sent_id and the OGA
token id of the anchor. Prototype only: scratch, not Mouseion code.
"""

import json
import re
import sys
import unicodedata

# ---------------------------------------------------------------- lexical data (Mouseion-authored drafts)
SAY_THINK = {
    # saying
    "φημί", "φάσκω", "λέγω", "εἶπον", "ἐρῶ", "ἀγγέλλω", "ἀπαγγέλλω", "ἐξαγγέλλω",
    "ἀποκρίνω", "ὁμολογέω", "ἀρνέομαι", "ἀπαρνέομαι", "ὑπισχνέομαι", "ὄμνυμι",
    # thinking
    "νομίζω", "οἴομαι", "ἡγέομαι", "δοκέω", "ὑπολαμβάνω", "ἐλπίζω", "ὑποπτεύω", "πιστεύω",
}
PERCEPTION = {"ἀκούω", "ἀκροάομαι", "αἰσθάνομαι", "πυνθάνομαι"}

COND = {"εἰ", "ἐάν"}
COND_LOOSE_LEM = {"εἰ", "ἐάν", "εἴπερ", "ἐάνπερ", "ἤνπερ"}
PURPOSE = {"ἵνα", "ὅπως", "ὡς"}
LIKE = {"ὡς", "ὥσπερ"}


def strip_acc(s):
    s = unicodedata.normalize("NFD", s or "")
    s = "".join(c for c in s if not unicodedata.combining(c))
    return s.lower().replace("ς", "σ")


COND_LOOSE_FORMS = {unicodedata.normalize("NFC", f) for f in ("ἐάν", "ἤν", "κἄν", "κεἰ", "ἐὰν", "ἢν", "κἂν", "Ἐάν", "Ἐὰν", "Ἤν", "Ἢν", "Κἄν", "Κἂν")}


class N:
    __slots__ = ("i", "form", "lemma", "pos", "rel", "h", "head", "kids", "tid", "art", "sent")

    def __init__(self, c, sent):
        self.i = int(c[0])
        self.form, self.pos, self.rel = c[1], c[4].ljust(9, "-"), c[7]
        self.lemma = re.sub(r"\d+$", "", c[2])
        self.h = int(c[6])
        self.tid = c[9].split("|")[0]
        self.art = self.tid.startswith("e_")
        self.kids, self.head, self.sent = [], None, sent

    # morphology
    def verb(self, moods=None):
        return self.pos[0] == "v" and (moods is None or self.pos[4] in moods)

    @property
    def case(self):
        return self.pos[7]


def read(path):
    sents, cur, meta = [], [], {}
    for line in open(path, encoding="utf-8"):
        line = line.rstrip("\n")
        if line.startswith("# source = "):
            meta = {"source": line[11:]}
        elif line.startswith("# sent_id = "):
            meta["sent_id"] = line[12:]
        elif line.startswith("#"):
            continue
        elif not line:
            if cur:
                sents.append(finish(cur, meta))
                cur = []
        else:
            c = line.split("\t")
            if "-" in c[0] or "." in c[0]:
                continue
            cur.append(c)
    if cur:
        sents.append(finish(cur, meta))
    return sents


def finish(rows, meta):
    s = {"meta": meta, "nodes": []}
    ns = [N(c, s) for c in rows]
    by = {n.i: n for n in ns}
    for n in ns:
        n.head = by.get(n.h)
        if n.head is not None:
            n.head.kids.append(n)
    s["nodes"] = ns
    s["ref"] = ns[0].sent and (rows[0][9].split("ref=")[-1] if "ref=" in rows[0][9] else "")
    return s


def eff_kids(n, rels=None):
    """Children, looking down through COORD groups."""
    out = []
    for k in n.kids:
        if k.rel == "COORD":
            out.extend(eff_kids(k))
        else:
            out.append(k)
    return [k for k in out if rels is None or k.rel in rels]


def up(n):
    """Head, looking up through COORD groups."""
    h = n.head
    while h is not None and h.rel == "COORD":
        h = h.head
    return h


def subtree(n):
    out, st = [], [n]
    while st:
        x = st.pop()
        out.append(x)
        st.extend(x.kids)
    return out


def real_order(s):
    return [n for n in s["nodes"] if not n.art]


# ---------------------------------------------------------------- detectors
def clause_kids(n, moods=None, allow_elided=True):
    return [k for k in eff_kids(n, {"ADV"})
            if k.verb(moods) or (allow_elided and moods is None and k.art)]


def prev_real(n):
    rs = real_order(n.sent)
    j = rs.index(n)
    return rs[j - 1] if j > 0 else None


def is_cond_conj(n):
    if n.art or n.rel != "AuxC":
        return False
    if n.lemma in COND:
        return True
    return n.lemma == "ἄν"


def det_conditional(n):
    if not is_cond_conj(n) or not clause_kids(n):
        return None
    p = prev_real(n)
    if (p is not None and p.lemma in LIKE) or (n.head is not None and n.head.lemma in LIKE):
        return None
    return ("hit", None)


def neut_sg_article(k):
    return not k.art and k.pos[0] == "l" and k.pos[6] == "n" and k.pos[2] == "s"


def det_art_inf(n):
    if n.art or not n.verb("n"):
        return None
    arts = [k for k in n.kids if neut_sg_article(k)]
    if not arts:
        return None
    case = {"n": "nom", "g": "gen", "d": "dat", "a": "acc"}.get(arts[0].case)
    return ("hit", case)


def det_result(n):
    if n.art or n.rel != "AuxC" or n.lemma != "ὥστε":
        return None
    ks = clause_kids(n)
    if not ks:
        return None
    if any(k.verb("p") for k in ks):
        return ("abstain", "participle")
    k = ks[0]
    if k.verb("n"):
        return ("hit", "natural")
    if k.verb("i"):
        return ("hit", "actual")
    return ("hit", None)


def det_purpose(n):
    if n.art or n.rel != "AuxC" or n.lemma not in PURPOSE:
        return None
    if clause_kids(n, "so", allow_elided=False):
        return ("hit", None)
    return None


def gen_subjects(n):
    subs = [k for k in eff_kids(n, {"SBJ"}) if not k.art]
    h = n.head
    while h is not None and h.rel == "COORD":
        subs += [k for k in eff_kids(h, {"SBJ"}) if not k.art]
        h = h.head
    return [k for k in subs if k.case == "g"]


def det_gen_abs(n):
    if n.rel != "ADV":
        return None
    if n.art:
        # elided ADV node with a genitive subject: the participle is unprinted
        if gen_subjects(n):
            return ("abstain", "elided participle")
        return None
    if not (n.verb("p") and n.case == "g") or not gen_subjects(n):
        return None
    h = up(n)
    if h is not None and h.lemma in PERCEPTION:
        return ("excluded", "perception head")
    return ("hit", None)


def det_ind_stmt(n):
    if n.art:
        return None
    if n.verb("n") and n.rel in ("OBJ", "SBJ"):
        h = up(n)
        if h is not None and h.lemma in SAY_THINK:
            subs = eff_kids(n, {"SBJ"})
            if all(s.case == "a" for s in subs):
                return ("hit", "infinitive")
        return None
    if n.rel == "AuxC" and n.lemma in ("ὅτι", "ὡς"):
        if [k for k in eff_kids(n, {"OBJ", "SBJ"}) if k.verb("iso")]:
            h = up(n)
            if h is not None and h.lemma in SAY_THINK:
                return ("hit", "hoti-hos")
    return None


DETECTORS = {
    "conditional": det_conditional,
    "articular_infinitive": det_art_inf,
    "result": det_result,
    "purpose": det_purpose,
    "genitive_absolute": det_gen_abs,
    "indirect_statement": det_ind_stmt,
}


# ---------------------------------------------------------------- loose queries
def loose(n):
    """Yield (construction, loose_subtype) for loose-query candidates at n."""
    if n.art:
        if n.rel == "ADV" and gen_subjects(n):
            yield ("genitive_absolute", None)
        return
    rs = real_order(n.sent)
    j = rs.index(n)
    if n.lemma in COND_LOOSE_LEM or (n.lemma == "ἄν" and n.rel == "AuxC") or unicodedata.normalize("NFC", n.form) in COND_LOOSE_FORMS:
        yield ("conditional", None)
    if n.verb("n"):
        if any(neut_sg_article(k) for k in rs[max(0, j - 8):j]):
            yield ("articular_infinitive", None)
    if neut_sg_article(n) and n.head is not None and (n.head.verb() or n.head.art):
        yield ("articular_infinitive", None)
    if n.lemma == "ὥστε":
        yield ("result", None)
    if n.lemma in PURPOSE:
        near = subtree(n) + rs[j + 1:j + 11]
        if any(k.verb("so") for k in near if not k.art):
            yield ("purpose", None)
    if n.verb("p") and n.case == "g":
        yield ("genitive_absolute", None)
    if n.verb("n"):
        h = up(n)
        if (h is not None and h.lemma in SAY_THINK) or any(s.case == "a" for s in eff_kids(n, {"SBJ"})) \
                or (n.rel == "OBJ" and h is not None and (h.verb() or h.art)):
            yield ("indirect_statement", "infinitive")
    if n.rel == "AuxC" and (n.lemma == "ὅτι" or (n.lemma == "ὡς" and eff_kids(n, {"OBJ", "SBJ"}))):
        yield ("indirect_statement", "hoti-hos")


def main(argv):
    out = open(argv[1], "w", encoding="utf-8")
    for path in argv[2:]:
        work = path.rsplit("/", 1)[-1].split(".perseus")[0]
        for s in read(path):
            src = s["meta"].get("source", "")
            author = src.split(",")[0]
            ref = src.split(", ", 1)[-1]
            base = {"work": work, "author": author, "ref": ref, "sent_id": s["meta"].get("sent_id")}
            for n in s["nodes"]:
                hits = set()
                for name, fn in DETECTORS.items():
                    r = fn(n)
                    if r:
                        hits.add(name)
                        out.write(json.dumps(dict(base, anchor=n.tid, construction=name,
                                                  stratum=r[0], subtype=r[1]), ensure_ascii=False) + "\n")
                seen = set()
                for name, sub in loose(n):
                    if name in hits or (name, sub) in seen:
                        continue
                    seen.add((name, sub))
                    out.write(json.dumps(dict(base, anchor=n.tid, construction=name,
                                              stratum="loose", subtype=sub), ensure_ascii=False) + "\n")
    out.close()


if __name__ == "__main__":
    main(sys.argv)
