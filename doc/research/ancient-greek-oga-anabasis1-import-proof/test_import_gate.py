"""Invented Unicode/XML examples: binding must fail independently of file hashes."""

from copy import deepcopy
import unittest
import xml.etree.ElementTree as ET

import import_gate as gate


def layer(tag, kind, base, children):
    root = ET.Element("paula")
    entries = ET.SubElement(root, tag, {"type": kind, gate.BASE: base})
    for child in children:
        entries.append(child)
    return ET.tostring(root)


def fixture(surface="α", second="β"):
    text = surface + " [0] " + second
    values = [("t_1", surface, "n"), ("e_1", "[0]", "-"), ("t_2", second, "n")]
    tokens, citations, lemmas, tags, laula = [], [], [], [], []
    offset = 1
    for index, (identifier, value, pos) in enumerate(values):
        tokens.append(ET.Element("mark", {"id": identifier, gate.X:
                      f"#xpointer(string-range(//body,'',{offset},{len(value)}))"}))
        citations.append(ET.Element("feat", {"id": f"c_{index}", gate.X: "#" + identifier, "value": "1_1_1"}))
        lemmas.append(ET.Element("feat", {"id": f"l_{index}", gate.X: "#" + identifier, "value": value}))
        tag = ET.Element("multiFeat", {"id": f"p_{index}", gate.X: "#" + identifier})
        ET.SubElement(tag, "feat", {"name": "pos", "value": pos})
        tags.append(tag)
        laula.append(ET.Element("m", {"i": identifier, "r": "" if pos == "-" else value,
                     gate.X: "#xpointer(string-range(/*:TEI[1],'',1," + ("0" if pos == "-" else str(len(value))) + "))"}))
        offset += len(value) + 1
    unit = gate.UNIT
    return [text, layer("markList", "tok", unit + ".xml", tokens),
            layer("featList", "cts", unit + ".tok01.xml", citations),
            layer("featList", "lemma", unit + ".tok01.xml", lemmas),
            layer("multiFeatList", "postag", unit + ".tok01.xml", tags),
            layer("markList", "tok", unit + ".xml", laula)]


def edit(data, transform):
    root = ET.fromstring(data)
    transform(root)
    return ET.tostring(root)


def section(content):
    return ET.fromstring(f'<div xmlns="{gate.NS}">{content}</div>')


def invented_book():
    root = ET.Element(gate.T + "TEI")
    book = ET.SubElement(root, gate.T + "div", {"subtype": "book", "n": "1"})
    for chapter, count in enumerate((11, 27, 21, 19, 17, 11, 20, 29, 31, 19), 1):
        c = ET.SubElement(book, gate.T + "div", {"subtype": "chapter", "n": str(chapter)})
        for number in range(1, count + 1):
            ET.SubElement(c, gate.T + "div", {"subtype": "section", "n": str(number)}).text = "α"
    return root


class BindingTests(unittest.TestCase):
    def test_native_ranges_and_typed_marker_projection(self):
        args = fixture()
        nodes = gate.nodes_from_layers(*args)
        reading, indices = gate.marker_free(args[0], nodes)
        self.assertEqual(reading, "α β")
        self.assertEqual(indices, [0, 1, 6])
        self.assertEqual(nodes[0]["reading_span"], [0, 1])
        self.assertIsNone(nodes[1]["reading_span"])
        self.assertEqual(nodes[2]["reading_span"], [2, 3])

    def test_nfd_and_non_bmp_use_code_points_without_normalizing(self):
        for surface in ("α\u0313\u0301", "𐅂"):
            args = fixture(surface)
            nodes = gate.nodes_from_layers(*args)
            reading, _ = gate.marker_free(args[0], nodes)
            self.assertEqual(reading, surface + " β")
            self.assertEqual(nodes[0]["reading_span"], [0, len(surface)])

    def test_wrong_span_fails_even_with_valid_xml(self):
        args = fixture()
        args[1] = args[1].replace(b",1,1))", b",2,1))")
        with self.assertRaisesRegex(ValueError, "surface_disagreement"):
            gate.nodes_from_layers(*args)

    def test_out_of_bounds_span_fails(self):
        args = fixture()
        args[1] = args[1].replace(b",1,1))", b",999,1))")
        with self.assertRaisesRegex(ValueError, "range_out_of_bounds"):
            gate.nodes_from_layers(*args)

    def test_wrong_xml_base_fails(self):
        args = fixture()
        args[1] = args[1].replace((gate.UNIT + ".xml").encode(), b"other.xml")
        with self.assertRaisesRegex(ValueError, "type_or_base"):
            gate.nodes_from_layers(*args)

    def test_missing_native_node_fails(self):
        args = fixture()
        args[3] = edit(args[3], lambda r: r[0].remove(r[0][-1]))
        with self.assertRaisesRegex(ValueError, "coverage_mismatch"):
            gate.nodes_from_layers(*args)

    def test_duplicate_native_reference_fails(self):
        args = fixture()
        args[3] = edit(args[3], lambda r: r[0][-1].set(gate.X, "#t_1"))
        with self.assertRaisesRegex(ValueError, "duplicate_feature_target"):
            gate.nodes_from_layers(*args)

    def test_unresolved_reference_fails(self):
        args = fixture()
        args[3] = edit(args[3], lambda r: r[0][-1].set(gate.X, "#t_999"))
        with self.assertRaisesRegex(ValueError, "coverage_mismatch"):
            gate.nodes_from_layers(*args)

    def test_duplicate_node_ids_fail(self):
        args = fixture()
        args[1] = edit(args[1], lambda r: r[0][-1].set("id", "t_1"))
        with self.assertRaisesRegex(ValueError, "duplicate_identifier"):
            gate.nodes_from_layers(*args)

    def test_non_placeholder_empty_node_fails(self):
        args = fixture()
        args[0] = args[0].replace("[0]", "zzz")
        with self.assertRaisesRegex(ValueError, "unverified_empty"):
            gate.nodes_from_layers(*args)

    def test_empty_node_with_printed_referent_fails(self):
        args = fixture()
        args[5] = edit(args[5], lambda r: r[0][1].set("r", "x"))
        with self.assertRaisesRegex(ValueError, "printed_referent"):
            gate.nodes_from_layers(*args)

    def test_empty_node_wrong_pos_fails(self):
        args = fixture()
        args[4] = edit(args[4], lambda r: r[0][1][0].set("value", "n"))
        with self.assertRaisesRegex(ValueError, "unverified_empty"):
            gate.nodes_from_layers(*args)

    def test_unaccounted_text_between_nodes_fails(self):
        args = fixture()
        args[0] = args[0][:1] + "x" + args[0][2:]
        with self.assertRaisesRegex(ValueError, "unaccounted_source_text"):
            gate.nodes_from_layers(*args)

    def test_noncontiguous_book1_nodes_fail(self):
        args = fixture()
        args[2] = edit(args[2], lambda r: r[0][1].set("value", "2_1_1"))
        with self.assertRaisesRegex(ValueError, "noncontiguous_book1"):
            gate.nodes_from_layers(*args)

    def test_printed_placeholder_lookalike_is_not_stripped(self):
        args = fixture("[0]")
        nodes = gate.nodes_from_layers(*args)
        reading, _ = gate.marker_free(args[0], nodes)
        self.assertEqual(reading, "[0] β")

    def test_external_or_internal_entity_declarations_rejected(self):
        with self.assertRaisesRegex(ValueError, "entity_declaration"):
            gate.xml(b'<!DOCTYPE x [<!ENTITY e "x">]><x>&e;</x>')


class EditorialAndIdentityTests(unittest.TestCase):
    def test_tei_editorial_rules_and_original_coordinates(self):
        s = section('<p>α<del>β</del><choice><sic>γ</sic><corr>δ</corr></choice><add>ε</add><sic>ζ</sic></p>')
        raw, kept, events = gate.tei_projection(s)
        self.assertEqual(raw, "αβγδεζ")
        self.assertEqual(kept, [("α", 0), ("δ", 3), ("ε", 4), ("ζ", 5)])
        self.assertEqual(len(events), 6)

    def test_complete_scope_cannot_shrink(self):
        root = invented_book()
        _, sections = gate.sections_from_tei(ET.tostring(root))
        self.assertEqual(len(sections), 205)
        root[0][-1].remove(root[0][-1][-1])
        with self.assertRaisesRegex(ValueError, "incomplete_book1_extent"):
            gate.sections_from_tei(ET.tostring(root))

    def test_missing_interior_section_fails(self):
        root = invented_book()
        root[0][2].remove(root[0][2][4])
        with self.assertRaisesRegex(ValueError, "noncontiguous_sections"):
            gate.sections_from_tei(ET.tostring(root))

    def test_missing_chapter_fails(self):
        root = invented_book()
        root[0].remove(root[0][4])
        with self.assertRaisesRegex(ValueError, "incomplete_chapters"):
            gate.sections_from_tei(ET.tostring(root))

    def test_all_printed_characters_must_match(self):
        args = fixture("ά", "β")
        nodes = gate.nodes_from_layers(*args)
        s = {"1_1_1": section("<p>α β</p>")}
        with self.assertRaisesRegex(ValueError, "character_correspondence_failed"):
            gate.bind_sections(nodes, s, s)

    def test_omitted_lexical_material_fails(self):
        args = fixture()
        nodes = gate.nodes_from_layers(*args)
        s = {"1_1_1": section("<p>α γ β</p>")}
        with self.assertRaisesRegex(ValueError, "character_correspondence_failed"):
            gate.bind_sections(nodes, s, s)

    def test_original_edition_revision_difference_fails(self):
        nodes = gate.nodes_from_layers(*fixture())
        s = {"1_1_1": section("<p>α β</p>")}
        original = {"1_1_1": section("<p>α γ β</p>")}
        with self.assertRaisesRegex(ValueError, "text_disagreement"):
            gate.bind_sections(nodes, s, original)

    def test_empty_citation_does_not_reorder_printed_text(self):
        nodes = gate.nodes_from_layers(*fixture())
        nodes[1]["cts"] = "1_1_2"
        s = {"1_1_1": section("<p>α</p>"), "1_1_2": section("<p>β</p>")}
        nodes[-1]["cts"] = "1_1_2"
        summary, coordinates = gate.bind_sections(nodes, s, s)
        self.assertEqual(len(summary), 2)
        self.assertEqual(len(coordinates), 2)

    def test_numbered_prediction_is_blocker_not_default_identity(self):
        nodes = gate.nodes_from_layers(*fixture())
        nodes[0]["raw_lemma"] = "α3"
        before = deepcopy(nodes)
        blockers = gate.lexical_blockers(nodes)
        self.assertEqual(len(blockers), 1)
        self.assertEqual(blockers[0]["raw_lemma"], "α3")
        self.assertEqual(nodes, before)
        self.assertNotIn("canonical_lemma", blockers[0])

    def test_optional_laula_disagreement_is_a_diagnostic_not_a_fake_binding_failure(self):
        nodes = gate.nodes_from_layers(*fixture())
        root = ET.fromstring(f'<TEI xmlns="{gate.NS}">α β</TEI>')
        failures = gate.laula_diagnostics(nodes, root)
        self.assertEqual([f["id"] for f in failures], ["t_2"])
        s = {"1_1_1": section("<p>α β</p>")}
        summary, _ = gate.bind_sections(nodes, s, s)
        self.assertEqual(summary[0]["printed_nodes"], 2)


if __name__ == "__main__":
    unittest.main()
