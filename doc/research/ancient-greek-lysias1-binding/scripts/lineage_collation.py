#!/usr/bin/env python3
"""Same-lineage revision collation for Lysias I (#2048, revision 3).

Stdlib only. Run with `python3 -I`:

    python3 -I lineage_collation.py <inputs_dir> <out_dir>

<inputs_dir> is produced by scripts/fetch_inputs.sh, which also exports every
distinct historical blob of the canonical-greekLit Lysias I Greek file into
<inputs_dir>/history/<commit>.xml. This script asserts each blob's SHA-256.

It answers lineage questions only. Every revision below is one digital text (the
1992 Perseus encoding of the 1930 Loeb, CTS-ized in 2014, renamed grc1 -> grc2
in 2017). None of them is an independent witness of the speech.

Extraction (identical for every revision, chosen before comparison):
- body text in document order; <note> subtrees and <teiHeader> are dropped;
- every other element's text is kept (del, add, q, quote, title, label, foreign);
- <gap> contributes no characters;
- word = maximal run of Unicode L*/M* characters; any other non-space character is
  one punctuation token; NFC.

Outputs (ASCII only; surfaces are written as U+XXXX code-point sequences):
- lineage_revisions.tsv   one row per historical blob: hashes, token counts,
                          markup counts, rubric section placement
- lineage_changes.tsv     every non-equal token operation between consecutive
                          text revisions (opcode, section, old/new code points)
- agdt_vs_revisions.tsv   printed AGDT node sequence (v1.6 and v2.1) against every
                          revision: equal / non-equal token counts
- agdt_v16_vs_annotated_base.tsv   every non-equal operation between the AGDT v1.6
                          printed node sequence and the revision closest to the
                          annotation date, with native node keys
- lineage_summary.json
"""
import csv
import difflib
import hashlib
import json
import os
import sys
import unicodedata
import xml.etree.ElementTree as ET

CG_FILE_OLD = "tlg0540/tlg001/tlg0540.tlg001.perseus-grc1.xml"
CG_FILE_GRC1 = "data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc1.xml"
CG_FILE_GRC2 = "data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml"

# (commit, date, path in commit, git blob id, sha256 of blob bytes, role)
# Ordered by date. Blobs equal to a predecessor are listed once (header-only commits
# with an identical blob are omitted; see lineage_revisions.tsv `same_blob_commits`).
REVISIONS = [
    ("dc36cfe1fae36acd800418baf16e8a2c403749fa", "2014-08-18", CG_FILE_OLD,
     "fb81bc751ad0fe121257e2b527e0a6ef7270ff38", None, "grc1 CTS-ized (perseids_docs#142)"),
    ("aa0dd9e6a25c848c04e75169e525ec413f033499", "2014-08-19", CG_FILE_OLD,
     "418c8f2edd402480a4a3a8feadff1b207a3cb6f9", None, "grc1 xml:lang added"),
    ("365e93f4543788b25f25013f2dc1932151a7e210", "2015-06-01", CG_FILE_GRC1,
     "cc4a90984779892ce1f754fcf888b0395329cfae", None, "grc1 CTS5 packaging"),
    ("b9d99e44a9552cc0ba12c785b4a61cbbdb09a6f0", "2015-06-04", CG_FILE_GRC1,
     "522d75b652aefa90a9207841bf5b602043b1d20e", None, "grc1 urn added"),
    ("8d9937ed69e5af4edecde098fab680a5c0045468", "2015-06-24", CG_FILE_GRC1,
     "bf6fad90728330851e4bbafe0823b92a7357fda1", None, "grc1 cRefPattern"),
    ("0a3eab7fccf72ca099241662cd1eda7737086779", "2016-06-17", CG_FILE_GRC1,
     "6e04e3e966267df480f8c88186a24aa8471dee21", None, "grc1 typo-correx #122"),
    ("8177550ef85696f54327ed9ca310b9661e31a932", "2017-04-28", CG_FILE_GRC1,
     "262869f8391818f2eeb94f3403ba09cbd0b18be1", None, "grc1 header #401"),
    ("16d2842d68bb588577164e8a28dfa006317a6ac5", "2017-04-28", CG_FILE_GRC1,
     "eee8603d90145581a825df908475727873612002", None, "grc1 refsDecl/sections/notes/gaps #401"),
    ("7a61daad6fad68ef2f74058419ef62de0312312b", "2017-04-28", CG_FILE_GRC1,
     "2dd14d2247ea6ac06baa9889cdd2aefb1efb8771", None, "grc1 URN bumped (blob reused as grc2 in 7b433f06)"),
    ("82af5ab7086d198c7994b9f6dc43345149d96f54", "2017-05-01", CG_FILE_GRC2,
     "3b5b7c870bbef4db426b40a129b55479b44c951f", None, "grc2 #403"),
    ("0b7bef19fa1f759d1702e20280e35f37ba787ace", "2017-05-02", CG_FILE_GRC2,
     "7ca389aaec19a433dc7f49898897f04f0cda9bff", None, "grc2 further conversion #403"),
    ("b01744e86530350db8f81a019072e68880e470fd", "2021-04-20", CG_FILE_GRC2,
     "5888744e79203aa827ef640a9b8cb985f1eff898", None, "grc2 typos-errors"),
    ("50c80234c89c8977531f09a3136c079dc24eeef9", "2023-04-13", CG_FILE_GRC2,
     "2bc667c165304247420ec35ba34637431a1627e9", None, "grc2 (tlg0540)"),
    ("5d9443283069e004e989b8e611561ff02c86fa12", "2023-11-20", CG_FILE_GRC2,
     "9cd35c0880c369f6a8d450c90f73547c8d26be01", None, "grc2 greek_conversion cleanup"),
    ("0c75c6009460b05198e1050d67e48a16966acc72", "2026-03-29", CG_FILE_GRC2,
     "c7c2a7ffd1082b1027763086e8ec8e69244fe6a0", None, "grc2 schema cleanup"),
    ("feb8f32e56399f91e5c98ccb0a6aaab5c062879e", "2026-05-04", CG_FILE_GRC2,
     "821dd60ebdbc0d822965ceb4df749b7f6de8e52a", None, "grc2 schema call"),
    ("dd96fe31bc5150a84a0f7259518abbcaf12a2454", "2026-08-26", CG_FILE_GRC2,
     "8e35bc45e2448781744d3a7e918cfabb38a89533", None, "grc2 citeStructure"),
    ("70427323b61267cdcd0256b9b02009c23392de07", "2026-08-26", CG_FILE_GRC2,
     "bd5e2daf5bec71550aaa44e0552f814d9b4096fa", None, "grc2 HookTest attrs (= pinned 01b725d8 blob)"),
]
PINNED_BLOB = "bd5e2daf5bec71550aaa44e0552f814d9b4096fa"
PINNED_SHA = "21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a"
AGDT21 = "agdt/v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml"
AGDT16 = "agdt/v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml"
AGDT21_SHA = "44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568"
AGDT16_SHA = "5c8f187a489c67fa06bb70219f27177c8dd9b8061e198b9ee63a56f3d9bd9e27"
# AGDT header: annotationDate "Thu Aug 21 19:31:06 +0000 2014".
ANNOTATION_DATE = "2014-08-21"
RUBRIC_TAGS = ("label", "title")
MARKUP_TAGS = ("del", "add", "q", "quote", "label", "title", "gap", "foreign", "note")


def nfc(s):
    return unicodedata.normalize("NFC", s)


def cps(s):
    return " ".join(f"U+{ord(c):04X}" for c in s)


def local(tag):
    return tag.rsplit("}", 1)[-1] if isinstance(tag, str) else ""


def git_blob_id(data):
    return hashlib.sha1(b"blob %d\0" % len(data) + data).hexdigest()


def is_word_char(ch):
    return unicodedata.category(ch)[0] in "LM"


def extract(path):
    """Tokens (text, kind, section, rubric) of a TEI body; notes and header dropped."""
    root = ET.parse(path).getroot()
    body = next(e for e in root.iter() if local(e.tag) == "body")
    chars = []  # (ch, section, rubric)
    markup = {t: 0 for t in MARKUP_TAGS}
    rubric_sections = []

    def section_of(el, inherited):
        n = el.get("n")
        if local(el.tag) == "div" and n and n.isdigit():
            return int(n)
        return inherited

    def walk(el, sec, rub):
        tag = local(el.tag)
        if tag in markup:
            markup[tag] += 1
        if tag == "note":
            return
        sec = section_of(el, sec)
        r = rub or tag in RUBRIC_TAGS
        if tag in RUBRIC_TAGS:
            rubric_sections.append(sec)
        if el.text:
            chars.extend((c, sec, r) for c in el.text)
        for child in el:
            walk(child, sec, r)
            if child.tail:
                chars.extend((c, sec, rub) for c in child.tail)

    walk(body, 0, False)
    # Tokenize the raw characters, then NFC each token, so section/rubric metadata stays
    # attached to the source characters.
    text = "".join(c for c, _, _ in chars)
    toks, i, n = [], 0, len(text)
    while i < n:
        ch = text[i]
        if ch.isspace():
            i += 1
        elif is_word_char(ch):
            j = i
            while j < n and is_word_char(text[j]):
                j += 1
            toks.append((nfc(text[i:j]), "w", chars[i][1], chars[i][2]))
            i = j
        else:
            toks.append((ch, "p", chars[i][1], chars[i][2]))
            i += 1
    return toks, markup, rubric_sections


def agdt_printed(path):
    seq = []
    for s in ET.parse(path).getroot().iter("sentence"):
        sid = int(s.get("id"))
        for w in s.findall("word"):
            if "artificial" in w.attrib:
                continue
            f = nfc(w.get("form", ""))
            kind = "p" if w.get("postag", "")[:1] == "u" else "w"
            seq.append((f, kind, f"s{sid:03d}w{int(w.get('id')):03d}"))
    return seq


def sha256_bytes(b):
    return hashlib.sha256(b).hexdigest()


def opcodes(a, b):
    sm = difflib.SequenceMatcher(None, a, b, autojunk=False)
    return [op for op in sm.get_opcodes() if op[0] != "equal"]


def write_tsv(path, rows, fields):
    with open(path, "w", newline="", encoding="ascii") as f:
        w = csv.DictWriter(f, fieldnames=fields, delimiter="\t", lineterminator="\n")
        w.writeheader()
        for r in rows:
            w.writerow(r)


def main(inputs, out):
    os.makedirs(out, exist_ok=True)
    revs = []
    for commit, date, path, blob, _, role in REVISIONS:
        p = os.path.join(inputs, "history", f"{commit}.xml")
        with open(p, "rb") as f:
            data = f.read()
        assert git_blob_id(data) == blob, (commit, "git blob id mismatch")
        toks, markup, rub_secs = extract(p)
        revs.append(dict(commit=commit, date=date, path=path, blob=blob, role=role,
                         sha256=sha256_bytes(data), toks=toks, markup=markup, rub_secs=rub_secs))
    assert revs[-1]["blob"] == PINNED_BLOB and revs[-1]["sha256"] == PINNED_SHA
    for f, h in ((AGDT21, AGDT21_SHA), (AGDT16, AGDT16_SHA)):
        with open(os.path.join(inputs, f), "rb") as fh:
            assert sha256_bytes(fh.read()) == h, f

    # Distinct text revisions (token surface sequence differs from predecessor).
    rev_rows, change_rows = [], []
    prev = None
    for r in revs:
        surf = [(t, k) for t, k, _, _ in r["toks"]]
        text_changed = prev is None or surf != [(t, k) for t, k, _, _ in prev["toks"]]
        rubric_changed = prev is not None and r["rub_secs"] != prev["rub_secs"]
        rev_rows.append({
            "commit": r["commit"], "date": r["date"], "path": r["path"], "git_blob": r["blob"],
            "sha256": r["sha256"], "role": r["role"],
            "word_tokens": sum(1 for t in r["toks"] if t[1] == "w"),
            "punct_tokens": sum(1 for t in r["toks"] if t[1] == "p"),
            "rubric_tokens": sum(1 for t in r["toks"] if t[3]),
            "rubric_sections": ",".join(str(s) for s in r["rub_secs"]),
            "markup_counts": ";".join(f"{k}={v}" for k, v in r["markup"].items()),
            "token_text_changed": "yes" if text_changed else "no",
            "rubric_placement_changed": "yes" if rubric_changed else "no",
        })
        if prev is not None and text_changed:
            a = [(t, k) for t, k, _, _ in prev["toks"]]
            b = surf
            for tag, i1, i2, j1, j2 in opcodes(a, b):
                secs = {s for _, _, s, _ in prev["toks"][i1:i2]} | {s for _, _, s, _ in r["toks"][j1:j2]}
                if not secs:
                    secs = {prev["toks"][min(i1, len(a) - 1)][2]}
                change_rows.append({
                    "from_commit": prev["commit"], "to_commit": r["commit"], "to_date": r["date"],
                    "op": tag, "sections": ",".join(str(s) for s in sorted(secs)),
                    "from_index": f"{i1}:{i2}", "to_index": f"{j1}:{j2}",
                    "from_codepoints": " | ".join(cps(t) for t, _ in a[i1:i2]),
                    "to_codepoints": " | ".join(cps(t) for t, _ in b[j1:j2]),
                })
        prev = r

    # AGDT printed sequences against every revision.
    agdt = {"v1.6": agdt_printed(os.path.join(inputs, AGDT16)),
            "v2.1": agdt_printed(os.path.join(inputs, AGDT21))}
    vs_rows = []
    for rel, seq in agdt.items():
        a = [(f, k) for f, k, _ in seq]
        for r in revs:
            b = [(t, k) for t, k, _, _ in r["toks"]]
            ops = opcodes(a, b)
            vs_rows.append({
                "agdt_release": rel, "commit": r["commit"], "date": r["date"],
                "nonequal_ops": len(ops),
                "agdt_tokens_in_nonequal": sum(i2 - i1 for _, i1, i2, _, _ in ops),
                "text_tokens_in_nonequal": sum(j2 - j1 for _, _, _, j1, j2 in ops),
            })

    # Ledger: AGDT v1.6 (pre-normalization treebank text) vs the revision current on the
    # annotation date (latest revision dated <= ANNOTATION_DATE).
    base = [r for r in revs if r["date"] <= ANNOTATION_DATE][-1]
    seq = agdt["v1.6"]
    a = [(f, k) for f, k, _ in seq]
    b = [(t, k) for t, k, _, _ in base["toks"]]
    base_rows = []
    for tag, i1, i2, j1, j2 in opcodes(a, b):
        secs = sorted({s for _, _, s, _ in base["toks"][j1:j2]}) or [base["toks"][j1][2]]
        base_rows.append({
            "op": tag, "node_keys": ",".join(k for _, _, k in seq[i1:i2]),
            "anchor_prev": seq[i1 - 1][2] if i1 else "", "base_sections": ",".join(map(str, secs)),
            "base_index": f"{j1}:{j2}",
            "agdt_codepoints": " | ".join(cps(f) for f, _, _ in seq[i1:i2]),
            "base_codepoints": " | ".join(cps(t) for t, _, _, _ in base["toks"][j1:j2]),
            "base_rubric": "yes" if any(t[3] for t in base["toks"][j1:j2]) else "no",
        })

    write_tsv(os.path.join(out, "lineage_revisions.tsv"), rev_rows, list(rev_rows[0]))
    write_tsv(os.path.join(out, "lineage_changes.tsv"), change_rows,
              ["from_commit", "to_commit", "to_date", "op", "sections", "from_index", "to_index",
               "from_codepoints", "to_codepoints"])
    write_tsv(os.path.join(out, "agdt_vs_revisions.tsv"), vs_rows, list(vs_rows[0]))
    write_tsv(os.path.join(out, "agdt_v16_vs_annotated_base.tsv"), base_rows,
              ["op", "node_keys", "anchor_prev", "base_sections", "base_index", "agdt_codepoints",
               "base_codepoints", "base_rubric"])
    summary = {
        "annotation_date": ANNOTATION_DATE,
        "annotated_base_commit": base["commit"],
        "distinct_text_revisions": sum(1 for r in rev_rows if r["token_text_changed"] == "yes"),
        "lineage_change_ops": len(change_rows),
        "agdt_v16_vs_base_nonequal_ops": len(base_rows),
        "best_match": {rel: min((x for x in vs_rows if x["agdt_release"] == rel),
                                key=lambda x: (x["agdt_tokens_in_nonequal"] + x["text_tokens_in_nonequal"],
                                               x["date"]))["commit"] for rel in agdt},
    }
    with open(os.path.join(out, "lineage_summary.json"), "w", encoding="ascii") as f:
        json.dump(summary, f, indent=1, sort_keys=True)
        f.write("\n")
    print(json.dumps(summary, indent=1, sort_keys=True))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
