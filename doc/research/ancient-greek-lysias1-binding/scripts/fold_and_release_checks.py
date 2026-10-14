#!/usr/bin/env python3
"""Second-pass checks for #2048 (stdlib only). Run with `python3 -I`:

    python3 -I fold_and_release_checks.py <inputs_dir> <manifest_dir> <detail_dir>

Reads occurrence_binding.tsv written by binding_probe.py and:
- tabulates the elision/accent/NFC mechanism of every fold and concatenation binding
  (code-point classes only, no Greek surfaces in the committed output);
- compares the same native node IDs in AGDT v2.0 and v1.6 (cross-release stability);
- writes release_stability.tsv (ASCII only) and fold_mechanisms.json.
Greek surfaces go only to <detail_dir> (scratch).
"""
import csv
import json
import os
import sys
import unicodedata
import xml.etree.ElementTree as ET
from collections import Counter

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import binding_probe as bp  # noqa: E402


def cp_names(s):
    return " ".join(f"U+{ord(c):04X}" for c in s)


def mark_class(form):
    nf = unicodedata.normalize("NFD", form)
    tail = nf[-1] if nf else ""
    if tail == "̓":
        return "final_U+0313_combining_comma_above"
    if tail == "ʼ":
        return "final_U+02BC_modifier_apostrophe"
    if tail == "’":
        return "final_U+2019_right_quote"
    if tail == "'":
        return "final_ASCII_apostrophe"
    return "no_final_elision_mark"


def main(inputs, manifest, detail):
    os.makedirs(detail, exist_ok=True)
    occ_path = os.path.join(manifest, "occurrence_binding.tsv")
    rows = list(csv.DictReader(open(occ_path, encoding="utf-8"), delimiter="\t"))
    canon_file = os.path.join(inputs, "canon", bp.CG_PATH)
    can = bp.canonical(canon_file)
    text = can["text"]
    words, _ = bp.tokens_of(text, can["flags_at"], can["section_at"])
    wkey = {f"cg2:w{i:04d}": w for i, w in enumerate(words, 1)}
    agdt_file = os.path.join(inputs, "agdt", bp.AGDT_PATH)
    nodes = {f"s{int(n['sentence']):03d}w{int(n['word']):03d}": n for n in bp.agdt_nodes(agdt_file)}

    mech = Counter()
    detail_rows = []
    concat_pairs = []
    for r in rows:
        cls = r["binding_class"]
        if cls not in ("fold_elision_1to1", "fold_nfd_1to1", "concat_exact"):
            continue
        n = nodes[r["key"]]
        form = n["form"]
        tok = wkey[r["canon_token_keys"]]["text"] if r["canon_token_keys"] in wkey else ""
        if cls.startswith("fold"):
            mech[(cls, mark_class(form), "canon_final:" + cp_names(tok[-1:]) if tok else "")] += 1
        else:
            pieces = [bp.strip_hyphen(form)]
            mech[(cls, "hyphen_pos:" + ("trailing" if form.endswith("-") else "leading" if form.startswith("-") else "none"))] += 1
        detail_rows.append({"key": r["key"], "class": cls, "form": form, "canon": tok,
                            "nfc_form_equal": unicodedata.normalize("NFC", form) == form,
                            "canon_nfc": unicodedata.normalize("NFC", tok) == tok})
    with open(os.path.join(detail, "fold_concat_detail.tsv"), "w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(detail_rows[0].keys()), delimiter="\t")
        w.writeheader()
        w.writerows(detail_rows)

    # Concatenation pairs grouped by canonical token (scratch detail only: ASCII counts in manifest).
    tok_nodes = {}
    for r in detail_rows:
        if r["class"] == "concat_exact":
            tok_nodes.setdefault(r["canon"], []).append(r["form"])

    # Cross-release: same native node IDs in v2.0 and v1.6.
    release_rows = []
    for name, rev, path, rel in bp.WITNESSES:
        wpath = os.path.join(inputs, rel)
        wn = {}
        for s in ET.parse(wpath).getroot().iter("sentence"):
            for wd in s.findall("word"):
                wn[(s.get("id"), wd.get("id"))] = wd.attrib
        same = diff = missing = 0
        diff_by_class = Counter()
        cls_of = {r["key"]: r["binding_class"] for r in rows}
        for k, n in nodes.items():
            if n["class"] == "artificial":
                continue
            other = wn.get((str(int(n["sentence"])), str(int(n["word"]))))
            if other is None:
                missing += 1
                continue
            if other.get("form", "") == n["form"]:
                same += 1
            else:
                diff += 1
                diff_by_class[cls_of.get(k, "")] += 1
                release_rows.append({"witness": name, "key": k, "class_v21": cls_of.get(k, ""),
                                     "form_v21_len": len(n["form"]), "form_witness_len": len(other.get("form", "")),
                                     "nfc_equal_pair": unicodedata.normalize("NFC", n["form"]) == unicodedata.normalize("NFC", other.get("form", "")),
                                     "nfd_equal_pair": unicodedata.normalize("NFD", n["form"]) == unicodedata.normalize("NFD", other.get("form", "")),
                                     "same_postag": other.get("postag", "") == n["postag"]})
        print(name, "same", same, "diff", diff, "missing", missing, dict(diff_by_class))
    with open(os.path.join(manifest, "release_stability.tsv"), "w", encoding="utf-8", newline="") as f:
        fields = ["witness", "key", "class_v21", "form_v21_len", "form_witness_len", "nfc_equal_pair",
                  "nfd_equal_pair", "same_postag"]
        w = csv.DictWriter(f, fieldnames=fields, delimiter="\t", lineterminator="\n")
        w.writeheader()
        w.writerows(release_rows)

    out = {"fold_and_concat_mechanisms": {str(k): v for k, v in mech.items()},
           "concat_canonical_tokens": len(tok_nodes),
           "concat_node_count": sum(len(v) for v in tok_nodes.values())}
    with open(os.path.join(manifest, "fold_mechanisms.json"), "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")
    print(json.dumps(out, ensure_ascii=False, indent=1))


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2], sys.argv[3])
