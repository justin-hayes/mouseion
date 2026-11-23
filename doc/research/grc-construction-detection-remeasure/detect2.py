"""Amended first-slice detectors (#2615), built on the #2607 prototype. Scratch only.

Usage: python3 -I v2/detect2.py OUT.jsonl FILE.conllu [...]   (run from ~/.cache/scratch/grc-syntax)
"""

import json
import os
import sys
import unicodedata

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import detect as d1  # noqa: E402
eff_kids, up, clause_kids, prev_real, real_order, subtree, neut_sg_article, gen_subjects = (d1.eff_kids, d1.up, d1.clause_kids, d1.prev_real, d1.real_order, d1.subtree, d1.neut_sg_article, d1.gen_subjects)

read = d1.read

SAY_THINK_BASE = set(d1.SAY_THINK)
ADDED = {"διηγέομαι", "ἐννοέω"}
INF_VERBS = (SAY_THINK_BASE - {"δοκέω"}) | ADDED
FUT_GATED = {"ὑπισχνέομαι", "ὄμνυμι"}
HOTI_ONLY = {"οἶδα", "ἐπίσταμαι", "μανθάνω", "ὁράω", "ἀκούω", "μιμνῄσκω", "μιμνήσκω",
             "ἀναμιμνῄσκω", "ἀναμιμνήσκω", "πείθω"}
HOTI_VERBS = SAY_THINK_BASE | ADDED | HOTI_ONLY
WHETHER_HEADS = {"οἶδα", "ἐπίσταμαι", "γιγνώσκω", "σκοπέω", "ἐπισκοπέω", "διασκοπέω", "σκέπτομαι",
                 "ἐρωτάω", "ἀνερωτάω", "ἔρομαι", "πυνθάνομαι", "βουλεύω", "ζητέω", "ἐξετάζω",
                 "ὁράω", "μανθάνω", "ἀπορέω", "ἐννοέω"}
COND = {"εἰ", "ἐάν", "εἴπερ"}
PERCEPTION = d1.PERCEPTION
LIKE = d1.LIKE


def det_conditional(n):
    if n.art or n.rel != "AuxC" or not (n.lemma in COND or n.lemma == "ἄν") or not clause_kids(n):
        return None
    p = prev_real(n)
    if (p is not None and p.lemma in LIKE) or (n.head is not None and n.head.lemma in LIKE):
        return None
    if n.lemma == "εἰ":
        h = up(n)
        if h is not None and h.lemma in WHETHER_HEADS:
            return ("excluded", "whether head")
    return ("hit", None)


det_art_inf = d1.det_art_inf


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
    if k.verb("isom"):
        return ("hit", "actual")
    return ("hit", None)


def det_purpose(n):
    if n.art or n.rel != "AuxC" or n.lemma != "ἵνα":
        return None
    if clause_kids(n, "so", allow_elided=False):
        return ("hit", None)
    return None


def other_gen_object(h, part):
    return any(k is not part and not k.art and k.case == "g" and k.rel == "OBJ" for k in eff_kids(h))


def det_gen_abs(n):
    """Variant A (unchanged). Perception exclusions carry subtype 'B-readmit' when variant B would hit."""
    if n.rel != "ADV":
        return None
    if n.art:
        if gen_subjects(n):
            return ("abstain", "elided participle")
        return None
    if not (n.verb("p") and n.case == "g") or not gen_subjects(n):
        return None
    h = up(n)
    if h is not None and h.lemma in PERCEPTION:
        return ("excluded", "B-readmit" if other_gen_object(h, n) else "B-exclude")
    return ("hit", None)


def det_ind_stmt(n):
    if n.art:
        return None
    if n.verb("n") and n.rel in ("OBJ", "SBJ"):
        h = up(n)
        if h is not None and h.lemma in INF_VERBS:
            if h.lemma in FUT_GATED and n.pos[3] != "f":
                return None
            subs = eff_kids(n, {"SBJ"})
            if all(s.case == "a" for s in subs):
                return ("hit", "infinitive")
        return None
    if n.rel == "AuxC" and n.lemma in ("ὅτι", "ὡς"):
        if [k for k in eff_kids(n, {"OBJ", "SBJ"}) if k.verb("iso")]:
            h = up(n)
            if h is not None and h.lemma in HOTI_VERBS:
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


def loose(n):
    """#2607 loose queries, with purpose narrowed to ἵνα and the infinitive query using the amended list."""
    for con, sub in d1.loose(n):
        if con == "purpose" and n.lemma != "ἵνα":
            continue
        yield con, sub
    if n.verb("n") and not n.art:
        h = up(n)
        if h is not None and h.lemma in ADDED:
            yield ("indirect_statement", "infinitive")


def main(argv):
    out = open(argv[1], "w", encoding="utf-8")
    for path in argv[2:]:
        work = path.rsplit("/", 1)[-1].split(".perseus")[0]
        for s in read(path):
            src = s["meta"].get("source", "")
            base = {"work": work, "author": src.split(",")[0], "ref": src.split(", ", 1)[-1],
                    "sent_id": s["meta"].get("sent_id")}
            ai_hits = {n.tid for n in s["nodes"] if det_art_inf(n)}
            for n in s["nodes"]:
                taken = set()
                if neut_sg_article(n) and n.head is not None and n.head.tid in ai_hits:
                    taken.add("articular_infinitive")
                for name, fn in DETECTORS.items():
                    r = fn(n)
                    if r:
                        taken.add(name)
                        out.write(json.dumps(dict(base, anchor=n.tid, construction=name,
                                                  stratum=r[0], subtype=r[1]), ensure_ascii=False) + "\n")
                seen = set()
                for name, sub in loose(n):
                    if name in taken or (name, sub) in seen:
                        continue
                    seen.add((name, sub))
                    out.write(json.dumps(dict(base, anchor=n.tid, construction=name,
                                              stratum="loose", subtype=sub), ensure_ascii=False) + "\n")
    out.close()


if __name__ == "__main__":
    main(sys.argv)
