"""Candidate #2147 extraction simulation over the pinned PG #21000 EPUB (read-only).

Reads XHTML members straight from the EPUB zip (nothing extracted to disk) and
applies the accepted break rules:
  - <br/> -> "\\n"; block boundary (p, h1-6, div, li, blockquote, hr, ul, table...)
    -> group break, groups joined by "\\n\\n" in the unit text;
  - incidental ASCII whitespace (including source newlines) collapses to one space
    and is trimmed at line edges;
  - <ins> contributes its visible text only (title attribute dropped);
  - x-ebookmaker-pageno spans are zero width;
  - emphasis, indentation and alignment classes are dropped;
  - roles only from the allow-list: p/span.speaker, entrance, sdcenter, sdsetting,
    dirlong, dirlongind, span.sd (nested spans recorded with group offsets).

SUPERSEDED candidate segmentation: every spine item is kept (including the cover wrapper and
<title> text). It is retained only as the input that reproduces the original out1 Producer run,
whose unit indexes and ordinals run_producer.py selects. The corrected body-only oracle is
segment_body.py.

Output: JSON with per-unit text, digests and group records.
Usage: python3 -I segment_epub.py <epub> <out.json>
"""

import hashlib
import json
import re
import sys
import zipfile
from html.parser import HTMLParser
from xml.etree import ElementTree

ROLE_BLOCK_CLASSES = {
    "speaker": "speaker_label",
    "entrance": "stage_direction",
    "sdcenter": "stage_direction",
    "sdsetting": "stage_direction",
    "dirlong": "stage_direction",
    "dirlongind": "stage_direction",
}
ROLE_SPAN_CLASSES = {"speaker": "speaker_label", "sd": "stage_direction"}
GROUP_TAGS = {"p", "h1", "h2", "h3", "h4", "h5", "h6", "div", "li", "blockquote", "ul", "ol", "hr", "table", "tr", "td", "dd", "dt"}
VOID = {"br", "hr", "img", "link", "meta", "input", "col"}
ASCII_WS = re.compile(r"[ \t\r\n\f\v]+")


class UnitParser(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.groups = []
        self.buf = ""
        self.block_tag = None
        self.block_classes = set()
        self.block_role = None
        self.stack = []  # open non-void elements: (tag, classes, role, start)
        self.suppress = 0
        self.spans = []
        self.anomalies = {"double_br": 0, "nbsp_chars": 0, "ins_visible_only": 0, "pageno_spans": 0, "unknown_role_class": 0}
        self.br_run = 0
        self.nbsp_seen = 0

    def _flush(self):
        text = self.buf.rstrip(" \n")
        if text.strip():
            self.groups.append({
                "block_tag": self.block_tag or "text",
                "block_classes": sorted(self.block_classes),
                "block_role": self.block_role,
                "text": text,
                "spans": [s for s in self.spans if s["end"] <= len(text)],
            })
        self.buf = ""
        self.spans = []
        self.br_run = 0

    def _block_start(self, tag, classes):
        self._flush()
        self.block_tag = tag
        self.block_classes = set(classes)
        self.block_role = None
        for cls in classes:
            if cls in ROLE_BLOCK_CLASSES:
                self.block_role = ROLE_BLOCK_CLASSES[cls]

    def handle_starttag(self, tag, attrs):
        classes = [c for a, v in attrs if a == "class" and v for c in v.split()]
        if tag == "br":
            if self.buf.endswith("\n"):
                self.anomalies["double_br"] += 1
            self.buf = self.buf.rstrip(" ") + "\n"
            return
        if tag == "hr":
            self._flush()
            return
        if tag in GROUP_TAGS:
            self._block_start(tag, classes)
        if "x-ebookmaker-pageno" in classes:
            self.anomalies["pageno_spans"] += 1
            self.suppress += 1
            self.stack.append((tag, classes, None, None, True))
            return
        role = None
        if tag == "span" or tag == "ins":
            for cls in classes:
                if cls in ROLE_SPAN_CLASSES:
                    role = ROLE_SPAN_CLASSES[cls]
        if tag not in VOID:
            self.stack.append((tag, classes, role, len(self.buf), False))

    def handle_endtag(self, tag):
        for index in range(len(self.stack) - 1, -1, -1):
            entry_tag, classes, role, start, is_pageno = self.stack[index]
            if entry_tag != tag:
                continue
            del self.stack[index:]
            if is_pageno:
                self.suppress -= 1
            elif role is not None and start is not None:
                self.spans.append({"role": role, "start": start, "end": len(self.buf), "tag": tag, "classes": classes})
            break
        if tag in GROUP_TAGS:
            self._flush()
            self.block_tag = None
            self.block_classes = set()
            self.block_role = None

    def handle_data(self, data):
        if self.suppress:
            return
        if " " in data:
            self.nbsp_seen += data.count(" ")
        text = ASCII_WS.sub(" ", data)
        if not text:
            return
        if text.startswith(" ") and (self.buf == "" or self.buf.endswith((" ", "\n"))):
            text = text[1:]
        self.buf += text


def unit_order(opf_xml):
    root = ElementTree.fromstring(opf_xml)
    ns = {"opf": "http://www.idpf.org/2007/opf"}
    manifest = {item.get("id"): item.get("href") for item in root.iterfind("opf:manifest/opf:item", ns)}
    return [manifest[ref.get("idref")] for ref in root.iterfind("opf:spine/opf:itemref", ns)]


def main(epub_path, out_path):
    with zipfile.ZipFile(epub_path) as archive:
        opf_name = next(n for n in archive.namelist() if n.endswith("content.opf"))
        opf_dir = opf_name.rsplit("/", 1)[0] + "/" if "/" in opf_name else ""
        hrefs = unit_order(archive.read(opf_name))
        units = []
        for href in hrefs:
            member = opf_dir + href
            raw = archive.read(member)
            parser = UnitParser()
            parser.feed(raw.decode("utf-8"))
            parser._flush()
            unit_text_parts = []
            groups = []
            offset = 0
            for ordinal, group in enumerate(parser.groups, start=1):
                text = group["text"]
                # Unit-relative start, groups separated by a block boundary "\n\n".
                if unit_text_parts:
                    offset += 2
                unit_text_parts.append(text)
                groups.append({**group, "ordinal": ordinal, "unit_start": offset, "unit_end": offset + len(text),
                               "text_sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
                               "lines": text.count("\n") + 1})
                offset += len(text)
            unit_text = "\n\n".join(unit_text_parts)
            units.append({
                "xhtml_path": member,
                "xhtml_sha256": hashlib.sha256(raw).hexdigest(),
                "unit_text_sha256": hashlib.sha256(unit_text.encode("utf-8")).hexdigest(),
                "unit_codepoints": len(unit_text),
                "anomalies": parser.anomalies,
                "nbsp_chars": parser.nbsp_seen,
                "groups": groups,
                "unit_text": unit_text,
            })
    summary = {
        "units": len(units),
        "groups": sum(len(u["groups"]) for u in units),
        "source_lines": sum(g["lines"] for u in units for g in u["groups"]),
        "multi_line_groups": sum(1 for u in units for g in u["groups"] if g["lines"] > 1),
        "role_groups": {role: sum(1 for u in units for g in u["groups"] if g["block_role"] == role) for role in set(ROLE_BLOCK_CLASSES.values())},
        "anomalies": {k: sum(u["anomalies"][k] for u in units) for k in units[0]["anomalies"]} if units else {},
    }
    with open(out_path, "w", encoding="utf-8") as sink:
        json.dump({"summary": summary, "units": units}, sink, ensure_ascii=False)
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
