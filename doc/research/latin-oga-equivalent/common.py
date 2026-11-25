"""Readers, normalisation and alignment shared by the measurement scripts.

Only aggregates leave these modules; nothing here writes token text.
"""
import difflib
import re
import unicodedata
import xml.etree.ElementTree as ET


def norm_form(s):
    s = unicodedata.normalize("NFD", s)
    s = "".join(c for c in s if unicodedata.category(c) != "Mn").lower()
    s = s.replace("j", "i").replace("v", "u")
    return re.sub(r"[^a-z]", "", s)


def norm_lemma(s):
    s = unicodedata.normalize("NFD", s)
    s = "".join(c for c in s if unicodedata.category(c) != "Mn").lower()
    s = s.replace("j", "i").replace("v", "u")
    s = re.sub(r"[\d#_]", "", s)
    return re.sub(r"[^a-z]", "", s)


class Tok:
    __slots__ = ("form", "lemma", "pos", "feats", "head", "rel", "sent", "punct", "skip", "gid")

    def __init__(self, form, lemma, pos, feats, head, rel, sent, punct, skip=False):
        self.form, self.lemma, self.pos, self.feats = form, lemma, pos, feats
        self.head, self.rel, self.sent, self.punct, self.skip = head, rel, sent, punct, skip
        self.gid = -1


def _finish(toks):
    for i, t in enumerate(toks):
        t.gid = i
    return toks


def read_ldt(path):
    """AGDT XML (treebank_data v2.1). head is a global index or -1 (root)/None."""
    toks = []
    root = ET.parse(path).getroot()
    sid = 0
    for s in root.iter("sentence"):
        local = {}
        rows = []
        for w in s.findall("word"):
            art = w.get("insertion_id") is not None
            rows.append((w, art))
        base = len(toks)
        for k, (w, art) in enumerate(rows):
            pt = w.get("postag") or "---------"
            pt = (pt + "---------")[:9]
            feats = {"person": pt[1], "number": pt[2], "tense": pt[3], "mood": pt[4],
                     "voice": pt[5], "gender": pt[6], "case": pt[7], "degree": pt[8]}
            feats = {k2: v for k2, v in feats.items() if v != "-"}
            pos = "v" if pt[0] == "t" else pt[0]
            t = Tok(w.get("form") or "", w.get("lemma") or "", pos, feats, w.get("head"),
                    w.get("relation") or "", sid, pt[0] == "u", skip=art)
            local[w.get("id")] = base + k
            toks.append(t)
        for k, (w, art) in enumerate(rows):
            h = w.get("head")
            toks[base + k].head = local.get(h, -1) if h not in (None, "0") else -1
            if h not in (None, "0") and h not in local:
                toks[base + k].skip = True
        sid += 1
    return _finish(toks)


AGDT_FEATS = {"Case": "case", "Gender": "gender", "Number": "number", "Mood": "mood",
              "Tense": "tense", "Voice": "voice", "Person": "person", "Degree": "degree"}


def read_ola(path):
    """OLA CoNLL-U (AGDT labels, AGDT one-letter pos, feats with AGDT letter values)."""
    toks = []
    sid = 0
    base = 0
    n = 0
    for line in open(path, encoding="utf8"):
        if line.startswith("#"):
            continue
        line = line.rstrip("\n")
        if not line:
            if n:
                sid += 1
                base = len(toks)
                n = 0
            continue
        c = line.split("\t")
        feats = {}
        if c[5] != "_":
            for kv in c[5].split("|"):
                k, _, v = kv.partition("=")
                if k in AGDT_FEATS:
                    feats[AGDT_FEATS[k]] = v
        h = int(c[6])
        t = Tok(c[1], c[2], c[3], feats, base + h - 1 if h > 0 else -1, c[7], sid, c[3] == "u")
        toks.append(t)
        n += 1
    return _finish(toks)


def read_ud(path, only_sent_ids=None):
    """Plain CoNLL-U (UD). Multi-word-token ranges are skipped, words kept."""
    toks = []
    sid = 0
    base = 0
    n = 0
    keep = True
    sent_ids = []
    for line in open(path, encoding="utf8"):
        if line.startswith("#"):
            if line.startswith("# sent_id"):
                cur = line.split("=", 1)[1].strip()
                keep = only_sent_ids is None or cur in only_sent_ids
                sent_ids.append(cur)
            continue
        line = line.rstrip("\n")
        if not line:
            if n:
                sid += 1
                base = len(toks)
                n = 0
            continue
        c = line.split("\t")
        if "-" in c[0] or "." in c[0]:
            continue
        if not keep:
            continue
        feats = {}
        if c[5] != "_":
            for kv in c[5].split("|"):
                k, _, v = kv.partition("=")
                feats[k] = v
        h = int(c[6])
        t = Tok(c[1], c[2], c[3], feats, base + h - 1 if h > 0 else -1, c[7], sid, c[3] == "PUNCT")
        toks.append(t)
        n += 1
    return _finish(toks)


def read_scaife(path):
    """Scaife tagged TSV: ref, token index in chunk, text, UPOS, tag, morph, lemma, dep, head index.

    Sentence boundaries are not stored; each citation chunk is one 'sentence'
    for head purposes (the head index is relative to the chunk).
    """
    toks = []
    chunk_base = {}
    chunk_len = {}
    rows = []
    for line in open(path, encoding="utf8"):
        c = line.rstrip("\n").split("\t")
        if len(c) < 9:
            continue
        rows.append(c)
    base = 0
    cur = None
    idx = {}
    for c in rows:
        ref = c[0]
        if ref != cur:
            cur = ref
            base = len(toks)
        feats = {}
        if c[5]:
            for kv in c[5].split("|"):
                k, _, v = kv.partition("=")
                feats[k] = v
        form = c[2]
        skip = form.strip() == ""
        h = int(c[8])
        t = Tok(form, c[6], c[3], feats, base + h, c[7], ref, c[3] == "PUNCT", skip=skip)
        t.head = base + h if h != int(c[1]) else -1
        if c[7] == "ROOT":
            t.head = -1
        toks.append(t)
    return _finish(toks)


def align(a, b, key=lambda t: norm_form(t.form)):
    """Map a-index -> b-index for tokens whose normalised forms match in a global alignment."""
    ia = [i for i, t in enumerate(a) if key(t) and not t.skip]
    ib = [i for i, t in enumerate(b) if key(t) and not t.skip]
    sa = [key(a[i]) for i in ia]
    sb = [key(b[i]) for i in ib]
    sm = difflib.SequenceMatcher(None, sa, sb, autojunk=False)
    m = {}
    for blk in sm.get_matching_blocks():
        for k in range(blk.size):
            m[ia[blk.a + k]] = ib[blk.b + k]
    return m
