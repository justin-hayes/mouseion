#!/usr/bin/env python3
"""Focused tests for witness_collation.py and lineage_collation.py (#2048 r3).

    python3 -I doc/research/ancient-greek-lysias1-binding/scripts/test_witness_collation.py

Stdlib only, synthetic inputs only. Fixtures use ASCII, combining marks and a few
single code points written as escapes; no witness text is embedded.
"""
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lineage_collation as lc  # noqa: E402
import witness_collation as wc  # noqa: E402

SCRATCH = os.path.expanduser("~/.cache/scratch/greek-2048-binding")
RHO, RHO_SMOOTH, RHO_ROUGH = "ρ", "ῤ", "ῥ"


class KeyTests(unittest.TestCase):
    def test_elision_marks_attach_to_word_and_unify(self):
        for mark in ("'", "’", "᾽", "ʼ"):
            toks, _, _ = wc.tokenize_witness(f"ab{mark} cd")
            self.assertEqual([t["kind"] for t in toks], ["w", "w"], mark)
            self.assertEqual(toks[0]["key"], "abʼ")

    def test_quote_after_space_is_punctuation_not_elision(self):
        toks, _, converted = wc.tokenize_witness("ab 'cd'")
        self.assertEqual([t["raw"] for t in toks], ["ab", "'", "cd'"])
        self.assertEqual(converted, 1)  # the trailing mark is reported as converted

    def test_rho_breathing_fold_only_inside_word(self):
        self.assertEqual(wc.word_key("a" + RHO_SMOOTH + RHO_ROUGH + "b"), "a" + RHO + RHO + "b")
        # word-initial rough rho is kept
        self.assertEqual(wc.word_key(RHO_ROUGH + "a"), RHO_ROUGH + "a")

    def test_accents_stay_significant(self):
        self.assertNotEqual(wc.word_key("é"), wc.word_key("è"))
        self.assertNotEqual(wc.word_key("o̓"), wc.word_key("o̔"))

    def test_rubric_key_is_disjoint_from_word_keys(self):
        self.assertEqual(wc.rubric_key("LEX"), wc.rubric_key("Léx"))
        self.assertNotEqual(wc.rubric_key("LEX"), wc.word_key("lex"))

    def test_capitals_rubric_token_gets_rubric_key(self):
        toks, _, _ = wc.tokenize_witness("lex LEX")
        self.assertEqual(toks[0]["key"], "lex")
        self.assertEqual(toks[1]["key"], wc.rubric_key("LEX"))


class ExtractionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir=SCRATCH)

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, name, text):
        p = os.path.join(self.tmp.name, name)
        with open(p, "w", encoding="utf-8") as f:
            f.write(text)
        return p

    def test_wikisource_templates_become_section_markers(self):
        p = self.write("ws.txt", "{{Τίτλος2|A|B}}\n{{χ|1}} alpha\n"
                                  "{{χ|2}} beta\n[[Cat:x]]\n</div>")
        toks, markers, _ = wc.tokenize_witness(wc.extract_wikisource([p]))
        self.assertEqual([t["raw"] for t in toks], ["alpha", "beta"])
        self.assertEqual([t["section"] for t in toks], [1, 2])
        self.assertEqual([m["section"] for m in markers], [1, 2])

    def test_wikisource_unknown_template_fails(self):
        p = self.write("ws.txt", "{{other}} alpha")
        with self.assertRaises(AssertionError):
            wc.extract_wikisource([p])

    def test_mnemosyne_takes_first_text_block_only(self):
        page = ('<div class="lbr_text anth_text"><table><tr><td><p><span class="verse">[1]</span> '
                'alpha<br> LEX<br> beta</p></td></tr></table></div>'
                '<div class="right-part"><div class="lbr_text"><table><tr><td>translation'
                '</td></tr></table></div></div>')
        p = self.write("p.html", page)
        toks, markers, _ = wc.tokenize_witness(wc.extract_mnemosyne([p]))
        self.assertEqual([t["raw"] for t in toks], ["alpha", "LEX", "beta"])
        self.assertEqual(markers, [{"after_token": 0, "section": 1}])

    def test_lineage_extract_drops_notes_and_keeps_markup_text(self):
        xml = ('<TEI><teiHeader><title>h</title></teiHeader><text><body>'
               '<div type="section" n="1"><p>alpha <del>beta</del><note>app</note> gamma.'
               '<quote><title>Lex</title></quote></p></div></body></text></TEI>')
        p = self.write("t.xml", xml)
        toks, markup, rub = lc.extract(p)
        self.assertEqual([t[0] for t in toks], ["alpha", "beta", "gamma", ".", "Lex"])
        self.assertEqual([t[3] for t in toks], [False, False, False, False, True])
        self.assertEqual(rub, [1])
        self.assertEqual(markup["note"], 1)


class ClassificationTests(unittest.TestCase):
    def tok(self, raw, flags=()):
        return {"raw": raw, "key": wc.word_key(raw), "flags": list(flags), "section": 1}

    def test_word_division_is_not_hidden(self):
        cls = wc.classify_word_op("replace", [self.tok("ab")], [self.tok("a"), self.tok("b")], set())
        self.assertEqual(cls, "word_division")

    def test_diacritics_only(self):
        cls = wc.classify_word_op("replace", [self.tok("é")], [self.tok("è")], set())
        self.assertEqual(cls, "diacritics_only")

    def test_large_insert_is_block(self):
        cls = wc.classify_word_op("insert", [], [self.tok("x")] * 20, set())
        self.assertEqual(cls, "witness_block_insert")

    def test_same_comparator(self):
        self.assertTrue(wc.same("a b", "a b", "word"))
        self.assertFalse(wc.same("ab", "a b", "word"))
        self.assertTrue(wc.same(",…", ",...", "punct_after"))
        self.assertTrue(wc.same("LEX", "Léx", "rubric"))


if __name__ == "__main__":
    unittest.main()
