#!/usr/bin/env python3
"""Bounded OGA public-example reader; standard library only, not an app adapter."""

import argparse
from collections import Counter, defaultdict
import hashlib
import json
from pathlib import Path
import re
import sys
import unicodedata
from urllib.parse import urljoin, urlsplit
import xml.etree.ElementTree as ET


HERE = Path(__file__).resolve().parent
PIN = "a69d917dbe117a307829977dd556e22f05ce0715"
UNIT = "ggm0001.ggm001.1st1K-grc1"
PREFIX = f"annotation_example/{UNIT}/"
XLINK = "{http://www.w3.org/1999/xlink}href"
BASE = "{http://www.w3.org/XML/1998/namespace}base"
RANGE = re.compile(r"#xpointer\(string-range\(//body,'',(\d+),(\d+)\)\)")
VERSION = "oga-public-example-v1"
# Narrow category meanings only. No claim of complete AGDT -> UD conversion.
# Evidence: pinned OGA query/morphology.md, Part-of-speech table.
POS = {"a": "ADJ", "l": "DET", "r": "ADP", "m": "NUM"}


def encoded(value):
    """Stable UTF-8 JSON, no timestamps or machine paths."""
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def lemma_key(lemma):
    """Only plain Greek Unicode citation strings; ambiguous suffixes stay raw."""
    if not lemma or not all(
        unicodedata.category(c)[0] in "LM" and
        ("GREEK" in unicodedata.name(c, "") or unicodedata.combining(c))
        for c in lemma
    ):
        return None
    return unicodedata.normalize(
        "NFC", unicodedata.normalize("NFC", lemma).lower().replace("ς", "σ")
    )


def original_span(href, text):
    """PAULA example's absolute 1-based UCS start + length convention, only."""
    match = RANGE.fullmatch(href)
    if match is None:
        raise ValueError("unsupported_xpointer")
    first, length = map(int, match.groups())
    if first < 1 or length < 1 or first - 1 + length > len(text):
        raise ValueError("out_of_range_span")
    return first - 1, first - 1 + length


class Package:
    """Local closed set of XML documents. Never follows network XML references."""

    def __init__(self, files):
        self.files = files
        self.roots = {}
        self.ids = {}
        self.bases = {}
        self.lines = {}
        self.diagnostics = []
        self.references = []
        self.missing_optional_ids = []
        for path, data in sorted(files.items()):
            if b"<!ENTITY" in data.upper():
                raise ValueError("entity_declaration_forbidden")
            root = ET.fromstring(data, ET.XMLParser(target=ET.TreeBuilder(insert_comments=True)))
            self.roots[path] = root
            ids = {}
            self.ids[path] = ids
            for element in root.iter():
                native_id = element.get("id")
                if native_id:
                    if native_id in ids:
                        self.report("duplicate_id", path, native_id)
                    ids[native_id] = element
                elif element.tag in ("mark", "rel", "multiFeat"):
                    self.report("missing_id", path, element.tag)
                elif element.tag == "feat" and element.get(XLINK):
                    self.missing_optional_ids.append({"file": path, "href": element.get(XLINK)})
            source = data.decode("utf-8")
            self.lines[path] = {
                native_id: source.count("\n", 0, source.index(f'id="{native_id}"')) + 1
                for native_id in ids
            }
            self._bases(root, "/" + path)

    def _bases(self, element, inherited):
        base = urljoin(inherited, element.get(BASE, ""))
        self.bases[id(element)] = base
        for child in element:
            self._bases(child, base)

    def report(self, code, path, record, **extra):
        self.diagnostics.append({"code": code, "file": path, "record": record, **extra})

    def evidence(self, path, element):
        native_id = element.get("id")
        return {"file": path, "id": native_id, "line": self.lines[path].get(native_id),
                "href": element.get(XLINK), "xml_base_resolved": self.bases[id(element)].lstrip("/")}

    def resolve(self, path, element, href):
        uri = urlsplit(urljoin(self.bases[id(element)], href))
        target = uri.path.lstrip("/")
        record = element.get("id", element.tag)
        if uri.scheme or uri.netloc or uri.query or target not in self.roots:
            self.report("unsupported_document_link", path, record, href=href, target=target)
            return None
        if not uri.fragment or uri.fragment not in self.ids[target]:
            self.report("dangling_id", path, record, href=href, target=target)
            return None
        self.references.append({"file": path, "record": record, "target_file": target,
                                "target_id": uri.fragment})
        return target, uri.fragment

    def features(self, kind, expected):
        values = defaultdict(list)
        for path, root in self.roots.items():
            for layer in root.findall("featList"):
                if layer.get("type") != kind:
                    continue
                for feat in layer:
                    ref = self.resolve(path, feat, feat.get(XLINK, ""))
                    if ref is not None:
                        if ref[0] != expected:
                            self.report("wrong_layer_target", path, feat.get("id"), kind=kind)
                        values[ref].append({"value": feat.get("value"),
                                            "evidence": self.evidence(path, feat)})
        return values


def graph_summary(nodes, edges):
    incoming = defaultdict(list)
    adjacent = defaultdict(set)
    for edge in edges:
        a, b = edge["source"], edge["target"]
        incoming[b].append(a)
        adjacent[a].add(b)
        adjacent[b].add(a)
    # Kahn's algorithm: structural orientation only; never invent canonical heads.
    degree = {node: len(incoming[node]) for node in nodes}
    outgoing = defaultdict(list)
    for edge in edges:
        outgoing[edge["source"]].append(edge["target"])
    queue = [node for node in nodes if degree[node] == 0]
    removed = 0
    while queue:
        node = queue.pop()
        removed += 1
        for target in outgoing[node]:
            degree[target] -= 1
            if degree[target] == 0:
                queue.append(target)
    unseen = set(nodes)
    components = []
    while unseen:
        todo = [min(unseen)]
        component = set()
        while todo:
            node = todo.pop()
            if node in component:
                continue
            component.add(node)
            todo.extend(adjacent[node] - component)
        unseen -= component
        components.append(sorted(component))
    return {"orientation": "native source href -> target; no canonical dependency mapping",
            "zero_incoming_nodes": sorted(node for node in nodes if not incoming[node]),
            "multiple_incoming_nodes": sorted(node for node in nodes if len(incoming[node]) > 1),
            "acyclic": removed == len(nodes), "weak_components": components}


def read_package(files):
    package = Package(files)
    text_file = PREFIX + UNIT + ".xml"
    token_file = PREFIX + UNIT + ".tok01.xml"
    dep_file = PREFIX + "dep." + UNIT + ".tok01_dep01.xml"
    bodies = package.roots[text_file].findall("body")
    if len(bodies) != 1:
        raise ValueError("expected_one_body")
    # XML decoded string-value, excluding comment/PI text; no normalization/trim.
    def content(element):
        return (element.text or "") + "".join(
            (content(child) if isinstance(child.tag, str) else "") + (child.tail or "")
            for child in element
        )
    text = content(bodies[0])
    layers = {kind: package.features(kind, token_file)
              for kind in ("lemma", "cts", "ipa01")}
    relation_labels = package.features("dep_fnc", dep_file)
    postags = defaultdict(list)
    for path, root in package.roots.items():
        for layer in root.findall("multiFeatList"):
            if layer.get("type") != "postag":
                package.report("unsupported_layer", path, layer.get("type"))
                continue
            for element in layer:
                ref = package.resolve(path, element, element.get(XLINK, ""))
                if ref is None:
                    continue
                if ref[0] != token_file:
                    package.report("wrong_layer_target", path, element.get("id"), kind="postag")
                raw = defaultdict(list)
                for feat in element:
                    raw[feat.get("name")].append(feat.get("value"))
                for name, values in raw.items():
                    if len(values) != 1:
                        package.report("duplicate_grammatical_feature", path, element.get("id"), name=name)
                if len(raw.get("pos", [])) != 1:
                    package.report("missing_or_duplicate_pos", path, element.get("id"))
                postags[ref].append({"values": dict(raw), "evidence": package.evidence(path, element)})
    nodes = []
    cover = [0] * len(text)
    token_root = package.roots[token_file]
    mark_lists = token_root.findall("markList")
    if len(mark_lists) != 1:
        raise ValueError("expected_one_mark_list")
    marks = list(mark_lists[0])
    for index, mark in enumerate(marks):
        if mark.tag is ET.Comment:
            continue
        native_id = mark.get("id")
        if mark.tag != "mark" or not native_id:
            package.report("unsupported_mark", token_file, native_id)
            continue
        ref = (token_file, native_id)
        span = None
        form = None
        base = urlsplit(package.bases[id(mark)])
        if base.scheme or base.netloc or base.path.lstrip("/") != text_file:
            package.report("wrong_text_base", token_file, native_id)
        else:
            try:
                span = original_span(mark.get(XLINK, ""), text)
                form = text[slice(*span)]
                for position in range(*span):
                    cover[position] += 1
            except ValueError as error:
                package.report(str(error), token_file, native_id)
        witness = marks[index + 1].text if index + 1 < len(marks) and marks[index + 1].tag is ET.Comment else None
        if witness is None or witness != form:
            package.report("surface_witness_mismatch", token_file, native_id)
        linked = {kind: layer.get(ref, []) for kind, layer in layers.items()}
        linked["postag"] = postags.get(ref, [])
        for kind, records in linked.items():
            if len(records) != 1:
                package.report("layer_cardinality", token_file, native_id, kind=kind, count=len(records))
        lemma = linked["lemma"][0]["value"] if len(linked["lemma"]) == 1 else None
        tags = linked["postag"][0]["values"] if len(linked["postag"]) == 1 else {}
        raw_pos = tags.get("pos", [])
        raw_pos = raw_pos[0] if len(raw_pos) == 1 else None
        marker = native_id.startswith("e_") and form == "[0]" and lemma == "[0]" and raw_pos == "-"
        punctuation = bool(form) and all(unicodedata.category(c).startswith("P") for c in form)
        if native_id.startswith("e_") and not marker:
            package.report("unexpected_empty_representation", token_file, native_id)
        if marker:
            classification = "native_empty_marker; upstream insertion history unresolved"
        elif punctuation:
            classification = "source_punctuation"
        elif raw_pos == "u":
            classification = "native_u_nonpunctuation_unresolved"
        else:
            classification = "lexical_candidate_not_vocabulary_approval"
        key = lemma_key(lemma) if not (marker or punctuation) else None
        mapped_pos = POS.get(raw_pos) if not (marker or punctuation) else None
        nodes.append({"native_id": native_id, "stable_id": f"oga:{PIN}:{token_file}#{native_id}",
                      "source": package.evidence(token_file, mark), "source_span": span,
                      "native_coordinate": {"selector": "//body", "index_unit": "Unicode code point",
                                            "start_1based": span[0] + 1 if span else None,
                                            "length": span[1] - span[0] if span else None},
                      "lexical_span": None if marker or punctuation else span,
                      "form": form, "comment_witness": witness, "classification": classification,
                      "layers": linked, "raw_pos": raw_pos,
                      "native_pos_status": "absent" if raw_pos is None else
                                           "unknown" if raw_pos == "x" else
                                           "not_applicable" if raw_pos == "-" else "present",
                      "canonical": {"lemma": key, "lemma_status": "plain_unicode_profile_only" if key else "unresolved_or_nonlexical",
                                    "upos": mapped_pos, "upos_status": "narrow_category_mapping" if mapped_pos else "unresolved_or_nonlexical",
                                    "headword": lemma if key else None,
                                    "lexical_discriminator": {"status": "unknown_not_asserted_absent", "value": None},
                                    "vocabulary_identity": None},
                      "optional_canonical_layers": {"morphology": None, "syntax": None,
                                                    "status": "native_only_not_mapped"},
                      "provenance": {"annotation": "automatic; OGA release-level claim, no per-node review evidence",
                                     "model_revision": None},
                      "normalization": {"raw_is_nfc": bool(form) and unicodedata.is_normalized("NFC", form),
                                        "nfd_codepoints": len(unicodedata.normalize("NFD", form)) if form else None}})
    by_id = {node["native_id"]: node for node in nodes}
    edges = []
    for layer in package.roots[dep_file].findall("relList"):
        for rel in layer:
            source = package.resolve(dep_file, rel, rel.get(XLINK, ""))
            target = package.resolve(dep_file, rel, rel.get("target", ""))
            labels = relation_labels.get((dep_file, rel.get("id")), [])
            if len(labels) != 1:
                package.report("relation_label_cardinality", dep_file, rel.get("id"), count=len(labels))
            if source and target:
                if source[0] != token_file or target[0] != token_file:
                    package.report("cross_document_edge", dep_file, rel.get("id"))
                elif source[1] not in by_id or target[1] not in by_id:
                    package.report("non_token_edge", dep_file, rel.get("id"))
                else:
                    edges.append({"id": rel.get("id"), "source": source[1], "target": target[1],
                                  "labels": labels, "evidence": package.evidence(dep_file, rel)})
    metadata = []
    for path, root in package.roots.items():
        for layer in root.findall("featList"):
            if layer.get("type") in (*layers, "dep_fnc"):
                continue
            for feat in layer:
                package.resolve(path, feat, feat.get(XLINK, ""))
                metadata.append({"type": layer.get("type"), "value": feat.get("value"),
                                 "evidence": package.evidence(path, feat)})
    gaps = [i for i, count in enumerate(cover) if count == 0 and not text[i].isspace()]
    overlaps = [i for i, count in enumerate(cover) if count > 1]
    if gaps:
        package.report("uncovered_nonwhitespace", text_file, "body", positions=gaps)
    if overlaps:
        package.report("overlapping_spans", text_file, "body", positions=overlaps)
    graph = graph_summary(by_id, edges)
    if not graph["acyclic"]:
        package.report("dependency_cycle", dep_file, "relList")
    if graph["multiple_incoming_nodes"]:
        package.report("multiple_incoming", dep_file, "relList", nodes=graph["multiple_incoming_nodes"])
    groups = defaultdict(list)
    for node in nodes:
        for feature in node["layers"]["cts"]:
            groups[feature["value"]].append(node["native_id"])
    node_group = {node: group for group, ids in groups.items() for node in ids}
    cross_groups = [edge["id"] for edge in edges if node_group.get(edge["source"]) != node_group.get(edge["target"])]
    # Public ledger contains structure, not source forms/lemmas/tags or metadata values.
    ledger = {"reader_version": VERSION, "input_pin": PIN,
              "unicode_version": unicodedata.unidata_version,
              "result": "conditional_format_consumption_not_reading_or_vocabulary_readiness",
              "text": {"extraction": "XML body string-value, no trim/Unicode/spacing changes",
                       "codepoints": len(text), "utf8_sha256": digest(text.encode()),
                       "is_nfc": unicodedata.is_normalized("NFC", text),
                       "nfd_codepoints": len(unicodedata.normalize("NFD", text)),
                       "uncovered_whitespace_codepoints": sum(c == 0 for c in cover) - len(gaps)},
              "counts": {"nodes": len(nodes), "edges": len(edges), "metadata_fields": len(metadata),
                         "classifications": dict(Counter(n["classification"] for n in nodes)),
                         "narrow_pos_mapped": sum(n["canonical"]["upos"] is not None for n in nodes),
                         "plain_lemma_normalized": sum(n["canonical"]["lemma"] is not None for n in nodes)},
              "span_checks": {"comment_witnesses": len(nodes), "uncovered_nonwhitespace": gaps,
                              "overlap_positions": overlaps, "comments_are_independent_witness": False},
              "nodes": [{"id": n["native_id"], "source_span": n["source_span"],
                         "lexical_span": n["lexical_span"], "classification": n["classification"],
                         "pos_status": n["canonical"]["upos_status"],
                         "lemma_status": n["canonical"]["lemma_status"]} for n in nodes],
              "graph": graph, "cross_cts_group_edges": cross_groups,
              "native_cts_groups": dict(groups),
              "reference_resolution": {
                  "resolved": len(package.references),
                  "by_document_pair": [
                      {"file": a, "target_file": b, "resolved": count}
                      for (a, b), count in sorted(Counter(
                          (ref["file"], ref["target_file"]) for ref in package.references
                      ).items())
                  ]},
              "optional_metadata_ids_absent": package.missing_optional_ids,
              "diagnostics": package.diagnostics,
              "capabilities": {"original_text_spans": not package.diagnostics,
                               "linked_native_layers": not package.diagnostics,
                               "canonical_complete_syntax": False, "complete_reading_sentences": False,
                               "vocabulary_ready": False, "book1_coverage": "untested",
                               "prediction_accuracy": "unassessed", "reading_text_transformation": "none",
                               "empty_marker_original_edition_status": "unresolved",
                               "rights": "see RIGHTS.md; no source data redistribution in this branch"}}
    normalized = {"reader_version": VERSION, "unicode_version": unicodedata.unidata_version,
                  "native_documents": {path: {"header": root.find("header").attrib,
                                               "paula_version": root.get("version")}
                                       for path, root in package.roots.items()},
                  "lemma_profile": "ancient-greek-v1: NFC/lower/final-sigma/NFC; plain strings only",
                  "text_file": text_file, "text": text, "nodes": nodes, "edges": edges, "metadata": metadata}
    return normalized, ledger


def acquire(cache, registry):
    """Exact allowlist only. No recursion, archives, redirects, or retries."""
    from urllib.request import HTTPRedirectHandler, build_opener

    class NoRedirect(HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            raise ValueError("redirect_not_authorized")

    opener = build_opener(NoRedirect)
    for item in registry["files"]:
        path = cache / item["path"]
        if not path.exists():
            url = item["url"]
            allowed = (
                f"https://raw.githubusercontent.com/OperaGraecaAdnotata/OGA/{PIN}/",
                "https://raw.githubusercontent.com/korpling/paula-xml/316e01d43b43f99f683d5d0ebe631bb5ffe4973f/src/",
                "https://creativecommons.org/licenses/by-sa/4.0/legalcode.txt",
            )
            if not url.startswith(allowed) or ".." in Path(item["path"]).parts or Path(item["path"]).is_absolute():
                raise ValueError("input_outside_allowlist")
            with opener.open(url, timeout=60) as response:
                data = response.read(item["size"] + 1)
            if digest(data) != item["sha256"] or len(data) != item["size"]:
                raise ValueError("download_hash_mismatch: " + item["path"])
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        data = path.read_bytes()
        if digest(data) != item["sha256"] or len(data) != item["size"]:
            raise ValueError("cached_hash_mismatch: " + item["path"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache", type=Path, default=Path.home() / ".cache/scratch/greek-oga-format-probe/inputs")
    parser.add_argument("--output", type=Path, default=Path.home() / ".cache/scratch/greek-oga-format-probe/output")
    parser.add_argument("--fetch", action="store_true", help="explicit local noncommercial public-example acquisition")
    parser.add_argument("--check", action="store_true", help="compare regenerated structural ledger byte-for-byte")
    parser.add_argument("--test", action="store_true", help="run synthetic and pinned-package tests")
    args = parser.parse_args()
    registry = json.loads((HERE / "inputs.json").read_text())
    if args.fetch:
        acquire(args.cache, registry)
    files = {}
    for item in registry["files"]:
        data = (args.cache / item["path"]).read_bytes()
        if digest(data) != item["sha256"]:
            raise ValueError("input_hash_mismatch: " + item["path"])
        if item["path"].startswith(PREFIX):
            files[item["path"]] = data
    normalized, ledger = read_package(files)
    ledger["input_hashes"] = {item["path"]: item["sha256"] for item in registry["files"]}
    args.output.mkdir(parents=True, exist_ok=True)
    outputs = {"normalized.json": encoded(normalized), "ledger.json": encoded(ledger),
               "text.txt": normalized["text"].encode()}
    manifest = {name: {"sha256": digest(data), "bytes": len(data)} for name, data in outputs.items()}
    for name, data in outputs.items():
        (args.output / name).write_bytes(data)
    (args.output / "outputs.json").write_bytes(encoded(manifest))
    if args.check and (HERE / "ledger.json").read_bytes() != encoded(ledger):
        raise ValueError("committed_ledger_differs")
    if args.check and (HERE / "outputs.json").read_bytes() != encoded(manifest):
        raise ValueError("committed_output_hashes_differ")
    if args.test:
        import os
        import unittest

        os.environ["OGA_PROBE_CACHE"] = str(args.cache.resolve())
        suite = unittest.defaultTestLoader.discover(str(HERE), pattern="test_probe.py")
        if not unittest.TextTestRunner(verbosity=1).run(suite).wasSuccessful():
            return 1
    print(json.dumps({"result": ledger["result"], "counts": ledger["counts"],
                      "diagnostics": len(ledger["diagnostics"]), "byte_identical": args.check}))
    return 1 if ledger["diagnostics"] else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, ET.ParseError) as error:
        print(f"probe failed: {error}", file=sys.stderr)
        sys.exit(2)
