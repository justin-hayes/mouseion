#!/usr/bin/env python3
"""Offline Book 1 binding proof; fail closed before unsupported lexical mapping."""

import argparse
import base64
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import sys
import unicodedata
import unittest
import xml.etree.ElementTree as ET

from extract_work import ARCHIVE_NAME, HERE, OGA, LAULA, ORIGINAL, UNIT, encoded, extract, require

X = "{http://www.w3.org/1999/xlink}href"
BASE = "{http://www.w3.org/XML/1998/namespace}base"
NS = "http://www.tei-c.org/ns/1.0"
T = "{" + NS + "}"
VERSION = "oga-anabasis1-native-binding/1"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def xml(data):
    # ElementTree does not load the external PAULA DTD. No XInclude or resolver.
    require(b"<!ENTITY" not in data, "xml_entity_declaration_forbidden")
    return ET.fromstring(data)


def unique(elements, key):
    result = {}
    for element in elements:
        identifier = element.get(key)
        require(identifier and identifier not in result, "missing_or_duplicate_identifier")
        result[identifier] = element
    return result


def layer(data, tag, kind, base):
    root = xml(data)
    require(root.tag in ("paula", "laula"), "wrong_layer_root")
    lists = [e for e in root if e.tag == tag]
    require(len(lists) == 1, "missing_or_duplicate_layer_list")
    elements = lists[0]
    require(elements.get("type") == kind and elements.get(BASE) == base, "wrong_layer_type_or_base")
    return elements


def features(data, kind):
    entries = layer(data, "featList", kind, UNIT + ".tok01.xml")
    unique(entries, "id")
    result = {}
    for entry in entries:
        ref = entry.get(X, "")
        require(re.fullmatch(r"#[te]_\d+", ref) and ref[1:] not in result, "invalid_or_duplicate_feature_target")
        require(entry.get("value") is not None, "missing_feature_value")
        result[ref[1:]] = entry.get("value")
    return result


def nodes_from_layers(text, token_data, cts_data, lemma_data, pos_data, laula_data):
    tokens = layer(token_data, "markList", "tok", UNIT + ".xml")
    token_ids = unique(tokens, "id")
    cts, lemmas = features(cts_data, "cts"), features(lemma_data, "lemma")
    tags = layer(pos_data, "multiFeatList", "postag", UNIT + ".tok01.xml")
    unique(tags, "id")
    poses = {}
    for tag in tags:
        ref = tag.get(X, "")
        require(re.fullmatch(r"#[te]_\d+", ref) and ref[1:] not in poses, "invalid_or_duplicate_pos_target")
        fields = unique(tag, "name")
        require("pos" in fields, "missing_native_pos")
        poses[ref[1:]] = {name: e.get("value") for name, e in fields.items()}
    laula = layer(laula_data, "markList", "tok", UNIT + ".xml")
    laula = unique(laula, "i")
    # Parsing metadata for the indivisible work is necessary to select Book 1;
    # no linguistic measurements of Books 2–7 are made.
    for mapping in (cts, lemmas, poses, laula):
        require(set(mapping) == set(token_ids), "layer_node_coverage_mismatch")
    selected = []
    passed_book1 = False
    for identifier, mark in token_ids.items():
        if not cts[identifier].startswith("1_"):
            if selected:
                passed_book1 = True
            continue
        require(not passed_book1, "noncontiguous_book1_nodes")
        require(re.fullmatch(r"[te]_\d+", identifier), "unsupported_node_id")
        match = re.fullmatch(r"#xpointer\(string-range\(//body,'',(\d+),(\d+)\)\)", mark.get(X, ""))
        require(match is not None, "unsupported_native_range")
        start, length = map(int, match.groups())
        start -= 1
        require(start >= 0 and length > 0 and start + length <= len(text), "native_range_out_of_bounds")
        surface = text[start:start + length]
        empty = identifier.startswith("e_")
        la = laula[identifier]
        if empty:
            require(poses[identifier]["pos"] == "-" and re.fullmatch(r"\[\d+\]", surface),
                    "unverified_empty_placeholder")
            require(la.get("r") == "" and re.search(r",\d+,0\)\)$", la.get(X, "")),
                    "empty_node_has_printed_referent")
        else:
            require(surface == la.get("r"), "native_laula_surface_disagreement")
            require(poses[identifier]["pos"] != "-", "printed_node_has_empty_pos")
        selected.append({"id": identifier, "cts": cts[identifier], "source_span": [start, start + length],
                         "surface": surface, "empty": empty, "raw_lemma": lemmas[identifier],
                         "native_morphology": poses[identifier], "laula_pointer": la.get(X)})
    require(selected and selected[0]["source_span"][0] == 0, "book1_start_not_bound")
    previous = 0
    for index, node in enumerate(selected):
        start, end = node["source_span"]
        require(text[previous:start] == ("" if index == 0 else " ") and start >= previous,
                "overlapping_nodes_or_unaccounted_source_text")
        previous = end
    return selected


def marker_free(text, nodes):
    end = nodes[-1]["source_span"][1]
    removed = set()
    for node in nodes:
        if node["empty"]:
            start, stop = node["source_span"]
            require(stop < end and text[stop] == " ", "empty_placeholder_separator_missing")
            removed.update(range(start, stop + 1))
    indices = [i for i in range(end) if i not in removed]
    reading = "".join(text[i] for i in indices)
    reverse = {source: target for target, source in enumerate(indices)}
    for node in nodes:
        start, stop = node["source_span"]
        if node["empty"]:
            node["reading_span"] = None
        else:
            target = [reverse[i] for i in range(start, stop)]
            require(target == list(range(target[0], target[-1] + 1)), "noncontiguous_printed_projection")
            node["reading_span"] = [target[0], target[-1] + 1]
            require(reading[target[0]:target[-1] + 1] == node["surface"], "projection_changed_printed_surface")
    return reading, indices


def tei_projection(section):
    """Exact XML string-value offsets; del/head suppressed, choice/corr selected.

    Whitespace is retained here; the comparison later explicitly permits only
    whitespace reflow, never accent/punctuation/case folding or lexical matching.
    """
    raw, kept, editorial = [], [], []

    def append(value, enabled):
        for character in value or "":
            if enabled:
                kept.append(len(raw))
            raw.append(character)

    def visit(element, enabled=True):
        name = element.tag.removeprefix(T)
        if name in ("del", "head"):
            enabled = False
        start = len(raw)
        append(element.text, enabled)
        for child in element:
            visit(child, enabled and (name != "choice" or child.tag == T + "corr"))
            append(child.tail, enabled)
        if name in ("del", "sic", "corr", "add", "choice"):
            editorial.append({"element": name, "xml_string_span": [start, len(raw)]})

    visit(section)
    raw = "".join(raw)
    require(raw == "".join(section.itertext()), "xml_string_coordinate_mismatch")
    retained = [(raw[i], i) for i in kept if not raw[i].isspace()]
    return raw, retained, editorial


def sections_from_tei(data):
    root = xml(data)
    require(root.tag == T + "TEI", "wrong_tei_root")
    books = root.findall(".//" + T + "div[@subtype='book'][@n='1']")
    require(len(books) == 1, "missing_or_duplicate_book1")
    result = {}
    chapters = books[0].findall(T + "div[@subtype='chapter']")
    require([c.get("n") for c in chapters] == [str(n) for n in range(1, 11)], "incomplete_chapters")
    for chapter in chapters:
        sections = chapter.findall(T + "div[@subtype='section']")
        require([s.get("n") for s in sections] == [str(n) for n in range(1, len(sections) + 1)],
                "noncontiguous_sections")
        for section in sections:
            citation = "1_" + chapter.get("n") + "_" + section.get("n")
            require(citation not in result, "duplicate_tei_section")
            result[citation] = section
    require(len(result) == 205 and list(result)[-1] == "1_10_19", "incomplete_book1_extent")
    return root, result


def bind_sections(nodes, sections, original_sections):
    by_section = {citation: [] for citation in sections}
    for node in nodes:
        require(node["cts"] in by_section, "unknown_book1_citation")
        by_section[node["cts"]].append(node)
    summary, coordinates = [], []
    for citation, section in sections.items():
        raw, characters, editorial = tei_projection(section)
        original_raw = "".join(original_sections[citation].itertext())
        require(original_raw == raw, "original_merged_book1_text_disagreement")
        group = by_section[citation]
        printed = [n for n in group if not n["empty"]]
        require(printed, "uncovered_tei_section")
        form = "".join(n["surface"] for n in printed)
        require(not any(c.isspace() for c in form), "whitespace_inside_native_form")
        require(form == "".join(c for c, _ in characters), "section_character_correspondence_failed:" + citation)
        offset = 0
        for node in printed:
            source_indices = [i for _, i in characters[offset:offset + len(node["surface"])]]
            coordinates.append({"id": node["id"], "cts": citation, "tei_string_indices": source_indices})
            offset += len(node["surface"])
        summary.append({"cts": citation, "native_nodes": len(group), "printed_nodes": len(printed),
                        "empty_nodes": len(group) - len(printed), "first_id": group[0]["id"],
                        "last_id": group[-1]["id"], "nonwhitespace_code_points": len(characters),
                        "tei_string_sha256": digest(raw.encode()), "editorial_elements": editorial})
    # Empty-node citation metadata can lag behind the printed passage. Preserve
    # those labels; they locate no printed character and must not reorder text.
    require([n["cts"] for n in nodes if not n["empty"]] ==
            [c for c in sections for n in by_section[c] if not n["empty"]],
            "printed_citation_order_disagrees_with_tei")
    return summary, coordinates


def laula_diagnostics(nodes, root):
    failures = []
    for node in nodes:
        match = re.fullmatch(r"#xpointer\(string-range\((.*),'',(\d+),(\d+)\)\)", node["laula_pointer"] or "")
        require(match is not None, "unsupported_laula_pointer")
        path, start, length = match.groups()
        steps = re.findall(r"/\*:(\w+)\[(\d+)\]", path)
        require("".join(f"/*:{name}[{index}]" for name, index in steps) == path and steps[0] == ("TEI", "1"),
                "unsupported_laula_path")
        element = root
        for name, index in steps[1:]:
            matches = [e for e in element if e.tag == T + name]
            require(1 <= int(index) <= len(matches), "laula_path_does_not_resolve")
            element = matches[int(index) - 1]
        # This is an explicitly declared diagnostic interpretation, not a repair
        # of upstream pointers. The validated binding uses exact character maps.
        value = " ".join("".join(element.itertext()).split())
        start, length = int(start) - 1, int(length)
        expected = "" if node["empty"] else node["surface"]
        if value[start:start + length] != expected:
            failures.append({"id": node["id"], "cts": node["cts"], "start": start, "length": length})
    return failures


def lexical_blockers(nodes):
    # Raw numeric endings are evidence of a required decoding question, not a
    # claim that every prediction is a correct Morpheus lexical discriminator.
    return [{"id": n["id"], "cts": n["cts"], "raw_lemma": n["raw_lemma"],
             "native_pos": n["native_morphology"]["pos"],
             "reason": "citation_decoding_and_lexical_discriminator_namespace_unestablished"}
            for n in nodes if not n["empty"] and re.search(r"[0-9]+$", n["raw_lemma"])]


def inputs(cache):
    manifest = json.loads((HERE / "archive-members.json").read_bytes())
    result = {}
    for record in manifest["members"]:
        path = Path(record["local_relative_path"])
        require(not path.is_absolute() and ".." not in path.parts, "unsafe_input_path")
        data = (cache / path).read_bytes()
        require(len(data) == record["bytes"] and digest(data) == record["sha256"], "retained_member_hash_mismatch")
        key = (record["container"], record["member"])
        require(key not in result, "duplicate_manifest_input")
        result[key] = data
    return result


def verify_producer_documents(cache):
    registry = json.loads((HERE / "producer-docs.json").read_bytes())
    for name, record in registry.items():
        require(Path(name).name == name, "unsafe_document_path")
        data = (cache / name).read_bytes()
        require(len(data) == record["bytes"] and digest(data) == record["sha256"], "producer_document_hash_mismatch")
        if "decoded_readme_sha256" in record:
            payload = json.loads(data)
            require(payload["commit_id"] == "267dbf045dfca1129115aef05ba9ac6ebc49f4db", "wrong_lemmatizer_document_pin")
            readme = base64.b64decode(payload["content"], validate=True)
            require(digest(readme) == record["decoded_readme_sha256"], "decoded_producer_document_hash_mismatch")
    return registry


def assess(source):
    def paula(name):
        return source[("workspace/oga.zip", OGA + name)]

    def laula(name):
        return source[("workspace/oga.zip", LAULA + name)]

    text_root = xml(paula(UNIT + ".xml"))
    require(text_root.tag == "paula" and len(text_root.findall("body")) == 1, "invalid_paula_text")
    text = "".join(text_root.find("body").itertext())
    nodes = nodes_from_layers(text, paula(UNIT + ".tok01.xml"), paula(UNIT + ".tok01_cts01.xml"),
                              paula(UNIT + ".tok01_lemma01.xml"), paula("postag." + UNIT + ".tok01_postag01.xml"),
                              laula(UNIT + ".tok01.xml"))
    reading, source_indices = marker_free(text, nodes)
    root, sections = sections_from_tei(laula(UNIT + ".xml"))
    _, originals = sections_from_tei(source[("original_Greek_files.zip", ORIGINAL + "data/tlg0032/tlg006/" + UNIT + ".xml")])
    section_summary, tei_coordinates = bind_sections(nodes, sections, originals)
    bad_pointers = laula_diagnostics(nodes, root)
    blockers = lexical_blockers(nodes)
    require(blockers, "expected_lexical_stop_changed_review_required")
    printed = [n for n in nodes if not n["empty"]]
    lagging_empty_citations = []
    previous_printed = None
    for node in nodes:
        if not node["empty"]:
            previous_printed = node
        elif previous_printed and node["cts"] != previous_printed["cts"]:
            lagging_empty_citations.append({"id": node["id"], "native_cts": node["cts"],
                                           "preceding_printed_cts": previous_printed["cts"]})
    public = {
        "version": VERSION, "python": "3.13", "unicode": "15.1.0", "language": "grc",
        "acquisition": "verified_official_archive_after_explicit_human_authorization",
        "rights": "private_inspection_and_attributed_bounded_diagnostics; not_export_or_provider_approval",
        "unit": {"work": UNIT, "book": "1", "chapters": 10, "sections": 205,
                 "first_cts": "1_1_1", "last_cts": "1_10_19", "edition": "Marchant 1904/reprint 1961",
                 "original_vs_merged_book1_string_values": "exact_in_all_sections"},
        "binding": {"verdict": "verified_native_spans_and_explicit_marker_free_projection",
                    "authority": "annotation_carried_PAULA_text; upstream_TEI_character_correspondence_verified",
                    "source_span": [0, nodes[-1]["source_span"][1]], "native_nodes": len(nodes),
                    "printed_nodes": len(printed), "empty_nodes": len(nodes) - len(printed),
                    "empty_placeholder_literals": dict(Counter(n["surface"] for n in nodes if n["empty"])),
                    "reading_code_points": len(reading), "reading_utf8_sha256": digest(reading.encode()),
                    "nonwhitespace_code_points": sum(s["nonwhitespace_code_points"] for s in section_summary),
                    "reading_to_paula_coordinate_sha256": digest(encoded(source_indices)),
                    "node_to_tei_coordinate_sha256": digest(encoded(tei_coordinates)),
                    "rules": "typed_e_node_and_following_single_space_removed; printed_chars_and_other_spacing_unchanged; TEI_del_omitted_choice_corr_selected_add_and_standalone_sic_retained"},
        "native_pos_printed_counts": dict(sorted(Counter(n["native_morphology"]["pos"] for n in printed).items())),
        "empty_node_citation_disagreements_with_preceding_printed_node": lagging_empty_citations,
        "optional_laula_pointer_diagnostic": {"interpretation": "literal_resolved_element_string_value_with_whitespace_folded",
                                              "affected_nodes": len(bad_pointers),
                                              "affected_sections": dict(sorted(Counter(n["cts"] for n in bad_pointers).items())),
                                              "fatal_to_verified_primary_binding": False,
                                              "nodes": bad_pointers},
        "required_lexical_identities": {"verdict": "stopped_unresolved_producer_decoding",
                                        "numbered_lemma_occurrences": len(blockers),
                                        "distinct_numbered_raw_lemmas": len({n["raw_lemma"] for n in blockers}),
                                        "blockers": blockers,
                                        "canonical_identity_count": None,
                                        "all_other_canonical_mappings": "not_established_or_published"},
        "sentences": "native_group_file_retained; completeness_not_assessed_after_required_identity_stop",
        "optional_morphology_syntax": "native_files_retained; canonical_capabilities_not_assessed",
        "practical_quality": "no_scholarly_or_sentence_input_assessment; raw_numbered_predictions_not_certified",
        "selection_go_independent_model_known_comparison": "not_run",
        "selected_identities": None, "selected_occurrence_coverage": None,
        "deterministic_card_inputs": None, "omissions": None,
        "gloss_translation_dictionary_anki_client_semantics": "unperformed",
        "vocabulary_ready": False, "source_or_mvp_accepted": False,
        "sections": section_summary,
    }
    private = {"reading.txt": reading.encode(), "native-nodes.json": encoded(nodes),
               "reading-to-paula.json": encoded(source_indices), "node-to-tei.json": encoded(tei_coordinates)}
    return public, private


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache", type=Path, default=HERE / "private/archive-members")
    parser.add_argument("--from-archive", action="store_true", help="reverify retained official archive and re-extract only allowlisted work members")
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    require(sys.version_info[:2] == (3, 13) and unicodedata.unidata_version == "15.1.0", "unicode_runtime_mismatch")
    if args.from_archive:
        manifest = extract(HERE / "private/archive" / ARCHIVE_NAME, args.cache)
        require((HERE / "archive-members.json").read_bytes() == encoded(manifest), "archive_member_manifest_differs")
    verify_producer_documents(HERE / "private/producer-docs")
    public, private = assess(inputs(args.cache))
    diagnostics = HERE / "import-ledger.json"
    if args.write:
        diagnostics.write_bytes(encoded(public))
    else:
        require(diagnostics.read_bytes() == encoded(public), "import_diagnostics_not_byte_identical")
    output = HERE / "private/import"
    output.mkdir(parents=True, exist_ok=True)
    for name, data in private.items():
        path = output / name
        if path.exists():
            require(path.read_bytes() == data, "retained_projection_differs")
        else:
            path.write_bytes(data)
    suite = unittest.defaultTestLoader.discover(str(HERE), pattern="test_*.py")
    require(unittest.TextTestRunner(verbosity=1).run(suite).wasSuccessful(), "focused_tests_failed")
    print("Book 1 binding reproduced; required identities unresolved; no vocabulary selection or card inputs published.")


if __name__ == "__main__":
    main()
