#!/usr/bin/env python3
"""Tests for the Lysias I vocabulary-mapping research.

Stdlib only. Run after fetch_inputs.sh:

  LYSIAS1_INPUTS=<dest> python3 -I test_mapping.py

Without LYSIAS1_INPUTS only the unit tests run; input-dependent tests are skipped.
"""
import csv
import json
import os
import re
import shutil
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)  # the script directory is not on sys.path under python3 -I

import derive_evidence as de  # noqa: E402
import extract_pdf_text  # noqa: E402

RESEARCH = os.path.abspath(os.path.join(HERE, ".."))
MANIFEST = os.path.join(RESEARCH, "manifest")
REPO_ROOT = os.path.abspath(os.path.join(RESEARCH, "..", "..", ".."))
SELECTION_GO = os.path.join(REPO_ROOT, "internal", "selection", "selection.go")
INPUTS = os.environ.get("LYSIAS1_INPUTS", "")
SCRATCH = os.path.expanduser(os.environ.get("LYSIAS1_SCRATCH", "~/.cache/scratch/lysias1-mapping-test"))


def node(**fields):
    base = {"sentence_id": "1", "subdoc": "", "word_id": "1", "position": 1, "form": "x", "lemma": "λόγος",
            "postag": "n", "relation": "OBJ", "head": "0", "artificial": "", "copula": False}
    base.update(fields)
    return base


def read_tsv(path):
    with open(path, encoding="utf-8", newline="") as handle:
        return list(csv.DictReader(handle, delimiter="\t"))


def read_json(path):
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def read_text(path):
    with open(path, encoding="utf-8") as handle:
        return handle.read()


def have_inputs():
    return bool(INPUTS) and os.path.isfile(os.path.join(INPUTS, "agdt/v2.1/Greek/texts", de.LYSIAS_FILE))


class CanonicalKeyTests(unittest.TestCase):
    def test_capital_and_final_sigma_fold_only(self):
        self.assertEqual(de.canonical_key("Νόμος"), "νόμοσ")
        self.assertEqual(de.canonical_key("λόγος"), "λόγοσ")

    def test_accents_are_not_folded(self):
        self.assertNotEqual(de.canonical_key("δέ"), de.canonical_key("δὲ"))
        self.assertNotEqual(de.canonical_key("καί"), de.canonical_key("καὶ"))

    def test_hyphen_compound_is_kept_raw(self):
        self.assertEqual(de.canonical_key("κατά-καθεύδω"), "κατά-καθεύδω")


class RuleTests(unittest.TestCase):
    def test_letter_rules(self):
        cases = {"n": "NOUN", "a": "ADJ", "d": "ADV", "r": "ADP", "l": "DET", "p": "PRON",
                 "i": "INTJ", "u": "PUNCT", "m": "NUM", "g": "PART"}
        for letter, upos in cases.items():
            rule, got, status, vocab, _ = de.classify(node(postag=letter + "-------", relation="ATR"))
            self.assertEqual(got, upos, letter)
            self.assertNotEqual(status, "unresolved", letter)

    def test_verb_and_auxiliary(self):
        self.assertEqual(de.classify(node(postag="v3siia---", relation="PRED"))[0], "R06")
        self.assertEqual(de.classify(node(postag="v3spia---", relation="AuxV"))[1], "AUX")

    def test_copula_is_unresolved_not_verb(self):
        rule, upos, status, vocab, flags = de.classify(node(postag="v3spia---", relation="PRED", copula=True))
        self.assertEqual((rule, status, vocab), ("R05", "unresolved", "unresolved"))
        self.assertIn("copula-unresolved", flags)

    def test_particle_adverb_is_resolved_conditional(self):
        rule, upos, status, vocab, flags = de.classify(node(postag="d--------", relation="AuxY"))
        self.assertEqual((rule, upos, status, vocab), ("R03P", "ADV", "resolved_conditional", "eligible"))
        self.assertIn("particle-adverb-lineage", flags)

    def test_conjunction_relations(self):
        self.assertEqual(de.classify(node(postag="c--------", relation="COORD"))[1], "CCONJ")
        self.assertEqual(de.classify(node(postag="c--------", relation="AuxC"))[1], "SCONJ")
        rule, upos, status, vocab, _ = de.classify(node(postag="c--------", relation="AuxP"))
        self.assertEqual((rule, status, vocab), ("R09", "class_resolved_upos_unresolved", "non_vocab"))

    def test_elided_and_unknown(self):
        self.assertEqual(de.classify(node(artificial="elliptic", postag=""))[3], "unprinted")
        self.assertEqual(de.classify(node(postag="z--------"))[3], "unresolved")

    def test_copula_marker_requires_predicate_nominal_child(self):
        nodes = [
            node(word_id="1", head="0", lemma="εἰμί", postag="v3spia---", relation="PRED"),
            node(word_id="2", head="1", lemma="ἀγαθός", postag="a-s---mn-", relation="PNOM"),
            node(sentence_id="2", word_id="1", head="0", lemma="εἰμί", postag="v3spia---", relation="PRED"),
        ]
        marked = de.mark_copulas(nodes)
        self.assertTrue(marked[0]["copula"])
        self.assertFalse(marked[2]["copula"])


class EligibilityTests(unittest.TestCase):
    def test_mirrors_repo_default_config(self):
        self.assertEqual(de.ALLOWED_UPOS, frozenset({"NOUN", "VERB", "ADJ", "ADV"}))
        if os.path.isfile(SELECTION_GO):
            match = re.search(r'AllowedPOS: map\[string\]bool\{([^}]*)\}', read_text(SELECTION_GO))
            self.assertIsNotNone(match)
            repo = set(re.findall(r'"([A-Z]+)": true', match.group(1)))
            self.assertEqual(repo, set(de.ALLOWED_UPOS))

    def test_repo_rule_excludes_prt_and_letterless(self):
        self.assertFalse(de.repo_eligible("VERB", "λύω", "compound:prt"))
        self.assertFalse(de.repo_eligible("NOUN", ".", "OBJ"))
        self.assertTrue(de.repo_eligible("ADV", "δέ", "AuxY"))


@unittest.skipUnless(have_inputs(), "set LYSIAS1_INPUTS to the fetch_inputs.sh destination")
class InputTests(unittest.TestCase):
    def test_pinned_scheme_is_1_1(self):
        root = ET.parse(os.path.join(INPUTS, "agdt/v2.1/Greek/texts", de.LYSIAS_FILE)).getroot()
        self.assertIn("1.1", root.findtext("header/annotationScheme"))

    def test_pdf_quote_for_1_1_particles(self):
        text = pdf_text(os.path.join(INPUTS, "sources/v1/greek/docs/guidelines.pdf"))
        self.assertIn("Aparticleisamorphologicalcategory", re.sub(r"\s", "", text))

    def test_manifest_reconciles_to_committed_summary(self):
        summary = read_json(os.path.join(MANIFEST, "lysias1-summary.json"))
        rows = read_tsv(os.path.join(MANIFEST, "lysias1-nodes.tsv"))
        self.assertEqual(len(rows), summary["nodes_total"])
        self.assertEqual(summary["nodes_total"], 2834)
        self.assertEqual(summary["nodes_printed"], 2802)
        self.assertEqual(summary["nodes_unprinted_elliptic"], 32)
        self.assertTrue(all(summary["reconciliation"].values()))
        counts = {}
        for row in rows:
            if row["vocab_class"] != "unprinted":
                counts[row["vocab_class"]] = counts.get(row["vocab_class"], 0) + 1
        self.assertEqual(counts, summary["printed_by_vocab_class"])
        eligible_ids = {r["identity"]: int(r["occurrences"]) for r in read_tsv(os.path.join(MANIFEST, "lysias1-identities.tsv"))
                        if r["vocab_class"] == "eligible"}
        self.assertEqual(sum(eligible_ids.values()), summary["eligible_occurrences"])

    def test_no_per_token_ud_fields_committed(self):
        header = read_text(os.path.join(MANIFEST, "lysias1-nodes.tsv")).splitlines()[0]
        self.assertNotIn("ud_", header)
        self.assertNotIn("feats", header)

    def test_regeneration_is_byte_identical(self):
        os.makedirs(SCRATCH, exist_ok=True)
        out = tempfile.mkdtemp(dir=SCRATCH)
        try:
            de.derive(INPUTS, out)
            for name in sorted(os.listdir(MANIFEST)):
                with open(os.path.join(MANIFEST, name), "rb") as a, open(os.path.join(out, name), "rb") as b:
                    self.assertEqual(a.read(), b.read(), name)
        finally:
            shutil.rmtree(out, ignore_errors=True)


def pdf_text(path):
    """Extract text from the pinned 1.1 PDF with the stdlib extractor (no external tools)."""
    import io
    buffer = io.StringIO()
    stdout, sys.stdout = sys.stdout, buffer
    try:
        extract_pdf_text.main(path)
    finally:
        sys.stdout = stdout
    return buffer.getvalue()


if __name__ == "__main__":
    unittest.main(verbosity=2)
