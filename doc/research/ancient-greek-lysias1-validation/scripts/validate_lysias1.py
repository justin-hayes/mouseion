#!/usr/bin/env python3
"""Validate the pinned AGDT v2.1 Lysias I file against the pinned canonical comparator (v2).

Usage (stdlib only, isolated mode):
    python3 -I validate_lysias1.py <inputs_dir> <detail_dir> <committed_dir>

Per-node, per-identity and per-sentence detail goes to <detail_dir> (scratch, not
committed because of the output-specific rights question). Aggregate counts go to
<committed_dir>/summary.json. If <detail_dir>/dictionary.json exists (written by
check_dictionary.py) its counts are added to the summary.

v2 changes from v1: the canonical file is parsed with ElementTree (TEI body, section
divs, note/label/del/add/q/gap semantics) instead of regular expressions. Every span
is checked against the canonical text itself. A shared span is bound only where
consecutive AGDT nodes concatenate exactly to adjacent canonical units; nodes are
never reordered or re-spelled. Lemma, UPOS and identity accounting covers every
printed node.
"""
import difflib
import hashlib
import json
import os
import platform
import statistics
import sys
import time
import unicodedata
import xml.etree.ElementTree as ET
from collections import Counter, defaultdict

RECON_VERSION = "lysias1-recon-v2"
AGDT_REL = "agdt/v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml"
CANON_REL = "canon/data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml"
AGDT_SHA = "44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568"
CANON_SHA = "21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a"
CAP_UNITS = 50
CAP_CODEPOINTS = 400
SELECTION_MIN_OCCURRENCES = 3  # provisional 3/15 rule; the 15-across-Books clause is subsumed (one Book)
TEI = "{http://www.tei-c.org/ns/1.0}"
DEL, ADD, QUOTE, GAP = 1, 2, 4, 8
EDITORIAL = DEL | ADD
# ASCII ';' and U+037E are Greek/Latin question marks; U+0387 and U+00B7 are ano teleia (medial by default).
TERMINALS = set(".;!?··;…")
UPOS_BY_LETTER = {"n": "NOUN", "a": "ADJ", "v": "VERB", "d": "ADV", "l": "DET", "p": "PRON",
                  "r": "ADP", "c": "CCONJ", "m": "NUM", "i": "INTJ", "g": "PART", "u": "PUNCT", "x": "X"}
# Letters whose UPOS candidate is not in doubt. d (adverb/particle) and c without AuxC
# (coordinating vs subordinating) are unresolved in the pinned guidelines.
DETERMINATE_LETTERS = set("navrmigpl")
# contentPOS in internal/enrichment/lexical.go (the only UPOS filter in the code that
# applies to lexical items). PUNCT/SYM are also excluded by internal/webapp/sentence_study.go.
NON_CONTENT_UPOS = {"PUNCT", "SYM", "SPACE", "X", "DET", "AUX", "PRON"}
RUBRIC_WORDS = {"Νόμος", "Μάρτυρες"}


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def fold_apos(s):
    return s.replace("’", "ʼ")


def word_key(s):
    """Letters, combining marks and elision marks; punctuation (incl. hyphen clitic marks) removed."""
    s = unicodedata.normalize("NFC", fold_apos(s))
    return "".join(c for c in s if c == "ʼ" or not unicodedata.category(c).startswith("P"))


def fold_key(k):
    """Elision/breathing fold: NFD, U+0313 treated as the elision mark. Used only for classification."""
    return unicodedata.normalize("NFC", unicodedata.normalize("NFD", k).replace("̓", "ʼ"))


def lemma_key(lemma):
    """Canonical lemma: NFC, lowercase, final sigma folded (ADR-approved #2033 profile)."""
    return unicodedata.normalize("NFC", unicodedata.normalize("NFC", lemma).lower().replace("ς", "σ"))


def is_word_char(c):
    return c in "ʼ’" or not (c.isspace() or unicodedata.category(c).startswith("P"))


# ---- canonical comparator ----------------------------------------------------

def walk(el, fl, out, rubrics, sec, stats):
    tag = el.tag.rsplit("}", 1)[-1]
    if tag == "note":
        stats["note_skipped"] += 1
        return
    if tag == "label":
        stats["label_skipped"] += 1
        rubrics.append((sec, " ".join("".join(el.itertext()).split())))
        return
    if tag == "milestone":
        stats["milestone_in_section"] += 1
        return
    if tag == "gap":
        stats["gap_reason_" + el.get("reason", "unspecified")] += 1
        out.append((None, GAP, sec))
        return
    sub = fl | {"del": DEL, "add": ADD, "q": QUOTE}.get(tag, 0)
    if tag in ("del", "add", "q"):
        stats["element_" + tag] += 1
    if el.text:
        out.append((el.text, sub, sec))
    for child in el:
        walk(child, sub, out, rubrics, sec, stats)
        if child.tail:
            out.append((child.tail, sub, sec))


def parse_canonical(path):
    root = ET.parse(path).getroot()
    editions = [d for d in root.iter(TEI + "div") if d.get("type") == "edition"]
    assert len(editions) == 1, "expected one edition div"
    edition = editions[0]
    stats = Counter()
    rubrics = []
    sections = []
    for child in edition:
        tag = child.tag.rsplit("}", 1)[-1]
        if child.tag == TEI + "div" and child.get("subtype") == "section":
            sections.append((int(child.get("n")), child))
        else:
            stats["edition_child_not_section:" + tag] += 1
    stats["sections_found"] = len(sections)
    sec_numbers = [n for n, _ in sections]
    stats["section_numbers_consecutive"] = int(sec_numbers == list(range(1, len(sec_numbers) + 1)))
    stats["section_divs_nested"] = sum(1 for _, d in sections for _ in d.iter(TEI + "div")) - len(sections)

    per_section = []
    for n, sec in sections:
        out = []
        walk(sec, 0, out, rubrics, n, stats)
        per_section.append((n, out))

    # Build running text: whitespace collapsed; sections joined by one space; gaps mark the next char.
    chars, flags, secs = [], [], []
    for n, out in per_section:
        cs, fs = [], []
        pending = 0
        for text, fl, _ in out:
            if text is None:
                pending |= GAP
                continue
            for ch in text:
                if ch.isspace():
                    if not cs or cs[-1] == " ":
                        continue
                    ch = " "
                cs.append(ch)
                fs.append(fl | pending)
                pending = 0
        while cs and cs[-1] == " ":
            cs.pop()
            fs.pop()
        if not cs:
            continue
        if chars:
            chars.append(" ")
            flags.append(0)
            secs.append(n)
        chars.extend(cs)
        flags.extend(fs)
        secs.extend([n] * len(cs))
    running = "".join(chars)

    # Units: word runs and single punctuation characters.
    units = []
    i = 0
    while i < len(running):
        c = running[i]
        if c == " ":
            i += 1
            continue
        if is_word_char(c):
            j = i
            fl = 0
            while j < len(running) and is_word_char(running[j]):
                fl |= flags[j]
                j += 1
            units.append({"kind": "W", "s": i, "e": j, "flags": fl, "sec": secs[i]})
            i = j
        else:
            units.append({"kind": "P", "s": i, "e": i + 1, "flags": flags[i], "sec": secs[i]})
            i += 1
    stats["units_total"] = len(units)
    stats["word_units"] = sum(u["kind"] == "W" for u in units)
    stats["punct_units"] = sum(u["kind"] == "P" for u in units)
    stats["chars_with_del"] = sum(1 for f in flags if f & DEL)
    stats["chars_with_add"] = sum(1 for f in flags if f & ADD)
    stats["chars_with_quote"] = sum(1 for f in flags if f & QUOTE)
    stats["units_with_gap_before"] = sum(1 for u in units if flags[u["s"]] & GAP)
    stats["running_nfc"] = int(unicodedata.is_normalized("NFC", running))
    return running, flags, secs, units, rubrics, stats, sec_numbers


# ---- AGDT --------------------------------------------------------------------

def load_agdt(path):
    nodes = []
    kinds = Counter()
    sents = []
    root = ET.parse(path).getroot()
    for si, s in enumerate(root.iter("sentence")):
        sents.append({"id": s.get("id"), "subdoc": s.get("subdoc")})
        ids = [int(w.get("id")) for w in s.findall("word")]
        assert ids == sorted(ids), f"word ids not ascending in sentence {s.get('id')}"
        for w in s.findall("word"):
            a = dict(w.attrib)
            form = a.get("form", "")
            if a.get("artificial") is not None:
                kinds["artificial:" + ("with_form" if form else "empty")] += 1
                continue
            if not form:
                kinds["printed_candidate:empty_form"] += 1
                continue
            if form.startswith("["):
                kinds["bracket_form"] += 1
                continue
            kinds["printed"] += 1
            letter = a.get("postag", "")[:1]
            relation = a.get("relation", "")
            upos = UPOS_BY_LETTER.get(letter, "UNMAPPED")
            if letter == "v" and relation == "AuxV":
                upos = "AUX"
            if letter == "c" and relation == "AuxC":
                upos = "SCONJ"
            nodes.append({"sent": si, "id": a["id"], "form": form, "lemma": a.get("lemma", ""),
                          "postag": a.get("postag", ""), "letter": letter, "relation": relation,
                          "upos": upos, "key": word_key(form), "tok": None, "bind": None})
    return sents, nodes, kinds


def main():
    t0 = time.time()
    inputs, detail_dir, committed_dir = sys.argv[1:4]
    os.makedirs(detail_dir, exist_ok=True)
    os.makedirs(committed_dir, exist_ok=True)
    agdt_path = os.path.join(inputs, AGDT_REL)
    canon_path = os.path.join(inputs, CANON_REL)
    got_a, got_c = sha256(agdt_path), sha256(canon_path)
    if got_a != AGDT_SHA or got_c != CANON_SHA:
        raise SystemExit(f"pin mismatch: agdt={got_a} canon={got_c}")

    sents, nodes, node_kinds = load_agdt(agdt_path)
    A = []
    for idx, n in enumerate(nodes):
        if n["key"]:
            n["tok"] = len(A)
            A.append(n["key"])
    tok_node = {n["tok"]: i for i, n in enumerate(nodes) if n["tok"] is not None}

    running, flags, secs, units, rubrics, cstats, sec_numbers = parse_canonical(canon_path)
    W = [u for u in units if u["kind"] == "W"]
    C = [word_key(running[u["s"]:u["e"]]) for u in W]
    fA = [fold_key(x) for x in A]
    fC = [fold_key(x) for x in C]

    # ---- binding -----------------------------------------------------------
    bind = [None] * len(A)
    cbound = [False] * len(W)
    verify_failures = []
    blocks = []

    def contiguous(bi, be):
        return all(W[k]["e"] == W[k + 1]["s"] and W[k]["sec"] == W[k + 1]["sec"] for k in range(bi, be - 1))

    def bind_one(ai, bi, cls):
        u = W[bi]
        seg = running[u["s"]:u["e"]]
        ok = word_key(seg) == A[ai] if cls == "exact" else fold_key(word_key(seg)) == fold_key(A[ai])
        if not ok:
            verify_failures.append(ai)
            return
        bind[ai] = {"cls": cls, "s": u["s"], "e": u["e"], "units": [bi]}
        cbound[bi] = True

    def bind_concat(ai, ae, bi, be):
        base, end = W[bi]["s"], W[be - 1]["e"]
        if word_key(running[base:end]) != "".join(A[ai:ae]):
            verify_failures.append(("concat", ai, ae))
            return False
        pos = base
        for k in range(ai, ae):
            seg = running[pos:pos + len(A[k])]
            if len(seg) != len(A[k]) or word_key(seg) != A[k]:
                verify_failures.append(("concat_part", k))
                return False
            bind[k] = {"cls": "concat_exact", "s": pos, "e": pos + len(A[k]), "units": list(range(bi, be))}
            pos += len(A[k])
        for c in range(bi, be):
            cbound[c] = True
        return True

    def block_reason(op, ai, ae, bi, be):
        keys = A[ai:ae]
        if op == "delete":
            return "agdt_rubric_label" if all(k in RUBRIC_WORDS for k in keys) else "agdt_only"
        if op == "insert":
            fl = 0
            for u in W[bi:be]:
                fl |= u["flags"]
            if fl & EDITORIAL:
                return "canon_only_editorial"
            return "canon_only_quote" if fl & QUOTE else "canon_only"
        if (ae - ai) == (be - bi) == 1:
            return "replace_1to1_unexplained"
        return "replace_block_unexplained"

    sm = difflib.SequenceMatcher(None, A, C, autojunk=False)
    for tag, i1, i2, j1, j2 in sm.get_opcodes():
        if tag == "equal":
            for k in range(i2 - i1):
                bind_one(i1 + k, j1 + k, "exact")
            continue
        sub = difflib.SequenceMatcher(None, fA[i1:i2], fC[j1:j2], autojunk=False)
        for stag, a1, a2, b1, b2 in sub.get_opcodes():
            ai, ae, bi, be = i1 + a1, i1 + a2, j1 + b1, j1 + b2
            if stag == "equal":
                for k in range(ae - ai):
                    bind_one(ai + k, bi + k, "exact" if A[ai + k] == C[bi + k] else "fold")
                continue
            done = False
            if ae > ai and be > bi and contiguous(bi, be) and "".join(A[ai:ae]) == "".join(C[bi:be]):
                done = bind_concat(ai, ae, bi, be)
            if not done:
                blocks.append({"op": stag, "agdt_nodes": [nodes[tok_node[k]]["id"] for k in range(ai, ae)],
                               "agdt_sentences": sorted({nodes[tok_node[k]]["sent"] for k in range(ai, ae)}),
                               "agdt_keys": A[ai:ae], "canon_keys": C[bi:be],
                               "canon_flags": [u["flags"] for u in W[bi:be]],
                               "class": block_reason(stag, ai, ae, bi, be)})

    tok_class = Counter(b["cls"] if b else "unbound" for b in bind)
    canon_unbound = [i for i, ok in enumerate(cbound) if not ok]
    for i, n in enumerate(nodes):
        if n["tok"] is not None:
            n["bind"] = bind[n["tok"]]

    # ---- sentences ----------------------------------------------------------
    by_sent = defaultdict(list)
    for i, n in enumerate(nodes):
        by_sent[n["sent"]].append(i)
    sent_rows = []
    sent_status = {}
    for si in range(len(sents)):
        idxs = by_sent.get(si, [])
        toks = [i for i in idxs if nodes[i]["tok"] is not None]
        row = {"sent": si, "id": sents[si]["id"], "subdoc": sents[si]["subdoc"],
               "printed_nodes": len(idxs), "word_tokens": len(toks)}
        if not toks:
            row["status"] = "no_word_tokens"
            sent_rows.append(row)
            sent_status[si] = "no_word_tokens"
            continue
        problems = []
        for i in toks:
            if nodes[i]["bind"] is None:
                problems.append("unbound_token:" + nodes[i]["id"])
        for i, j in zip(toks, toks[1:]):
            if nodes[i]["bind"] is None or nodes[j]["bind"] is None:
                continue
            between = [k for k in idxs if i < k < j]
            exp = fold_apos("".join(nodes[k]["form"] for k in between))
            gap = fold_apos(running[nodes[i]["bind"]["e"]:nodes[j]["bind"]["s"]]).replace(" ", "")
            if gap != exp:
                problems.append(f"interword:{nodes[i]['id']}->{nodes[j]['id']}")
        if all(nodes[i]["bind"] is not None for i in toks):
            first, last = toks[0], toks[-1]
            s0, e0 = nodes[first]["bind"]["s"], nodes[last]["bind"]["e"]
            lead = fold_apos("".join(nodes[k]["form"] for k in idxs if k < first))
            trail = fold_apos("".join(nodes[k]["form"] for k in idxs if k > last))
            if lead:
                left = fold_apos(running[:s0]).rstrip(" ")
                if left.endswith(lead):
                    s0 = len(left) - len(lead)
                else:
                    problems.append("leading_punct_mismatch")
            if trail:
                right = fold_apos(running[e0:])
                r2 = right.lstrip(" ")
                if r2.startswith(trail):
                    e0 = e0 + (len(right) - len(r2)) + len(trail)
                else:
                    problems.append("trailing_punct_mismatch")
            text = running[s0:e0]
            row.update({"start": s0, "end": e0, "text": text, "units": len(text.split()),
                        "codepoints": len(text),
                        "start_context": ("text_start" if s0 == 0 else
                                          "section_start" if secs[s0] != secs[s0 - 1] else
                                          ("after_terminal" if running[:s0].rstrip(" ")[-1:] in TERMINALS
                                           else "after_nonterminal")),
                        "end_context": "terminal" if text[-1:] in TERMINALS else "nonterminal",
                        "medial_terminals": sum(c in TERMINALS for c in text[:-1]),
                        "over_units_cap": len(text.split()) > CAP_UNITS,
                        "over_codepoints_cap": len(text) > CAP_CODEPOINTS,
                        "editorial_flags_in_text": sum(1 for f in flags[s0:e0] if f & EDITORIAL),
                        "quote_flags_in_text": sum(1 for f in flags[s0:e0] if f & QUOTE),
                        "gap_flags_in_text": sum(1 for f in flags[s0:e0] if f & GAP)})
        row["problems"] = problems
        row["status"] = "verified" if not problems else "problem"
        sent_rows.append(row)
        sent_status[si] = row["status"]

    # ---- vocabulary accounting ---------------------------------------------
    upos_counts = Counter(n["upos"] for n in nodes)
    letter_counts = Counter(n["letter"] or "∅" for n in nodes)
    content_nodes = [i for i, n in enumerate(nodes)
                     if n["upos"] not in NON_CONTENT_UPOS and n["upos"] != "UNMAPPED"]
    lemma_flags = Counter()
    for n in nodes:
        if not n["lemma"] or n["lemma"] == "undefined":
            lemma_flags["missing_or_undefined"] += 1
        if "#" in n["lemma"]:
            lemma_flags["homograph_digit_marker"] += 1
        if len(n["postag"]) != 9:
            lemma_flags["postag_length_not_9"] += 1

    def occurrence(i):
        n = nodes[i]
        reasons = []
        if not n["lemma"] or n["lemma"] == "undefined":
            reasons.append("empty_headword")
        if n["bind"] is None:
            reasons.append("unbound_token")
        else:
            row = sent_rows[n["sent"]]
            if sent_status.get(n["sent"]) != "verified":
                reasons.append("sentence_unverified")
            if row.get("over_units_cap"):
                reasons.append("over_units_cap")
            if row.get("over_codepoints_cap"):
                reasons.append("over_codepoints_cap")
            if flags[n["bind"]["s"]] & EDITORIAL:
                reasons.append("editorial_del_add_token")
        return reasons

    identities = defaultdict(list)
    for i in content_nodes:
        n = nodes[i]
        if n["lemma"] and n["lemma"] != "undefined":
            identities[(lemma_key(n["lemma"]), n["upos"])].append(i)

    id_rows = {}
    upos_block = Counter()
    for (lk, upos), occ in sorted(identities.items(), key=lambda kv: (-len(kv[1]), kv[0])):
        raw = sorted({nodes[i]["lemma"] for i in occ})
        determinate = all(nodes[i]["letter"] in DETERMINATE_LETTERS or
                          (nodes[i]["letter"] == "c" and nodes[i]["relation"] == "AuxC") for i in occ)
        selected = len(occ) >= SELECTION_MIN_OCCURRENCES
        reasons = Counter()
        earliest = None
        for i in occ:
            r = occurrence(i)
            for x in r:
                reasons[x] += 1
            if not r and earliest is None:
                earliest = i
        if not selected:
            status = "not_selected"
        elif not determinate:
            status = "upos_unresolved"
            if earliest is not None:
                upos_block["upos_unresolved_but_eligible_occurrence"] += 1
        elif earliest is None:
            status = "no_eligible_occurrence"
        else:
            status = "card_candidate"
        id_rows[f"{lk}|{upos}"] = {
            "canonical_lemma": lk, "upos_candidate": upos, "raw_lemmas": raw,
            "occurrences": len(occ), "determinate": determinate, "selected": selected,
            "bound_exact": sum(1 for i in occ if nodes[i]["bind"] and nodes[i]["bind"]["cls"] == "exact"),
            "bound_fold": sum(1 for i in occ if nodes[i]["bind"] and nodes[i]["bind"]["cls"] == "fold"),
            "bound_concat": sum(1 for i in occ if nodes[i]["bind"] and nodes[i]["bind"]["cls"] == "concat_exact"),
            "unbound": sum(1 for i in occ if nodes[i]["bind"] is None),
            "occurrence_reasons": dict(reasons), "status": status,
            "earliest_candidate": None if earliest is None else {
                "sent": nodes[earliest]["sent"], "node_id": nodes[earliest]["id"], "form": nodes[earliest]["form"],
                "sentence_id": sents[nodes[earliest]["sent"]]["id"]},
        }
    id_status = Counter(r["status"] for r in id_rows.values())
    selected_ids = [r for r in id_rows.values() if r["selected"]]
    selected_raw = sorted({x for r in selected_ids for x in r["raw_lemmas"]})

    # ---- detail (scratch only) -----------------------------------------------
    def dump(name, obj):
        with open(os.path.join(detail_dir, name), "w", encoding="utf-8") as f:
            json.dump(obj, f, ensure_ascii=False, indent=1)

    with open(os.path.join(detail_dir, "canonical_running_text.txt"), "w", encoding="utf-8") as f:
        f.write(running + "\n")
    dump("nodes.json", [{k: n[k] for k in ("sent", "id", "form", "lemma", "postag", "relation", "upos")}
                        | {"tok_class": n["bind"]["cls"] if n["bind"] else None,
                           "canon_span": [n["bind"]["s"], n["bind"]["e"]] if n["bind"] else None,
                           "canon_text": running[n["bind"]["s"]:n["bind"]["e"]] if n["bind"] else None}
                       for n in nodes])
    dump("blocks.json", blocks)
    dump("sentences.json", sent_rows)
    dump("identities.json", id_rows)
    dump("rubrics.json", rubrics)
    dump("selected_lemmas.json", selected_raw)

    # ---- dictionary (optional, from check_dictionary.py) -----------------------
    dict_path = os.path.join(detail_dir, "dictionary.json")
    dictionary = None
    if os.path.exists(dict_path):
        dictionary = json.load(open(dict_path, encoding="utf-8"))
        dictionary = {"lemmas_checked": len(dictionary),
                      "lemmas_with_entry": sum(1 for v in dictionary.values() if v["entry_found"]),
                      "lemmas_missing_entry": sum(1 for v in dictionary.values() if not v["entry_found"]),
                      "http_status_counts": dict(Counter(str(v["http_status"]) for v in dictionary.values())),
                      "identities_with_entry_for_all_raw_lemmas": sum(
                          1 for r in selected_ids if all(dictionary.get(x, {}).get("entry_found") for x in r["raw_lemmas"])),
                      "identities_missing_entry_for_some_raw_lemma": sum(
                          1 for r in selected_ids if not all(dictionary.get(x, {}).get("entry_found") for x in r["raw_lemmas"]))}

    # ---- summary (committed; counts only) ------------------------------------
    sent_ok = [r for r in sent_rows if r.get("status") in ("verified", "problem")]
    problem_types = Counter(p.split(":")[0] for r in sent_rows for p in r.get("problems", []))
    summary = {
        "recon_version": RECON_VERSION,
        "pins": {"agdt": {"path": AGDT_REL, "sha256": got_a, "bytes": os.path.getsize(agdt_path)},
                 "canonical": {"path": CANON_REL, "sha256": got_c, "bytes": os.path.getsize(canon_path)}},
        "pin_checks": {"agdt_matches_pin": True, "canonical_matches_pin": True},
        "tools": {"python": sys.version.split()[0], "platform": platform.platform(),
                  "stdlib_only": True, "script_sha256": sha256(__file__)},
        "agdt": {"native_sentences": len(sents), "printed_nodes": len(nodes),
                 "node_kinds": dict(node_kinds), "word_tokens_with_key": len(A),
                 "nfc_nonconforming_forms": sum(1 for n in nodes if n["form"] != unicodedata.normalize("NFC", n["form"])),
                 "pos_letter_counts": dict(letter_counts), "upos_candidate_counts": dict(upos_counts),
                 "lemma_flags": dict(lemma_flags)},
        "canonical": {"sections_found": cstats["sections_found"],
                      "section_numbers_consecutive": bool(cstats["section_numbers_consecutive"]),
                      "section_divs_nested": cstats["section_divs_nested"],
                      "running_text_codepoints": len(running), "running_nfc": bool(cstats["running_nfc"]),
                      "word_units": cstats["word_units"], "punct_units": cstats["punct_units"],
                      "editorial": {k: cstats[k] for k in sorted(cstats) if k.startswith(("element_", "note", "label", "gap_", "chars_", "units_with"))},
                      "edition_children_not_section": {k: v for k, v in cstats.items() if k.startswith("edition_child")},
                      "rubric_labels_skipped": sorted({r[1] for r in rubrics}),
                      "rubrics_count": len(rubrics)},
        "alignment": {
            "method": ("level 1: difflib on exact NFC word keys; level 2: difflib on elision/breathing fold "
                       "(U+0313 treated as elision mark) within unequal blocks; level 3: exact concatenation of "
                       "consecutive AGDT nodes equal to adjacent canonical units. No reordering, no re-spelling."),
            "tokens_total": len(A), "tokens_bound_by_class": dict(tok_class),
            "canonical_word_units": len(W), "canonical_word_units_unbound": len(canon_unbound),
            "verification_failures": len(verify_failures),
            "unbound_blocks": len(blocks), "unbound_blocks_by_class": dict(Counter(b["class"] for b in blocks)),
            "unbound_tokens": tok_class.get("unbound", 0),
            "unbound_agdt_tokens_by_class": dict(Counter(
                b["class"] for b in blocks for _ in b["agdt_keys"] if b["agdt_keys"])),
            "unbound_block_examples": [{"nodes": b["agdt_nodes"], "sentences": b["agdt_sentences"],
                                        "class": b["class"]} for b in blocks if b["class"] not in ("canon_only_editorial", "canon_only")][:40],
        },
        "sentences": {
            "native_sentences": len(sents),
            "with_word_tokens": sum(1 for r in sent_rows if r.get("status") in ("verified", "problem")),
            "status_counts": dict(Counter(r["status"] for r in sent_rows)),
            "problem_types": dict(problem_types),
            "units": None if not sent_ok else statistics.median([r["units"] for r in sent_ok if "units" in r]),
            "over_units_cap_50_verified_or_problem": sum(1 for r in sent_ok if r.get("over_units_cap")),
            "over_codepoints_cap_400_verified_or_problem": sum(1 for r in sent_ok if r.get("over_codepoints_cap")),
            "start_context": dict(Counter(r.get("start_context") for r in sent_ok)),
            "end_context": dict(Counter(r.get("end_context") for r in sent_ok)),
            "sentences_with_medial_terminals": sum(1 for r in sent_ok if r.get("medial_terminals")),
            "completeness": "not certified: native segment boundaries are checked for contiguity and exact punctuation only",
        },
        "vocabulary": {
            "content_upos_rule": "UPOS not in NON_CONTENT_UPOS and not UNMAPPED (internal/enrichment/lexical.go contentPOS)",
            "content_printed_nodes": len(content_nodes),
            "content_nodes_bound": sum(1 for i in content_nodes if nodes[i]["bind"] is not None),
            "content_nodes_unbound": sum(1 for i in content_nodes if nodes[i]["bind"] is None),
            "distinct_identities": len(id_rows),
            "identity_key": "(canonical_lemma NFC/lower/final-sigma, UPOS candidate); no homograph discriminator exists in the AGDT lemmas",
            "selection_rule": f">= {SELECTION_MIN_OCCURRENCES} content occurrences in this Book (provisional 3/15)",
            "selected_identities": len(selected_ids),
            "selected_occurrences": sum(r["occurrences"] for r in selected_ids),
            "selected_status_counts": {k: v for k, v in id_status.items() if k != "not_selected"},
            "selected_upos_unresolved_but_eligible": upos_block.get("upos_unresolved_but_eligible_occurrence", 0),
            "selected_raw_lemma_strings": len(selected_raw),
            "dictionary": dictionary,
        },
        "runtime_seconds": round(time.time() - t0, 2),
    }
    with open(os.path.join(committed_dir, "summary.json"), "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(json.dumps({k: summary[k] for k in ("agdt", "canonical", "alignment", "sentences", "vocabulary")},
                     ensure_ascii=False, indent=1, default=str)[:12000])


if __name__ == "__main__":
    main()
