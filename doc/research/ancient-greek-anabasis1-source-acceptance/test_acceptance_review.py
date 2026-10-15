"""Unit tests for the acceptance-review diagnostics, on invented fixtures only."""

import unittest
from pathlib import Path

import acceptance_review as ar


def node(node_id, sentence, start, pos="n", cts="1_1_1", flags=()):
    return {
        "id": node_id,
        "sentence": sentence,
        "reading_span": [start, start + 1],
        "native_pos": pos,
        "cts": cts,
        "citation_flags": list(flags),
    }


class SpacingTest(unittest.TestCase):
    def setUp(self):
        # "οὐ δὲ λέγει ," in PAULA; TEI "οὐδὲ λέγει," -> two joins, one of them a word split.
        self.printed = [node("a", 0, 0, "d"), node("b", 0, 3, "d"), node("c", 0, 6, "v"), node("d", 0, 12, "u")]
        self.tei = {"a": [0, 1], "b": [2, 3], "c": [5, 6, 7], "d": [8]}

    def test_joins_and_splits(self):
        joins = ar.tei_joins(self.printed, self.tei)
        self.assertEqual(joins, {"a", "c"})
        self.assertEqual(ar.word_splits(self.printed, joins), [("a", "b")])

    def test_join_never_crosses_section_or_sentence(self):
        printed = [node("a", 0, 0), node("b", 1, 3), node("c", 1, 6, cts="1_1_2")]
        self.assertEqual(ar.tei_joins(printed, {"a": [0], "b": [1], "c": [2]}), set())

    def test_measures_subtract_joins(self):
        sentences = [{"index": 0, "units": 4, "code_points": 14}]
        measures = ar.sentence_measures(sentences, self.printed, {"a", "c"})
        self.assertEqual(measures[0]["tei_units"], 2)
        self.assertEqual(measures[0]["tei_code_points"], 12)

    def test_measures_require_one_unit_per_printed_node(self):
        with self.assertRaises(ar.ReviewError):
            ar.sentence_measures([{"index": 0, "units": 5, "code_points": 14}], self.printed, set())


class EarliestTest(unittest.TestCase):
    def test_earliest_respects_caps_and_order(self):
        measures = [
            {"units": 60, "code_points": 300, "tei_units": 45, "tei_code_points": 290},
            {"units": 20, "code_points": 100, "tei_units": 18, "tei_code_points": 98},
        ]
        nodes = [node("late", 1, 50), node("early", 0, 5)]
        self.assertEqual(ar.earliest(nodes, measures), "late")
        self.assertEqual(ar.earliest(nodes, measures, "tei_"), "early")

    def test_code_point_cap_and_headword_flags(self):
        measures = [{"units": 10, "code_points": 401}, {"units": 10, "code_points": 50}]
        self.assertIsNone(ar.earliest([node("x", 0, 0), node("y", 1, 9, flags=["malformed"])], measures))


class CompareTest(unittest.TestCase):
    def test_numbers_dropped_and_sets_split(self):
        oga = [("λέγω#morpheus:3", "VERB"), ("λέγω", "VERB"), ("δέ", "ADV")]
        glaux = [
            {"canonical_key": "λέγω", "upos": "VERB", "selected": "yes"},
            {"canonical_key": "οὐ", "upos": "ADV", "selected": "yes"},
            {"canonical_key": "δέ", "upos": "ADV", "selected": "no"},
        ]
        result = ar.compare_selected(oga, glaux)
        self.assertEqual(result["oga_selected_without_morpheus_numbers"], 2)
        self.assertEqual(result["shared"], 1)
        self.assertEqual(result["oga_only"], ["δέ|ADV"])
        self.assertEqual(result["glaux_only"], ["οὐ|ADV"])


class WriteGuardTest(unittest.TestCase):
    def test_refuses_other_paths(self):
        for path in (
            ar.PROOF / "opus-repair-summary.json",
            ar.PRIVATE / "opus-repair/out/card-inputs.json",
            Path.home() / "elsewhere.json",
        ):
            with self.assertRaises(ar.ReviewError):
                ar.write(path, "x")

    def test_pins_cover_only_retained_outputs(self):
        self.assertTrue(all(not rel.startswith("acceptance-review") for rel in ar.PINS))


if __name__ == "__main__":
    unittest.main()
