#!/usr/bin/env python3
"""Full-speech collation of the pinned canonical text against public witnesses (#2048 r3).

Stdlib only. Run with `python3 -I`:

    python3 -I witness_collation.py <inputs_dir> <manifest_dir> <out_dir> [<page_checks.tsv>]

<inputs_dir>  produced by fetch_inputs.sh followed by fetch_witnesses.sh
<manifest_dir> the committed binding manifest (canonical_tokens.tsv and
               occurrence_binding.tsv), used only to attach native AGDT node keys

Streams compared (complete speech, nothing skipped):
- C   pinned canonical-greekLit grc2 (01b725d8): running word tokens cg2:wNNNN with the
      five label documents cg2:LNNwNNN inserted at their TEI positions; running
      punctuation cg2:pNNNN. Extraction is binding_probe.canonical() unchanged.
- MN  Centre for the Greek Language, Mnemosyne digital library text_id=38, pages 1-7
      (Greek column only, i.e. the first `lbr_text` block of each page).
- WS  Greek Wikisource page 7431, revision 102844 (wikitext).

Declared comparison key (applied identically to every stream, before alignment; raw
code points are kept and every raw difference hidden by the key is counted by class
and listed in witness_key_equalities.tsv):
  K1 NFC
  K2 an apostrophe-like mark (U+0027, U+2019, U+1FBD, U+1FBF, U+02BC) directly after a
     letter is the elision mark U+02BC and belongs to the word
  K3 Unicode lowercase
  K4 U+1FE4 -> U+03C1, and U+1FE5 -> U+03C1 when it follows a rho in the same word
     (the Wikisource 2008 bot edit `-rr +rh-rh`; word-initial rough rho is kept)
  K5 rubric lines only: a canonical <label> word and a witness word written wholly in
     capitals are compared without diacritics, under a key no running word can take
Nothing else is folded: accents, breathings, iota subscript, spelling and word
division stay significant, and every remaining difference is a ledger row.

Outputs (ASCII; witness and canonical surfaces only as U+XXXX sequences):
  witness_word_ledger.tsv        every non-equal word operation, per witness
  witness_punct_ledger.tsv       every non-equal punctuation gap between word anchors
  witness_key_equalities.tsv     tokens equal under the key but different in raw form
  witness_named_checks.tsv       the named #2048 items, each witness's reading
  witness_structure.tsv          section markers, rubric lines and lacuna/ellipsis marks
  witness_summary.json
"""
import csv
import difflib
import html
import json
import os
import re
import sys
import unicodedata

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import binding_probe as bp  # noqa: E402  (our own module, same directory)

ELISION_MARKS = "'’᾽᾿ʼ"
EDITORIAL = "[]<>‹›⟨⟩"
QUOTES = "\"«»“”‘"
WITNESS_FILES = {
    "MN": [f"witness/mnemosyne/page{p}.html" for p in range(1, 8)],
    "WS": ["witness/wikisource/rev102844.txt"],
}
WITNESS_SHA = {
    "witness/mnemosyne/page1.html": "0c2c517bb98960b75d4f1bc7c04c55e02b019d2fe9e01165f0064a8bad700a43",
    "witness/mnemosyne/page2.html": "253f4ebe80a0bc9fdc1230fc53095936319aff68c76fbf53e61b9a6dff7a5710",
    "witness/mnemosyne/page3.html": "fe8208b4b66615d1057567e9ed1aa36efb2bf8bae28dfc225945a838fb08886d",
    "witness/mnemosyne/page4.html": "3889704c7eb876a080e915828d68e84d15349e5f61ca2954fe628a1bc84052f6",
    "witness/mnemosyne/page5.html": "2322afbdd5a4cdfff331d547987030c74fd499522ef76632dfc4ed75755fee75",
    "witness/mnemosyne/page6.html": "845acbd3d7b403b17a47a86b732ccb74860c6d4ab4fe9e0c5a892ab94451c51e",
    "witness/mnemosyne/page7.html": "3f91adea58bd994431050c5de82820be7beb28a582816b7fccfd6e5ad187cde2",
    "witness/wikisource/rev102844.txt": "d0606d69b6af003ce8e99ef2d2d46dc70492af371425d487c445c12409fb44ad",
}
SEC = ""  # private-use section marker in extracted witness text


def cps(s):
    return " ".join(f"U+{ord(c):04X}" for c in s)


def is_letter(ch):
    return unicodedata.category(ch)[0] in "LM"


def strip_marks(s):
    return "".join(c for c in unicodedata.normalize("NFD", s) if unicodedata.category(c) != "Mn")


# ------------------------------------------------------------ witness extraction

def extract_mnemosyne(paths):
    parts = []
    for p in paths:
        with open(p, encoding="utf-8") as f:
            s = f.read()
        i = s.index('<div class="lbr_text')
        j = s.index("</table>", i)
        block = s[i:j]
        block = re.sub(r'<span class="verse">\[(\d+)\]</span>', lambda m: f" {SEC}{m.group(1)}{SEC} ", block)
        block = re.sub(r"<br\s*/?>", "\n", block)
        block = re.sub(r"<[^>]+>", " ", block)
        parts.append(html.unescape(block))
    return "\n".join(parts)


def extract_wikisource(paths):
    with open(paths[0], encoding="utf-8") as f:
        s = f.read()
    s = re.sub(r"\{\{Τίτλος2\|[^}]*\}\}", " ", s)  # title template
    s = re.sub(r"\[\[[^\]]*\]\]", " ", s)  # categories
    s = s.replace("</div>", " ")
    s = re.sub(r"\{\{χ\|(\d+)\}\}", lambda m: f" {SEC}{m.group(1)}{SEC} ", s)
    assert "{{" not in s, "unhandled template"
    return s


def tokenize_witness(text):
    """Tokens: dict(raw, key, kind, section, idx). Section markers become structure rows."""
    text = unicodedata.normalize("NFC", text)
    toks, markers, sec = [], [], 0
    i, n = 0, len(text)
    elision_converted = 0
    while i < n:
        ch = text[i]
        if ch == SEC:
            j = text.index(SEC, i + 1)
            sec = int(text[i + 1:j])
            markers.append({"after_token": len(toks), "section": sec})
            i = j + 1
        elif ch.isspace():
            i += 1
        elif is_letter(ch):
            j = i
            while j < n and is_letter(text[j]):
                j += 1
            raw = text[i:j]
            if j < n and text[j] in ELISION_MARKS:
                raw += text[j]
                if text[j] != "ʼ":
                    elision_converted += 1
                j += 1
            toks.append({"raw": raw, "kind": "w", "section": sec})
            i = j
        else:
            toks.append({"raw": ch, "kind": "p", "section": sec})
            i += 1
    for t in toks:
        if t["kind"] != "w":
            t["key"] = t["raw"]
        elif len(t["raw"]) > 1 and t["raw"].isupper():
            t["key"] = rubric_key(t["raw"])
        else:
            t["key"] = word_key(t["raw"])
    return toks, markers, elision_converted


def rubric_key(raw):
    """K5: rubric lines. Witness rubrics are set in capitals without diacritics, canonical
    labels in mixed case with diacritics; both get this key, which no running word gets."""
    return "RUBRIC:" + strip_marks(unicodedata.normalize("NFC", raw)).lower()


def word_key(raw):
    s = unicodedata.normalize("NFC", raw)
    if s and s[-1] in ELISION_MARKS:
        s = s[:-1] + "ʼ"
    s = s.lower()
    out = []
    for c in s:
        if c == "ῤ":
            c = "ρ"
        elif c == "ῥ" and out and out[-1] == "ρ":
            c = "ρ"
        out.append(c)
    return unicodedata.normalize("NFC", "".join(out))


def key_classes(a, b):
    """Why two raw tokens with equal keys differ."""
    if a == b:
        return ""
    if (len(a) > 1 and a.isupper()) or (len(b) > 1 and b.isupper()):
        return "rubric_capitals"
    cls = []
    if unicodedata.normalize("NFC", a) != a or unicodedata.normalize("NFC", b) != b:
        cls.append("nfc")
    if a[-1:] in ELISION_MARKS and b[-1:] in ELISION_MARKS and a[-1] != b[-1]:
        cls.append("elision_mark")
    if a.lower() != a or b.lower() != b:
        if a.lower() != b.lower() or a != b:
            cls.append("case")
    if any(c in "ῤῥ" for c in a + b) and ("ῤ" in a) != ("ῤ" in b):
        cls.append("rho_breathing")
    return "+".join(sorted(set(cls))) or "other"


# ------------------------------------------------------------ canonical stream

def canonical_stream(inputs):
    path = os.path.join(inputs, "canon", bp.CG_PATH)
    assert bp.sha256_file(path) == bp.CG_SHA
    can = bp.canonical(path)
    words, puncts = bp.tokens_of(can["text"], can["flags_at"], can["section_at"], bp.RUN_DOC)
    for i, w in enumerate(words, 1):
        w["key"] = f"cg2:w{i:04d}"
    for i, p in enumerate(puncts, 1):
        p["key"] = f"cg2:p{i:04d}"
    items = []  # (sort position, order, token)
    for w in words:
        items.append((w["start"], 1, dict(id=w["key"], raw=w["text"], kind="w", section=w["section"],
                                          flags=sorted(w["flags"]), start=w["start"])))
    for p in puncts:
        items.append((p["start"], 1, dict(id=p["key"], raw=p["text"], kind="p", section=p["section"],
                                          flags=sorted(p["flags"]), start=p["start"])))
    for lab in can["labels"]:
        ws, ps = bp.tokens_of(lab["text"], [frozenset()] * len(lab["text"]),
                              [lab["section"]] * len(lab["text"]), "label")
        assert not ps
        for k, w in enumerate(ws, 1):
            items.append((lab["pos"], 0, dict(id=f"cg2:L{lab['ord']:02d}w{k:03d}", raw=w["text"], kind="w",
                                              section=lab["section"], flags=["label"], start=lab["pos"])))
    items.sort(key=lambda x: (x[0], x[1]))
    toks = [t for _, _, t in items]
    for t in toks:
        if t["kind"] != "w":
            t["key"] = t["raw"]
        elif "label" in t["flags"]:
            t["key"] = rubric_key(t["raw"])
        else:
            t["key"] = word_key(t["raw"])
    return toks, can


def read_tsv(path):
    with open(path, encoding="ascii") as f:
        return list(csv.DictReader(f, delimiter="\t"))


# ------------------------------------------------------------ collation

def collate(ctoks, wtoks):
    cw = [t for t in ctoks if t["kind"] == "w"]
    ww = [t for t in wtoks if t["kind"] == "w"]
    sm = difflib.SequenceMatcher(None, [t["key"] for t in cw], [t["key"] for t in ww], autojunk=False)
    return cw, ww, sm.get_opcodes()


def classify_word_op(tag, cs, ws, lacuna_sections):
    if any("label" in t["flags"] for t in cs) or (ws and all(t["raw"].isupper() and len(t["raw"]) > 1 for t in ws)):
        if tag == "replace" and len(cs) == len(ws) and all(
                strip_marks(a["raw"]).lower() == strip_marks(b["raw"]).lower() for a, b in zip(cs, ws)):
            return "rubric_capitals_without_diacritics"
        return "rubric"
    if tag == "insert" and len(ws) >= 20:
        return "witness_block_insert"
    if tag == "replace" and len(cs) == len(ws) and all(
            strip_marks(a["key"]) == strip_marks(b["key"]) for a, b in zip(cs, ws)):
        return "diacritics_only"
    if tag == "replace" and "".join(a["key"] for a in cs) == "".join(b["key"] for b in ws):
        return "word_division"
    if any(f in ("del", "add") for t in cs for f in t["flags"]):
        return "editorially_marked_in_canonical"
    return "reading"


# ------------------------------------------------------------ printed-page point checks

def decode_cps(s):
    return "".join(chr(int(c[2:], 16)) for c in s.split()) if s.strip() else ""


def agdt_printed_nodes(inputs):
    path = os.path.join(inputs, "agdt", bp.AGDT_PATH)
    assert bp.sha256_file(path) == bp.AGDT_SHA
    return [n for n in bp.agdt_nodes(path) if n["class"] != "artificial"]


def point_readings(point, kind, ctoks, align, witnesses, agdt, node_of_canon, lacunae):
    """Readings of one point in C, AGDT, MN and WS. Returns dict stream -> string."""
    key = point.split(":", 1)[1] if point.startswith("after:") else point
    cw = align["MN"][0]
    idx = next(i for i, t in enumerate(cw) if t["id"] == key)
    out = {}
    # canonical
    if kind in ("word", "rubric"):
        out["C"] = cw[idx]["raw"]
    elif kind == "insert":
        out["C"] = ""
    else:  # punct_after: canonical punctuation directly after the token
        k = next(i for i, t in enumerate(ctoks) if t["id"] == key) + 1
        s = ""
        while k < len(ctoks) and ctoks[k]["kind"] == "p" and "label" not in cw[idx]["flags"]:
            s += ctoks[k]["raw"]
            k += 1
        if "label" not in cw[idx]["flags"]:
            nxt_start = ctoks[k]["start"] if k < len(ctoks) else 10 ** 9
            if any(cw[idx]["start"] < off <= nxt_start for off in lacunae):
                s += "<gap>"
        out["C"] = s
    # witnesses
    for wid in ("MN", "WS"):
        _, ww, cmap = align[wid]
        wtoks = witnesses[wid][0]
        wpos = [k for k, t in enumerate(wtoks) if t["kind"] == "w"]
        js = cmap.get(idx, [])
        if kind in ("word", "rubric"):
            out[wid] = " ".join(ww[j]["raw"] for j in js)
        elif kind == "insert":
            nxt = cmap.get(idx + 1, [])
            lo = js[-1] + 1 if js else None
            hi = nxt[0] if nxt else None
            out[wid] = " ".join(ww[j]["raw"] for j in range(lo, hi)) if lo is not None and hi is not None else "?"
        else:
            k = wpos[js[-1]] + 1
            s = ""
            while k < len(wtoks) and wtoks[k]["kind"] == "p":
                s += wtoks[k]["raw"]
                k += 1
            out[wid] = s
    # AGDT printed nodes linked to the canonical token(s)
    nodes = node_of_canon.get(key, [])
    pos = {n["key"]: i for i, n in enumerate(agdt)}
    if kind in ("word", "rubric"):
        out["AGDT"] = " ".join(agdt[pos[k]]["form"] for k in nodes)
    elif kind == "insert":
        nxt_nodes = node_of_canon.get(cw[idx + 1]["id"], [])
        if nodes and nxt_nodes:
            lo, hi = max(pos[k] for k in nodes) + 1, min(pos[k] for k in nxt_nodes)
            out["AGDT"] = " ".join(agdt[i]["form"] for i in range(lo, hi) if agdt[i]["class"] == "word")
        else:
            out["AGDT"] = "?"
    else:
        if nodes:
            i = max(pos[k] for k in nodes) + 1
            s = ""
            while i < len(agdt) and agdt[i]["class"] == "punct":
                s += agdt[i]["form"]
                i += 1
            out["AGDT"] = s
        else:
            out["AGDT"] = "?"
    return out


def same(a, b, kind):
    if kind == "rubric":
        return rubric_key(a) == rubric_key(b)
    if kind in ("word", "insert"):
        return [word_key(x) for x in a.split()] == [word_key(x) for x in b.split()]
    norm = {"…": "..."}
    return "".join(norm.get(c, c) for c in a) == "".join(norm.get(c, c) for c in b)


def main(inputs, manifest, out, fixture=None):
    os.makedirs(out, exist_ok=True)
    for rel, h in WITNESS_SHA.items():
        assert bp.sha256_file(os.path.join(inputs, rel)) == h, rel
    ctoks, can = canonical_stream(inputs)
    lacuna_sections = {can["section_at"][o] for o in can["lacunae"]}

    canon_rows = {r["key"]: r for r in read_tsv(os.path.join(manifest, "canonical_tokens.tsv"))}
    occ = {r["key"]: r for r in read_tsv(os.path.join(manifest, "occurrence_binding.tsv"))}

    def nodes_of(cid):
        r = canon_rows.get(cid)
        return r["node_keys"] if r else ""

    witnesses = {}
    witnesses["MN"] = tokenize_witness(extract_mnemosyne([os.path.join(inputs, p) for p in WITNESS_FILES["MN"]]))
    witnesses["WS"] = tokenize_witness(extract_wikisource([os.path.join(inputs, p) for p in WITNESS_FILES["WS"]]))

    word_rows, punct_rows, keq_rows, struct_rows, named_rows = [], [], [], [], []
    summary = {"canonical": {"word_tokens_with_labels": sum(1 for t in ctoks if t["kind"] == "w"),
                             "punct_tokens": sum(1 for t in ctoks if t["kind"] == "p"),
                             "lacunae": len(can["lacunae"]), "lacuna_sections": sorted(lacuna_sections)},
               "witnesses": {}}
    align = {}

    for wid, (wtoks, markers, elision_converted) in witnesses.items():
        cw, ww, ops = collate(ctoks, wtoks)
        cmap = {}  # canonical word index -> list of witness word indices
        eq_tokens = 0
        classes = {}
        for tag, i1, i2, j1, j2 in ops:
            if tag == "equal":
                for k in range(i2 - i1):
                    cmap[i1 + k] = [j1 + k]
                    eq_tokens += 1
                    a, b = cw[i1 + k], ww[j1 + k]
                    if a["raw"] != b["raw"]:
                        kc = key_classes(a["raw"], b["raw"])
                        keq_rows.append({"witness": wid, "canon_key": a["id"], "witness_word_index": j1 + k,
                                         "class": kc, "node_keys": nodes_of(a["id"])})
                continue
            cs, ws = cw[i1:i2], ww[j1:j2]
            if tag == "replace" and len(cs) == len(ws):
                for k in range(len(cs)):
                    cmap[i1 + k] = [j1 + k]
            else:
                for k in range(len(cs)):
                    cmap[i1 + k] = list(range(j1, j2))
            cls = classify_word_op(tag, cs, ws, lacuna_sections)
            classes[cls] = classes.get(cls, 0) + 1
            secs = sorted({t["section"] for t in cs} | ({ws[0]["section"]} if not cs and ws else set()))
            word_rows.append({
                "witness": wid, "op": tag, "class": cls,
                "canon_keys": ",".join(t["id"] for t in cs),
                "canon_anchor_prev": cw[i1 - 1]["id"] if i1 else "",
                "sections": ",".join(map(str, secs)),
                "canon_flags": ",".join(sorted({f for t in cs for f in t["flags"]})),
                "node_keys": ";".join(nodes_of(t["id"]) for t in cs),
                "witness_word_index": f"{j1}:{j2}",
                "witness_section": ",".join(sorted({str(t["section"]) for t in ws})),
                "canon_codepoints": " | ".join(cps(t["raw"]) for t in cs),
                "witness_codepoints": " | ".join(cps(t["raw"]) for t in ws),
            })
        align[wid] = (cw, ww, cmap)

        # punctuation between consecutive equal word anchors (canonical side order)
        c_all = ctoks
        w_all = wtoks
        c_word_pos = [k for k, t in enumerate(c_all) if t["kind"] == "w"]
        w_word_pos = [k for k, t in enumerate(w_all) if t["kind"] == "w"]
        anchors = [(i, cmap[i][0]) for i in range(len(cw)) if i in cmap and len(cmap[i]) == 1
                   and cw[i]["key"] == ww[cmap[i][0]]["key"]]
        anchors = [(-1, -1)] + anchors + [(len(cw), len(ww))]
        p_equal = p_rows = c_acc = w_acc = 0
        pclasses = {}
        for (ci, wi), (cn, wn) in zip(anchors, anchors[1:]):
            c_lo = c_word_pos[ci] + 1 if ci >= 0 else 0
            c_hi = c_word_pos[cn] if cn < len(cw) else len(c_all)
            w_lo = w_word_pos[wi] + 1 if wi >= 0 else 0
            w_hi = w_word_pos[wn] if wn < len(ww) else len(w_all)
            spans_word_op = cn != ci + 1 or wn != wi + 1
            cp = [t for t in c_all[c_lo:c_hi] if t["kind"] == "p"]
            wp = [t for t in w_all[w_lo:w_hi] if t["kind"] == "p"]
            c_acc += len(cp)
            w_acc += len(wp)
            if [t["raw"] for t in cp] == [t["raw"] for t in wp]:
                p_equal += len(cp)
                continue
            wset = "".join(t["raw"] for t in wp)
            cset = "".join(t["raw"] for t in cp)

            def drop(s, chars):
                return "".join(c for c in s if c not in chars)
            w_noq, c_noq = drop(wset, QUOTES), drop(cset, QUOTES)
            if w_noq == c_noq:
                pcls = "quotation_mark_only"
            elif drop(w_noq, EDITORIAL) == drop(c_noq, EDITORIAL):
                pcls = "editorial_sign_only"
            elif "…" in wset or "..." in wset:
                pcls = "witness_ellipsis"
            else:
                pcls = "punctuation_mark"
            if spans_word_op:
                pcls = "gap_with_word_difference:" + pcls
            pclasses[pcls] = pclasses.get(pcls, 0) + 1
            p_rows += 1
            punct_rows.append({
                "witness": wid, "class": pcls,
                "anchor_before": cw[ci]["id"] if ci >= 0 else "", "anchor_after": cw[cn]["id"] if cn < len(cw) else "",
                "section": cw[ci]["section"] if ci >= 0 else cw[0]["section"],
                "canon_keys": ",".join(t["id"] for t in cp),
                "node_keys": ";".join(nodes_of(t["id"]) for t in cp),
                "canon_codepoints": " ".join(cps(t["raw"]) for t in cp),
                "witness_codepoints": " ".join(cps(t["raw"]) for t in wp),
            })
        c_total = sum(1 for t in c_all if t["kind"] == "p")
        w_total = sum(1 for t in w_all if t["kind"] == "p")
        assert (c_acc, w_acc) == (c_total, w_total), (wid, c_acc, c_total, w_acc, w_total)
        # structure: section markers sequence and rubric lines
        seq = [m["section"] for m in markers]
        for k, m in enumerate(markers):
            prev = seq[k - 1] if k else 0
            struct_rows.append({"witness": wid, "kind": "section_marker", "value": m["section"],
                                "at_witness_token": m["after_token"],
                                "note": "" if m["section"] == prev + 1 else f"non_sequential_after_{prev}"})
        summary["witnesses"][wid] = {
            "word_tokens": len(ww), "punct_tokens": len(wtoks) - len(ww),
            "elision_marks_converted_by_K2": elision_converted,
            "equal_word_tokens": eq_tokens, "nonequal_word_ops": sum(classes.values()),
            "nonequal_word_ops_by_class": classes,
            "key_equal_raw_different": sum(1 for r in keq_rows if r["witness"] == wid),
            "punct_accounted": {"canonical": c_acc, "witness": w_acc},
            "punct_equal_tokens_in_equal_gaps": p_equal, "punct_nonequal_gaps": p_rows,
            "punct_nonequal_gaps_by_class": pclasses,
            "section_markers": len(markers),
            "section_markers_non_sequential": sum(1 for r in struct_rows if r["witness"] == wid and r["note"]),
            "canonical_words_aligned": sum(1 for i in range(len(cw)) if i in cmap and cmap[i]),
            "bounds": {
                "first_canonical_word_aligned_to_first_witness_word": cmap.get(0) == [0],
                "last_canonical_word_aligned_to_last_witness_word": cmap.get(len(cw) - 1) == [len(ww) - 1],
                "first_op_equal": ops[0][0] == "equal", "last_op_equal": ops[-1][0] == "equal",
                "section_marker_first_last": [seq[0], seq[-1]] if seq else [],
            },
        }

    # cross-witness: do MN and WS share their departures from C?
    shared = 0
    both = 0
    cwlen = len(align["MN"][0])
    for i in range(cwlen):
        a = align["MN"][2].get(i, [])
        b = align["WS"][2].get(i, [])
        ra = "".join(align["MN"][1][j]["key"] for j in a)
        rb = "".join(align["WS"][1][j]["key"] for j in b)
        c = align["MN"][0][i]["key"]
        if ra != c and rb != c:
            both += 1
            if ra == rb:
                shared += 1
    mn_w = [t["key"] for t in witnesses["MN"][0] if t["kind"] == "w"]
    ws_w = [t["key"] for t in witnesses["WS"][0] if t["kind"] == "w"]
    mw_ops = [op for op in difflib.SequenceMatcher(None, mn_w, ws_w, autojunk=False).get_opcodes() if op[0] != "equal"]
    summary["cross_witness"] = {
        "canonical_words_where_both_witnesses_differ": both,
        "of_which_identical_witness_reading": shared,
        "mn_vs_ws_nonequal_word_ops": len(mw_ops),
        "mn_vs_ws_word_tokens_in_nonequal": sum(i2 - i1 + j2 - j1 for _, i1, i2, j1, j2 in mw_ops),
    }

    # named checks
    def reading(wid, cid):
        cw, ww, cmap = align[wid]
        idx = next(i for i, t in enumerate(cw) if t["id"] == cid)
        js = cmap.get(idx, [])
        raw = " | ".join(ww[j]["raw"] for j in js)
        wkey = "".join(ww[j]["key"] for j in js)
        status = ("equal_raw" if raw == cw[idx]["raw"] else "equal_key" if wkey == cw[idx]["key"]
                  else "differs" if js else "absent")
        return raw, status

    named = []
    named.append(("sentence125_variant", "cg2:w2180", "s125w015"))
    for cid in ("cg2:w0264", "cg2:w0266"):
        named.append(("sentence11_reversed_components", cid, nodes_of(cid)))
    for cid, r in canon_rows.items():
        if r["coverage_status"] == "candidate_only" and cid not in ("cg2:w0264", "cg2:w0266", "cg2:w2180"):
            named.append(("concatenation_candidate", cid, r["node_keys"]))
    for cid, r in canon_rows.items():
        if r["coverage_status"] == "conditional":
            named.append(("fold_conditional", cid, r["node_keys"]))
    s060 = occ["s060w009"]["canon_keys"]
    named.append(("iota_subscript_lineage_fix_0a3eab7f", s060, "s060w009"))
    for t in ctoks:
        if "label" in t["flags"]:
            named.append(("rubric_label", t["id"], nodes_of(t["id"])))
    for t in ctoks:
        if t["kind"] == "w" and any(f in ("del", "add") for f in t["flags"]):
            marks = "_".join(f for f in t["flags"] if f in ("del", "add"))
            named.append(("editorial_" + marks, t["id"], nodes_of(t["id"])))
    for name, cid, nkeys in named:
        row = {"check": name, "canon_key": cid, "node_keys": nkeys}
        for wid in ("MN", "WS"):
            raw, st = reading(wid, cid)
            row[f"{wid}_status"] = st
            row[f"{wid}_codepoints"] = " | ".join(cps(x) for x in raw.split(" | ")) if raw else ""
        named_rows.append(row)
    # s002w023: punctuation after the canonical token bound to s002w022
    prev_c = occ["s002w022"]["canon_keys"]
    for wid in ("MN", "WS"):
        cw, ww, cmap = align[wid]
        wtoks = witnesses[wid][0]
        idx = next(i for i, t in enumerate(cw) if t["id"] == prev_c)
        j = cmap[idx][-1]
        wpos = [k for k, t in enumerate(wtoks) if t["kind"] == "w"]
        after = []
        k = wpos[j] + 1
        while k < len(wtoks) and wtoks[k]["kind"] == "p":
            after.append(wtoks[k]["raw"])
            k += 1
        named_rows.append({"check": "sentence2_punct_s002w023", "canon_key": f"after:{prev_c}",
                           "node_keys": "s002w023", f"{wid}_status": "punct_present" if after else "no_punct",
                           f"{wid}_codepoints": cps("".join(after))})
    # merge the two s002 rows
    s2 = [r for r in named_rows if r["check"] == "sentence2_punct_s002w023"]
    merged = dict(s2[0])
    merged.update({k: v for k, v in s2[1].items() if k.startswith("WS")})
    named_rows = [r for r in named_rows if r["check"] != "sentence2_punct_s002w023"] + [merged]
    # lacuna: witness tokens between the canonical neighbours of each lacuna
    for off in can["lacunae"]:
        cw = align["MN"][0]
        before = max((t for t in cw if t.get("start", 0) < off and not t["id"].startswith("cg2:L")),
                     key=lambda t: t["start"])
        row = {"check": "lacuna_gap", "canon_key": f"after:{before['id']}", "node_keys": nodes_of(before["id"])}
        for wid in ("MN", "WS"):
            cw, ww, cmap = align[wid]
            wtoks = witnesses[wid][0]
            idx = next(i for i, t in enumerate(cw) if t["id"] == before["id"])
            j = cmap[idx][-1]
            wpos = [k for k, t in enumerate(wtoks) if t["kind"] == "w"]
            nxt = wpos[j + 1] if j + 1 < len(wpos) else len(wtoks)
            between = "".join(t["raw"] for t in wtoks[wpos[j] + 1:nxt])
            row[f"{wid}_status"] = "marks_between_words" if between else "nothing_between_words"
            row[f"{wid}_codepoints"] = cps(between)
        named_rows.append(row)

    # printed-page point checks (Hude OCT scan transcriptions, fixtures/)
    point_rows = []
    if fixture:
        agdt = agdt_printed_nodes(inputs)
        node_of_canon = {k: [x for x in r["node_keys"].split(";") if x] for k, r in canon_rows.items()}
        fx = read_tsv(fixture)
        for r in fx:
            rd = point_readings(r["point"], r["kind"], ctoks, align, witnesses, agdt, node_of_canon,
                                can["lacunae"])
            hude = decode_cps(r["hude_codepoints"])
            row = {k: r[k] for k in ("point", "kind", "section", "hude_item", "hude_leaf", "hude_line", "note")}
            row["hude_codepoints"] = r["hude_codepoints"]
            for s in ("C", "AGDT", "MN", "WS"):
                row[f"{s}_codepoints"] = cps(rd[s])
                row[f"{s}_vs_hude"] = ("unavailable" if rd[s] == "?" else
                                       "equal_raw" if rd[s] == hude else
                                       "equal_key" if same(rd[s], hude, r["kind"]) else "differs")
            point_rows.append(row)
        # every word-ledger row must have a printed-page point
        pts = {r["point"] for r in fx}
        missing = []
        for r in word_rows:
            for p in ([k for k in r["canon_keys"].split(",") if k] or [f"after:{r['canon_anchor_prev']}"]):
                if p not in pts:
                    missing.append((r["witness"], p))
        summary["hude_point_checks"] = {
            "points": len(point_rows),
            "word_ledger_points_without_page_check": sorted({p for _, p in missing}),
            "by_stream": {s: dict(sorted(
                {v: sum(1 for x in point_rows if x[f"{s}_vs_hude"] == v)
                 for v in {x[f"{s}_vs_hude"] for x in point_rows}}.items()))
                for s in ("C", "AGDT", "MN", "WS")},
        }

    def w(name, rows, fields):
        with open(os.path.join(out, name), "w", newline="", encoding="ascii") as f:
            wr = csv.DictWriter(f, fieldnames=fields, delimiter="\t", lineterminator="\n")
            wr.writeheader()
            for r in rows:
                wr.writerow({k: r.get(k, "") for k in fields})

    w("witness_word_ledger.tsv", word_rows,
      ["witness", "op", "class", "canon_keys", "canon_anchor_prev", "sections", "canon_flags", "node_keys",
       "witness_word_index", "witness_section", "canon_codepoints", "witness_codepoints"])
    w("witness_punct_ledger.tsv", punct_rows,
      ["witness", "class", "anchor_before", "anchor_after", "section", "canon_keys", "node_keys",
       "canon_codepoints", "witness_codepoints"])
    w("witness_key_equalities.tsv", keq_rows, ["witness", "canon_key", "witness_word_index", "class", "node_keys"])
    w("witness_named_checks.tsv", named_rows,
      ["check", "canon_key", "node_keys", "MN_status", "MN_codepoints", "WS_status", "WS_codepoints"])
    w("witness_structure.tsv", struct_rows, ["witness", "kind", "value", "at_witness_token", "note"])
    if point_rows:
        w("witness_point_checks.tsv", point_rows,
          ["point", "kind", "section", "hude_item", "hude_leaf", "hude_line", "hude_codepoints",
           "C_codepoints", "C_vs_hude", "AGDT_codepoints", "AGDT_vs_hude", "MN_codepoints", "MN_vs_hude",
           "WS_codepoints", "WS_vs_hude", "note"])
    named_counts = {}
    for r in named_rows:
        k = (r["check"], r["MN_status"], r["WS_status"])
        named_counts["|".join(k)] = named_counts.get("|".join(k), 0) + 1
    summary["named_checks"] = named_counts
    summary["witness_inputs_sha256"] = WITNESS_SHA
    with open(os.path.join(out, "witness_summary.json"), "w", encoding="ascii") as f:
        json.dump(summary, f, indent=1, sort_keys=True)
        f.write("\n")
    print(json.dumps(summary, indent=1, sort_keys=True))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4] if len(sys.argv) > 4 else None)
