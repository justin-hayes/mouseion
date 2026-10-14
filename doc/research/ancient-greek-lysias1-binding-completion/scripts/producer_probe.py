#!/usr/bin/env python3
"""Re-run the historical llt-tokenizer Greek path over Lysias I and compare it with AGDT v1.6 (#2048 task A).

Usage:
  python3 -I producer_probe.py <inputs-dir> <tokenizer-dir> <out-dir>

<inputs-dir> is written by fetch_inputs.sh (canon/, agdt/); <tokenizer-dir> by fetch_tokenizer.sh.
Only the Greek path of the tokenizer is reproduced: split_krasis, the enclitic map (split and
insert before the host) and PUNCTUATION splitting. Latin-only steps (que/ne/ve corrections, quam diu
merging, abbreviation lists) cannot touch Greek running text and are not run. The Ruby sources are
read as data; their character lists and regex literals are parsed, not retyped.

Two rule sets are run:
  annotation-era  llt-tokenizer c9b60a34 (2014-08-18), the last commit before the v1.6 date stamp
  post-fix        llt-tokenizer a4a810f (2014-08-25), the first commit with the accented οὔ split
Neither is proven to be the deployed producer. Each is a candidate baseline to test against output.
"""
import difflib
import json
import re
import sys
import unicodedata
import xml.etree.ElementTree as ET
from pathlib import Path

PUNCTUATION_SRC = r"&(?:amp|quot|apos|lt|gt);|([\.\?,!;\-:\"'”&\(\)\[\]†<>᾽·])\1*"
PUNCTUATION = re.compile(PUNCTUATION_SRC)
APOSTROPHE_JOIN = "᾽"  # ᾽, Ruby greek_apostrophe
ENCLITIC_MARK = "-"
KRASIS_MARK = "-"


def ruby_wordlists(src):
    """Parse `NAME = %w(...)` assignments from greek.rb."""
    lists = {}
    for m in re.finditer(r"^\s*([A-Z_]+) = %w\((.*?)\)\s*$", src, re.M):
        lists[m.group(1)] = m.group(2).split()
    return lists


def krasis_regex(lists):
    plain = lists["PLAIN_VOWELS"]
    consonants = lists["CONSONANTS"]
    vowels = (lists["PLAIN_VOWELS"] + lists["VOWELS_WITH_ACUTE"] + lists["VOWELS_WITH_GRAVE"]
              + lists["VOWELS_WITH_CIRCUMFLEX"] + lists["VOWELS_WITH_IOTA"])
    spiritus = (lists["SPIRITUS_LENIS"] + lists["SPIRITUS_LENIS_WITH_ACUTE"]
                + lists["SPIRITUS_LENIS_WITH_GRAVE"] + lists["SPIRITUS_LENIS_WITH_CIRCUMFLEX"]
                + lists["SPIRITUS_ASPER"] + lists["SPIRITUS_ASPER_WITH_ACUTE"]
                + lists["SPIRITUS_ASPER_WITH_GRAVE"] + lists["SPIRITUS_ASPER_WITH_CIRCUMFLEX"]
                + lists["SPIRITUS_WITH_IOTA"])
    cons_alt = "|".join(map(re.escape, consonants))
    starting_alt = "|".join(map(re.escape, spiritus))
    all_alt = "|".join(map(re.escape, consonants + vowels))
    # Ruby interpolates the Array PLAIN_VOWELS with #{} as its inspect form, not as an alternation.
    # The Ruby regex is therefore an optional character class of quotes, commas, spaces and vowels.
    plain_inspect = "[" + ", ".join('"%s"' % v for v in plain) + "]?"
    return re.compile("^(%s)(%s(?:%s))((?:%s)*)$" % (cons_alt, plain_inspect, starting_alt, all_alt))


def ruby_enclitic_map(src, mode):
    """Parse ENCLITICS_MAP from tokenizer.rb. Returns [(regex, enclitic_or_None)]."""
    start = src.index("ENCLITICS_MAP")
    block = src[start:src.index("def split_frequent_enclitics", start)]
    rules = []
    for line in block.splitlines():
        m = re.match(r"^\s*/(.+)/i(?: => '(.+)')?,?\s*$", line)
        if m:
            value = m.group(2)
            if mode == "hash" and value is None:
                continue
            rules.append((re.compile(m.group(1), re.I), value))
    return rules


def split_krasis(tokens, krasis_re):
    out = []
    for t in tokens:
        m = krasis_re.match(t)
        if m:
            out.append(m.group(1) + KRASIS_MARK)
            out.append(m.group(2) + m.group(3))
        else:
            out.append(t)
    return out


def join_greek_apostrophe(tokens, cons_re):
    out = []
    i = 0
    while i < len(tokens):
        e = tokens[i]
        if i + 1 < len(tokens) and tokens[i + 1] == APOSTROPHE_JOIN and cons_re.search(e):
            out.append(e + APOSTROPHE_JOIN)
            i += 2
        else:
            out.append(e)
            i += 1
    return out


def split_frequent_enclitics(tokens, rules, mode):
    """Port of split_frequent_enclitics with shifting: true (@shift_range = 0)."""
    container = []
    work = list(tokens)
    for i in range(len(work)):
        for regex, value in rules:
            m = regex.search(work[i])
            if m:
                enc = value if mode == "hash" else m.group(2)
                work[i] = work[i][: -len(enc)]
                container.append((enc, i + len(container)))
    for enc, idx in container:
        work.insert(idx, ENCLITIC_MARK + enc)
    return work, [(ENCLITIC_MARK + enc, idx) for enc, idx in container]


def producer_tokens(text, lists, rules, mode):
    """Return (tokens, insertion log) for one running-text string."""
    spaced = PUNCTUATION.sub(lambda m: " " + m.group(0) + " ", text)
    tokens = [t for t in re.split(r"[ \t\n\r\f\v]+", spaced) if t]
    cons_re = re.compile("[" + "".join(lists["CONSONANTS"]) + "]")
    tokens = join_greek_apostrophe(tokens, cons_re)
    tokens = split_krasis(tokens, krasis_regex(lists))
    tokens, log = split_frequent_enclitics(tokens, rules, mode)
    return tokens, log


SKIP_TAGS = {"note", "milestone"}


def running_text(path):
    """Concatenate the text of <body>, skipping notes and milestones (keeping their tails)."""
    root = ET.parse(path).getroot()
    body = next(el for el in root.iter() if el.tag.split("}")[-1] == "body")
    parts = []

    def walk(el):
        local = el.tag.split("}")[-1]
        if local not in SKIP_TAGS:
            if el.text:
                parts.append(el.text)
            for child in el:
                walk(child)
        if el.tail:
            parts.append(el.tail)

    if body.text:
        parts.append(body.text)
    for child in body:
        walk(child)
    return "".join(parts)


def native_nodes(path):
    """Printed AGDT v1.6 nodes (artificial nodes are unprinted and excluded)."""
    root = ET.parse(path).getroot()
    nodes = []
    for sentence in root.iter("sentence"):
        sid = int(sentence.get("id"))
        for w in sentence.iter("word"):
            if w.get("artificial") is not None:
                continue
            nodes.append({"sentence": sid, "node": "s%03dw%03d" % (sid, int(w.get("id"))),
                          "form": w.get("form")})
    return nodes


def cp(text):
    """Code-point escape for committed outputs: no raw Greek in manifests (see NOTICE.md)."""
    return " ".join("U+%04X" % ord(c) for c in text)


def compare(native, sim):
    sm = difflib.SequenceMatcher(None, [n["form"] for n in native], sim, autojunk=False)
    ops = []
    for tag, i1, i2, j1, j2 in sm.get_opcodes():
        ops.append({"tag": tag,
                    "native": [n["node"] for n in native[i1:i2]],
                    "native_forms": [n["form"] for n in native[i1:i2]],
                    "sim_forms": sim[j1:j2]})
    equal = sum(i2 - i1 for tag, i1, i2, _, _ in sm.get_opcodes() if tag == "equal")
    return ops, equal


def main(argv):
    inputs, tok_dir, out_dir = map(Path, argv[1:4])
    out_dir.mkdir(parents=True, exist_ok=True)
    tok_src = {}
    for stamp in ("c9b60a345708af72c721ec1183c9ab928f7b8c03", "a4a810f02f73bacf3f16e722425dfb80f3b2a5ab"):
        tok_src[stamp] = {
            "tokenizer": (tok_dir / stamp / "tokenizer.rb").read_text(encoding="utf-8"),
            "greek": (tok_dir / stamp / "greek.rb").read_text(encoding="utf-8"),
        }
    native = native_nodes(inputs / "agdt" / "v1.6.xml")
    native_forms = [n["form"] for n in native]
    results = {}
    for label, text_path in (("grc1-aa0dd9e6", inputs / "canon" / "grc1-aa0dd9e6.xml"),
                             ("grc2-pinned", inputs / "canon" / "grc2.xml")):
        text = running_text(text_path)
        for stamp, mode in (("c9b60a345708af72c721ec1183c9ab928f7b8c03", "hash"),
                            ("a4a810f02f73bacf3f16e722425dfb80f3b2a5ab", "array")):
            lists = ruby_wordlists(tok_src[stamp]["greek"])
            rules = ruby_enclitic_map(tok_src[stamp]["tokenizer"], mode)
            sim, log = producer_tokens(text, lists, rules, mode)
            ops, equal = compare(native, sim)
            key = "%s|%s" % (label, stamp[:7])
            results[key] = {"sim_tokens": len(sim), "native_nodes": len(native), "equal_tokens": equal,
                            "non_equal_ops": sum(1 for o in ops if o["tag"] != "equal"),
                            "enclitic_insertions": len(log)}
            with (out_dir / ("producer_alignment_%s_%s.tsv" % (label, stamp[:7]))).open("w", encoding="utf-8") as f:
                f.write("tag\tnative_nodes\tnative_forms\tsim_forms\n")
                for o in ops:
                    f.write("%s\t%s\t%s\t%s\n" % (o["tag"], ",".join(o["native"]),
                                                  " | ".join(cp(x) for x in o["native_forms"]),
                                                  " | ".join(cp(x) for x in o["sim_forms"])))
            if label == "grc1-aa0dd9e6":
                with (out_dir / ("producer_enclitic_insertions_%s.tsv" % stamp[:7])).open("w", encoding="utf-8") as f:
                    f.write("insert_index\tenclitic_token\n")
                    for tok, idx in log:
                        f.write("%d\t%s\n" % (idx, cp(tok)))
    (out_dir / "producer_summary.json").write_text(json.dumps(results, indent=2, ensure_ascii=False) + "\n",
                                                   encoding="utf-8")
    print(json.dumps(results, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main(sys.argv)
