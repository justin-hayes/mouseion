"""Tests for producer_probe.py. Run: python3 -m unittest -v test_producer_probe.py (from scripts/).

The synthetic tests need no external input. The last test re-runs the probe on the pinned inputs when
they are present under the scratch folder, and checks the recorded manifest/producer_summary.json.
"""
import contextlib
import io
import json
import os
import re
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import producer_probe as pp  # noqa: E402

SCRATCH = Path.home() / ".cache" / "scratch" / "greek-lysias1-binding-completion"
INPUTS = Path(os.environ.get("MOUSEION_LYSIAS_INPUTS", SCRATCH / "inputs"))
TOKENIZER = Path(os.environ.get("MOUSEION_TOKENIZER_DIR", SCRATCH / "tok"))
C9B = "c9b60a345708af72c721ec1183c9ab928f7b8c03"
A4A = "a4a810f02f73bacf3f16e722425dfb80f3b2a5ab"
MANIFEST = Path(__file__).resolve().parent.parent / "manifest"


class EncliticTests(unittest.TestCase):
    RULE = [(re.compile(r"^(μή)τε$", re.I), "τε")]

    def test_hash_mode_inserts_enclitic_before_host(self):
        tokens, log = pp.split_frequent_enclitics(["καὶ", "μήτε", "ἔλεγον"], self.RULE, "hash")
        self.assertEqual(tokens, ["καὶ", "-τε", "μή", "ἔλεγον"])
        self.assertEqual(log, [("-τε", 1)])

    def test_array_mode_takes_the_enclitic_from_the_capture_group(self):
        rules = [(re.compile(r"^(μή)(τε)$", re.I), None)]
        tokens, _ = pp.split_frequent_enclitics(["μήτε"], rules, "array")
        self.assertEqual(tokens, ["-τε", "μή"])

    def test_multiple_hosts_keep_their_order(self):
        tokens, log = pp.split_frequent_enclitics(["μήτε", "x", "μήτε"], self.RULE, "hash")
        self.assertEqual(tokens, ["-τε", "μή", "x", "-τε", "μή"])
        self.assertEqual([idx for _, idx in log], [0, 3])

    def test_unaccented_host_only_matches_the_pre_fix_rule(self):
        # Pre-fix rule (c9b60a34) uses unaccented οὐ; accented οὔτε is not split by it.
        pre = [(re.compile(r"^(οὐ|μή|εἰ)τε$", re.I), "τε")]
        self.assertEqual(pp.split_frequent_enclitics(["οὔτε"], pre, "hash")[0], ["οὔτε"])
        self.assertEqual(pp.split_frequent_enclitics(["οὐτε"], pre, "hash")[0][0], "-τε")


class KrasisTests(unittest.TestCase):
    def test_fragment_is_split_before_host(self):
        regex = re.compile(r"^(τ)(ἀ)(.*)$")
        self.assertEqual(pp.split_krasis(["τἀληθῆ", "λέγε"], regex), ["τ-", "ἀληθῆ", "λέγε"])

    def test_unmatched_tokens_pass_through(self):
        regex = re.compile(r"^(τ)(ἀ)(.*)$")
        self.assertEqual(pp.split_krasis(["ἀλλʼ"], regex), ["ἀλλʼ"])


class TokenisationTests(unittest.TestCase):
    def test_punctuation_is_split_into_its_own_token(self):
        cons = re.compile("[τ]")
        spaced = pp.PUNCTUATION.sub(lambda m: " " + m.group(0) + " ", "ἐγὼ , ἐκείνῃ .")
        tokens = [t for t in re.split(r"[ \t\n\r\f\v]+", spaced) if t]
        self.assertEqual(tokens, ["ἐγὼ", ",", "ἐκείνῃ", "."])
        self.assertEqual(pp.join_greek_apostrophe(["τ", "᾽", "x"], cons), ["τ᾽", "x"])

    def test_elision_mark_is_not_a_split_point(self):
        # U+02BC (ʼ) is not in PUNCTUATION, so elided forms stay whole.
        self.assertIsNone(pp.PUNCTUATION.search("ἀλλʼ"))


class CompareTests(unittest.TestCase):
    def test_non_equal_ops_name_the_native_node(self):
        native = [{"node": "s001w001", "form": "α"}, {"node": "s001w002", "form": "β"},
                  {"node": "s001w003", "form": "γ"}]
        ops, equal = pp.compare(native, ["α", "δ", "γ"])
        self.assertEqual(equal, 2)
        replaced = [o for o in ops if o["tag"] == "replace"]
        self.assertEqual(replaced[0]["native"], ["s001w002"])
        self.assertEqual(replaced[0]["sim_forms"], ["δ"])


class WordlistTests(unittest.TestCase):
    def test_ruby_arrays_are_parsed_from_source(self):
        src = "PLAIN_VOWELS = %w(α ε)\nCONSONANTS = %w(τ κ)\n"
        lists = pp.ruby_wordlists(src)
        self.assertEqual(lists["PLAIN_VOWELS"], ["α", "ε"])
        self.assertEqual(lists["CONSONANTS"], ["τ", "κ"])


class RecordedSummaryTests(unittest.TestCase):
    def test_probe_reproduces_recorded_summary(self):
        if not (INPUTS / "agdt" / "v1.6.xml").exists() or not (TOKENIZER / C9B / "tokenizer.rb").exists():
            self.skipTest("pinned inputs or tokenizer sources not present in scratch")
        with tempfile.TemporaryDirectory() as tmp:
            with contextlib.redirect_stdout(io.StringIO()):
                pp.main(["probe", str(INPUTS), str(TOKENIZER), tmp])
            got = json.loads((Path(tmp) / "producer_summary.json").read_text(encoding="utf-8"))
        want = json.loads((MANIFEST / "producer_summary.json").read_text(encoding="utf-8"))
        self.assertEqual(got, want)


if __name__ == "__main__":
    unittest.main()
