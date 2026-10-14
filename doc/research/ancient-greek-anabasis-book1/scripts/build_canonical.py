"""Extract Anabasis Book 1 from the pinned canonical-greekLit TEI file.

Writes out/canonical_book1.json with, per section, the raw text (as encoded,
including apparatus content) and a reading that drops <del>, uses <corr> inside
<choice>, and keeps <sic>/<add> text. Offsets are Unicode code points into the
concatenated reading string for the whole Book.
"""
import json
import os
import sys
import unicodedata as ud
import xml.etree.ElementTree as ET

sys.path.insert(0, os.path.dirname(__file__))
from common import RAW, OUT, TEI  # noqa: E402

os.makedirs(OUT, exist_ok=True)


def text_of(el, mode):
    """mode 'raw': all character data. mode 'reading': skip <del>, choice->corr, keep sic/add."""
    parts = []
    if el.text:
        parts.append(el.text)
    for ch in el:
        tag = ch.tag.replace(TEI, "")
        if mode == "reading":
            if tag == "del":
                pass
            elif tag == "choice":
                corr = ch.find(TEI + "corr")
                if corr is not None:
                    parts.append(text_of(corr, mode))
                else:
                    parts.append(text_of(ch, mode))
            elif tag == "milestone":
                pass
            else:
                parts.append(text_of(ch, mode))
        elif mode == "hybrid":
            # keep <del> text, take the <corr> form inside <choice>
            if tag == "choice":
                corr = ch.find(TEI + "corr")
                parts.append(text_of(corr, mode) if corr is not None else text_of(ch, mode))
            elif tag == "milestone":
                pass
            else:
                parts.append(text_of(ch, mode))
        elif mode == "printed":
            # keep <del> text (as encoded), take the <sic> form inside <choice>
            if tag == "corr":
                pass
            elif tag == "milestone":
                pass
            else:
                parts.append(text_of(ch, mode))
        else:
            if tag == "milestone":
                pass
            else:
                parts.append(text_of(ch, mode))
        if ch.tail:
            parts.append(ch.tail)
    return "".join(parts)


def main():
    root = ET.parse(os.path.join(RAW, "canon_tlg0032_tlg006_grc2.xml")).getroot()
    body = root.find(f".//{TEI}body")
    book = [d for d in body.iter(f"{TEI}div") if d.get("subtype") == "book" and d.get("n") == "1"][0]
    sections = []
    for ch in book:
        if ch.get("subtype") != "chapter":
            continue
        for sec in ch:
            if sec.get("subtype") != "section":
                continue
            ref = f"1.{ch.get('n')}.{sec.get('n')}"
            paras = sec.findall(f"{TEI}p")
            raw = "".join(text_of(p, "raw") for p in paras)
            reading = "".join(text_of(p, "reading") for p in paras)
            printed = "".join(text_of(p, "printed") for p in paras)
            hybrid = "".join(text_of(p, "hybrid") for p in paras)
            sections.append({"ref": ref, "raw": raw, "reading": reading,
                             "printed": printed, "hybrid": hybrid, "n_p": len(paras)})
    # Normalization status of the encoded text
    nfc = all(ud.is_normalized("NFC", s["reading"]) for s in sections)
    book_reading = ""
    offsets = []
    for s in sections:
        start = len(book_reading)
        # section separator: a single space, not part of any reading
        book_reading += s["reading"] + " "
        offsets.append({"ref": s["ref"], "start": start, "end": start + len(s["reading"])})
    # Variant P ("printed"): <del> kept, <choice> uses <sic>. Offsets are per-variant.
    book_printed = ""
    offsets_printed = []
    for s in sections:
        start = len(book_printed)
        book_printed += s["printed"] + " "
        offsets_printed.append({"ref": s["ref"], "start": start, "end": start + len(s["printed"])})
    book_hybrid = ""
    offsets_hybrid = []
    for s in sections:
        start = len(book_hybrid)
        book_hybrid += s["hybrid"] + " "
        offsets_hybrid.append({"ref": s["ref"], "start": start, "end": start + len(s["hybrid"])})
    with open(os.path.join(OUT, "canonical_book1.json"), "w", encoding="utf-8") as f:
        json.dump({"sections": sections, "offsets": offsets, "reading_nfc": nfc,
                   "reading": book_reading, "n_sections": len(sections),
                   "codepoints_reading": len(book_reading.rstrip()),
                   "codepoints_raw": sum(len(s["raw"]) for s in sections),
                   "printed": book_printed, "offsets_printed": offsets_printed,
                   "hybrid": book_hybrid, "offsets_hybrid": offsets_hybrid},
                  f, ensure_ascii=False, indent=1)
    print("sections", len(sections), "first", sections[0]["ref"], "last", sections[-1]["ref"])
    print("reading NFC", nfc, "cp reading", len(book_reading.rstrip()), "cp raw", sum(len(s["raw"]) for s in sections))
    print("NFC-not-normalized sections:", [s["ref"] for s in sections if not ud.is_normalized("NFC", s["reading"])][:20])
    # Apparatus diffs
    for s in sections:
        if s["raw"] != s["reading"]:
            print("APPARATUS", s["ref"], "raw-minus-reading lengths", len(s["raw"]) - len(s["reading"]))


if __name__ == "__main__":
    main()
