#!/usr/bin/env python3
"""Focused tests for binding_probe.py (#2048). Stdlib only, synthetic inputs only.

    python3 -I doc/research/ancient-greek-lysias1-binding/scripts/test_binding_probe.py

Fixtures use ASCII plus combining marks and U+02BC, never Greek surfaces.
"""
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import binding_probe as bp  # noqa: E402

SCRATCH = os.path.expanduser("~/.cache/scratch/greek-2048-binding")
TEI_NS = 'xmlns="http://www.tei-c.org/ns/1.0"'


def run_alignment(seq_forms, token_texts, v16=None):
    seq = [{"key": f"s001w{i:03d}", "form": f} for i, f in enumerate(seq_forms, 1)]
    words, pos = [], 0
    for i, t in enumerate(token_texts, 1):
        words.append({"key": f"cg2:w{i:04d}", "text": t, "start": pos, "end": pos + len(t)})
        pos += len(t) + 1
    nb, tb, _, _ = bp.align_running(seq, words, v16 or {})
    return nb, tb, seq, words


class RubricBindingTests(unittest.TestCase):
    def test_repeated_labels_bind_by_sequence_position(self):
        # Three rubric nodes share two label strings; each must get its own label ordinal.
        nodes = [{"key": "s078w001", "form": "Lex"}, {"key": "s082w001", "form": "Testes"},
                 {"key": "s084w001", "form": "Lex"}]
        labels = [{"ord": 1, "text": "Lex"}, {"ord": 2, "text": "Testes"}, {"ord": 3, "text": "Lex"}]
        self.assertEqual(bp.bind_rubric(nodes, labels),
                         {"s078w001": 1, "s082w001": 2, "s084w001": 3})

    def test_unmatched_rubric_node_stays_unbound(self):
        nodes = [{"key": "s001w001", "form": "Testes"}, {"key": "s002w001", "form": "Lex"}]
        labels = [{"ord": 1, "text": "Lex"}]
        self.assertEqual(bp.bind_rubric(nodes, labels), {"s002w001": 1})


class CanonicalMixedTagTests(unittest.TestCase):
    XML = f"""<TEI {TEI_NS}><text><body>
<div type="textpart" subtype="section" n="1">
<p>alpha <q>beta</q> <label>Lex</label></p>
<p><label>Testes</label></p>
<p>gam<del>ma</del> <note>apparatus</note>delta<gap reason="lost"/> <add>eps</add></p>
</div>
<div type="textpart" subtype="section" n="2">
<p><label>Lex <q>x</q></label> tail</p>
</div>
</body></text></TEI>
"""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir=SCRATCH)
        self.path = os.path.join(self.tmp.name, "canon.xml")
        with open(self.path, "w", encoding="utf-8") as f:
            f.write(self.XML)
        self.can = bp.canonical(self.path)

    def tearDown(self):
        self.tmp.cleanup()

    def test_running_text_excludes_labels_and_notes_keeps_flagged_text(self):
        self.assertEqual(self.can["text"], "alpha beta \ngamma delta eps\ntail")
        self.assertEqual(self.can["n_sections"], 2)
        self.assertEqual(self.can["lacunae"], [len("alpha beta \ngamma delta")])
        self.assertEqual(self.can["flags_at"][self.can["text"].index("ma")], frozenset({"del"}))

    def test_labels_get_distinct_tei_addresses_and_child_tags(self):
        labels = self.can["labels"]
        self.assertEqual([lab["text"] for lab in labels], ["Lex", "Testes", "Lex x"])
        self.assertEqual(labels[0]["address"], "div[@subtype='section'][@n='1']/p[1]/label[1]")
        self.assertEqual(labels[1]["address"], "div[@subtype='section'][@n='1']/p[2]/label[1]")
        self.assertEqual(labels[2]["address"], "div[@subtype='section'][@n='2']/p[1]/label[1]")
        self.assertEqual(labels[2]["children"], ["q"])
        self.assertEqual(labels[0]["children"], [])
        self.assertEqual([lab["ord"] for lab in labels], [1, 2, 3])
        # a label-only paragraph adds no running text, so its position is the start of the next text
        self.assertEqual(labels[1]["pos"], labels[0]["pos"])
        self.assertEqual(labels[2]["pos"], self.can["text"].index("tail"))


class NormalisationOffsetTests(unittest.TestCase):
    def test_token_spans_slice_back_to_token_text_for_nfd_input(self):
        text = "café ok."
        flags = [frozenset()] * len(text)
        sections = [1] * len(text)
        words, puncts = bp.tokens_of(text, flags, sections, "doc")
        self.assertEqual(words[0]["text"], "café")
        for w in words + puncts:
            self.assertEqual(text[w["start"]:w["end"]], w["text"])

    def test_piece_spans_use_nfc_code_points_of_the_token(self):
        token = bp.nfc("café")  # four code points after NFC
        self.assertEqual(bp.piece_spans(token, ["café"], 10), [(10, 14)])
        self.assertIsNone(bp.piece_spans(token, ["caf"], 0))

    def test_elision_fold_unifies_apostrophe_forms(self):
        self.assertEqual(bp.elision_fold("x'"), bp.elision_fold("xʼ"))
        self.assertEqual(bp.elision_fold("x’"), bp.elision_fold("xʼ"))


class OccurrenceStatusTests(unittest.TestCase):
    def test_fold_without_witness_is_candidate(self):
        nb, tb, _, _ = run_alignment(["x'"], ["xʼ"])
        rec = nb["s001w001"]
        self.assertEqual((rec["corr"], rec["status"], rec["conv"]),
                         ("fold_equal", "candidate", "none_documented"))

    def test_fold_with_equal_v16_witness_is_conditional_not_accepted(self):
        nb, tb, _, _ = run_alignment(["x'"], ["xʼ"], v16={"s001w001": "xʼ"})
        rec = nb["s001w001"]
        self.assertEqual((rec["status"], rec["conv"]), ("conditional", "cross_release_v1.6_equal"))

    def test_exact_equal_run_is_accepted(self):
        nb, _, _, _ = run_alignment(["ab", "cd"], ["ab", "cd"])
        self.assertEqual(nb["s001w001"]["status"], "accepted")
        self.assertEqual(nb["s001w002"]["toks"], ["cg2:w0002"])

    def test_identical_components_matching_two_windows_are_ambiguous_and_unbound(self):
        # nodes a,b,a,b tile token "ab" at two positions; neither window may be bound
        nb, tb, _, _ = run_alignment(["a", "b", "a", "b"], ["ab"])
        for key in ("s001w001", "s001w002", "s001w003", "s001w004"):
            self.assertEqual(nb[key]["status"], "ambiguous", key)
            self.assertEqual(nb[key]["toks"], [])
        self.assertIn(("ambiguous_window", "ambiguous"), tb[0])

    def test_reversed_only_concatenation_is_candidate_not_accepted(self):
        nb, _, _, _ = run_alignment(["b", "a"], ["ab"])
        self.assertEqual(nb["s001w001"]["corr"], "concat_reversed")
        self.assertEqual(nb["s001w001"]["status"], "candidate")
        self.assertEqual(nb["s001w001"]["surf"], "mismatch")
        self.assertEqual(nb["s001w001"]["toks"], ["cg2:w0001"])

    def test_concatenation_never_spans_an_exact_anchor(self):
        # "b" and "c" would form "bc" only across the exact anchor "x"
        nb, _, _, _ = run_alignment(["b", "x", "c"], ["bc", "x"])
        self.assertNotEqual(nb["s001w001"]["corr"], "concat_window")
        self.assertEqual(nb["s001w002"]["corr"], "exact_equal")

    def test_in_block_concatenation_is_candidate_with_both_components(self):
        nb, _, _, _ = run_alignment(["x", "b", "c"], ["x", "bc"])
        self.assertEqual(nb["s001w002"]["corr"], "concat_window")
        self.assertEqual(nb["s001w003"]["corr"], "concat_window")
        self.assertEqual(nb["s001w002"]["status"], "candidate")
        self.assertEqual(nb["s001w002"]["toks"], ["cg2:w0002"])


if __name__ == "__main__":
    unittest.main()
