"""Synthetic checks for research plumbing, not lexicon or sense validation.

Run: python3 -I -B /absolute/path/to/test_research_scripts.py
Temporary inputs are kept under ~/.cache/scratch, never the system tempdir.
"""

import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from betacode import beta_to_greek, canonical  # noqa: E402


class ResearchScriptTests(unittest.TestCase):
    def test_beta_marks_and_unknown_characters(self):
        greek, number, unknown = beta_to_greek("lo/gos2")
        self.assertEqual(canonical(greek), canonical("λόγος"))
        self.assertEqual(number, 2)
        self.assertEqual(unknown, [])
        self.assertTrue(beta_to_greek("b^")[2])
        self.assertNotEqual(canonical("εἰμί"), canonical("εἶμι"))

    def test_filter_uses_top_level_language(self):
        entries = [
            {"lang_code": "en", "translations": [{"lang_code": "grc"}]},
            {"lang_code": "grc", "word": "synthetic"},
        ]
        result = subprocess.run(
            [sys.executable, "-I", "-B", str(HERE / "filter_kaikki_grc.py")],
            input="".join(json.dumps(entry) + "\n" for entry in entries),
            text=True,
            capture_output=True,
            check=True,
        )
        self.assertEqual([json.loads(line) for line in result.stdout.splitlines()], [entries[1]])
        self.assertIn("kept=1 unparseable=0", result.stderr)

    def test_measure_separates_nodes_candidates_and_discriminators(self):
        scratch = pathlib.Path.home() / ".cache/scratch/greek-lexica-review-tests"
        scratch.mkdir(parents=True, exist_ok=True)
        try:
            with tempfile.TemporaryDirectory(dir=scratch) as directory:
                root = pathlib.Path(directory)
                oga = root / "oga.conllu"
                rows = [
                    ("λόγος", "λόγος2", "n", "1.1.1"),
                    ("β", "β", "n", "1.1.1"),
                    ("ἐστί", "εἰμί", "v", "1.1.1"),
                    ("σύνειμι", "σύνειμι", "v", "1.1.1"),
                    ("[0]", "_", "-", "1.1.1"),
                    ("ἄλλος", "ἄλλος", "a", "2.1.1"),
                ]
                oga.write_text(
                    "".join(
                        f"{i}\t{form}\t{lemma}\t{pos}\t_\t_\t0\t_\t_\tref=Xen. Anab._{ref}\n"
                        for i, (form, lemma, pos, ref) in enumerate(rows, 1)
                    ),
                    encoding="utf-8",
                )
                kaikki = root / "grc.jsonl"
                kaikki.write_text(
                    json.dumps({"lang_code": "grc", "word": "λόγος", "pos": "noun", "senses": []}) + "\n",
                    encoding="utf-8",
                )
                lsj = root / "lsj"
                lsj.mkdir()
                (lsj / "grc.lsj.perseus-eng1.xml").write_text(
                    '<entryFree type="main" key="lo/gos4">'
                    '<bibl>urn:cts:greekLit:tlg0032.tlg006.perseus-grc2:1:1:1</bibl>'
                    '</entryFree><entryFree type="main" key="b^">synthetic</entryFree>',
                    encoding="utf-8",
                )
                ml = root / "ml.xml"
                ml.write_text('<entry type="main" key="lo/gos1">synthetic</entry>', encoding="utf-8")
                output = root / "result.json"
                subprocess.run(
                    [sys.executable, "-I", "-B", str(HERE / "measure.py"), str(oga), str(kaikki),
                     str(lsj), str(ml), str(output)],
                    cwd=root,
                    capture_output=True,
                    text=True,
                    check=True,
                )
                data = json.loads(output.read_text(encoding="utf-8"))
                self.assertEqual(data["oga"]["native_book1_tokens_all"], 5)
                self.assertEqual(data["oga"]["printed_book1_tokens"], 4)
                self.assertEqual(data["oga"]["vocabulary_occurrences_after_pos_and_eimi"], 3)
                self.assertEqual(data["oga"]["numbered_identities"], 1)
                candidates = data["candidate_sets"]["lsj"]
                self.assertEqual(candidates["headword_any_entry"], 1)
                self.assertEqual(candidates["identity_with_book1_urn_candidate"], 1)
                self.assertEqual(candidates.get("numbered_identity_equal_digit_UNVERIFIED", 0), 0)
                self.assertEqual(candidates["only_excluded_unknown_char_key"], 1)
        finally:
            scratch.rmdir()


if __name__ == "__main__":
    unittest.main()
