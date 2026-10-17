"""Body-only candidate #2147 segmentation mirroring production unit selection (scratch only).

Corrected body-only oracle for sample provenance. Differences from segment_epub.py
(the superseded candidate, kept unchanged so the original out1 run reproduces):
  - unit selection follows internal/epub/epub.go readDocument: spine items with
    linear="no", non-XHTML media types, or the nav property are skipped; units whose
    extracted text is empty are dropped (the cover wrapper has only an empty-alt image);
  - head, script, style and nav subtrees are hidden (production extractXHTML skips
    head/script/style/nav), so <title> text never becomes a group;
  - the #2147 candidate break rules are otherwise unchanged (<br/> -> newline, etc.).
    Production's current reader instead treats <br> as a block boundary; that is a
    deliberate candidate difference, not a parity claim.

Output records the spine index, manifest id, member path, per-unit and per-group
SHA256 digests, and the decision for every spine item. Groups keep unit-relative
offsets, so a rebase only moves unit_start.

Usage: python3 -I segment_body.py <epub> <out.json>
"""

import hashlib
import importlib.util
import json
import os
import sys
import zipfile
from xml.etree import ElementTree

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("segment_epub", os.path.join(HERE, "segment_epub.py"))
seg = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(seg)

HIDDEN = {"head", "script", "style", "nav"}
OPF_NS = {"opf": "http://www.idpf.org/2007/opf"}


class BodyParser(seg.UnitParser):
    def __init__(self):
        super().__init__()
        self.hidden_depth = 0
        self.br_count = 0
        self.nbsp_chars = 0

    def handle_starttag(self, tag, attrs):
        if tag in HIDDEN:
            self.hidden_depth += 1
            return
        if self.hidden_depth:
            return
        if tag == "br":
            self.br_count += 1
        super().handle_starttag(tag, attrs)

    def handle_endtag(self, tag):
        if tag in HIDDEN:
            self.hidden_depth = max(0, self.hidden_depth - 1)
            return
        if self.hidden_depth:
            return
        super().handle_endtag(tag)

    def handle_data(self, data):
        if self.hidden_depth:
            return
        self.nbsp_chars += data.count(" ")
        super().handle_data(data)


def sha(text):
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def main(epub_path, out_path):
    with zipfile.ZipFile(epub_path) as archive:
        opf_name = next(n for n in archive.namelist() if n.endswith("content.opf"))
        opf_dir = opf_name.rsplit("/", 1)[0] + "/" if "/" in opf_name else ""
        root = ElementTree.fromstring(archive.read(opf_name))
        manifest = {item.get("id"): item for item in root.iterfind("opf:manifest/opf:item", OPF_NS)}
        decisions = []
        units = []
        for spine_index, ref in enumerate(root.iterfind("opf:spine/opf:itemref", OPF_NS)):
            item = manifest[ref.get("idref")]
            href = item.get("href")
            member = opf_dir + href
            record = {"spine_index": spine_index, "manifest_id": ref.get("idref"), "xhtml_path": member,
                      "linear": ref.get("linear", "yes"), "media_type": item.get("media-type"),
                      "properties": item.get("properties", "")}
            if record["linear"].lower() == "no":
                record["decision"] = "skipped_nonlinear"
            elif item.get("media-type") != "application/xhtml+xml" or "nav" in (item.get("properties") or "").split():
                record["decision"] = "skipped_not_content"
            else:
                raw = archive.read(member)
                parser = BodyParser()
                parser.feed(raw.decode("utf-8"))
                parser._flush()
                if not parser.groups:
                    record["decision"] = "dropped_empty_text"
                    record["raw_sha256"] = sha(raw.decode("utf-8"))
                    record["br_count"] = parser.br_count
                    decisions.append(record)
                    continue
                groups, parts, offset = [], [], 0
                for ordinal, group in enumerate(parser.groups, start=1):
                    text = group["text"]
                    if parts:
                        offset += 2
                    parts.append(text)
                    groups.append({**group, "ordinal": ordinal, "unit_start": offset, "unit_end": offset + len(text),
                                   "text_sha256": sha(text), "lines": text.count("\n") + 1})
                    offset += len(text)
                unit_text = "\n\n".join(parts)
                record.update({"decision": "kept", "raw_sha256": hashlib.sha256(raw).hexdigest(),
                               "unit_text_sha256": sha(unit_text), "unit_codepoints": len(unit_text),
                               "br_count": parser.br_count, "nbsp_chars": parser.nbsp_chars,
                               "anomalies": parser.anomalies, "groups": groups, "unit_text": unit_text})
                units.append(record)
            decisions.append(record)

    kept = [u for u in decisions if u.get("decision") == "kept"]
    groups_all = [g for u in kept for g in u["groups"]]
    summary = {
        "spine_items": len(decisions),
        "decisions": {d: sum(1 for r in decisions if r["decision"] == d) for d in
                      sorted({r["decision"] for r in decisions})},
        "kept_units": len(kept),
        "groups": len(groups_all),
        "source_lines": sum(g["lines"] for g in groups_all),
        "multi_line_groups": sum(1 for g in groups_all if g["lines"] > 1),
        "br_elements": sum(u["br_count"] for u in kept),
        "nbsp_chars": sum(u["nbsp_chars"] for u in kept),
    }
    with open(out_path, "w", encoding="utf-8") as sink:
        json.dump({"summary": summary, "spine": decisions}, sink, ensure_ascii=False)
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
