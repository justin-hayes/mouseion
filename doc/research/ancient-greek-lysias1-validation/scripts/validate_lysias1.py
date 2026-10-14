#!/usr/bin/env python3
"""Probe the pinned AGDT v2.1 Lysias I file against the pinned canonical comparator.

Usage (stdlib only, isolated mode):
    python3 -I validate_lysias1.py <inputs_dir> <detail_dir> <committed_dir>

Inputs are laid out as written by fetch_inputs.sh. Per-token, per-lemma and
per-sentence detail goes to <detail_dir> (scratch, not committed because of the
output-specific rights question). Aggregate counts go to
<committed_dir>/summary.json.
"""
import bisect
import difflib
import hashlib
import html
import json
import os
import platform
import re
import statistics
import sys
import time
import unicodedata
import xml.etree.ElementTree as ET
from collections import Counter, defaultdict

RECON_VERSION = "lysias1-recon-v1"
AGDT_REL = "agdt/v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml"
CANON_REL = "canon/data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml"
AGDT_SHA = "44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568"
CANON_SHA = "21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a"
CAP_UNITS = 50
CAP_CODEPOINTS = 400
LEXICAL_POS = {"n": "NOUN", "v": "VERB", "a": "ADJ", "d": "ADV"}
SELECTION_MIN_OCCURRENCES = 3  # provisional 3/15 rule; the 15-across-Books clause is subsumed (one Book)
RUBRIC_WORDS = {"Νόμος", "Μάρτυρες"}
SECTION_RE = re.compile(r'<div type="textpart" subtype="section" xml:base="[^"]*" n="(\d+)">')


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def is_punct(form):
    return bool(form) and all(unicodedata.category(c).startswith("P") for c in form)


def printed(w):
    """A printed occurrence: not an artificial/elliptic node and carries a real form."""
    return w.get("artificial") is None and bool(w.get("form")) and not w["form"].startswith("[")


def load_agdt(path):
    root = ET.parse(path).getroot()
    sentences = []
    for s in root.iter("sentence"):
        words = [dict(w.attrib) for w in s.findall("word")]
        ids = [int(w["id"]) for w in words]
        assert ids == sorted(ids), f"word ids not ascending in sentence {s.get('id')}"
        sentences.append({"id": s.get("id"), "subdoc": s.get("subdoc"), "words": words})
    return sentences


def reconstruct(sentences):
    """Join printed forms. Hyphen clitics and punctuation attach without a space."""
    text = ""
    prev = None
    nodes = []
    for si, s in enumerate(sentences):
        for w in s["words"]:
            if not printed(w):
                continue
            form = w["form"]
            if prev is not None:
                attach = is_punct(form) or prev.endswith("-") or form.startswith("-")
                text += "" if attach else " "
            start = len(text)
            text += form
            nodes.append({"sent": si, "id": w["id"], "start": start, "end": len(text),
                          "form": form, "postag": w.get("postag", ""),
                          "lemma": w.get("lemma", ""), "relation": w.get("relation", "")})
            prev = form
    for n in nodes:
        assert text[n["start"]:n["end"]] == n["form"], n
    return text, nodes


def canonical_sections(path):
    raw = open(path, encoding="utf-8").read()
    marks = list(SECTION_RE.finditer(raw))
    tail = re.search(r"</body>|</text>", raw[marks[-1].end():])
    cut = marks[-1].end() + (tail.start() if tail else len(raw) - marks[-1].end())
    sections = []
    counters = Counter()
    rubrics = []
    for i, m in enumerate(marks):
        end = marks[i + 1].start() if i + 1 < len(marks) else cut
        chunk = raw[m.end():end]
        counters["note"] += len(re.findall(r"<note\b", chunk))
        counters["del"] += len(re.findall(r"<del\b", chunk))
        counters["add"] += len(re.findall(r"<add\b", chunk))
        counters["gap"] += len(re.findall(r"<gap\b", chunk))
        for lab in re.findall(r"<label\b[^>]*>(.*?)</label>", chunk, flags=re.S):
            rubrics.append((int(m.group(1)), re.sub(r"<[^>]+>", "", lab).strip()))
            counters["label"] += 1
        chunk = re.sub(r"<note\b[^>]*>.*?</note>", " ", chunk, flags=re.S)
        chunk = re.sub(r"<label\b[^>]*>.*?</label>", " ", chunk, flags=re.S)
        for tag in re.findall(r"<([A-Za-z]+)", chunk):
            counters["tag:" + tag] += 1
        text = html.unescape(re.sub(r"<[^>]+>", " ", chunk))
        sections.append({"n": int(m.group(1)), "text": re.sub(r"\s+", " ", text).strip()})
    counters["sections_found"] = len(sections)
    counters["section_numbers_consecutive"] = int([s["n"] for s in sections] == list(range(1, len(sections) + 1)))
    return sections, rubrics, counters


def core_letters(tokens):
    """Letters only, with the elision-mark variants folded. Used to classify blocks, never to align."""
    s = "".join(tokens).replace("̓", "ʼ").replace("’", "ʼ")
    return "".join(c for c in s if not unicodedata.category(c).startswith("P") and c != "-")


def classify_block(block):
    a = block["agdt_tokens"]
    c = block["canon_tokens"]
    if block["op"] == "delete" and all(t.rstrip(".") in RUBRIC_WORDS for t in a):
        return "rubric_in_agdt_only"
    if core_letters(a) == core_letters(c):
        return "elision_mark_clitic_or_punctuation_convention"
    return "unexplained"


def describe_block(tag, a, b, i1, i2, j1, j2):
    return {"op": tag, "agdt_range": [i1, i2], "canon_range": [j1, j2],
            "agdt_tokens": a[i1:i2][:12], "canon_tokens": b[j1:j2][:12],
            "agdt_count": i2 - i1, "canon_count": j2 - j1}


def pct(xs, p):
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(round(p / 100 * (len(xs) - 1))))]


def length_stats(values):
    return {"min": min(values), "p25": pct(values, 25), "median": statistics.median(values),
            "p75": pct(values, 75), "p90": pct(values, 90), "max": max(values)}


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

    sentences = load_agdt(agdt_path)
    text, nodes = reconstruct(sentences)

    # ---- AGDT node accounting -------------------------------------------------
    all_words = [w for s in sentences for w in s["words"]]
    node_kinds = Counter()
    for w in all_words:
        if w.get("artificial") is not None:
            kind = "artificial:" + w["artificial"] + (":bracket_form" if w.get("form", "").startswith("[") else
                                                    (":form" if w.get("form") else ":empty"))
        elif not w.get("form"):
            kind = "printed_candidate:empty_form"
        elif w["form"].startswith("["):
            kind = "printed_candidate:bracket_form"
        else:
            kind = "printed"
        node_kinds[kind] += 1
    nfc_bad = sum(1 for n in nodes if n["form"] != unicodedata.normalize("NFC", n["form"]))
    char_counts = Counter(c for n in nodes for c in n["form"] if c in "̓··;’ʼ᾽")

    # ---- Canonical comparator ------------------------------------------------
    sections, rubrics, canon_counters = canonical_sections(canon_path)
    canon_text = " ".join(s["text"] for s in sections)
    a = text.split()
    b = canon_text.split()
    token_spans = [(m.start(), m.end()) for m in re.finditer(r"\S+", text)]
    tok_starts = [s for s, _ in token_spans]

    sm = difflib.SequenceMatcher(None, a, b, autojunk=False)
    aligned_a = [False] * len(a)
    blocks = []
    op_counts = Counter()
    for tag, i1, i2, j1, j2 in sm.get_opcodes():
        if tag == "equal":
            for k in range(i1, i2):
                aligned_a[k] = True
        else:
            blocks.append(describe_block(tag, a, b, i1, i2, j1, j2))
            op_counts[tag] += 1
    for bl in blocks:
        bl["class"] = classify_block(bl)
    block_classes = Counter(bl["class"] for bl in blocks)
    equal_a = sum(aligned_a)
    eq_b = sum(i2 - i1 for tag, i1, i2, _, _ in sm.get_opcodes() if tag == "equal")

    # ---- Vocabulary ---------------------------------------------------------
    pos_letters = Counter(n["postag"][:1] or "∅" for n in nodes)
    lemma_flags = Counter()
    for n in nodes:
        lem = n["lemma"]
        if not lem or lem == "undefined":
            lemma_flags["missing_or_undefined"] += 1
        if "#" in lem:
            lemma_flags["homograph_digit"] += 1
        if re.search(r"\.[RI]\b", lem):
            lemma_flags[".R_or_.I_notation"] += 1
        if len(n["postag"]) not in (0, 9):
            lemma_flags["postag_length_not_9"] += 1
    lexical = []
    unresolved = []
    for n in nodes:
        letter = n["postag"][:1]
        if letter in LEXICAL_POS:
            if not n["lemma"] or n["lemma"] == "undefined":
                unresolved.append(n)
            else:
                lexical.append(n)
    identity_counts = Counter(n["lemma"] for n in lexical)
    selected = [lem for lem, c in identity_counts.items() if c >= SELECTION_MIN_OCCURRENCES]

    tok_index = {}
    for n in nodes:
        tok_index[(n["sent"], n["id"])] = bisect.bisect_right(tok_starts, n["start"]) - 1

    # Per-sentence completeness and lengths (native segmentation; not certified)
    sent_texts = defaultdict(list)
    for n in nodes:
        sent_texts[n["sent"]].append(n)
    sent_rows = []
    for si in range(len(sentences)):
        ns = sent_texts.get(si, [])
        if not ns:
            sent_rows.append({"sent": si, "id": sentences[si]["id"], "empty": True})
            continue
        stext = text[ns[0]["start"]:ns[-1]["end"]]
        last = ns[-1]["form"][-1:]
        sent_rows.append({"sent": si, "id": sentences[si]["id"], "subdoc": sentences[si]["subdoc"],
                          "units": len(stext.split()), "codepoints": len(stext),
                          "terminal": last if is_punct(last) else "none",
                          "over_units_cap": len(stext.split()) > CAP_UNITS,
                          "over_codepoints_cap": len(stext) > CAP_CODEPOINTS,
                          "text": stext})
    nonempty = [r for r in sent_rows if not r.get("empty")]

    per_identity = {}
    omission = Counter()
    for lem in selected:
        occ = [n for n in lexical if n["lemma"] == lem]
        first_aligned = None
        rows = []
        for n in occ:
            al = aligned_a[tok_index[(n["sent"], n["id"])]]
            rows.append({"sent": n["sent"], "id": n["id"], "form": n["form"], "aligned": al})
            if al and first_aligned is None:
                first_aligned = n
        if first_aligned is None:
            omission["selected_identity_no_aligned_occurrence"] += 1
        per_identity[lem] = {
            "occurrences": len(occ),
            "aligned_occurrences": sum(r["aligned"] for r in rows),
            "earliest_aligned": None if first_aligned is None else
                {"sent": first_aligned["sent"], "id": first_aligned["id"], "form": first_aligned["form"],
                 "sentence_text": sent_rows[first_aligned["sent"]]["text"]},
            "occurrence_rows": rows,
        }
    terminal_counts = Counter(r["terminal"] for r in nonempty)

    # ---- Detail (scratch only) ------------------------------------------------
    with open(os.path.join(detail_dir, "reconstruction.txt"), "w", encoding="utf-8") as f:
        f.write(text + "\n")
    with open(os.path.join(detail_dir, "canonical_running_text.txt"), "w", encoding="utf-8") as f:
        f.write(canon_text + "\n")
    with open(os.path.join(detail_dir, "alignment_blocks.json"), "w", encoding="utf-8") as f:
        json.dump(blocks, f, ensure_ascii=False, indent=1)
    with open(os.path.join(detail_dir, "per_identity.json"), "w", encoding="utf-8") as f:
        json.dump(per_identity, f, ensure_ascii=False, indent=1)
    with open(os.path.join(detail_dir, "sentences.json"), "w", encoding="utf-8") as f:
        json.dump(sent_rows, f, ensure_ascii=False, indent=1)
    with open(os.path.join(detail_dir, "rubrics.json"), "w", encoding="utf-8") as f:
        json.dump(rubrics, f, ensure_ascii=False, indent=1)

    # ---- Summary (committed; counts only) ------------------------------------
    occ_total = len(lexical) + len(unresolved)
    summary = {
        "recon_version": RECON_VERSION,
        "pins": {"agdt": {"path": AGDT_REL, "sha256": got_a, "bytes": os.path.getsize(agdt_path)},
                 "canonical": {"path": CANON_REL, "sha256": got_c, "bytes": os.path.getsize(canon_path)}},
        "pin_checks": {"agdt_matches_pin": True, "canonical_matches_pin": True},
        "tools": {"python": sys.version.split()[0], "platform": platform.platform(),
                  "stdlib_only": True, "script_sha256": sha256(__file__)},
        "agdt": {
            "native_sentences": len(sentences),
            "nodes_total": len(all_words),
            "node_kinds": dict(node_kinds),
            "printed_occurrences": len(nodes),
            "printed_span_checks_failed": 0,
            "reconstructed_codepoints": len(text),
            "reconstructed_whitespace_tokens": len(a),
            "nfc_nonconforming_forms": nfc_bad,
            "marks_count": {"U+0313": char_counts.get("̓", 0), "U+0387": char_counts.get("·", 0),
                            "U+00B7": char_counts.get("·", 0), "ASCII_semicolon": char_counts.get(";", 0),
                            "U+2019": char_counts.get("’", 0), "U+02BC": char_counts.get("ʼ", 0),
                            "U+1FBD": char_counts.get("᾽", 0)},
            "pos_letter_counts": dict(pos_letters),
            "lemma_flags": dict(lemma_flags),
            "aux_relation_counts": {r: c for r, c in Counter(n["relation"] for n in nodes
                                                              if n["relation"].startswith("Aux")).items()},
        },
        "canonical": {
            "file_sha256": got_c,
            "sections_found": canon_counters["sections_found"],
            "section_numbers_consecutive": bool(canon_counters["section_numbers_consecutive"]),
            "editorial_elements": {k: canon_counters[k] for k in ("note", "del", "add", "gap", "label")},
            "tags_seen_outside_note_label": sorted(k[4:] for k in canon_counters if k.startswith("tag:")),
            "rubric_labels_removed_from_running_text": sorted({r[1] for r in rubrics}),
            "running_whitespace_tokens": len(b),
        },
        "alignment": {
            "method": "difflib.SequenceMatcher on exact whitespace tokens, autojunk=False; no case/accent folding",
            "agdt_tokens_equal": equal_a,
            "canonical_tokens_equal": eq_b,
            "agdt_token_equal_fraction": round(equal_a / len(a), 6),
            "nonequal_blocks": len(blocks),
            "nonequal_by_op": dict(op_counts),
            "nonequal_by_class": dict(block_classes),
            "nonequal_unexplained_examples": [[" ".join(bl["agdt_tokens"]), " ".join(bl["canon_tokens"])]
                                              for bl in blocks if bl["class"] == "unexplained"],
            "nonequal_agdt_tokens": sum(bl["agdt_count"] for bl in blocks),
            "nonequal_canonical_tokens": sum(bl["canon_count"] for bl in blocks),
            "first_agdt_token": a[0] if a else None,
            "last_agdt_token": a[-1] if a else None,
            "first_canonical_token": b[0] if b else None,
            "last_canonical_token": b[-1] if b else None,
        },
        "vocabulary": {
            "lexical_pos_classes": LEXICAL_POS,
            "lexical_occurrences": len(lexical),
            "unresolved_lexical_occurrences": len(unresolved),
            "lexical_occurrences_total_checked": occ_total,
            "distinct_raw_lemma_identities": len(identity_counts),
            "selection_rule": f">= {SELECTION_MIN_OCCURRENCES} lexical occurrences in this Book (provisional 3/15)",
            "selected_identities": len(selected),
            "selected_identities_with_aligned_occurrence": len(selected) - omission["selected_identity_no_aligned_occurrence"],
            "selected_identity_omissions": dict(omission),
            "selected_occurrences": sum(v["occurrences"] for v in per_identity.values()),
            "selected_occurrences_unaligned": sum(v["occurrences"] - v["aligned_occurrences"]
                                                  for v in per_identity.values()),
            "selected_identities_with_unaligned_occurrence": sum(
                v["occurrences"] > v["aligned_occurrences"] for v in per_identity.values()),
            "status": "conditional: canonical lemma-to-headword mapping not performed in this probe",
        },
        "sentences": {
            "native_nonempty": len(nonempty),
            "empty_native_segments": len(sent_rows) - len(nonempty),
            "units": length_stats([r["units"] for r in nonempty]),
            "codepoints": length_stats([r["codepoints"] for r in nonempty]),
            "over_units_cap_50": sum(r["over_units_cap"] for r in nonempty),
            "over_codepoints_cap_400": sum(r["over_codepoints_cap"] for r in nonempty),
            "terminal_marks": dict(terminal_counts),
            "completeness": "not certified: native boundaries are reported, not validated",
        },
        "runtime_seconds": round(time.time() - t0, 2),
        "detail_bytes": {name: os.path.getsize(os.path.join(detail_dir, name)) for name in sorted(os.listdir(detail_dir))},
    }
    with open(os.path.join(committed_dir, "summary.json"), "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(json.dumps({k: summary[k] for k in ("agdt", "canonical", "alignment", "vocabulary", "sentences")},
                     ensure_ascii=False, indent=1)[:6000])


if __name__ == "__main__":
    main()
