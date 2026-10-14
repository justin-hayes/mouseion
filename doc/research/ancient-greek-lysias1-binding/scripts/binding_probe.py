#!/usr/bin/env python3
"""Lysias I text authority and occurrence binding probe (#2048, revision 2).

Stdlib only. Run with `python3 -I`:

    python3 -I binding_probe.py <inputs_dir> <manifest_out_dir> [<detail_dir>]

<inputs_dir> is produced by scripts/fetch_inputs.sh. The probe reads the pinned
AGDT v2.1 file (native annotation), the pinned canonical-greekLit grc2 text
(canonical text), and two earlier AGDT releases (v2.0, v1.6) used for
cross-release comparison of the same native node IDs.

Outputs under <manifest_out_dir> contain ASCII keys, code-point spans, classes,
reasons, counts and SHA-256 digests only. Greek surfaces are not written there.
If <detail_dir> is given, per-node surfaces are written there for local review;
it is scratch and not committed.

Documents and keys
- cg2:run          canonical running text (<p> text of the 50 sections, joined by "\\n")
- cg2:labelNN      one document per canonical <label> (rubric text, in document order)
- cg2:wNNNN / cg2:pNNNN   running-text word / punctuation tokens
- cg2:LNNwNNN      label word tokens
- sNNNwNNN         AGDT node (native sentence id and word id, v2.1)
Every span is a half-open code-point range into the document named in the row,
and every row records that document's SHA-256.

Status model (per AGDT node; canonical rows mirror it through node_keys):
- correspondence: exact_equal | exact_within_block | fold_equal | concat_window |
  positional_block | rubric_sequence | punct_gap | none
- surface: exact | fold_elision | fold_nfd | concat_exact | label_exact | mismatch | none
- convention: not_applicable | none_documented | cross_release_v1.6_equal
- occurrence_status:
    accepted      exact NFC equality on a monotone equal run; rubric text matched in
                  the rubric sequence with at least one agreeing neighbour anchor
    conditional   fold equality whose v1.6 witness equals the canonical token exactly
                  (encoding variance between releases; no convention document)
    candidate     correspondence proposed but not accepted (concatenation with no
                  documented convention; reversed-order components; positional
                  block with a surface mismatch; fold without a witness)
    ambiguous     more than one candidate correspondence in the same local block;
                  nothing bound
    unbound       no correspondence in the local block
    annotation_only   AGDT stop mark with no canonical counterpart (rubric stops)

Alignment
- Running text: one global monotone difflib alignment on NFC forms gives equal runs
  (accepted). Each unequal run between equal runs is a local block. Fold and
  concatenation evidence is searched only inside one block, never across blocks.
- Concatenation windows are 2-3 consecutive free nodes inside the block; a window is
  a candidate only if its components tile the canonical token exactly in code points.
- Rubric nodes are single-word sentences whose form equals a label text; they are
  bound to label documents by the order of the rubric sequence, never by string.
- Punctuation is matched only inside the gap between two consecutive word anchors
  (accepted or conditional tokens), so distant equal marks cannot pair.
"""
import csv
import difflib
import hashlib
import json
import os
import sys
import unicodedata
import xml.etree.ElementTree as ET
from collections import Counter

AGDT_REV = "bf4334f0af5e13d16b04c1cccd6237e683ac6f5f"
AGDT_PATH = "v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml"
AGDT_SHA = "44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568"
CG_REV = "01b725d835e6e733062ffd79e0efdbae1ba06e5c"
CG_PATH = "data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml"
CG_SHA = "21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a"
WITNESSES = {
    "agdt-v2.0": "witnesses/agdt-v2.0.xml",
    "agdt-v1.6": "witnesses/agdt-v1.6.xml",
}
TEI = "{http://www.tei-c.org/ns/1.0}"
FLAG_TAGS = ("del", "add", "q", "foreign")
LACUNA = ""  # private-use marker, removed from the text and recorded by offset
ELISION_LIKE = ("̓", "ʼ", "’", "'")
RUN_DOC = "cg2:run"
ANCHOR_STATUSES = ("accepted", "conditional")


def nfc(s):
    return unicodedata.normalize("NFC", s)


def local(tag):
    return tag.rsplit("}", 1)[-1]


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 16), b""):
            h.update(block)
    return h.hexdigest()


def sha256_text(s):
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def is_word_char(ch):
    return unicodedata.category(ch)[0] in "LM"


def elision_fold(s):
    """NFD with a final elision-like mark (U+0313, U+02BC, U+2019, apostrophe) unified to U+02BC."""
    t = unicodedata.normalize("NFD", s)
    if t and t[-1] in ELISION_LIKE:
        t = t[:-1] + "ʼ"
    return unicodedata.normalize("NFD", t)


def strip_hyphen(form):
    return form.strip("-")


def piece_spans(tok_text, pieces, base):
    """Exact NFC code-point spans of consecutive pieces tiling tok_text (NFC), offset by base.

    Returns None unless every piece is a non-empty exact substring at its position and
    the pieces cover tok_text completely."""
    spans, pos = [], 0
    for p in pieces:
        pn = nfc(p)
        if not pn or tok_text[pos:pos + len(pn)] != pn:
            return None
        spans.append((base + pos, base + pos + len(pn)))
        pos += len(pn)
    return spans if pos == len(tok_text) else None


# ------------------------------------------------------------ canonical text

def para_items(p):
    """Flatten one <p>: ('c', char, flags) | ('label', element, flags) | ('gap', None, flags)."""
    items = []

    def walk(el, flags):
        tag = local(el.tag)
        if tag == "note":
            return
        if tag == "label":
            items.append(("label", el, flags))
            return
        if tag == "gap":
            items.append(("gap", None, flags))
            return
        f = flags | {tag} if tag in FLAG_TAGS else flags
        if el.text:
            items.extend(("c", ch, f) for ch in el.text)
        for child in el:
            walk(child, f)
            if child.tail:
                items.extend(("c", ch, f) for ch in child.tail)

    if p.text:
        items.extend(("c", ch, frozenset()) for ch in p.text)
    for child in p:
        walk(child, frozenset())
        if child.tail:
            items.extend(("c", ch, frozenset()) for ch in child.tail)
    return items


def canonical(path):
    root = ET.parse(path).getroot()
    body = root.find(f"{TEI}text/{TEI}body")
    sections = [d for d in body.iter(f"{TEI}div") if d.get("subtype") == "section"]
    ns = [int(d.get("n")) for d in sections]
    assert ns == list(range(1, len(sections) + 1)), ns
    chars, flags_at, section_at, lacunae, labels = [], [], [], [], []
    child_tags = Counter()

    def push(ch, flags, sec_n):
        chars.append(ch)
        flags_at.append(flags)
        section_at.append(sec_n)

    for sec in sections:
        sec_n = int(sec.get("n"))
        child_tags.update(local(c.tag) for c in sec)
        for p_ord, p in enumerate(sec.findall(f"{TEI}p"), 1):
            para, lab_no, prev_space = [], 0, True
            for kind, val, f in para_items(p):
                if kind == "label":
                    lab_no += 1
                    para.append(("label", (lab_no, val), None))
                elif kind == "gap":
                    para.append(("gap", None, f))
                    prev_space = False
                elif val.isspace():
                    if not prev_space:
                        para.append(("c", " ", frozenset()))
                        prev_space = True
                else:
                    para.append(("c", val, f))
                    prev_space = False
            while para and para[-1][0] == "c" and para[-1][1] == " ":
                para.pop()
            while para and para[0][0] == "c" and para[0][1] == " ":
                para.pop(0)
            has_text = any(k == "c" and v != " " for k, v, _ in para)
            if has_text and chars:
                push("\n", frozenset(), sec_n)
            for kind, val, f in para:
                if kind == "c":
                    push(val, f, sec_n)
                elif kind == "gap":
                    lacunae.append(len(chars))
                else:
                    k, el = val
                    labels.append({
                        "section": sec_n, "p_ord": p_ord, "label_ord": k,
                        "address": f"div[@subtype='section'][@n='{sec_n}']/p[{p_ord}]/label[{k}]",
                        "text": "".join(el.itertext()),
                        "children": sorted({local(c.tag) for c in el.iter() if c is not el}),
                        "pos": len(chars),
                    })
    for i, lab in enumerate(labels, 1):
        lab["ord"] = i
    return dict(text="".join(chars), flags_at=flags_at, section_at=section_at, lacunae=lacunae,
                labels=labels, n_sections=len(sections), child_tags=dict(child_tags))


def tokens_of(text, flags_at, section_at, doc):
    words, puncts = [], []
    i, n = 0, len(text)
    while i < n:
        ch = text[i]
        if ch.isspace():
            i += 1
        elif is_word_char(ch):
            j = i
            while j < n and is_word_char(text[j]):
                j += 1
            fl = frozenset().union(*[flags_at[k] for k in range(i, j)])
            words.append({"start": i, "end": j, "text": text[i:j], "flags": fl,
                          "section": section_at[i], "doc": doc})
            i = j
        else:
            puncts.append({"start": i, "end": i + 1, "text": ch, "flags": flags_at[i],
                           "section": section_at[i], "doc": doc})
            i += 1
    return words, puncts


# ------------------------------------------------------------ AGDT nodes

def agdt_nodes(path):
    root = ET.parse(path).getroot()
    nodes = []
    for s in root.iter("sentence"):
        sid = int(s.get("id"))
        for w in s.findall("word"):
            a = w.attrib
            wid = int(a["id"])
            if "artificial" in a:
                cls = "artificial"
            elif a.get("postag", "")[:1] == "u":
                cls = "punct"
            else:
                cls = "word"
            nodes.append({
                "sentence": sid, "word": wid, "form": a.get("form", ""), "lemma": a.get("lemma", ""),
                "postag": a.get("postag", ""), "relation": a.get("relation", ""), "head": a.get("head", ""),
                "artificial": a.get("artificial", ""), "class": cls, "key": f"s{sid:03d}w{wid:03d}",
            })
    return nodes


def witness_forms(path):
    out = {}
    for s in ET.parse(path).getroot().iter("sentence"):
        for wd in s.findall("word"):
            out[f"s{int(s.get('id')):03d}w{int(wd.get('id')):03d}"] = wd.get("form", "")
    return out


# ------------------------------------------------------------ alignment

def rec(corr, surf, conv, status, toks=(), spans=(), reason="", block=None, **extra):
    d = {"corr": corr, "surf": surf, "conv": conv, "status": status, "toks": list(toks),
         "spans": list(spans), "reason": reason, "block": block}
    d.update(extra)
    return d


def align_running(seq, words, v16):
    """Global monotone alignment on NFC forms, then local fold/concatenation per unequal block.

    seq: printed word nodes excluding rubric nodes (AGDT order). words: running tokens.
    v16: {node key: v1.6 form}. Returns (node records, token index -> [(node key, status)], blocks)."""
    A = [nfc(n["form"]) for n in seq]
    W = [nfc(w["text"]) for w in words]
    nb, tb = {}, {j: [] for j in range(len(words))}
    blocks, cur = [], None
    bid = 0
    equal_runs = []
    ops = difflib.SequenceMatcher(None, A, W, autojunk=False).get_opcodes()

    def flush():
        nonlocal cur, bid
        if cur is not None:
            bid += 1
            blocks.append([bid] + cur)
            cur = None

    for tag, i1, i2, j1, j2 in ops:
        if tag == "equal":
            flush()
            equal_runs.append((i2 - i1))
            for k in range(i2 - i1):
                i, j = i1 + k, j1 + k
                key = seq[i]["key"]
                nb[key] = rec("exact_equal", "exact", "not_applicable", "accepted",
                              [words[j]["key"]], [(words[j]["start"], words[j]["end"])],
                              "NFC-equal on a monotone equal run", None, equal_run_len=i2 - i1,
                              node_form_count=A.count(A[i]), canon_form_count=W.count(W[j]))
                tb[j].append((key, "accepted"))
        else:
            if cur is None:
                cur = [i1, i2, j1, j2]
            else:
                cur[1], cur[3] = i2, j2
    flush()

    block_log = []
    for bidx, i1, i2, j1, j2 in blocks:
        free_n, free_t = set(range(i1, i2)), set(range(j1, j2))
        n_before = len(nb)
        # 1. fold equality inside the block
        fa = [elision_fold(seq[i]["form"]) for i in range(i1, i2)]
        fb = [elision_fold(words[j]["text"]) for j in range(j1, j2)]
        for tag, a1, a2, b1, b2 in difflib.SequenceMatcher(None, fa, fb, autojunk=False).get_opcodes():
            if tag != "equal":
                continue
            for k in range(a2 - a1):
                i, j = i1 + a1 + k, j1 + b1 + k
                key = seq[i]["key"]
                tok = words[j]
                span = [(tok["start"], tok["end"])]
                if A[i] == W[j]:
                    nb[key] = rec("exact_within_block", "exact", "not_applicable", "accepted",
                                  [tok["key"]], span, "NFC-equal inside an unequal block", bidx)
                    status = "accepted"
                else:
                    surf = "fold_nfd" if unicodedata.normalize("NFC", seq[i]["form"]) != seq[i]["form"] else "fold_elision"
                    if v16.get(key) == tok["text"]:
                        status = "conditional"
                        reason = "elision-fold; AGDT v1.6 form equals canonical token exactly"
                        conv = "cross_release_v1.6_equal"
                    else:
                        status = "candidate"
                        reason = "elision-fold only; no witness of the convention"
                        conv = "none_documented"
                    nb[key] = rec("fold_equal", surf, conv, status, [tok["key"]], span, reason, bidx)
                tb[j].append((key, nb[key]["status"]))
                free_n.discard(i)
                free_t.discard(j)
        # 2. concatenation: windows of 2-3 consecutive free nodes inside this block only
        last = i1 - 1
        for j in sorted(free_t):
            tok = words[j]
            tgt = W[j]
            fwd, rev = [], []
            for s in range(i1, i2):
                for w in (2, 3):
                    idx = list(range(s, s + w))
                    if idx[-1] >= i2 or any(k not in free_n for k in idx):
                        break
                    pieces = [strip_hyphen(seq[k]["form"]) for k in idx]
                    if nfc("".join(pieces)) == tgt:
                        spans = piece_spans(tok["text"], pieces, tok["start"])
                        if spans is not None:
                            fwd.append((tuple(idx), spans))
                    if nfc("".join(reversed(pieces))) == tgt:
                        rev.append(tuple(idx))
            if not fwd and not rev:
                continue
            if len(fwd) == 1 and not rev and fwd[0][0][0] > last:
                idx, spans = fwd[0]
                for k, sp in zip(idx, spans):
                    key = seq[k]["key"]
                    nb[key] = rec("concat_window", "concat_exact", "none_documented", "candidate",
                                  [tok["key"]], [sp],
                                  "hyphen-marked components tile the token in order; no documented split convention",
                                  bidx, window_size=len(idx))
                    tb[j].append((key, "candidate"))
                    free_n.discard(k)
                last = idx[-1]
                free_t.discard(j)
            elif not fwd and len(rev) == 1 and len(rev[0]) == 2:
                idx = rev[0]
                for k in idx:
                    key = seq[k]["key"]
                    nb[key] = rec("concat_reversed", "mismatch", "none_documented", "candidate", [tok["key"]],
                                  [], "components concatenate to the token only in reversed order; candidate link only, not accepted",
                                  bidx, window_size=2)
                    tb[j].append((key, "candidate"))
                    free_n.discard(k)
                free_t.discard(j)
            else:
                involved = {k for w, _ in fwd for k in w} | {k for w in rev for k in w}
                for k in sorted(involved):
                    if k in free_n:
                        key = seq[k]["key"]
                        nb[key] = rec("concat_multiple", "concat_exact" if fwd else "mismatch",
                                      "none_documented", "ambiguous", [], [],
                                      f"{len(fwd)} forward and {len(rev)} reversed local windows match one token; none bound",
                                      bidx)
                        free_n.discard(k)
                tb[j].append(("ambiguous_window", "ambiguous"))
                free_t.discard(j)
        # 3. a sole remaining node and token in the block: positional candidate only
        if len(free_n) == 1 and len(free_t) == 1:
            i = free_n.pop()
            j = free_t.pop()
            key = seq[i]["key"]
            tok = words[j]
            nb[key] = rec("positional_block", "mismatch", "none_documented", "candidate", [tok["key"]],
                          [(tok["start"], tok["end"])],
                          "sole free node and sole free token in the block; surfaces differ; not accepted",
                          bidx)
            tb[j].append((key, "candidate"))
        for i in sorted(free_n):
            key = seq[i]["key"]
            nb[key] = rec("none", "none", "not_applicable", "unbound", [], [],
                          "no exact, fold, concatenation or positional correspondence in the local block", bidx)
        block_log.append({"block": bidx, "nodes": i2 - i1, "tokens": j2 - j1,
                          "bound_nodes": len(nb) - n_before})
    # every node in seq has a record
    for n in seq:
        assert n["key"] in nb, n["key"]
    return nb, tb, block_log, equal_runs


def monotone_violations(nb, seq, word_pos):
    """Count running-text bound node pairs whose token position decreases along node order."""
    idx = {n["key"]: i for i, n in enumerate(seq)}
    pairs = sorted((idx[k], word_pos[r["toks"][0]]) for k, r in nb.items()
                   if r["toks"] and k in idx and r["toks"][0] in word_pos)
    return sum(1 for a, b in zip(pairs, pairs[1:]) if b[1] < a[1])


# ------------------------------------------------------------ rubric

def bind_rubric(rub_nodes, labels):
    """Order-based binding: the rubric sequence must equal the label sequence (NFC) exactly."""
    a = [nfc(n["form"]) for n in rub_nodes]
    b = [nfc(lab["text"]) for lab in labels]
    out = {}
    for tag, i1, i2, j1, j2 in difflib.SequenceMatcher(None, a, b, autojunk=False).get_opcodes():
        if tag == "equal":
            for k in range(i2 - i1):
                out[rub_nodes[i1 + k]["key"]] = labels[j1 + k]["ord"]
    return out


# ------------------------------------------------------------ main

def main(inputs, out, detail=None):
    os.makedirs(out, exist_ok=True)
    agdt_file = os.path.join(inputs, "agdt", AGDT_PATH)
    canon_file = os.path.join(inputs, "canon", CG_PATH)
    assert sha256_file(agdt_file) == AGDT_SHA, "AGDT pin mismatch"
    assert sha256_file(canon_file) == CG_SHA, "canonical pin mismatch"

    can = canonical(canon_file)
    text = can["text"]
    words, puncts = tokens_of(text, can["flags_at"], can["section_at"], RUN_DOC)
    for i, w in enumerate(words, 1):
        w["key"] = f"cg2:w{i:04d}"
    for i, p in enumerate(puncts, 1):
        p["key"] = f"cg2:p{i:04d}"
    label_words, label_puncts, label_docs = [], [], {}
    for lab in can["labels"]:
        doc = f"cg2:label{lab['ord']:02d}"
        ws, ps = tokens_of(lab["text"], [frozenset()] * len(lab["text"]), [lab["section"]] * len(lab["text"]), doc)
        for i, w in enumerate(ws, 1):
            w["key"] = f"cg2:L{lab['ord']:02d}w{i:03d}"
        for i, p in enumerate(ps, 1):
            p["key"] = f"cg2:L{lab['ord']:02d}p{i:03d}"
        label_words += ws
        label_puncts += ps
        label_docs[doc] = {"label": lab, "words": ws, "puncts": ps}
    tok_by_key = {w["key"]: w for w in words + label_words}

    nodes = agdt_nodes(agdt_file)
    printed = [n for n in nodes if n["class"] != "artificial"]
    word_nodes = [n for n in printed if n["class"] == "word"]
    punct_nodes = [n for n in printed if n["class"] == "punct"]
    node_by_key = {n["key"]: n for n in nodes}

    # rubric nodes: single-word sentences whose form equals a label text
    label_text_set = {nfc(lab["text"]) for lab in can["labels"]}
    wcount = Counter(n["sentence"] for n in word_nodes)
    rub_nodes = [n for n in word_nodes if wcount[n["sentence"]] == 1 and nfc(n["form"]) in label_text_set]
    rub_keys = {n["key"] for n in rub_nodes}
    rub_sents = {n["sentence"] for n in rub_nodes}
    seq = [n for n in word_nodes if n["key"] not in rub_keys]

    v16 = witness_forms(os.path.join(inputs, WITNESSES["agdt-v1.6"]))
    v20 = witness_forms(os.path.join(inputs, WITNESSES["agdt-v2.0"]))

    nb, tb_idx, block_log, equal_runs = align_running(seq, words, v16)
    word_pos = {w["key"]: i for i, w in enumerate(words)}
    tbk = {words[j]["key"]: v for j, v in tb_idx.items()}
    viol = monotone_violations(nb, seq, word_pos)

    # rubric binding and anchor check
    rub_map = bind_rubric(rub_nodes, can["labels"])
    rub_rows = []
    word_order = [n for n in word_nodes]  # AGDT document order
    word_index = {n["key"]: i for i, n in enumerate(word_order)}

    def prev_running_token(pos):
        prev = [w for w in words if w["end"] <= pos]
        return prev[-1]["key"] if prev else None

    def next_running_token(pos):
        nxt = [w for w in words if w["start"] >= pos]
        return nxt[0]["key"] if nxt else None

    def bound_running(key):
        r = nb.get(key)
        if r and r["status"] in ANCHOR_STATUSES and r["toks"]:
            return r["toks"][0]
        return None

    for n in rub_nodes:
        ord_ = rub_map.get(n["key"])
        if ord_ is None:
            reason = "rubric sequence does not match the label sequence"
            rub_rows.append({"key": n["key"], "label_doc": "", "status": "unbound", "reason": reason})
            nb[n["key"]] = rec("none", "none", "not_applicable", "unbound", [], [], reason, None)
            continue
        lab = can["labels"][ord_ - 1]
        doc = f"cg2:label{ord_:02d}"
        lw = label_docs[doc]["words"]
        lab_tok = lw[0]["key"] if len(lw) == 1 and lw[0]["text"] == lab["text"] else None
        idx = word_index[n["key"]]
        # previous/next printed word nodes in AGDT order that are running-text nodes
        prev_nodes = [x for x in word_order[:idx] if x["key"] not in rub_keys]
        next_nodes = [x for x in word_order[idx + 1:] if x["key"] not in rub_keys]
        pv = bound_running(prev_nodes[-1]["key"]) if prev_nodes else None
        nx = bound_running(next_nodes[0]["key"]) if next_nodes else None
        pos = lab["pos"]
        cp = prev_running_token(pos)
        cn = next_running_token(pos)
        prev_agree = (pv is not None and cp is not None and pv == cp)
        next_agree = (nx is not None and cn is not None and nx == cn)
        status = "accepted" if (prev_agree or next_agree) else "conditional"
        rub_rows.append({"key": n["key"], "label_doc": doc, "label_ord": ord_, "address": lab["address"],
                         "section": lab["section"], "text_codepoints": len(lab["text"]),
                         "text_sha256": sha256_text(lab["text"]), "label_token": lab_tok or "",
                         "prev_agdt": pv or "", "prev_canon": cp or "", "prev_agree": prev_agree,
                         "next_agdt": nx or "", "next_canon": cn or "", "next_agree": next_agree,
                         "status": status,
                         "reason": ("exact label text in order; anchor agrees on at least one side"
                                    if status == "accepted" else
                                    "exact label text in order; no neighbouring anchor agrees")})
        nb[n["key"]] = rec("rubric_sequence", "label_exact", "not_applicable", status,
                           [lab_tok] if lab_tok else [],
                           [(lw[0]["start"], lw[0]["end"])] if lab_tok else [],
                           rub_rows[-1]["reason"], None, label_doc=doc)
        if lab_tok:
            tbk.setdefault(lab_tok, []).append((n["key"], status))

    # ---------------------------------------------- punctuation (gap-local)
    # Events in AGDT document order. Anchors are running tokens bound by accepted or conditional nodes.
    anchor_of = {}
    for k, r in nb.items():
        if r["status"] in ANCHOR_STATUSES and r["toks"] and r["toks"][0] in word_pos:
            anchor_of[k] = word_pos[r["toks"][0]]
    a_events, a_keys = [], []
    emitted = set()
    for n in printed:
        if n["class"] == "punct":
            if n["sentence"] in rub_sents:
                a_events.append(("RP", None, n["key"]))
            else:
                a_events.append(("P", nfc(n["form"]), n["key"]))
        elif n["key"] in rub_keys:
            continue
        else:
            if n["key"] in anchor_of:
                j = anchor_of[n["key"]]
                if j not in emitted:
                    emitted.add(j)
                    a_events.append(("A", j, n["key"]))
            else:
                a_events.append(("X", None, n["key"]))
    c_events = []
    for j, w in enumerate(words):
        if j in emitted:
            c_events.append((w["start"], ("A", j, w["key"])))
        else:
            c_events.append((w["start"], ("T", None, w["key"])))
    for p in puncts:
        c_events.append((p["start"], ("P", nfc(p["text"]), p["key"])))
    c_events = [e for _, e in sorted(c_events, key=lambda x: x[0])]
    a_anchor_seq = [e[1] for e in a_events if e[0] == "A"]
    c_anchor_seq = [e[1] for e in c_events if e[0] == "A"]
    anchor_seq_equal = a_anchor_seq == c_anchor_seq

    def split_gaps(events):
        gaps, cur_g = [[]], []
        for e in events:
            if e[0] == "A":
                gaps.append([])
            else:
                gaps[-1].append(e)
        return gaps

    punct_map = {}      # AGDT punct key -> canonical punct key
    canon_punct_node = {}
    punct_status = {}
    gap_pairs = list(zip(split_gaps(a_events), split_gaps(c_events))) if anchor_seq_equal else []
    for ga, gc in gap_pairs:
        pa = [e for e in ga if e[0] == "P"]
        pc = [e for e in gc if e[0] == "P"]
        for tag, i1, i2, j1, j2 in difflib.SequenceMatcher(
                None, [e[1] for e in pa], [e[1] for e in pc], autojunk=False).get_opcodes():
            if tag != "equal":
                continue
            for k in range(i2 - i1):
                ka, kc = pa[i1 + k][2], pc[j1 + k][2]
                punct_map[ka] = kc
                canon_punct_node[kc] = ka
    for e in a_events:
        if e[0] == "RP":
            punct_status[e[2]] = ("annotation_only", "stop mark after a canonical label; the label text carries no stop")
        elif e[0] == "P":
            if e[2] in punct_map:
                punct_status[e[2]] = ("accepted", "exact punctuation in the gap between word anchors")
            else:
                punct_status[e[2]] = ("unaligned", "no exact punctuation in the gap between word anchors")

    # ---------------------------------------------- AGDT occurrence rows
    occ = []
    cls_count = Counter()
    running_by_key = {w["key"]: w for w in words}
    for n in nodes:
        base = {"key": n["key"], "sentence": n["sentence"], "word": n["word"], "node_class": n["class"],
                "postag": n["postag"], "relation": n["relation"], "head": n["head"],
                "form_codepoints": len(n["form"]), "form_sha256": sha256_text(n["form"]),
                "lemma_sha256": sha256_text(n["lemma"]), "artificial": n["artificial"],
                "correspondence": "", "surface": "", "convention": "", "occurrence_status": "",
                "canon_keys": "", "canon_doc": "", "span_start": "", "span_end": "", "span_len": "",
                "witness_v16_equal_canonical": "", "block": "", "reason": ""}
        if n["class"] == "artificial":
            base.update(occurrence_status="not_printed", correspondence="none",
                        reason=f"artificial={n['artificial']}; no printed occurrence")
            cls_count["artificial"] += 1
        elif n["class"] == "punct":
            st, why = punct_status.get(n["key"], ("unaligned", "no exact punctuation in the gap between word anchors"))
            base.update(occurrence_status=st, reason=why, correspondence="punct_gap" if st == "accepted" else "none",
                        surface="exact" if st == "accepted" else "none",
                        convention="not_applicable" if st == "accepted" else "none_documented")
            if st == "accepted":
                ck = punct_map[n["key"]]
                pt = next(p for p in puncts if p["key"] == ck)
                base.update(canon_keys=ck, canon_doc=RUN_DOC, span_start=pt["start"], span_end=pt["end"], span_len=1)
            cls_count["punct_" + st] += 1
        else:
            r = nb[n["key"]]
            base.update(correspondence=r["corr"], surface=r["surf"], convention=r["conv"],
                        occurrence_status=r["status"], reason=r["reason"], block=r["block"] or "")
            if r["toks"]:
                base["canon_keys"] = ";".join(r["toks"])
                base["canon_doc"] = tok_by_key[r["toks"][0]]["doc"]
                if r["spans"]:
                    base["span_start"] = r["spans"][0][0]
                    base["span_end"] = r["spans"][-1][1]
                    base["span_len"] = sum(e - s for s, e in r["spans"])
            if n["key"] in v16 and r["toks"]:
                base["witness_v16_equal_canonical"] = v16[n["key"]] == tok_by_key[r["toks"][0]]["text"]
            base["equal_run_len"] = r.get("equal_run_len", "")
            base["node_form_count"] = r.get("node_form_count", "")
            base["canon_form_count"] = r.get("canon_form_count", "")
            cls_count[r["status"]] += 1
        occ.append(base)

    # ---------------------------------------------- canonical token rows (both directions)
    node_links = {}
    for n in word_nodes:
        r = nb[n["key"]]
        for t in r["toks"]:
            node_links.setdefault(t, []).append(n["key"])
    tok_rows = []
    for w in words + label_words:
        keys = node_links.get(w["key"], [])
        sts = [st for _, st in tbk.get(w["key"], [])]
        if "accepted" in sts:
            cov = "accepted"
        elif "conditional" in sts:
            cov = "conditional"
        elif "candidate" in sts:
            cov = "candidate_only"
        elif any(s == "ambiguous" for s in sts):
            cov = "ambiguous"
        else:
            cov = "unbound"
        kind = "word_run" if w["doc"] == RUN_DOC else "word_label"
        tok_rows.append({"key": w["key"], "kind": kind, "doc": w["doc"], "span_start": w["start"],
                         "span_end": w["end"], "section": w["section"], "flags": ";".join(sorted(w["flags"])),
                         "text_codepoints": len(w["text"]), "nfc": nfc(w["text"]) == w["text"],
                         "text_sha256": sha256_text(w["text"]), "node_keys": ";".join(keys),
                         "coverage_status": cov})
    for p in puncts + label_puncts:
        ak = canon_punct_node.get(p["key"])
        tok_rows.append({"key": p["key"], "kind": "punct_run" if p["doc"] == RUN_DOC else "punct_label",
                         "doc": p["doc"], "span_start": p["start"], "span_end": p["end"],
                         "section": p["section"], "flags": ";".join(sorted(p["flags"])),
                         "text_codepoints": 1, "nfc": True, "text_sha256": sha256_text(p["text"]),
                         "node_keys": ak or "", "coverage_status": "accepted" if ak else "unbound"})

    # ---------------------------------------------- sentence rows
    sent_rows = []
    by_sent = {}
    for n in word_nodes:
        by_sent.setdefault(n["sentence"], []).append(n)
    for sid in sorted(by_sent):
        ns = by_sent[sid]
        cl = Counter(nb[n["key"]]["status"] for n in ns)
        sent_rows.append({"sentence": sid, "word_nodes": len(ns),
                          **{s: cl.get(s, 0) for s in ("accepted", "conditional", "candidate", "ambiguous", "unbound")}})

    # ---------------------------------------------- discrepancy rows
    disc = []
    for row in occ:
        if row["node_class"] == "word" and row["occurrence_status"] not in ("accepted",):
            disc.append({"key": row["key"], "sentence": row["sentence"], "class": row["occurrence_status"],
                         "correspondence": row["correspondence"], "surface": row["surface"],
                         "canon_keys": row["canon_keys"], "reason": row["reason"]})
        elif row["node_class"] == "punct" and row["occurrence_status"] != "accepted":
            disc.append({"key": row["key"], "sentence": row["sentence"], "class": row["occurrence_status"],
                         "correspondence": row["correspondence"], "surface": row["surface"],
                         "canon_keys": "", "reason": row["reason"]})
    for r in tok_rows:
        if r["coverage_status"] != "accepted":
            disc.append({"key": r["key"], "sentence": "", "class": "canon_" + r["coverage_status"],
                         "correspondence": "", "surface": "", "canon_keys": "", "reason": r["kind"]})

    # ---------------------------------------------- exact-form audit and concatenation audit
    exact_audit = Counter()
    for n in seq:
        r = nb[n["key"]]
        if r["corr"] == "exact_equal":
            exact_audit["exact_nodes"] += 1
            if r["canon_form_count"] > 1 or r["node_form_count"] > 1:
                exact_audit["exact_repeated_form"] += 1
            if r["equal_run_len"] == 1:
                exact_audit["exact_single_node_run"] += 1
    concat_audit = Counter()
    for n in seq:
        r = nb[n["key"]]
        if r["corr"] == "concat_window":
            concat_audit["concat_nodes"] += 1
            concat_audit["concat_window_size_" + str(r["window_size"])] += 1
        if r["corr"] == "concat_reversed":
            concat_audit["reversed_nodes"] += 1

    # ---------------------------------------------- release and fold mechanisms
    rel_rows = []
    for name, wf in (("agdt-v2.0", v20), ("agdt-v1.6", v16)):
        for n in printed:
            if n["class"] == "artificial":
                continue
            wform = wf.get(n["key"])
            if wform is None:
                continue
            rel_rows.append({"witness": name, "key": n["key"], "class_v21": "", "form_v21_len": len(n["form"]),
                             "form_witness_len": len(wform), "equal_v21": wform == n["form"],
                             "nfc_equal_pair": nfc(wform) == nfc(n["form"]),
                             "nfd_equal_pair": unicodedata.normalize("NFD", wform) == unicodedata.normalize("NFD", n["form"])})
    occ_by_key = {r["key"]: r for r in occ}
    for r in rel_rows:
        r["class_v21"] = occ_by_key[r["key"]]["occurrence_status"]
    mech = Counter()
    for r in occ:
        if r["surface"] in ("fold_elision", "fold_nfd"):
            form = node_by_key[r["key"]]["form"]
            tail = unicodedata.normalize("NFD", form)[-1:]
            name = {"̓": "final_U+0313", "ʼ": "final_U+02BC", "’": "final_U+2019", "'": "final_ASCII_apostrophe"}.get(tail, "no_final_mark")
            mech[("fold", r["occurrence_status"], name)] += 1
        elif r["surface"] == "concat_exact":
            form = node_by_key[r["key"]]["form"]
            mech[("concat", r["occurrence_status"], "trailing_hyphen" if form.endswith("-") else "no_hyphen")] += 1

    res = dict(text=text, can=can, words=words, puncts=puncts, label_words=label_words, label_puncts=label_puncts,
               nodes=nodes, printed=printed, word_nodes=word_nodes, occ=occ, tok_rows=tok_rows, sent_rows=sent_rows,
               disc=disc, rub_rows=rub_rows, cls_count=cls_count, block_log=block_log, equal_runs=equal_runs,
               viol=viol, anchor_seq_equal=anchor_seq_equal, exact_audit=exact_audit, concat_audit=concat_audit,
               rel_rows=rel_rows, mech=mech, out=out, detail=detail, nb=nb, seq=seq, rub_nodes=rub_nodes,
               punct_map=punct_map, label_docs=label_docs, punct_status=punct_status, v16=v16, v20=v20)
    return res


def write_tsv(path, rows, fields):
    with open(path, "w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields, delimiter="\t", extrasaction="ignore", lineterminator="\n")
        w.writeheader()
        for r in rows:
            w.writerow(r)


def write_all(res):
    out = res["out"]
    text = res["text"]
    write_tsv(os.path.join(out, "occurrence_binding.tsv"), res["occ"],
              ["key", "sentence", "word", "node_class", "postag", "relation", "head", "form_codepoints",
               "form_sha256", "lemma_sha256", "artificial", "correspondence", "surface", "convention",
               "occurrence_status", "canon_keys", "canon_doc", "span_start", "span_end", "span_len",
               "witness_v16_equal_canonical", "block", "equal_run_len", "node_form_count", "canon_form_count",
               "reason"])
    write_tsv(os.path.join(out, "canonical_tokens.tsv"), res["tok_rows"],
              ["key", "kind", "doc", "span_start", "span_end", "section", "flags", "text_codepoints", "nfc",
               "text_sha256", "node_keys", "coverage_status"])
    write_tsv(os.path.join(out, "rubric_bindings.tsv"), res["rub_rows"],
              ["key", "label_doc", "label_ord", "address", "section", "text_codepoints", "text_sha256",
               "label_token", "prev_agdt", "prev_canon", "prev_agree", "next_agdt", "next_canon", "next_agree",
               "status", "reason"])
    write_tsv(os.path.join(out, "sentences.tsv"), res["sent_rows"],
              ["sentence", "word_nodes", "accepted", "conditional", "candidate", "ambiguous", "unbound"])
    write_tsv(os.path.join(out, "discrepancies.tsv"), res["disc"],
              ["key", "sentence", "class", "correspondence", "surface", "canon_keys", "reason"])
    write_tsv(os.path.join(out, "release_stability.tsv"), res["rel_rows"],
              ["witness", "key", "class_v21", "form_v21_len", "form_witness_len", "equal_v21",
               "nfc_equal_pair", "nfd_equal_pair"])
    occ_c = Counter(r["occurrence_status"] for r in res["occ"] if r["node_class"] == "word")
    punct_c = Counter(r["occurrence_status"] for r in res["occ"] if r["node_class"] == "punct")
    tok_c = Counter(r["coverage_status"] for r in res["tok_rows"] if r["kind"].startswith("word"))
    tokp_c = Counter(r["coverage_status"] for r in res["tok_rows"] if r["kind"].startswith("punct"))
    summary = {
        "schema": "lysias1-binding-manifest/2",
        "source_agdt": {"rev": AGDT_REV, "path": AGDT_PATH, "sha256": AGDT_SHA},
        "source_canonical": {"rev": CG_REV, "path": CG_PATH, "sha256": CG_SHA},
        "running_text": {"doc": RUN_DOC, "sha256": sha256_text(text), "codepoints": len(text),
                         "nfc": unicodedata.normalize("NFC", text) == text,
                         "sections": res["can"]["n_sections"], "lacunae_at": res["can"]["lacunae"],
                         "labels": len(res["can"]["labels"]), "child_tags_in_sections": res["can"]["child_tags"]},
        "label_documents": [{"doc": d, "text_sha256": sha256_text(v["label"]["text"]),
                             "codepoints": len(v["label"]["text"]), "address": v["label"]["address"]}
                            for d, v in sorted(res["label_docs"].items())],
        "denominators": {"agdt_nodes": len(res["nodes"]), "artificial": res["cls_count"]["artificial"],
                         "printed": len(res["printed"]), "word_nodes": len(res["word_nodes"]),
                         "punct_nodes": len(res["printed"]) - len(res["word_nodes"]),
                         "running_word_tokens": len(res["words"]), "running_punct_tokens": len(res["puncts"]),
                         "label_word_tokens": len(res["label_words"]), "label_punct_tokens": len(res["label_puncts"])},
        "word_node_status": dict(sorted(occ_c.items())),
        "punct_node_status": dict(sorted(punct_c.items())),
        "canonical_word_coverage": dict(sorted(tok_c.items())),
        "canonical_punct_coverage": dict(sorted(tokp_c.items())),
        "exact_audit": dict(res["exact_audit"]),
        "concat_audit": dict(res["concat_audit"]),
        "blocks": {"count": len(res["block_log"]),
                   "largest_nodes": max((b["nodes"] for b in res["block_log"]), default=0),
                   "largest_tokens": max((b["tokens"] for b in res["block_log"]), default=0)},
        "equal_runs": {"count": len(res["equal_runs"]), "max": max(res["equal_runs"], default=0)},
        "monotone_violations": res["viol"],
        "anchor_sequence_equal": res["anchor_seq_equal"],
        "fold_and_concat_mechanisms": {str(k): v for k, v in sorted(res["mech"].items())},
    }
    with open(os.path.join(out, "summary.json"), "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")
    if res["detail"]:
        os.makedirs(res["detail"], exist_ok=True)
        tok_by = {w["key"]: w for w in res["words"] + res["label_words"]}
        rows = []
        for n in res["word_nodes"]:
            r = res["nb"][n["key"]]
            rows.append({"key": n["key"], "form": n["form"], "canon": tok_by[r["toks"][0]]["text"] if r["toks"] else "",
                         "status": r["status"]})
        write_tsv(os.path.join(res["detail"], "word_surfaces.tsv"), rows, ["key", "form", "canon", "status"])
    return summary


if __name__ == "__main__":
    if len(sys.argv) not in (3, 4):
        sys.exit(__doc__)
    res = main(sys.argv[1], sys.argv[2], sys.argv[3] if len(sys.argv) == 4 else None)
    s = write_all(res)
    print(json.dumps({k: s[k] for k in ("denominators", "word_node_status", "punct_node_status",
                                        "canonical_word_coverage", "canonical_punct_coverage",
                                        "monotone_violations", "anchor_sequence_equal",
                                        "exact_audit", "concat_audit", "blocks")},
                     ensure_ascii=False, indent=1, sort_keys=True))
