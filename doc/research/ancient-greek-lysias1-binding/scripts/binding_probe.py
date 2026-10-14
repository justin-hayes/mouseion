#!/usr/bin/env python3
"""Lysias I text authority and occurrence binding probe (#2048).

Stdlib only. Run with `python3 -I`:

    python3 -I binding_probe.py <inputs_dir> <manifest_out_dir> [<detail_dir>]

<inputs_dir> is produced by scripts/fetch_inputs.sh. The probe reads the pinned
AGDT v2.1 file (native annotation), the pinned canonical-greekLit grc2 text
(canonical running text), and two earlier AGDT releases (v2.0, v1.6) used only
for cross-release stability of the same native node IDs.

Outputs under <manifest_out_dir> contain ASCII keys, code-point spans, classes,
reasons, counts and SHA-256 digests only. Greek surfaces are not written there
(see the report's rights section). If <detail_dir> is given, per-occurrence
surfaces are written there for local inspection; it is scratch and not committed.

Conventions:
- Canonical running text: <p> text of each edition section in document order.
  <note> apparatus is excluded (its tail is kept). <label> rubric text is
  excluded from the running text and recorded separately with its element
  address. <del>, <add> and <q> text is kept and flagged. <gap reason="lost">
  is a lacuna marker that adds no characters. Whitespace runs collapse to one
  space; paragraphs are joined by "\\n". Spans are half-open code-point offsets
  into that string, whose SHA-256 is recorded.
- Word tokens are maximal runs of characters in Unicode categories L* or M*
  (this includes U+02BC and U+0313). Every other non-space character is a
  punctuation token.
- Binding order: exact NFC equality in difflib blocks; then, inside unequal
  blocks, elision-fold equality (final elision-like mark unified to U+02BC,
  NFD); then exact concatenation of 2-3 consecutive AGDT nodes whose
  hyphen-stripped forms concatenate to one canonical token. A reversed-order
  concatenation is recorded as a candidate only and is never bound.
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
WITNESSES = [
    ("agdt-v2.0", "bf4334f0af5e13d16b04c1cccd6237e683ac6f5f", "v2.0/Greek/nonArethusaCompliant/tlg0540.tlg001.perseus-grc1.tb.xml", "witnesses/agdt-v2.0.xml"),
    ("agdt-v1.6", "bf4334f0af5e13d16b04c1cccd6237e683ac6f5f", "v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml", "witnesses/agdt-v1.6.xml"),
]
TEI = "{http://www.tei-c.org/ns/1.0}"
ELISION_LIKE = ("̓", "ʼ", "’", "'")
LACUNA = ""  # private-use marker, removed after normalisation


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


# ------------------------------------------------------------ canonical text

def para_chars(p):
    """Return (chars, labels) for one <p>. chars is a list of (char, flags)."""
    chars, labels = [], []

    def add(text, flags):
        for ch in text:
            chars.append((ch, flags))

    def walk(el, flags):
        tag = local(el.tag)
        if tag == "note":
            return
        if tag == "label":
            labels.append("".join(el.itertext()))
            return
        if tag == "gap":
            chars.append((LACUNA, frozenset({"lacuna"})))
            return
        f = flags | {tag} if tag in ("del", "add", "q", "foreign") else flags
        if el.text:
            add(el.text, f)
        for child in el:
            walk(child, f)
            if child.tail:
                add(child.tail, f)

    if p.text:
        add(p.text, frozenset())
    for child in p:
        walk(child, frozenset())
        if child.tail:
            add(child.tail, frozenset())
    return chars, labels


def normalise_ws(chars):
    out, prev_space = [], True
    for ch, f in chars:
        if ch == LACUNA:
            out.append((ch, f))
            prev_space = False
        elif ch.isspace():
            if not prev_space:
                out.append((" ", frozenset()))
                prev_space = True
        else:
            out.append((ch, f))
            prev_space = False
    while out and out[-1][0] == " ":
        out.pop()
    return out


def canonical(path):
    root = ET.parse(path).getroot()
    body = root.find(f"{TEI}text/{TEI}body")
    sections = [d for d in body.iter(f"{TEI}div") if d.get("subtype") == "section"]
    ns = [int(d.get("n")) for d in sections]
    assert ns == list(range(1, len(sections) + 1)), ns
    flat, para_meta, labels = [], [], []
    child_tags = Counter()
    for sec in sections:
        k = 0
        for c in sec:
            child_tags[local(c.tag)] += 1
        for p in sec.findall(f"{TEI}p"):
            chars, plabels = para_chars(p)
            for lab in plabels:
                labels.append({"section": int(sec.get("n")), "para": k + 1, "text": lab})
            norm = normalise_ws(chars)
            if not any(ch not in (" ", LACUNA) for ch, _ in norm):
                continue
            k += 1
            start = len(flat)
            flat.extend(norm)
            para_meta.append({"section": int(sec.get("n")), "para": k, "start": start, "end": len(flat)})
            flat.append(("\n", frozenset()))
    if flat and flat[-1][0] == "\n":
        flat.pop()
    chars_out, flags_at, section_at, lacunae = [], [], [], []
    pm = iter(para_meta)
    cur = next(pm, None)
    for idx, (ch, f) in enumerate(flat):
        if ch == LACUNA:
            lacunae.append(len(chars_out))
            continue
        while cur is not None and idx >= cur["end"]:
            cur = next(pm, None)
        chars_out.append(ch)
        flags_at.append(f)
        section_at.append(cur["section"] if cur else None)
    text = "".join(chars_out)
    return dict(text=text, flags_at=flags_at, section_at=section_at, lacunae=lacunae,
                labels=labels, n_sections=len(sections), child_tags=dict(child_tags))


def tokens_of(text, flags_at, section_at):
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
            words.append({"start": i, "end": j, "text": text[i:j], "flags": fl, "section": section_at[i]})
            i = j
        else:
            puncts.append({"start": i, "end": i + 1, "text": ch, "flags": flags_at[i], "section": section_at[i]})
            i += 1
    return words, puncts


# ------------------------------------------------------------ AGDT nodes

def agdt_nodes(path):
    root = ET.parse(path).getroot()
    nodes = []
    for s in root.iter("sentence"):
        sid = s.get("id")
        for w in s.findall("word"):
            a = w.attrib
            if "artificial" in a:
                cls = "artificial"
            elif a.get("postag", "")[:1] == "u":
                cls = "punct"
            else:
                cls = "word"
            nodes.append({
                "sentence": sid, "word": a.get("id"), "form": a.get("form", ""),
                "lemma": a.get("lemma", ""), "postag": a.get("postag", ""),
                "relation": a.get("relation", ""), "head": a.get("head", ""),
                "artificial": a.get("artificial", ""), "class": cls,
            })
    return nodes


# ------------------------------------------------------------ alignment

def align(seq_nodes, words, text):
    """Return binding dict {node_key: record} and candidate list."""
    binding, tok_bound = {}, {w["key"]: [] for w in words}
    candidates = []

    def bind(node, toks, cls, spans, verified, reason=""):
        binding[node["key"]] = {"class": cls, "tokens": [t["key"] for t in toks], "spans": spans,
                                "verified": verified, "reason": reason}
        for t in toks:
            tok_bound[t["key"]].append(node["key"])

    keys_a = [nfc(n["form"]) for n in seq_nodes]
    keys_b = [nfc(w["text"]) for w in words]
    unbound_a, unbound_b = set(), set()

    def fold_block(i1, i2, j1, j2):
        sub_a = [elision_fold(keys_a[i]) for i in range(i1, i2)]
        sub_b = [elision_fold(keys_b[j]) for j in range(j1, j2)]
        for tag, a1, a2, b1, b2 in difflib.SequenceMatcher(None, sub_a, sub_b, autojunk=False).get_opcodes():
            if tag == "equal":
                for k in range(a2 - a1):
                    na, tb = seq_nodes[i1 + a1 + k], words[j1 + b1 + k]
                    if keys_a[i1 + a1 + k] == keys_b[j1 + b1 + k]:
                        cls = "exact_1to1"
                    elif unicodedata.normalize("NFC", na["form"]) != na["form"]:
                        cls = "fold_nfd_1to1"
                    else:
                        cls = "fold_elision_1to1"
                    bind(na, [tb], cls, [(tb["start"], tb["end"])], True)
            else:
                unbound_a.update(range(i1 + a1, i1 + a2))
                unbound_b.update(range(j1 + b1, j1 + b2))

    for tag, i1, i2, j1, j2 in difflib.SequenceMatcher(None, keys_a, keys_b, autojunk=False).get_opcodes():
        if tag == "equal":
            for k in range(i2 - i1):
                bind(seq_nodes[i1 + k], [words[j1 + k]], "exact_1to1", [(words[j1 + k]["start"], words[j1 + k]["end"])], True)
        elif tag == "replace":
            fold_block(i1, i2, j1, j2)
        elif tag == "delete":
            unbound_a.update(range(i1, i2))
        elif tag == "insert":
            unbound_b.update(range(j1, j2))

    # Concatenation: consecutive free nodes whose hyphen-stripped forms make one canonical token.
    free_a = set(unbound_a)
    for j in sorted(unbound_b):
        tb = words[j]
        target = keys_b[j]
        done = False
        for i in sorted(free_a):
            if done:
                break
            for width in (2, 3):
                window = list(range(i, i + width))
                if not all(k in free_a for k in window):
                    continue
                forms = [seq_nodes[k]["form"] for k in window]
                if nfc("".join(strip_hyphen(f) for f in forms)) == target:
                    spans, pos = [], tb["start"]
                    for f in forms:
                        s = strip_hyphen(f)
                        spans.append((pos, pos + len(s)))
                        pos += len(s)
                    if pos != tb["end"]:
                        continue
                    for k, sp in zip(window, spans):
                        bind(seq_nodes[k], [tb], "concat_exact", [sp], True,
                             "hyphen-marked split forms; exact code-point concatenation")
                    free_a.difference_update(window)
                    done = True
                    break
                if nfc("".join(strip_hyphen(f) for f in reversed(forms))) == target and width == 2:
                    candidates.append({"token": tb["key"], "nodes": [seq_nodes[k]["key"] for k in window],
                                       "forms_len": [len(f) for f in forms]})
    for i in sorted(free_a):
        k = seq_nodes[i]["key"]
        if k not in binding:
            binding[k] = {"class": "unbound", "tokens": [], "spans": [], "verified": False,
                          "reason": "no exact, fold or concatenation correspondence in its aligned block"}
    unbound_tokens = [words[j]["key"] for j in sorted(unbound_b) if not tok_bound[words[j]["key"]]]
    return binding, tok_bound, candidates, unbound_tokens


# ------------------------------------------------------------ main

def main(inputs, out, detail=None):
    os.makedirs(out, exist_ok=True)
    agdt_file = os.path.join(inputs, "agdt", AGDT_PATH)
    canon_file = os.path.join(inputs, "canon", CG_PATH)
    assert sha256_file(agdt_file) == AGDT_SHA, "AGDT pin mismatch"
    assert sha256_file(canon_file) == CG_SHA, "canonical pin mismatch"

    can = canonical(canon_file)
    text = can["text"]
    words, puncts = tokens_of(text, can["flags_at"], can["section_at"])
    for i, w in enumerate(words, 1):
        w["key"] = f"cg2:w{i:04d}"
    for i, p in enumerate(puncts, 1):
        p["key"] = f"cg2:p{i:04d}"

    nodes = agdt_nodes(agdt_file)
    for n in nodes:
        n["key"] = f"s{int(n['sentence']):03d}w{int(n['word']):03d}"
    printed = [n for n in nodes if n["class"] != "artificial"]
    word_nodes = [n for n in printed if n["class"] == "word"]
    punct_nodes = [n for n in printed if n["class"] == "punct"]

    # Rubric nodes: a one-node native sentence whose NFC form equals a canonical <label> text.
    label_by_nfc = {nfc(l["text"]): l for l in can["labels"]}
    word_count = Counter(n["sentence"] for n in word_nodes)
    rubric = [n for n in word_nodes if word_count[n["sentence"]] == 1 and nfc(n["form"]) in label_by_nfc]
    rubric_keys = {n["key"] for n in rubric}
    seq_nodes = [n for n in word_nodes if n["key"] not in rubric_keys]

    binding, tok_bound, candidates, unbound_tokens = align(seq_nodes, words, text)
    for n in rubric:
        lab = label_by_nfc[nfc(n["form"])]
        binding[n["key"]] = {"class": "rubric_label_exact", "tokens": [], "spans": [], "verified": True,
                             "reason": f"exact match of canonical <label> (section {lab['section']}, "
                                       f"paragraph {lab['para']}); label text is not running text"}

    # ---------------------------------------------- punctuation alignment
    # One ordered stream per side. Bound words are anchors ('W', canonical index), so punctuation
    # can only pair with canonical punctuation between the same word anchors. Unbound and rubric
    # words are unique items that never match.
    tok_index = {w["key"]: i for i, w in enumerate(words)}
    a_stream, a_keys = [], []
    emitted = set()
    rubric_sentences = {n["sentence"] for n in rubric}
    for n in printed:
        if n["class"] == "punct" and n["sentence"] in rubric_sentences:
            continue  # AuxK stop attached to a rubric segment; no running-text counterpart
        if n["class"] == "punct":
            a_stream.append(("P", nfc(n["form"])))
            a_keys.append(n["key"])
        else:
            b = binding[n["key"]]
            if b["tokens"]:
                t = b["tokens"][0]
                if t not in emitted:
                    emitted.add(t)
                    a_stream.append(("W", tok_index[t]))
                    a_keys.append(t)
            else:
                a_stream.append(("U", n["key"]))
                a_keys.append(n["key"])
    b_stream, b_keys = [], []
    for i, w in enumerate(words):
        if tok_bound[w["key"]]:
            b_stream.append(("W", i))
        else:
            b_stream.append(("C", i))
        b_keys.append(w["key"])
    for p in puncts:
        b_stream.append(("P", nfc(p["text"])))
        b_keys.append(p["key"])
    # Restore document order on the canonical side (words and punctuation interleaved by position).
    order = sorted(range(len(b_stream)), key=lambda i: (words[b_stream[i][1]]["start"] if b_stream[i][0] in "WC"
                                                        else puncts[i - len(words)]["start"]))
    b_stream = [b_stream[i] for i in order]
    b_keys = [b_keys[i] for i in order]
    punct_map = {}
    punct_ops = []
    for tag, i1, i2, j1, j2 in difflib.SequenceMatcher(None, a_stream, b_stream, autojunk=False).get_opcodes():
        punct_ops.append(tag)
        if tag == "equal":
            for k in range(i2 - i1):
                ka, kb = a_keys[i1 + k], b_keys[j1 + k]
                if a_stream[i1 + k][0] == "P":
                    punct_map[ka] = (kb, "exact_punct")
    punct_counts = Counter(punct_ops)

    # ---------------------------------------------- occurrence rows
    occ = []
    cls_count = Counter()
    for n in nodes:
        base = {"key": n["key"], "sentence": n["sentence"], "word": n["word"], "node_class": n["class"],
                "postag": n["postag"], "relation": n["relation"], "head": n["head"],
                "form_codepoints": len(n["form"]), "form_sha256": sha256_text(n["form"]),
                "lemma_sha256": sha256_text(n["lemma"]), "artificial": n["artificial"],
                "flags": "", "binding_class": "", "canon_token_keys": "", "span_start": "", "span_end": "",
                "span_len": "", "verified": "", "reason": ""}
        if n["class"] == "artificial":
            base["binding_class"] = "not_printed_artificial"
            base["reason"] = f"artificial={n['artificial']}; no printed occurrence"
            cls_count["artificial"] += 1
        elif n["class"] == "punct":
            if n["sentence"] in rubric_sentences and n["key"] not in punct_map:
                base.update(binding_class="rubric_attached_punct_no_running_text",
                            reason="AuxK stop of a canonical <label> rubric segment; the label is not running text")
            elif n["key"] in punct_map:
                tk, cl = punct_map[n["key"]]
                pt = next(p for p in puncts if p["key"] == tk)
                base.update(binding_class=cl, canon_token_keys=tk, span_start=pt["start"], span_end=pt["end"],
                            span_len=1, verified=True, flags=";".join(sorted(pt["flags"])))
            else:
                base.update(binding_class="punct_unaligned", reason="no exact punctuation counterpart in aligned block")
            cls_count["punct_" + base["binding_class"]] += 1
        else:
            b = binding[n["key"]]
            base["binding_class"] = b["class"]
            base["reason"] = b["reason"]
            base["verified"] = b["verified"]
            cls_count[b["class"]] += 1
            if b["spans"]:
                base["canon_token_keys"] = ";".join(b["tokens"])
                base["span_start"] = b["spans"][0][0]
                base["span_end"] = b["spans"][-1][1]
                base["span_len"] = sum(e - s for s, e in b["spans"])
                fl = set()
                for t in b["tokens"]:
                    fl |= next(w for w in words if w["key"] == t)["flags"]
                base["flags"] = ";".join(sorted(fl))
        occ.append(base)

    # ---------------------------------------------- canonical token rows
    tok_rows = []
    tok_by_key = {w["key"]: w for w in words}
    for w in words:
        bound = tok_bound[w["key"]]
        status = "bound" if bound else "unbound_canonical_token"
        tok_rows.append({"key": w["key"], "span_start": w["start"], "span_end": w["end"], "section": w["section"],
                         "flags": ";".join(sorted(w["flags"])), "text_sha256": sha256_text(w["text"]),
                         "n_nodes": len(bound), "node_keys": ";".join(bound), "status": status,
                         "text_codepoints": len(w["text"]), "nfc": nfc(w["text"]) == w["text"]})

    # ---------------------------------------------- sentence rows
    sent_rows = []
    by_sent = {}
    for n in word_nodes:
        by_sent.setdefault(n["sentence"], []).append(n)
    for sid in sorted(by_sent, key=int):
        ns = by_sent[sid]
        cl = Counter(binding[n["key"]]["class"] for n in ns)
        tok_idx = sorted({words.index(tok_by_key[t]) for n in ns for t in binding[n["key"]]["tokens"]})
        contiguous = (not tok_idx) or (tok_idx[-1] - tok_idx[0] + 1 == len(tok_idx))
        sent_rows.append({"sentence": sid, "word_nodes": len(ns),
                          "unbound": cl.get("unbound", 0), "rubric": cl.get("rubric_label_exact", 0),
                          "exact": cl.get("exact_1to1", 0), "fold": cl.get("fold_elision_1to1", 0) + cl.get("fold_nfd_1to1", 0),
                          "concat": cl.get("concat_exact", 0),
                          "canon_token_range": f"{tok_idx[0] + 1}-{tok_idx[-1] + 1}" if tok_idx else "",
                          "canon_contiguous_bound": contiguous})

    # ---------------------------------------------- discrepancy rows
    disc = []
    for n in word_nodes:
        b = binding[n["key"]]
        if b["class"] in ("unbound", "rubric_label_exact") or b["class"].startswith("fold") or b["class"].startswith("concat"):
            disc.append({"key": n["key"], "sentence": n["sentence"], "word": n["word"], "class": b["class"],
                         "reason": b["reason"]})
    for c in candidates:
        disc.append({"key": ";".join(c["nodes"]), "sentence": "", "word": "", "class": "reversed_order_candidate_not_bound",
                     "reason": f"reversed-order hyphen-stripped concatenation equals canonical token {c['token']}"})
    for t in unbound_tokens:
        disc.append({"key": t, "sentence": "", "word": "", "class": "unbound_canonical_token",
                     "reason": "canonical word token with no bound node"})
    return {
        "text": text, "can": can, "words": words, "puncts": puncts, "nodes": nodes, "printed": printed,
        "word_nodes": word_nodes, "punct_nodes": punct_nodes, "rubric": rubric, "seq_nodes": seq_nodes,
        "binding": binding, "candidates": candidates, "unbound_tokens": unbound_tokens, "occ": occ,
        "cls_count": cls_count, "tok_rows": tok_rows, "sent_rows": sent_rows, "disc": disc,
        "punct_counts": punct_counts, "punct_ops": punct_ops, "punct_map": punct_map, "agdt_file": agdt_file,
        "canon_file": canon_file, "inputs": inputs, "out": out, "detail": detail,
    }


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
               "form_sha256", "lemma_sha256", "artificial", "binding_class", "canon_token_keys", "span_start",
               "span_end", "span_len", "verified", "flags", "reason"])
    write_tsv(os.path.join(out, "canonical_tokens.tsv"), res["tok_rows"],
              ["key", "span_start", "span_end", "section", "flags", "text_codepoints", "nfc", "text_sha256",
               "n_nodes", "node_keys", "status"])
    write_tsv(os.path.join(out, "sentences.tsv"), res["sent_rows"],
              ["sentence", "word_nodes", "exact", "fold", "concat", "rubric", "unbound", "canon_token_range",
               "canon_contiguous_bound"])
    write_tsv(os.path.join(out, "discrepancies.tsv"), res["disc"], ["key", "sentence", "word", "class", "reason"])
    summary = {
        "schema": "lysias1-binding-manifest/1",
        "source_agdt": {"rev": AGDT_REV, "path": AGDT_PATH, "sha256": AGDT_SHA},
        "source_canonical": {"rev": CG_REV, "path": CG_PATH, "sha256": CG_SHA},
        "running_text": {"sha256": sha256_text(text), "codepoints": len(text),
                         "nfc": unicodedata.normalize("NFC", text) == text,
                         "sections": res["can"]["n_sections"], "lacunae_at": res["can"]["lacunae"],
                         "labels": len(res["can"]["labels"]), "child_tags_in_sections": res["can"]["child_tags"]},
        "denominators": {"agdt_nodes": len(res["nodes"]), "artificial": res["cls_count"]["artificial"],
                         "printed": len(res["printed"]), "word_nodes": len(res["word_nodes"]),
                         "punct_nodes": len(res["punct_nodes"]),
                         "canonical_word_tokens": len(res["words"]), "canonical_punct_tokens": len(res["puncts"])},
        "word_node_classes": {k: v for k, v in sorted(res["cls_count"].items())
                              if not k.startswith("punct") and k != "artificial"},
        "punct_node_classes": {k: v for k, v in sorted(res["cls_count"].items()) if k.startswith("punct")},
        "punct_alignment_ops": dict(res["punct_counts"]),
        "canonical_tokens_unbound": len(res["unbound_tokens"]),
        "reversed_order_candidates": res["candidates"],
    }
    with open(os.path.join(out, "summary.json"), "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")
    return summary


if __name__ == "__main__":
    if len(sys.argv) not in (3, 4):
        sys.exit(__doc__)
    res = main(sys.argv[1], sys.argv[2], sys.argv[3] if len(sys.argv) == 4 else None)
    s = write_all(res)
    print(json.dumps(s["denominators"], ensure_ascii=False, indent=1))
    print(json.dumps(s["word_node_classes"], ensure_ascii=False, indent=1))
    print(json.dumps(s["punct_node_classes"], ensure_ascii=False, indent=1))
    print(json.dumps({"canonical_tokens_unbound": s["canonical_tokens_unbound"],
                      "punct_alignment_ops": s["punct_alignment_ops"],
                      "running_text": s["running_text"]["codepoints"],
                      "candidates": len(s["reversed_order_candidates"])}, ensure_ascii=False, indent=1))
