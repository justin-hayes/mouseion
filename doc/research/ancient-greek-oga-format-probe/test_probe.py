"""Invented fixtures + exact public-example checks; no source text is bundled."""

import copy
import json
import os
from pathlib import Path
import re
import tempfile
import unittest
from unittest.mock import Mock, patch
import unicodedata
import xml.etree.ElementTree as ET

import probe


def fixture(text="ἄλφα , [0]", lemma="ἄλφα", pos="n"):
    """Small invented PAULA package; names match the bounded reader's allowlist."""
    prefix = probe.PREFIX
    unit = probe.UNIT
    namespace = 'xmlns:xlink="http://www.w3.org/1999/xlink"'

    def doc(body):
        return f'<paula version="1.1"><header paula_id="synthetic"/>{body}</paula>'.encode()

    length = len(text.split()[0])
    comma = text.index(",") + 1
    empty = text.index("[0]") + 1
    files = {
        prefix + unit + ".xml": doc(f"<body>{text}</body>"),
        prefix + unit + ".tok01.xml": doc(
            f'<markList {namespace} type="tok" xml:base="{unit}.xml">'
            f'<mark id="t_1" xlink:href="#xpointer(string-range(//body,\'\',1,{length}))"/>'
            f'<!--{text.split()[0]}-->'
            f'<mark id="t_2" xlink:href="#xpointer(string-range(//body,\'\',{comma},1))"/><!--,-->'
            f'<mark id="e_1" xlink:href="#xpointer(string-range(//body,\'\',{empty},3))"/><!--[0]-->'
            '</markList>'
        ),
        prefix + "postag.xml": doc(
            f'<multiFeatList {namespace} type="postag" xml:base="{unit}.tok01.xml">'
            f'<multiFeat id="p_1" xlink:href="#t_1"><feat name="pos" value="{pos}"/></multiFeat>'
            '<multiFeat id="p_2" xlink:href="#t_2"><feat name="pos" value="u"/></multiFeat>'
            '<multiFeat id="p_3" xlink:href="#e_1"><feat name="pos" value="-"/></multiFeat>'
            '</multiFeatList>'
        ),
        prefix + "dep." + unit + ".tok01_dep01.xml": doc(
            f'<relList {namespace} type="dep" xml:base="{unit}.tok01.xml">'
            '<rel id="r_1" xlink:href="#e_1" target="#t_1"/>'
            '<rel id="r_2" xlink:href="#t_1" target="#t_2"/></relList>'
        ),
    }
    for kind, values in {"lemma": [lemma, ",", "[0]"], "cts": ["group"] * 3, "ipa01": ["//"] * 3}.items():
        files[prefix + kind + ".xml"] = doc(
            f'<featList {namespace} type="{kind}" xml:base="{unit}.tok01.xml">' +
            "".join(f'<feat id="f_{i}" xlink:href="#{node}" value="{value}"/>'
                    for i, (node, value) in enumerate(zip(["t_1", "t_2", "e_1"], values, strict=True))) +
            '</featList>'
        )
    files[prefix + "fnc.xml"] = doc(
        f'<featList {namespace} type="dep_fnc" xml:base="dep.{unit}.tok01_dep01.xml">'
        '<feat id="f_1" xlink:href="#r_1" value="SBJ"/>'
        '<feat id="f_2" xlink:href="#r_2" value="AuxX"/></featList>'
    )
    return files


def mutate(files, suffix, old, new):
    changed = copy.copy(files)
    path = next(p for p in changed if p.endswith(suffix))
    changed[path] = changed[path].replace(old.encode(), new.encode(), 1)
    return changed


def codes(files):
    return {row["code"] for row in probe.read_package(files)[1]["diagnostics"]}


class SyntheticTests(unittest.TestCase):
    def test_acquisition_rejects_changed_cached_bytes_without_network(self):
        scratch = Path.home() / ".cache/scratch/greek-oga-format-probe/tests"
        scratch.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=scratch) as directory:
            cache = Path(directory)
            (cache / "sample.xml").write_bytes(b"changed")
            with self.assertRaisesRegex(ValueError, "cached_hash_mismatch"):
                probe.acquire(cache, {"files": [{"path": "sample.xml", "size": 8, "sha256": probe.digest(b"original")} ]})

    def test_download_failure_is_not_retried_or_bypassed(self):
        opener = Mock()
        opener.open.side_effect = OSError("forbidden")
        with patch("urllib.request.build_opener", return_value=opener):
            with self.assertRaisesRegex(OSError, "forbidden"):
                probe.acquire(Path("/nonexistent-oga-test-cache"), {"files": [{
                    "path": "sample.xml", "size": 1, "sha256": "unused",
                    "url": f"https://raw.githubusercontent.com/OperaGraecaAdnotata/OGA/{probe.PIN}/README.md"}]})
        self.assertEqual(opener.open.call_count, 1)

    def test_input_outside_authorized_host_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "input_outside_allowlist"):
            probe.acquire(Path("/nonexistent-oga-test-cache"), {"files": [{
                "path": "sample.xml", "size": 1, "sha256": "unused", "url": "https://example.invalid/archive.zip"}]})

    def test_exact_source_and_stable_native_identity(self):
        normalized, ledger = probe.read_package(fixture())
        self.assertEqual(ledger["diagnostics"], [])
        self.assertEqual(normalized["text"], "ἄλφα , [0]")
        self.assertEqual(normalized["nodes"][0]["source_span"], (0, 4))
        self.assertTrue(normalized["nodes"][0]["stable_id"].endswith(".tok01.xml#t_1"))
        self.assertEqual(normalized["nodes"][0]["native_coordinate"]["start_1based"], 1)

    def test_different_documents_can_reuse_id(self):
        package = probe.Package(fixture())
        self.assertNotIn("duplicate_id", [d["code"] for d in package.diagnostics])

    def test_empty_marker_is_not_lexical_span_or_lemma(self):
        normalized, ledger = probe.read_package(fixture())
        empty = normalized["nodes"][-1]
        self.assertEqual(empty["form"], "[0]")
        self.assertEqual(empty["source_span"], (7, 10))
        self.assertIsNone(empty["lexical_span"])
        self.assertIsNone(empty["canonical"]["lemma"])
        self.assertEqual(ledger["graph"]["zero_incoming_nodes"], ["e_1"])

    def test_punctuation_is_source_content_not_vocabulary(self):
        node = probe.read_package(fixture())[0]["nodes"][1]
        self.assertEqual(node["classification"], "source_punctuation")
        self.assertEqual(node["source_span"], (5, 6))
        self.assertIsNone(node["canonical"]["lemma"])

    def test_nfd_offsets_stay_in_nfd_source(self):
        text = unicodedata.normalize("NFD", "ἄλφα") + " , [0]"
        normalized, ledger = probe.read_package(fixture(text))
        self.assertEqual(ledger["diagnostics"], [])
        self.assertEqual(normalized["text"], text)
        self.assertEqual(normalized["nodes"][0]["source_span"], (0, 6))
        self.assertEqual(probe.lemma_key(text.split()[0]), probe.lemma_key("ἄλφα"))

    def test_normalizing_text_before_spans_is_detected(self):
        files = fixture(unicodedata.normalize("NFD", "ἄλφα") + " , [0]")
        files = mutate(files, probe.UNIT + ".xml", unicodedata.normalize("NFD", "ἄλφα"), "ἄλφα")
        self.assertIn("surface_witness_mismatch", codes(files))

    def test_unicode_distinctions(self):
        self.assertEqual(probe.lemma_key("ΛΟΓΟΣ"), probe.lemma_key("λογος"))
        self.assertNotEqual(probe.lemma_key("εἰμί"), probe.lemma_key("εἶμι"))
        self.assertNotEqual(probe.lemma_key("ᾳ"), probe.lemma_key("αι"))
        self.assertNotEqual(probe.lemma_key("ὰ"), probe.lemma_key("ά"))
        self.assertIsNone(probe.lemma_key("λέγω3"))
        self.assertIsNone(probe.lemma_key("a)/nqrwpos"))

    def test_supplementary_codepoint_is_not_utf16_or_bytes(self):
        self.assertEqual(probe.original_span("#xpointer(string-range(//body,'',2,1))", "𐅀α"), (1, 2))

    def test_xml_decoding_not_serialized_byte_offsets(self):
        files = fixture()
        # Entity spelling and inline markup do not alter the decoded string-value.
        files = mutate(files, probe.UNIT + ".xml", "ἄλφα", "&#x1f04;<hi>λ</hi>φα<!--ignored-->")
        self.assertEqual(codes(files), set())

    def test_external_entity_declarations_are_rejected(self):
        with self.assertRaisesRegex(ValueError, "entity_declaration_forbidden"):
            probe.Package({"x": b'<!DOCTYPE x [<!ENTITY a SYSTEM "file:///etc/passwd">]><x>&a;</x>'})

    def test_invalid_xml(self):
        with self.assertRaises(ET.ParseError):
            probe.Package({"x": b"<x>"})

    def test_one_based_bounds(self):
        for href in ["#xpointer(string-range(//body,'',0,1))", "#xpointer(string-range(//body,'',1,0))", "#xpointer(string-range(//body,'',2,2))", "#xpointer(id('x'))"]:
            with self.subTest(href=href), self.assertRaises(ValueError):
                probe.original_span(href, "a")

    def test_missing_and_duplicate_ids(self):
        files = fixture()
        self.assertIn("missing_id", codes(mutate(files, ".tok01.xml", 'id="t_1"', 'other="t_1"')))
        self.assertIn("duplicate_id", codes(mutate(files, ".tok01.xml", 'id="t_2"', 'id="t_1"')))

    def test_dangling_id_and_missing_annotation(self):
        self.assertIn("dangling_id", codes(mutate(fixture(), "lemma.xml", "#t_1", "#missing")))
        self.assertIn("layer_cardinality", codes(mutate(fixture(), "lemma.xml", "#t_1", "#t_2")))

    def test_wrong_document_and_cross_layer_links(self):
        for replacement in ["https://example.invalid/token.xml#t_1", "missing.xml#t_1"]:
            self.assertIn("unsupported_document_link", codes(mutate(fixture(), "lemma.xml", "#t_1", replacement)))
        self.assertIn("wrong_layer_target", codes(mutate(fixture(), "lemma.xml", "#t_1", "lemma.xml#f_0")))

    def test_explicit_relative_document_reference(self):
        files = mutate(fixture(), "lemma.xml", "#t_1", probe.UNIT + ".tok01.xml#t_1")
        self.assertEqual(codes(files), set())

    def test_out_of_range_and_wrong_text_base(self):
        self.assertIn("out_of_range_span", codes(mutate(fixture(), ".tok01.xml", "'',1,4", "'',99,4")))
        self.assertIn("wrong_text_base", codes(mutate(fixture(), ".tok01.xml", f'xml:base="{probe.UNIT}.xml"', 'xml:base="missing.xml"')))

    def test_gaps_overlaps_and_wrong_witness(self):
        self.assertIn("overlapping_spans", codes(mutate(fixture(), ".tok01.xml", "'',6,1", "'',4,1")))
        self.assertIn("uncovered_nonwhitespace", codes(mutate(fixture(), ".tok01.xml", "'',1,4", "'',1,3")))
        self.assertIn("surface_witness_mismatch", codes(mutate(fixture(), ".tok01.xml", "<!--ἄλφα-->", "<!--other-->")))

    def test_unexpected_empty_and_grammatical_duplicates(self):
        self.assertIn("unexpected_empty_representation", codes(mutate(fixture(), "postag.xml", 'value="-"', 'value="n"')))
        self.assertIn("duplicate_grammatical_feature", codes(mutate(fixture(), "postag.xml", '<feat name="pos" value="n"/>', '<feat name="pos" value="n"/><feat name="pos" value="n"/>')))

    def test_partial_pos_mapping_not_guessed(self):
        for pos, expected in {"a": "ADJ", "l": "DET", "r": "ADP", "m": "NUM", "n": None, "v": None, "c": None, "x": None, "d": None}.items():
            with self.subTest(pos=pos):
                normalized, _ = probe.read_package(fixture(pos=pos))
                self.assertEqual(normalized["nodes"][0]["canonical"]["upos"], expected)
                self.assertIsNone(normalized["nodes"][0]["canonical"]["vocabulary_identity"])

    def test_suffix_discriminator_and_unknown_are_not_discarded(self):
        node = probe.read_package(fixture(lemma="λέγω3"))[0]["nodes"][0]
        self.assertEqual(node["layers"]["lemma"][0]["value"], "λέγω3")
        self.assertIsNone(node["canonical"]["lemma"])
        self.assertEqual(node["canonical"]["lexical_discriminator"]["status"], "unknown_not_asserted_absent")

    def test_cycles_and_multiple_incoming(self):
        files = mutate(fixture(), ".tok01_dep01.xml", 'xlink:href="#e_1" target="#t_1"', 'xlink:href="#t_2" target="#t_1"')
        self.assertIn("dependency_cycle", codes(files))
        files = mutate(fixture(), ".tok01_dep01.xml", 'target="#t_2"', 'target="#t_1"')
        self.assertIn("multiple_incoming", codes(files))

    def test_native_groups_never_become_reading_sentences(self):
        _, ledger = probe.read_package(fixture())
        self.assertFalse(ledger["capabilities"]["complete_reading_sentences"])
        self.assertFalse(ledger["capabilities"]["canonical_complete_syntax"])


class PinnedExampleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cache = Path(os.environ.get("OGA_PROBE_CACHE", Path.home() / ".cache/scratch/greek-oga-format-probe/inputs"))
        cls.registry = json.loads((probe.HERE / "inputs.json").read_text())
        cls.files = {item["path"]: (cache / item["path"]).read_bytes() for item in cls.registry["files"] if item["path"].startswith(probe.PREFIX)}
        cls.normalized, cls.ledger = probe.read_package(cls.files)

    def test_all_links_spans_and_comments(self):
        self.assertEqual(self.ledger["diagnostics"], [])
        self.assertEqual(self.ledger["counts"]["nodes"], 454)
        self.assertEqual(self.ledger["counts"]["edges"], 423)
        self.assertEqual(self.ledger["text"]["codepoints"], 2666)
        self.assertEqual(self.ledger["text"]["utf8_sha256"], "25a887cc80701debc6752fda1d1e8037f39ffd11667a6fa02b9404faac0d5718")
        for node in self.normalized["nodes"]:
            start, end = node["source_span"]
            self.assertEqual(self.normalized["text"][start:end], node["comment_witness"])
            self.assertEqual(len(node["layers"]["lemma"]), 1)

    def test_marker_and_graph_accounting(self):
        empty = [n for n in self.normalized["nodes"] if n["native_id"].startswith("e_")]
        self.assertEqual(len(empty), 16)
        self.assertTrue(all(n["form"] == "[0]" and n["lexical_span"] is None for n in empty))
        self.assertEqual(len(self.ledger["graph"]["weak_components"]), 31)
        self.assertEqual(len(self.ledger["cross_cts_group_edges"]), 6)

    def test_independent_whitespace_segmentation_witness(self):
        # Not used to create offsets: independently test the supplied segmentation.
        extents = [match.span() for match in re.finditer(r"\S+", self.normalized["text"])]
        self.assertEqual(extents, [n["source_span"] for n in self.normalized["nodes"]])

    def test_raw_discriminator_affected_record_is_not_normalized(self):
        node = next(n for n in self.normalized["nodes"] if n["native_id"] == "t_206")
        self.assertEqual(node["layers"]["lemma"][0]["value"], "λέγω3")
        self.assertIsNone(node["canonical"]["lemma"])

    def test_exact_committed_regeneration(self):
        ledger = copy.deepcopy(self.ledger)
        ledger["input_hashes"] = {item["path"]: item["sha256"] for item in self.registry["files"]}
        self.assertEqual(probe.encoded(ledger), (probe.HERE / "ledger.json").read_bytes())

    def test_committed_ledger_omits_source_forms(self):
        self.assertNotIn('"form":', probe.encoded(self.ledger).decode())
        self.assertNotIn('"text": "', probe.encoded(self.ledger).decode())
        self.assertEqual(len(self.ledger["optional_metadata_ids_absent"]), 7)


if __name__ == "__main__":
    unittest.main()
